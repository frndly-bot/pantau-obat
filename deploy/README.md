# Deploy — Pantau Obat

Folder ini berisi seluruh konfigurasi deployment. Saat ini baru
**PostgreSQL 18**. Service lain (backend Go, frontend user + admin, Caddy)
akan menyusul di folder yang sama.

> Sebelum deploy ke server, baca bagian **Batasan platform** di paling bawah.

## Prasyarat

| Alat | Versi | Keterangan |
|---|---|---|
| Docker Engine | 24+ | |
| Docker Compose | v2+ | perintah `docker compose`, bukan `docker-compose` |

Tidak perlu Go atau Node untuk menjalankan database saja.

## Struktur

```
deploy/
  docker-compose.yml     # definisi service
  .env.example           # contoh environment; salin jadi .env
  postgres/
    postgresql.conf      # tuning hemat resource
    initdb/
      00-init.sql        # dijalankan otomatis saat volume masih kosong
  backup/                # hasil pg_dump (tidak masuk git)
  README.md
```

## Mulai cepat

Dari root repo (lebih disarankan, supaya perintah konsisten):

```bash
make db-up         # nyalakan PostgreSQL 18
make db-migrate    # jalankan migrasi goose
make verify        # cek toolchain + status DB + jumlah tabel
```

Atau langsung di folder ini:

```bash
cd deploy
cp .env.example .env    # lalu edit password
docker compose up -d
docker compose ps
docker compose logs -f postgres
```

## Environment

Semua nilai dibaca dari `.env` di folder ini (tidak masuk git). Docker Compose
memuat `.env` secara otomatis, jadi tidak perlu flag tambahan.

| Variabel | Default | Keterangan |
|---|---|---|
| `POSTGRES_DB` | `pantau_obat` | nama database |
| `POSTGRES_USER` | `pantau_obat` | user aplikasi, bukan superuser |
| `POSTGRES_PASSWORD` | — | **wajib diisi** |
| `POSTGRES_PORT` | `5432` | port di host |

Buat password yang kuat:

```bash
openssl rand -base64 32
```

## Profil resource

Dituning untuk VPS kecil (1 vCPU / 1 GB RAM). Postgres tidak akan memakai
lebih dari sekitar 300 MB.

Tuning ada di `postgres/postgresql.conf`:

| Parameter | Nilai | Alasan |
|---|---|---|
| `shared_buffers` | `128MB` | cukup untuk data awal |
| `effective_cache_size` | `512MB` | asumsi cache OS di VPS 1 GB |
| `work_mem` | `4MB` | hindari swap saat banyak sort/join |
| `maintenance_work_mem` | `64MB` | cukup untuk vacuum/analyze |
| `max_connections` | `20` | app kecil; pool di backend cukup sedikit |
| `max_parallel_workers_per_gather` | `1` | VPS hanya 1 vCPU |
| `wal_compression` | `on` | hemat I/O |
| `statement_timeout` | `30s` | cegah query menggantung |
| `idle_in_transaction_session_timeout` | `5min` | cegah slot pool habis |

Batasan container:

- `mem_limit: 320m` — mencegah VPS kehabisan RAM.
- `cpus: "1.0"`
- `shm_size: 128mb`

## Data dan volume

| Volume | Isi |
|---|---|
| `po-pgdata` | seluruh data cluster |
| `po-pgbackup` | cadangan untuk arsip `pg_dump` |

**Hapus volume = data hilang.** Perintah `make db-reset` melakukan itu.

Perintah umum:

```bash
make db-up           # nyalakan
make db-down         # matikan, data tetap
make db-reset        # HATI-HATI: hapus volume, mulai dari nol
make db-migrate      # jalankan migrasi goose yang belum naik
make db-status       # lihat status migrasi
make db-rollback N=1 # turunkan satu langkah
make db-hard-reset   # HATI-HATI: rollback penuh lalu naik lagi
make db-shell        # psql interaktif
make db-ps           # status container
make db-logs         # ikuti log
make db-backup       # dump ke deploy/backup/
make db-restore FILE=deploy/backup/xxx.dump
```

## Skema database

Sumber: `../backend/migrations/` (19 file goose, hasilnya 23 tabel + 2 view).

```bash
make db-migrate
```

goose mencatat versi di tabel `goose_db_version`, jadi perintah ini hanya
menjalankan migrasi yang belum naik. Aman diulang dan tidak menghapus data.

Ekstensi `pgcrypto` dan `citext` dipasang oleh migrasi `00001_extensions.sql`,
bukan oleh `initdb`. Jadi tidak butuh hak superuser saat migrasi.

Untuk rollback ke keadaan kosong:

```bash
make db-hard-reset   # rollback semua, lalu migrasi ulang
```

Aturan menulis migrasi ada di `../db/README.md`.

## Batasan platform

Script setup toolchain (`../scripts/setup-toolchain.sh`) dan Makefile
didukung di:

- **macOS** — Intel (`x86_64`) dan Apple Silicon (`arm64`).
- **Linux Debian-based** — Ubuntu, Debian, Linux Mint, Pop!_OS.

**Windows tidak didukung langsung.** Pakai WSL2:

```powershell
# PowerShell sebagai Administrator
wsl --install -d Ubuntu
```

Restart komputer, buka Ubuntu, lalu jalankan dari dalam WSL:

```bash
git clone <repo> && cd pantau-obat
make setup-toolchain
make db-up
```

Docker Desktop dengan integrasi WSL2 juga sudah cukup untuk menjalankan
service di folder ini.

## Belum ada di sini

| Service | Status | Catatan |
|---|---|---|
| `postgres` | **ada** | PostgreSQL 18, resource minimal |
| `backend` | belum | binary Go, image multi-stage Alpine |
| `frontend-user` | belum | build statis `apps/user` |
| `frontend-admin` | belum | build statis `apps/admin` |
| `caddy` | belum | TLS + static + reverse proxy ke backend |
| `migrate` | belum | job sekali jalan menjalankan goose sebelum `api` start |
| `backup` | belum | cron pg_dump + retensi 30 hari |

Saat menambah service, ikuti pola yang ada: satu service satu blok,
environment dari `.env`, `depends_on` dengan `condition: service_healthy`,
dan pasang `healthcheck`. Jangan expose port ke publik kecuali Caddy.

## Catatan keamanan

- `POSTGRES_PASSWORD` di `.env.example` hanya placeholder.
- Di produksi, jangan bind port postgres ke `0.0.0.0`. Ubah jadi
  `127.0.0.1:${POSTGRES_PORT}:5432` supaya hanya bisa diakses dari host.
- Jaringan `internal` bersifat private; hanya Caddy yang nanti keluar ke publik.
- Backup rutin belum aktif. Jangan anggap deployment ini punya perlindungan
  data sampai service `backup` dipasang.
