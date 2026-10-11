# Pantau Obat — Personal Medication Adherence + Caregiver Monitoring

Status: **backend jalan & teruji.** 77 rute, 92 unit test lulus, dan uji e2e
apidoc **75/75 request dengan 159/159 assertion** (idempoten). Frontend belum
dimulai; wireframe HTML ada di `design/`.
Stack: Backend Go 1.27 | DB PostgreSQL 18 | Frontend SvelteKit + Svelte 5 + shadcn-svelte | PWA (native menyusul)

## Struktur

```
pantau-obat/
  Makefile               # titik masuk tunggal untuk semua perintah
  scripts/
    setup-toolchain.sh   # pasang Go + Node + pnpm + TypeScript
    dev-api.sh           # jalankan backend lokal (muat env, isi APP_PUBLIC_URL)
  db/
    README.md            # aturan migrasi goose
  backend/
    migrations -> internal/repo/postgres/migrations   # 25 migrasi (symlink)
    cmd/ internal/       # Go 1.27, satu binary (lihat README)
    README.md
  apidoc/                # OpenCollection spec + uji e2e API (75 endpoint)
    README.md
  deploy/
    docker-compose.yml   # PostgreSQL 18, tuning hemat resource
    .env.example         # contoh konfigurasi
    postgres/            # postgresql.conf + initdb
    README.md
  design/                # wireframe HTML interaktif (user + admin)
  frontend/              # KOSONG — workspace SvelteKit (lihat README)
  docs/
    trd-v2/              # TRD v2
    design-v2/           # arsitektur, data model, API, notifikasi
    decisions-v2/        # ADR 001-005
    design/erd.html      # ERD visual (buka langsung di browser)
    issues/              # catatan debug per-isu
```

## Menjalankan backend

```bash
make db-up                    # nyalakan PostgreSQL 18
make db-migrate               # jalankan migrasi goose (25 file)
scripts/dev-api.sh --bg       # jalankan API di latar, log ke /tmp
curl -s localhost:8080/readyz # pastikan database: ok

# admin pertama (peran admin tidak bisa diberikan lewat API)
cd backend && go run ./cmd/admin-bootstrap \
    --email admin@example.com --password 'Rahasia123!' --name "Admin"

make apidoc-run               # uji e2e: 75 request, 159 assertion
```

## Dokumen aktif

### TRD
- `docs/trd-v2/TRD-v2.md` — tujuan, persona, user story, AC, NFR, lingkup versi

### Desain
- `docs/design-v2/architecture-v2.md` — arsitektur, struktur backend/frontend, deployment
- `docs/design-v2/data-model-v2.md` — entitas database lengkap + alasan tiap index
- `docs/design-v2/api-v2.md` — kontrak endpoint, kode error, envelope
- `docs/design-v2/notification-v2.md` — strategi pengingat, antrean, terlewat
- `docs/design/erd.html` — ERD visual, offline, buka di browser

### Keputusan
- `docs/decisions-v2/ADR-001-stack-v2.md` — pilihan stack keseluruhan
- `docs/decisions-v2/ADR-002-postgresql-18-v2.md` — PostgreSQL 18, uuidv7, tuning
- `docs/decisions-v2/ADR-003-frontend-dua-app.md` — kenapa dua aplikasi frontend
- `docs/decisions-v2/ADR-004-katalog-preset.md` — preset publik vs katalog pribadi user
- `docs/decisions-v2/ADR-005-jadwal-aturan.md` — kenapa jadwal disimpan sebagai aturan

## Prinsip

- **Explore → plan → build.** Tidak ada kode sebelum rencana disetujui.
- **Wireframe = HTML interaktif per halaman dalam satu folder**, bukan ASCII.
- **Mobile responsive = requirement**, bukan polish.
- **`backend/migrations/` adalah sumber kebenaran skema.** Kalau dokumen berbeda, migrasi yang benar.
- **Enum di database berbahasa Inggris** demi maintainability; terjemahan ke Bahasa Indonesia di frontend.
- **Verifikasi live, bukan sekadar unit test hijau.** Bukti dari container yang berjalan.

## Perubahan besar dari v1

1. **Jadwal disimpan sebagai aturan**, bukan baris per hari. Terapi seumur hidup tetap 3 baris.
2. **Pola frekuensi diperluas** ke lima bentuk: harian, interval, mingguan, siklus bulanan, PRN.
3. **Stok dan restock** ditambahkan, termasuk peringatan obat hampir habis.
4. **Katalog obat** dengan pemisahan preset publik (kurasi admin) dan katalog pribadi user.
5. **Peran admin** ditambahkan, terpisah dari status aktif akun, dengan `admin_scope`.
6. **Frontend dipecah dua**: user mobile-first dan admin desktop-first.
7. **Backend tetap satu folder dan satu binary** — pemisahan admin di layer otorisasi.

## Menyiapkan lingkungan

**Didukung:** macOS (Intel dan Apple Silicon) dan Linux Debian-based
(Ubuntu, Debian, Linux Mint, Pop!_OS).

**Windows:** tidak didukung langsung. Pakai WSL2 dulu:

```powershell
wsl --install -d Ubuntu
```

Restart, buka Ubuntu, lalu jalankan perintah di bawah dari dalam WSL.

```bash
make setup-toolchain   # Go 1.27 + nvm + Node 24 LTS + pnpm + TypeScript
make check-tools       # hanya periksa, tidak mengubah apa pun
```

Semua dipasang **user-local** di `$HOME`, tanpa sudo.

## Menjalankan database

```bash
make db-up         # nyalakan PostgreSQL 18
make db-migrate    # jalankan migrasi goose
make verify        # cek toolchain + DB + jumlah tabel
```

Detail ada di `deploy/README.md`. Untuk melihat semua perintah:

```bash
make help
```
