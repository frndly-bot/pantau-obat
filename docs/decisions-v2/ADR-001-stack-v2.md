# ADR-001 v2: Pilihan Stack Teknologi

- Status: Accepted (revisi v2)
- Tanggal: 2026-10-10
- Menggantikan: `docs/decisions/ADR-001-stack.md`
- Terkait: `ADR-002-postgresql-18`, `ADR-003-frontend-dua-app`, `ADR-004-katalog-preset`

## Context

Pantau Obat adalah aplikasi personal + caregiver untuk memantau kepatuhan minum
obat, kini ditambah sisi admin untuk mengelola akun dan mengurasi katalog obat.
Persyaratan yang menentukan pilihan:

1. **Konsistensi data wajib.** Satu obat punya satu pola frekuensi, banyak jam,
   banyak catatan minum, relasi caregiver berstatus, dan stok berjalan. Log
   kepatuhan tidak boleh hilang atau ganda.
2. **Jadwal dan notifikasi latar belakang.** Pengingat harus terkirim walaupun
   browser ditutup. Perlu scheduler server-side + worker asinkron.
3. **Target pemakai bukan developer.** Banyak pengguna orang tua di HP Android
   kelas menengah. Bundle harus ringan.
4. **Tim kecil.** 1–2 orang. Biaya operasional dan kompleksitas deploy
   menentukan kelangsungan proyek.
5. **Mobile adalah syarat, bukan bonus.** Dua permukaan: user mobile-first,
   admin desktop-first.
6. **Deploy VPS tunggal + Docker.** Bukan managed service.
7. **Jadwal obat nyata tidak seragam.** Harus mendukung lima pola frekuensi
   tanpa membengkakkan database. Ini yang mengarahkan keputusan penyimpanan
   jadwal sebagai aturan, bukan baris per hari.

## Decision

### Backend: Go 1.27

Dipakai karena:

- **Goroutine + channel** membuat scheduler, dispatcher, dan pass harian jadi
  natural tanpa broker. Tiga loop dalam satu proses, bukan tiga service.
- **Binary tunggal.** Deploy = satu image Alpine. Rollback satu image. Tidak ada
  runtime Node/Python yang bisa rusak saat update OS.
- **Type-safe sampai layer SQL.** `pgx` + struct manual. Error di compile,
  bukan di produksi.
- **Satu binary untuk user dan admin.** Data sama, otorisasi cukup di layer
  middleware. Memisah proses menambah dua koneksi DB dan dua titik gagal tanpa
  manfaat.
- **Ekosistem standar cukup.** `net/http` + `chi` + `pgx` + `goose`.

Versi: **Go 1.27** (rilis stabil terbaru saat dokumen ini ditulis: 1.27.2,
2026-10-08). Naik dari 1.26 karena 1.26 hanya didukung sampai dua rilis utama
berikutnya, dan 1.27 masih menerima perbaikan keamanan.

Kelemahan diterima sadar: lebih verbose daripada Node untuk CRUD satu tabel.
Untuk app skala ini, verbosity = jelas.

### Frontend: SvelteKit + Svelte 5, dua aplikasi

Dipakai karena:

- **Runes Svelte 5** (`$state`, `$derived`, `$effect`) menghilangkan boilerplate
  reaktivitas untuk proyek sekecil ini.
- **Bundle kecil.** Svelte compile ke vanilla JS.
- **`adapter-static`** menghasilkan file statis yang diserve Caddy tanpa server
  Node di VPS.
- **Dua permukaan butuh shell berbeda.** Bottom-nav mobile dan sidebar admin
  tidak nyaman disatukan dalam satu shell.

Dua aplikasi dalam satu workspace (lihat `ADR-003`), berbagi paket komponen dan
klien API.

UI stack:

- **shadcn-svelte** — komponen masuk ke repo, bukan dependency. Bebas diubah,
  tidak terkunci versi.
- **Tailwind CSS v4** — satu konfigurasi untuk dua app.
- **Lucide** (`lucide-svelte`) — ikon konsisten, sudah jadi bawaan shadcn-svelte.
- **TanStack Table** untuk tabel admin (headless, tidak mengunci markup).
- **sveltekit-superforms + zod** untuk form dan validasi.
- Animasi: `svelte/transition` untuk user (terasa hidup), minim untuk admin.

### Database: PostgreSQL 18

Rincian di `ADR-002-postgresql-18`. Ringkas:

- `uuidv7()` untuk PK entitas (index locality).
- Generated column `STORED` untuk FTS katalog.
- Partial unique index untuk anti-dosis-ganda.
- `FOR UPDATE SKIP LOCKED` untuk antrean job.

### Penyimpanan jadwal: aturan, bukan baris per hari

Keputusan paling berdampak di v2. Dibahas penuh di `ADR-005-schedule-rules`.

Ringkas: jadwal disimpan sebagai pola + jam. Waktu dosis dihitung saat
dibutuhkan dengan *occurrence key*. Terapi seumur hidup tidak menambah satu
baris pun. Yang tumbuh hanya catatan dosis yang benar-benar terjadi.

### Transport: REST JSON

Satu-satunya transport. Frontend statis + trafik kecil + kontrak per endpoint
mudah dipahami. Tanpa GraphQL, tanpa gRPC.

### Deploy: Docker Compose + Caddy di VPS tunggal

Konfigurasi ada di `deploy/`. Satu VPS, image Go, Postgres container, Caddy
menangani TLS + static + reverse proxy.

## Alternatives Considered

| Alternatif | Kenapa Ditolak |
|---|---|
| **Next.js + React** | Server Node menambah beban produksi. App login-only, SEO rendah. Bundle lebih besar untuk HP kelas menengah. |
| **Nuxt + Vue** | Bukan kekurangan teknis. Tim memilih berdasarkan dokumentasi runes Svelte 5 dan rencana PWA. |
| **Flutter (web)** | Flutter Web belum matang untuk PWA offline-first + web push. Tetap pilihan bagus kalau target native sejak awal, dan itu bukan target v1. |
| **Node + TypeScript (Nest/Express)** | Satu event loop kurang cocok untuk job notifikasi. Binary tunggal tidak bisa. |
| **Python + FastAPI** | Startup lambat dan memory besar untuk VPS kecil. Scheduler loop intensif kurang idiomatic. |
| **Java + Spring Boot** | Terlalu berat untuk 1–2 orang dan VPS 1 GB. |
| **MongoDB** | Kepatuhan butuh relasi, transaksi, dan join. Model dokumen cocok untuk log, bukan untuk aturan kepatuhan + otorisasi caregiver + stok. |
| **SQLite** | Tidak cocok server multi-user (file lock, concurrent write). |
| **Supabase / Firebase** | Vendor lock-in, biaya setelah free tier, kontrol terbatas atas scheduler. |
| **Redis + BullMQ** | Lebih tepat untuk throughput tinggi. Untuk v1, tabel Postgres + `SKIP LOCKED` cukup dan mengurangi satu dependency. |
| **Microservices** | Overhead deploy dan debugging tidak sepadan untuk tim 1–2 orang. |
| **Serverless** | Cold start menyusahkan scheduler interval pendek. Worker berumur panjang bertentangan dengan model fungsi. |
| **Satu app frontend dengan `/admin`** | Bundle user ikut membawa kode admin. Shell bertabrakan. Guard mudah salah. |
| **Dua app benar-benar mandiri** | Komponen dasar (~15) dan klien API harus diduplikasi. Perawatan lebih mahal. Lihat `ADR-003`. |
| **PK BIGSERIAL** | Membocorkan urutan dan memudahkan enumerasi antar tenant. |

## Consequences

### Positive

1. **Satu artefak deploy untuk API.** Satu binary Go, tanpa dependency ekosistem.
2. **Integritas data ditegakkan DB.** `uq_log_occurrence` dan FK `RESTRICT`
   menghapus kelas bug "dosis ganda".
3. **Notifikasi sederhana.** Scheduler + dispatcher in-process, tanpa broker.
4. **Frontend statis = aman dan cepat.** Diserve Caddy, tidak ada Node runtime.
5. **Database tidak membengkak karena jadwal.** Terapi seumur hidup = 3 baris.
6. **Biaya operasional rendah.** Satu VPS.
7. **Dari PWA ke native masih bisa.** Backend API tetap sama.
8. **Admin dan user satu proses.** Satu titik audit, satu rollback.

### Negative

1. **Verbosity Go untuk CRUD.** Satu endpoint butuh struct, handler, service, repo.
2. **Adaptasi runes Svelte 5.** Sebagian library UI belum sepenuhnya kompatibel
   runes.
3. **Tidak ada ORM.** SQL ditulis manual. Wajib disiplin parametrized query.
4. **Type check tidak menjangkau boundary DB.** Enum Postgres dan konstanta Go
   bisa drift. Mitigasi: test kontrak yang membandingkan keduanya.
5. **PostgreSQL di VPS kecil butuh tuning.** Sudah disiapkan di
   `deploy/postgres/postgresql.conf`, belum diuji beban.
6. **Satu proses berarti satu titik gagal.** Kalau scheduler crash, API ikut.
   Mitigasi: restart policy. Jalur pecah worker sudah disiapkan.
7. **Workspace frontend menambah langkah build.** Perlu `pnpm` dan pemahaman
   monorepo.

### Neutral

1. **Tanpa GraphQL** = beberapa bentuk request per halaman. Tidak buruk untuk
   endpoint yang sedikit dan stabil.
2. **Tanpa ORM** = memakai goose untuk migrasi, bukan generate schema dari model.
3. **REST, bukan RPC** = endpoint bisa di-curl, tidak perlu generate client.
4. **Bahasa kode Inggris, pesan ke user Indonesia.** Semua enum di DB berbahasa
   Inggris supaya maintainable. Terjemahan ke Bahasa Indonesia dilakukan di
   frontend.
5. **Tidak ada cache Redis di v1.** Hot path harus sudah punya index benar.

## Long-term Consequences

1. **Pecah worker adalah escape valve.** Batas modul dijaga agar worker bisa
   jadi binary kedua tanpa tulis ulang.
2. **Partition tabel log** kalau data kepatuhan tumbuh besar. Rolling archive
   bulanan ke tabel partisi. Tidak perlu ganti DB.
3. **Jika native > PWA.** Backend API tetap sama. SvelteKit bisa dibungkus
   Capacitor.
4. **Auth custom, bukan OAuth-only.** Password + refresh token + Argon2id.
   Menambah "login dengan Google" nanti tidak kontradiktif.
5. **Materialized view** untuk laporan berat kalau sudah perlu. Sekarang belum.
