#!/usr/bin/env bash
# Jalankan backend API untuk pengembangan lokal.
#
# Kenapa ada: menjalankan `go run ./cmd/api` butuh 6 variabel lingkungan yang
# nilainya tersebar di dua tempat (deploy/.env untuk DB, .env.example untuk
# sisanya). Tanpa skrip ini, setiap orang menulis ulang rantai export yang
# sama dan cepat atau lambat ada yang lupa APP_PUBLIC_URL lalu bingung
# kenapa startup gagal.
#
# Pemakaian:
#   scripts/dev-api.sh            # jalan di depan, Ctrl-C untuk berhenti
#   scripts/dev-api.sh --bg       # jalan di latar, tulis PID ke scratch/
#
# Catatan: JWT_SECRET dibuat acak tiap kali dijalankan. Artinya token dari
# proses sebelumnya TIDAK berlaku setelah restart. Itu disengaja untuk lokal:
# tidak ada secret yang tersimpan di disk.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BE_DIR="$ROOT/backend"
ENV_FILE="$ROOT/deploy/.env"
LOG_FILE="${TMPDIR:-/tmp}/pantau-obat-api.log"

export PATH="$HOME/.local/opt/go/bin:$PATH"
export GOROOT="$HOME/.local/opt/go"

if [[ ! -f "$ENV_FILE" ]]; then
	printf 'deploy/.env tidak ada. Salin dari deploy/.env.example dulu.\n' >&2
	exit 1
fi

# Muat kredensial database dari deploy/.env (POSTGRES_DB, POSTGRES_USER,
# POSTGRES_PASSWORD, POSTGRES_PORT). File itu format KEY=VALUE tanpa export.
set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a

: "${POSTGRES_DB:?POSTGRES_DB kosong di deploy/.env}"
: "${POSTGRES_USER:?POSTGRES_USER kosong di deploy/.env}"
: "${POSTGRES_PASSWORD:?POSTGRES_PASSWORD kosong di deploy/.env}"

export DB_URL="postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@127.0.0.1:${POSTGRES_PORT:-5432}/${POSTGRES_DB}?sslmode=disable"
export JWT_SECRET="$(openssl rand -base64 48 | tr -d '\n')"
# Backend dev dijalankan dengan APP_ENV=development supaya cookie refresh
# TIDAK memakai atribut Secure (localhost bukan HTTPS — cookie Secure akan
# diabaikan browser) dan CORS mengizinkan origin localhost. Tanpa ini,
# login dari dev server selalu gagal.
export APP_ENV="${APP_ENV:-development}"
# APP_PUBLIC_URL adalah alamat APLIKASI WEB, bukan alamat backend.
#
# Backend memakainya untuk menyusun tautan undangan caregiver
# (`{APP_PUBLIC_URL}/terima-undangan?token=...`). Di produksi keduanya
# berbagi satu origin lewat Caddy, jadi nilainya adalah domain aplikasi.
# Di dev, aplikasi disajikan Vite di :5173 sedangkan backend di :8081 —
# menunjuk ke port backend membuat tautan undangan menuju 404, dan alur
# caregiver tidak bisa diuji sama sekali.
#
# PENTING: nilai ini DIPAKSA, bukan memakai `${VAR:-default}`.
#
# `deploy/.env` ikut dimuat di atas, dan setelah Cloudflare Tunnel dipakai
# berkas itu berisi `APP_PUBLIC_URL=https://….trycloudflare.com`. Dengan
# `${VAR:-default}` nilai itu MENANG, sehingga tautan undangan di dev
# menunjuk ke tunnel — uji lokal lalu membuka halaman publik dan gagal
# dengan gejala yang membingungkan (elemen tidak ditemukan, padahal
# halamannya memang halaman lain).
#
# Bila memang perlu menimpa, pakai variabel terpisah `DEV_PUBLIC_URL`.
export APP_PUBLIC_URL="${DEV_PUBLIC_URL:-http://127.0.0.1:5173}"
export APP_PORT="${APP_PORT:-8080}"
export MIGRATE_ON_START="${MIGRATE_ON_START:-false}"
export LOG_LEVEL="${LOG_LEVEL:-info}"

# SMTP untuk pengembangan menunjuk MAILHOG, bukan server surat sungguhan.
#
# Alur lupa sandi butuh mengirim email, dan menguji dengan SMTP nyata berarti
# mengirim surat ke alamat orang sungguhan. MailHog menampung semuanya dan
# menampilkannya di http://127.0.0.1:8025, jadi tautan reset bisa dibaca
# tanpa ada yang benar-benar terkirim.
#
# Bila SMTP_HOST diisi di lingkungan (misalnya untuk menguji server surat
# sungguhan), nilai itu yang menang.
if [[ -z "${SMTP_HOST:-}" ]]; then
	export SMTP_HOST="127.0.0.1"
	export SMTP_PORT="1025"
	export SMTP_FROM="Pantau Obat <no-reply@pantau-obat.local>"
	export SMTP_USER=""
	export SMTP_PASSWORD=""
fi

cd "$BE_DIR"

if [[ "${1:-}" == "--bg" ]]; then
	nohup go run ./cmd/api >"$LOG_FILE" 2>&1 &
	echo "pid=$! log=$LOG_FILE"
	exit 0
fi

exec go run ./cmd/api