# Pantau Obat — Backend

Status: **jalan dan terverifikasi.** 84 rute terdaftar di 12 kelompok handler,
92 unit test lulus, dan uji e2e apidoc **79/79 request, 173/173 assertion**.
Kanal notifikasi aktif untuk v0.1: in-app (push/email/telegram ditunda v0.2).
Dokumen arsitektur: `docs/design-v2/architecture-v2.md` bagian 3.

Backend adalah satu binary Go. Admin dan user memakai API yang sama;
pemisahan dilakukan di layer otorisasi (`RequireAdmin`), bukan di folder.

```
backend/
  cmd/
    api/main.go                # entrypoint: config, db, router, worker
    admin-bootstrap/main.go    # buat admin pertama (HANYA di luar produksi)
  internal/
    config/ domain/ service/ repo/postgres/ transport/http/ worker/ auth/
  migrations -> internal/repo/postgres/migrations   # symlink
  go.mod                       # go 1.27
  .env.example
```

Cara menjalankan di lokal:

```bash
scripts/dev-api.sh            # jalan di depan, Ctrl-C untuk berhenti
scripts/dev-api.sh --bg       # jalan di latar, log ke /tmp/pantau-obat-api.log
```

Skrip itu memuat kredensial DB dari `deploy/.env`, membuat `JWT_SECRET` acak,
dan mengisi `APP_PUBLIC_URL`. Tanpa itu, `go run ./cmd/api` langsung berhenti
dengan "konfigurasi tidak valid: APP_PUBLIC_URL wajib diisi".

Untuk alur admin, akun admin pertama harus dibuat lebih dulu — peran admin
tidak pernah bisa diberikan lewat API:

```bash
cd backend
go run ./cmd/admin-bootstrap --email admin@example.com \
    --password 'Rahasia123!' --name "Admin Aplikasi"
```

### Kenapa migrasi ada di dalam `internal/repo/postgres/`

Direktori migrasi ada di `internal/repo/postgres/migrations/`, dan
`backend/migrations` adalah **symlink** ke sana.

Alasannya teknis: `//go:embed` hanya bisa membaca berkas di dalam direktori
paketnya. Migrasi di-embed ke dalam binary supaya satu image cukup, tanpa
perlu menyalin folder `.sql` ke sampingnya. Symlink dipertahankan supaya
`make db-migrate` dari root repo tetap bekerja seperti sebelumnya.

## Toolchain

| Alat | Versi |
|---|---|
| Go | 1.27.2 |

Kalau `go` belum ada, dari root repo:

```bash
make setup-toolchain     # pasang Go + Node sekaligus
# atau hanya periksa:
make check-tools
```

Go dipasang **user-local** di `$HOME/.local/opt/go`, tanpa sudo.

## Perintah

Dari **root repo** (Makefile ada di sana):

```bash
make help                # daftar semua target
make check-tools         # periksa toolchain

make be-build            # build -> backend/bin/api
make be-run              # jalan dengan go run
make be-test             # go test ./...
make be-lint             # go vet + gofmt check
make be-tidy             # go mod tidy
make be-clean            # hapus artefak build
```

Kalau lebih suka langsung di folder ini:

```bash
cd backend
go build -o bin/api ./cmd/api
go run ./cmd/api
go test ./... -count=1
go vet ./...
gofmt -l .
```

## Menyiapkan project dari nol

```bash
cd backend
go mod init github.com/frndly-bot/pantau-obat/backend
go get github.com/jackc/pgx/v5
go get github.com/go-chi/chi/v5
go get github.com/pressly/goose/v3
mkdir -p cmd/api internal/{config,domain,service,repo/postgres,transport/http,worker,auth} migrations
```

## Konfigurasi

Repo hanya menyimpan `.env.example`. File `.env` asli tidak masuk git.

```bash
cp .env.example .env
```

Database: lihat `deploy/README.md`. Untuk menjalankan dari root:

```bash
make db-up        # nyalakan PostgreSQL 18
make db-migrate   # jalankan migrasi goose
```

## Catatan implementasi yang wajib dipatuhi

Empat hal ini berasal dari keputusan yang sudah dikunci. Melanggarnya
membuat schema dan kode tidak sinkron.

1. **Aturan import, tanpa pengecualian.**
   `transport → service, domain` · `service → repo, domain` ·
   `repo → domain` · `domain → (tidak impor paket internal)` ·
   `worker → service, repo` (tidak boleh `transport`).

2. **Enum di Go harus sama persis dengan CHECK di `backend/migrations/`.**
   Semuanya bahasa Inggris: `after_meal`, `morning`, `child`, `imported`.
   Buat test kontrak yang membandingkan nilai CHECK di DB dengan konstanta Go.
   Tipe enum Postgres dan konstanta Go sinkron manual, jadi bisa drift.

3. **Jadwal disimpan sebagai aturan, bukan baris per hari.**
   Tidak ada tabel slot. Waktu dosis dihitung dari `medication_frequency` +
   `medication_times`. Identitas dosis memakai *occurrence key*:
   `<schedule_id>|<YYYY-MM-DD>|<HH>:<MM>`. Buat **satu fungsi** pembentuk kunci
   dan pakai di semua tempat (`medication_logs`, `medication_dose_events`,
   `bump_queue`). Kalau formatnya beda di dua tempat, anti-ganda gagal.
   Rincian: `docs/decisions-v2/ADR-005-jadwal-aturan.md`.

4. **Nama obat tidak boleh masuk log aplikasi.** Pakai `medication_id`.
   Ini batas privasi, bukan preferensi.

## Yang belum ada

- **`Dockerfile` backend.** Belum ada; `deploy/docker-compose.yml` baru
  menyediakan service `postgres`.
- **Service `migrate` dan `backup`** di `deploy/docker-compose.yml`.
- **Importer KFA** — ditunda, menunggu kredensial per environment.
- **Kanal notifikasi nyata.** Kode push/email ada, tapi belum ada kredensial
  yang dikonfigurasi, jadi pengingat hanya dicatat ke log.

## Yang sudah ada

| Bagian | Isi |
|---|---|
| `go.mod` | Go 1.27.2, pgx v5, chi v5, goose v3, uuid |
| `config/` | pembacaan + validasi env, gagal cepat, kumpulkan semua masalah |
| `domain/` | sentinel error, **semua enum**, occurrence key, aturan pencabutan |
| `repo/postgres/` | 25 migrasi goose, query per agregat, pemetaan SQLSTATE |
| `service/` | auth, users, medications, schedules, logs, stock, catalog, carelinks, adherence, notifications, admin, webhook |
| `transport/http/` | envelope JSON, middleware, rate limit, validator parameter path |
| `transport/http/handlers/` | 12 kelompok: auth, users, medications, logs, stock, catalog, carelinks, adherence, notifications, admin, webhook, health |
| `worker/` | scheduler, dispatcher, missed_pass, sender push/email/log |
| `cmd/api/main.go` | config → db → migrasi → router → worker → graceful shutdown |
| `cmd/admin-bootstrap/` | buat admin pertama, menolak berjalan di produksi |

**84 rute** terdaftar. Rinciannya per kelompok:

| Kelompok | Rute | Kelompok | Rute |
|---|---|---|---|
| auth | 6 | stock | 6 |
| users | 4 | catalog | 6 |
| medications | 9 | carelinks | 16 |
| logs | 10 | adherence | 3 |
| notifications | 6 | admin | 16 |
| webhook | 5 | health | 2 (di luar `/api/v1`) |

Sudah terverifikasi: `go build ./...`, `go vet ./...`, `gofmt -l .` bersih,
**92 unit test lulus**, server berjalan, `/healthz` dan `/readyz` membalas 200,
SIGTERM berhenti rapi, dan `make apidoc-run` **lulus 75/75 request dengan
159/159 assertion**.

## Syarat kelayakan backend

Backend dianggap layak kalau **setiap endpoint** punya file di `../apidoc/`
dengan contoh request dan contoh response yang benar-benar bisa dijalankan.
Folder itu sekaligus berfungsi sebagai uji end-to-end.

```bash
make apidoc-check    # hitung file yang sudah ada
make apidoc-install  # pasang bruno CLI (bru)
make apidoc-run      # jalankan koleksi sebagai uji e2e
```

Detail dan daftar endpoint ada di `../apidoc/README.md`.

Urutan pengerjaan berikutnya: **`Dockerfile` + service migrate/backup** →
importer katalog → kanal notifikasi nyata (kredensial VAPID/SMTP) → frontend.

## Migrasi

Migrasi goose ada di `migrations/`, **25 file**. Perintahnya dari root repo:

```bash
make db-migrate      # naikkan semua yang belum
make db-status       # lihat status
make db-rollback N=1 # turunkan satu langkah
make db-new NAME=nama_perubahan
```

Aturan menulis migrasi ada di `../db/README.md`. Yang paling penting:
**fungsi plpgsql wajib memakai `-- +goose StatementBegin`/`StatementEnd`**,
karena goose tidak paham dolar-quote PostgreSQL.
