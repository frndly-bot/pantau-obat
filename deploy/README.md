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
  caddy/
    Caddyfile            # berkas statis frontend + reverse proxy /api
  backup/                # hasil pg_dump (tidak masuk git)
  README.md
```

## Arsitektur service

```
internet
   │
   ▼
┌─────────┐   /api/*    ┌─────┐        ┌──────────┐
│  caddy  │────────────▶│ api │───────▶│ postgres │
│  (web)  │             └─────┘        └──────────┘
│         │  sisanya: berkas statis        ▲
└─────────┘                                │
      ▲                              ┌─────┴──────┐
      │ hasil build frontend         │ db-backup  │
      │ (adapter-static)             │ retention  │
                                     └────────────┘
```

**Hanya `web` (Caddy) yang menghadap internet.** Postgres dan API berada di
jaringan `internal`.

### Kenapa Caddy menyajikan frontend DAN mem-proxy /api dari satu origin

Cookie refresh memakai `SameSite=Strict`. Bila halaman dan API berada di origin
yang berbeda, browser **menolak** menyimpan cookie itu — dan gejalanya menipu:
login tampak berhasil (access token ada di memori), tetapi sesi hilang setiap
kali halaman di-refresh. Menyatukan keduanya di satu origin membuat cookie
selalu pihak-pertama.

Ini juga cara kerja pengembangan: dev server Vite mem-proxy `/api` ke backend
(lihat `frontend/apps/web/vite.config.ts`). Jadi perilaku di produksi dan di
lokal sama, bukan dua jalur berbeda yang bisa berbeda diam-diam.

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
| `api` | **ada** | binary Go, image multi-stage (scratch) |
| `migrate` | **ada** | job sekali jalan menjalankan goose sebelum `api` |
| `web` | **ada** | Caddy: berkas statis frontend + reverse proxy `/api` |
| `db-backup` | **ada** | cron pg_dump + retensi 14 hari |
| `retention` | **ada** | pembersihan data kedaluwarsa |
| `frontend-admin` | belum | build statis `apps/admin` (ADR-003) |

Saat menambah service, ikuti pola yang ada: satu service satu blok,
environment dari `.env`, `depends_on` dengan `condition: service_healthy`,
dan pasang `healthcheck`. Jangan expose port ke publik kecuali Caddy.

## Catatan penting untuk `web`

- **Cadangan SPA bernama `spa.html`, bukan `index.html`.** Halaman publik
  (`/`, `/login`, `/register`) adalah HTML terisi hasil prerender. Bila
  cadangannya `index.html`, berkas itu akan **menimpa** landing page dan
  seluruh isinya hilang — termasuk alasan ia di-prerender (SEO).
- **`SITE_ADDRESS`** menentukan alamat. `:80` untuk pengujian lokal; isi nama
  domain di produksi supaya Caddy menerbitkan sertifikat TLS otomatis.
- **`/api/*` selalu `Cache-Control: no-store`.** Tanpa itu, peramban atau proxy
  perantara bisa menyimpan balasan berisi data kesehatan dan menyajikannya ke
  pengguna lain di perangkat yang sama.
- **Setelah kode backend berubah, image `api` harus di-build ulang.** Container
  yang masih menjalankan image lama akan membalas 404 untuk endpoint baru —
  gejalanya mudah disalahartikan sebagai masalah Caddy atau frontend.

```bash
docker build -f backend/Dockerfile -t pantau-obat-api:latest .
docker compose up -d --force-recreate api
```

## Catatan penting untuk `retention`

Service ini menjalankan `api retention --loop`: menunggu sampai `RETENTION_HOUR`
(UTC, bawaan 3 = 10:00 WIB), membersihkan data kedaluwarsa, lalu menunggu lagi.

**Jangan mengganti entrypoint-nya menjadi `api retention` tanpa `--loop`.**
Perintah tanpa `--loop` adalah pekerjaan SEKALI JALAN: binary selesai lalu
keluar, dan dengan `restart: unless-stopped` Docker akan memulainya lagi —
menghasilkan restart loop. Ini pernah terjadi: **854 restart**, dan karena
backoff Docker naik sampai 60 detik, pembersihan berjalan tiap menit alih-alih
harian.

Untuk menjalankan sekali saja secara manual (mis. saat menelusuri masalah):

```bash
docker compose run --rm retention /api retention
```

Aturan umur data ada di `backend/cmd/api/main.go` (`runRetentionOnce`) — satu
tempat, supaya tidak ada SQL berserakan dengan nilai berbeda antar tempat.
Logikanya diuji di `backend/cmd/api/retention_test.go`.

## Mengekspos ke internet lewat Cloudflare Tunnel

Cara tercepat mencoba aplikasi dari perangkat lain (mis. ponsel) tanpa
membuka port di router dan tanpa sertifikat TLS sendiri:

```bash
cd deploy
docker compose up -d postgres api web

docker run --rm -it --network po-internal cloudflare/cloudflared:latest \
  tunnel --url http://web:80
```

Perintah itu mencetak URL acak seperti
`https://kata-acak-1234.trycloudflare.com`. Buka URL itu dari mana saja.

### Kenapa menunjuk ke `web`, bukan langsung ke `api`

Halaman dan API **harus** berada di satu origin. Cookie refresh memakai
`SameSite=Strict` (peramban menolak cookie antar-situs), dan CSP aplikasi
memakai `connect-src 'self'` (permintaan ke origin lain diblokir). Caddy
memenuhi keduanya: ia menyajikan berkas statis dan mem-proxy `/api` di
origin yang sama. Karena itu **tidak ada CORS yang perlu diatur**.

### Setelah URL tunnel diketahui, perbarui `APP_PUBLIC_URL`

Tautan undangan caregiver disusun dari nilai ini. Bila tidak diperbarui,
undangan menunjuk alamat lama dan penerimanya mendapat 404.

```bash
# deploy/.env
APP_PUBLIC_URL=https://kata-acak-1234.trycloudflare.com

docker compose up -d --force-recreate api
```

### Tunnel dengan URL tetap

Untuk domain sendiri, pakai named tunnel — contoh konfigurasi lengkap ada di
`cloudflared/config.yml.example`.

### Menguji dari luar

```bash
cd frontend/apps/web
TUNNEL_URL=https://kata-acak-1234.trycloudflare.com \
  CHROME_PATH=/usr/bin/google-chrome \
  node scripts/e2e-tunnel.mjs
```

Uji ini memeriksa hal yang **tidak bisa dibuktikan dari localhost**: cookie
refresh benar-benar tersimpan sebagai `HttpOnly` + `Secure` +
`SameSite=Strict`, sesi bertahan setelah muat ulang, dan data bertahan antar
halaman — semuanya lewat HTTPS publik.

**Jalankan uji ini setiap kali menyentuh `Caddyfile`, `svelte.config.js`,
atau berkas rute.** Tiga bug produksi pernah lolos ke sini karena seluruh
pengujian lain berjalan lewat Vite dev server, yang tidak mengirim header
CSP dan selalu menyediakan server load:

1. **CSP memblokir seluruh JavaScript.** `script-src 'self'` menolak skrip
   inline yang dipakai SvelteKit untuk hidrasi — form terkunci, aplikasi
   tidak bisa dipakai sama sekali. Diperbaiki dengan `csp: { mode: 'hash' }`
   di `svelte.config.js`; header CSP di Caddy dihapus karena dua kebijakan
   CSP diterapkan bersamaan (irisan) dan yang tanpa hash akan memblokir lagi.
2. **`+layout.server.ts` tingkat akar merusak rute ber-guard.** Keberadaannya
   membuat SvelteKit meminta `<rute>/__data.json` untuk semua rute, termasuk
   yang `ssr = false` dan tidak punya berkas itu. Dihilangkan seluruhnya —
   server load itu memang tidak berguna untuk halaman yang di-prerender.
3. **`data.locale` pada rute CSR melempar `TypeError`**, sehingga penjaga
   rute tidak pernah selesai berjalan dan tamu melihat halaman error alih-alih
   dialihkan ke `/login`.

## Catatan keamanan

- `POSTGRES_PASSWORD` di `.env.example` hanya placeholder.
- Di produksi, jangan bind port postgres ke `0.0.0.0`. Ubah jadi
  `127.0.0.1:${POSTGRES_PORT}:5432` supaya hanya bisa diakses dari host.
- Jaringan `internal` bersifat private; hanya Caddy yang nanti keluar ke publik.
- Backup rutin belum aktif. Jangan anggap deployment ini punya perlindungan
  data sampai service `backup` dipasang.
