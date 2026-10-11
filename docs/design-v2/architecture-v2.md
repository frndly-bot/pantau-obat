# Arsitektur v2 — Pantau Obat

Status: v2, selaras dengan migrasi goose
Tanggal: 2026-10-10
Stack: SvelteKit + Svelte 5 + shadcn-svelte | Go 1.27 | PostgreSQL 18 | Caddy

> Menggantikan `docs/design/architecture.md`.

## 1. Diagram Komponen

```
                    Internet
                        |
                   HTTPS :443
                        |
                   +---------+
                   |  Caddy  |  TLS otomatis, static, reverse proxy
                   +---------+
                    |       |
      file statis  |       |  /api/*  /healthz
                    |       v
                    |   +-----------------------------+
                    |   |        api (Go 1.27)       |
                    |   |  chi mux + middleware       |
                    |   |  service layer              |
                    |   |  repo (pgx)                 |
                    |   |  worker (goroutine)         |
                    |   |    - scheduler              |
                    |   |    - dispatcher             |
                    |   |    - catch-up pass          |
                    |   +-----------------------------+
                    |               |
                    |               v
                    |   +-----------------------------+
                    |   |     PostgreSQL 18           |
                    |   |  FOR UPDATE SKIP LOCKED     |
                    |   +-----------------------------+
                    |
        +-----------------------------+
        |   apps/user  (static build) |  -> /srv/user
        |   apps/admin (static build) |  -> /srv/admin
        +-----------------------------+
```

Admin dan user memakai **API yang sama**. Pemisahan dilakukan di layer
otorisasi (`RequireAdmin(scope)`), bukan di proses terpisah. Alasan: keduanya
memakai data yang sama; memisah proses menambah dua koneksi DB dan dua titik
gagal tanpa manfaat.

## 2. Pilihan Arsitektur

### Opsi A — Monolith modular Go (satu binary) — **DIPILIH**

Semua di satu proses: HTTP server, scheduler, dispatcher.

- Satu binary, satu image, satu deploy.
- Scheduler dan dispatcher berbagi koneksi pool yang sama.
- Debugging satu proses, bukan menelusuri antar service.
- Cocok untuk VPS 1 GB.

### Opsi B — Microservice (api + worker + scheduler terpisah)

Ditolak untuk v0.1. Overhead deploy dan debugging tidak sepadan untuk tim 1–2 orang.

### Opsi C — Serverless

Ditolak. Scheduler butuh proses berumur panjang; model fungsi berumur pendek
bertentangan dengan itu.

Batas modul tetap dijaga ketat sehingga **opsi B bisa diambil nanti tanpa
tulis ulang**. Worker hanya boleh impor `service` dan `repo`, tidak
`transport`, jadi memindahkannya ke binary kedua hanya soal `main.go` baru.

## 3. Struktur Backend Go

Satu folder `backend/`, satu binary. Tidak ada pemisahan folder admin.

```
backend/
  cmd/
    api/                       # entrypoint utama: config, db, router, worker
      main.go
    catalog-import/            # importer katalog, dijalankan manual/cron
      main.go
  internal/
    config/
      config.go                # satu struct Config dari env
    domain/                    # struct murni, tanpa import sql/http
      user.go                  # User, Role, AdminScope
      medication.go            # Medication, Frequency, Time, Log
      dose_event.go            # DoseEvent
      stock.go                 # Stock, StockTransaction
      catalog.go               # CatalogEntry, CurationStatus
      template.go              # MedicationTemplate
      carelink.go              # CareLink, Invite
      notification.go          # Job, Delivery, Prefs, DeviceToken, BumpItem
      adherence.go             # hasil hitung kepatuhan
      errors.go                # sentinel: ErrNotFound, ErrForbidden, ErrScopeDenied
    service/
      auth.go                  # register, login, refresh, revoke
      users.go                 # profil, timezone, prefs
      medications.go           # CRUD obat
      schedules.go             # frekuensi + jam
      logs.go                  # catat, koreksi, snooze
      adherence.go             # hitung dari aturan + log + dose_events
      stock.go                 # konsumsi, restock, adjust
      catalog.go               # cari preset + katalog pribadi
      carelinks.go             # invite, accept, revoke
      notifications.go         # buat job, kirim, retry, digest
      admin.go                 # kelola user, kurasi preset, agregat
    repo/
      postgres/
        db.go                  # pgxpool, ping
        migrate.go             # goose embedded
        user.go
        medication.go
        schedule.go
        log.go
        dose_event.go
        stock.go
        catalog.go
        template.go
        carelink.go
        notification.go
        bump_queue.go
        admin.go
    transport/
      http/
        router.go              # chi mux + mount
        middleware.go          # request-id, logger, recover, cors, jwt, ratelimit
        require_admin.go       # cek role + admin_scope
        respond.go             # satu fungsi tulis JSON envelope
        handlers/
          auth.go              # /auth/*
          users.go             # /users/*
          medications.go       # /medications/*
          times.go             # /medications/{id}/times/*
          logs.go              # /logs/*, /medications/{id}/logs
          stock.go             # /medications/{id}/stock/*, /stock/low
          catalog.go           # /catalog/*
          carelinks.go         # /care-links/*, /care/*
          notifications.go     # /notification-prefs, /device-tokens
          admin.go             # /admin/*  (semua rute admin di sini)
          health.go            # /healthz, /readyz
    worker/
      scheduler.go             # hitung dosis jatuh tempo dari aturan -> bump_queue
      dispatcher.go            # klaim bump_queue -> buat job -> kirim
      missed_pass.go           # pass harian: tulis dose_events missed
      sender_push.go           # web push VAPID
      sender_email.go          # SMTP relay (v0.2)
      sender_telegram.go       # Bot API (v0.2)
      digest.go                # ringkasan harian
      refill.go                # peringatan restock
      retention.go             # sapu sesi basi, arsip log, purge
    auth/
      jwt.go                   # sign/verify access token
      password.go              # Argon2id
      session.go               # refresh token opaque + rotasi
  migrations/                  # goose, embed
    00001_extensions.sql
    ...
  go.mod                       # go 1.27
  Dockerfile                   # multi-stage -> image alpine
  .env.example
```

### Urutan import (ditegakkan, tanpa pengecualian)

```
transport  ->  service, domain
service    ->  repo, domain
repo       ->  domain
domain     ->  (tidak impor paket internal lain)
worker     ->  service, repo   (tidak boleh transport)
auth       ->  domain
```

Ditegakkan lewat review dan `go vet`. Kalau nanti perlu otomatis, pakai
`depguard`. Belum dipasang supaya tidak menambah tooling di awal.

### Kenapa admin tidak dipisah folder

- **Data sama.** Admin dan user menyentuh tabel yang sama.
- **Otorisasi cukup.** `role` + `admin_scope` di `users` sudah membedakan
  kemampuan tanpa memecah proses.
- **Satu titik audit.** Semua aksi lewat satu jalur, sehingga `audit_log`
  konsisten dan `actor_role` diisi dari konteks, bukan ditebak.
- **Deploy ringan.** Satu image, satu rollback.

Yang **dipisah** hanya permukaan rutenya: `/admin/*` dengan
`RequireAdmin(scope)`. Ini penting supaya rate limit, audit, dan
log bisa dipisah tanpa memisah proses.

## 4. Struktur Frontend

Dua aplikasi, satu workspace.

```
frontend/
  pnpm-workspace.yaml
  package.json
  packages/
    ui/                        # komponen bersama (shadcn-svelte)
      src/lib/components/
        button/ input/ dialog/ table/ toast/ badge/ card/ form/ ...
      src/lib/styles/theme.css
    api-client/                # tipe + fetch wrapper hasil generate dari OpenAPI
      src/index.ts
      src/types.generated.ts
    config/                    # preset tailwind, tsconfig, eslint
  apps/
    user/                      # SvelteKit — mobile-first
      src/routes/
        +layout.svelte         # shell: bottom nav (mobile) / sidebar (desktop)
        +page.svelte           # redirect ke /hari-ini
        login/+page.svelte
        register/+page.svelte
        (app)/
          +layout.svelte       # guard: wajib login
          hari-ini/+page.svelte
          obat/+page.svelte
          obat/[id]/+page.svelte
          riwayat/+page.svelte
          caregiver/+page.svelte
          pengaturan/+page.svelte
      src/lib/
        api/                   # pembungkus api-client
        stores/session.svelte.ts
        stores/today.svelte.ts
        utils/time.ts
      static/manifest.webmanifest
      src/service-worker/sw.ts
    admin/                     # SvelteKit — desktop-first
      src/routes/
        +layout.svelte         # shell: sidebar tetap
        login/+page.svelte
        (admin)/
          +layout.svelte       # guard: wajib admin + scope
          dashboard/+page.svelte
          users/+page.svelte
          users/[id]/+page.svelte
          catalog/+page.svelte
          catalog/[id]/+page.svelte
          catalog/import/+page.svelte
          notifications/+page.svelte
          stats/+page.svelte
      src/lib/
        api/
        stores/session.svelte.ts
      svelte.config.js
  svelte.config.js             # dasar bersama
```

### Kenapa workspace, bukan dua repo terpisah

| Aspek | Workspace (dipilih) | Dua app mandiri |
|---|---|---|
| Komponen dasar | satu kali tulis | duplikat ~15 komponen |
| Tipe API | satu sumber | dua sumber, mudah drift |
| Setup | sedikit lebih rumit | paling sederhana |
| Perawatan | murah | mahal saat komponen berubah |

Yang beda antar app hanyalah **shell, navigasi, dan prioritas layout** — bukan
tombolnya. Jadi komponen dipakai bersama, sementara rute dan shell dipisah.

### Kenapa bukan satu app dengan `if (isAdmin)`

- Bundle user ikut membawa kode admin yang tidak pernah dia pakai.
- Shell bertabrakan: bottom-nav user dan sidebar admin harus hidup berdampingan.
- Guard jadi rumit dan mudah salah.

### Pendekatan layout

| | `apps/user` | `apps/admin` |
|---|---|---|
| Prioritas | 360px | ≥1280px |
| Navigasi kecil | bottom nav 4 item | drawer |
| Navigasi besar | sidebar ringkas | sidebar tetap |
| Kerapatan | longgar, target sentuh 44px | padat, tabel dan filter |
| Animasi | transisi halaman, kartu, haptic-feel | minimal, fokus kecepatan |
| Tema | terang utama, gelap opsional | terang, kontras tinggi |

### `adapter-static` tetap dipakai

Sesuai ADR-001, kedua app tetap di-prerender penuh. Tidak ada server Node di
produksi. Hasil build:

- `apps/user/build` → `/srv/user`
- `apps/admin/build` → `/srv/admin`

Caddy menyajikan keduanya, dengan `try_files {path} /index.html` untuk SPA
fallback.

## 5. Transport & Serialisasi

### Protokol

REST + JSON, prefix `/api/v1`. Satu-satunya transport. Bukan GraphQL, bukan gRPC.

### Envelope

Satu bentuk, dipakai di semua endpoint:

```json
{"data": {...}}
{"data": [...], "page": {"next_cursor": "...", "has_more": false, "limit": 20}}
{"error": {"code": "VALIDATION_FAILED", "message": "...", "details": []}}
```

Kontrak lengkap, daftar kode error, dan pagination ada di
`docs/design-v2/api-v2.md`. Dokumen itu yang jadi rujukan; bagian ini hanya
merangkum.

### Middleware (urutan eksekusi)

1. `RequestID` — buat UUID, masukkan ke context, set header.
2. `RealIP` — ambil IP dari header proxy Caddy.
3. `Logger` — catat method, path, status, durasi. **Nama obat tidak dicatat.**
4. `Recover` — tangkap panic, balas 500 generik, catat stack ke log.
5. `CORS` — hanya origin yang terdaftar.
6. `RateLimit` — per IP untuk rute publik, per user untuk rute terautentikasi.
7. `Auth` — parse JWT, isi context dengan user id, role, admin_scope.
8. `RequireAdmin(scope)` — hanya pada grup `/admin/*`.
9. `Audit` — tulis `audit_log` untuk aksi yang berjejak.

Urutan ini penting: `Recover` harus setelah `Logger` supaya panic tetap
tercatat, dan `Auth` harus sebelum `RequireAdmin`.

## 6. Konfigurasi & Secret

| Var | Wajib | Default | Keterangan |
|---|---|---|---|
| `APP_ENV` | ya | `development` | `development`/`staging`/`production` |
| `APP_PORT` | tidak | `8080` | port listen API |
| `APP_PUBLIC_URL` | ya | — | URL publik, untuk link undangan dan CORS |
| `DB_URL` | ya | — | `postgres://user:pass@postgres:5432/pantau_obat?sslmode=disable` |
| `DB_MAX_CONNS` | tidak | `20` | pool maks |
| `DB_MIN_CONNS` | tidak | `2` | pool min idle |
| `JWT_SECRET` | ya | — | min 32 byte acak |
| `JWT_TTL_MINUTES` | tidak | `15` | umur access token |
| `REFRESH_TTL_DAYS` | tidak | `30` | umur refresh token |
| `VAPID_PUBLIC_KEY` | ya* | — | *wajib bila kanal push aktif |
| `VAPID_PRIVATE_KEY` | ya* | — | *wajib bila kanal push aktif |
| `VAPID_SUBJECT` | ya* | — | `mailto:` atau URL |
| `SMTP_*` | ya* | — | *wajib bila kanal email aktif (v0.2) |
| `TELEGRAM_BOT_TOKEN` | ya* | — | *wajib bila kanal telegram aktif (v0.2) |
| `RATE_LIMIT_IP_PER_MIN` | tidak | `60` | batas per IP |
| `LOG_LEVEL` | tidak | `info` | `debug`/`info`/`warn`/`error` |
| `MIGRATE_ON_START` | tidak | `true` | jalankan goose saat start |

Validasi saat start: var wajib kosong = log fatal + `exit(1)`. Gagal cepat
lebih baik daripada jalan setengah konfigurasi.

## 7. Logging, Observability, Healthcheck, Shutdown

### Logging

- JSON terstruktur ke stdout. Docker menangkapnya.
- Field wajib: `level`, `time`, `request_id`, `method`, `path`, `status`, `duration_ms`.
- **Aturan keras: nama obat tidak boleh muncul di log.** Ini batas privasi,
  bukan preferensi. Gunakan id, bukan nama.
- Level: `debug` untuk detail, `info` untuk aksi sukses, `warn` untuk kegagalan
  yang bisa di-retry, `error` untuk kegagalan total.

### Observability minimal

Tanpa Prometheus di v1. Yang ada:

- Endpoint `/metrics` internal (opsional, hanya untuk jaringan internal).
- Event log terstruktur untuk: job dibuat, job sukses, job gagal, token dicabut,
  backpressure aktif.
- Halaman admin `stats` membaca agregat langsung dari DB.

### Healthcheck

| Path | Fungsi |
|---|---|
| `GET /healthz` | proses hidup. Selalu 200 kalau server jalan |
| `GET /readyz` | siap melayani. 503 kalau ping DB gagal |

Dipakai `docker-compose` healthcheck dan Caddy.

### Graceful shutdown

SIGTERM → berhenti menerima koneksi baru → tunggu request berjalan (maks 30 detik)
→ hentikan worker loop → tutup pool DB → keluar.

Worker loop memeriksa context cancellation di setiap iterasi supaya tidak
menggantung.

### Catch-up pass

Kalau worker mati beberapa jam, saat start ia menjalankan catch-up: menghitung
dosis jatuh tempo yang terlewat selama proses berhenti, lalu menuliskannya.
Tanpa ini, jadwal "at rule" akan kehilangan pengingat tanpa jejak.

## 8. Strategi Test

| Lapisan | Alat | Yang diuji | Frekuensi |
|---|---|---|---|
| Unit domain | `go test` | aturan frekuensi, perhitungan dosis, batas snooze | tiap commit |
| Unit service | `go test` + mock repo | orotisasi caregiver, batas admin_scope | tiap commit |
| Integration | `go test` + `testcontainers-go` (Postgres 18) | constraint unik, partial index, trigger stok | tiap PR |
| Kontrak enum | `go test` | nilai CHECK di DB vs konstanta Go | tiap PR |
| E2E | Playwright | alur daftar → obat → minum → caregiver, dan alur admin | sebelum rilis |
| Visual | Playwright screenshot | 360px dan 1280px | sebelum rilis |

Test kontrak enum penting: tipe enum di Postgres dan konstanta di Go sinkron
manual, jadi bisa drift. Test ini yang mendeteksi.

## 9. Deployment

Topologi mengikuti `deploy/` yang sudah ada.

```
Internet --HTTPS--> Caddy (:443)
                      |-- /            -> /srv/user    (apps/user/build)
                      |-- /admin/*     -> /srv/admin   (apps/admin/build)
                      |-- /api/*       -> api:8080
                    api  -> postgres:5432 (network internal, tanpa port publik)
                    postgres -> volume po-pgdata + po-pgbackup
```

File di VPS: `deploy/docker-compose.yml`, `deploy/.env`, `deploy/Caddyfile`.
Volume: `po-pgdata`, `po-pgbackup`.

Reverse proxy pakai **Caddy** karena TLS otomatis tanpa certbot, konfigurasi
sekitar 10 baris, dan reload tanpa downtime.

Service di `deploy/docker-compose.yml` saat ini baru `postgres`. Yang akan
menyusul: `api`, `caddy`, `migrate`, `backup`.

### Matriks environment

| Aspek | Development | Staging | Production |
|---|---|---|---|
| DB | container lokal, port 5432 | container, tanpa port publik | container, tanpa port publik |
| TLS | tidak | Caddy internal | Caddy Let's Encrypt |
| Migrasi | `MIGRATE_ON_START=true` | job terpisah | job terpisah |
| Backup | tidak | harian | harian, retensi 30 hari |
| Log level | `debug` | `info` | `info` |

## 10. Tradeoff yang Perlu Dikonfirmasi

1. **Satu proses = satu titik gagal.** Kalau scheduler crash, API ikut crash.
   Mitigasi: restart policy Docker. Kalau perlu uptime lebih tinggi, pecah
   worker jadi binary kedua (jalur sudah disiapkan).
2. **Perhitungan dosis di kode, bukan di DB.** Artinya logika jadwal ada di Go,
   dan tidak bisa dites dengan SQL saja. Mitigasi: unit test domain ketat.
3. **Tidak ada cache.** Hot path harus sudah punya index benar sejak awal.
4. **Backup belum aktif.** `deploy/` belum punya service backup. Jangan anggap
   deployment sekarang punya perlindungan data.
5. **Shared package frontend menambah satu langkah build.** Kalau tim hanya
   satu orang dan tidak mau setup workspace, dua app mandiri bisa diambil,
   dengan konsekuensi duplikasi.
6. **Foto resep di object storage.** Rekomendasi Cloudflare R2. Belum diputuskan.
