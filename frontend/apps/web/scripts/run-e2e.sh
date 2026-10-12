#!/usr/bin/env bash
# ============================================================================
# Menjalankan seluruh uji end-to-end frontend.
# ============================================================================
#
# Uji-uji ini memakai peramban sungguhan (Playwright) karena hal yang
# dibuktikan di sini TIDAK BISA dibuktikan oleh uji API:
#   - cookie HttpOnly benar-benar diterima peramban
#   - token tidak pernah masuk localStorage
#   - sesi bertahan setelah halaman di-refresh
#   - tombol benar-benar mengubah keadaan di backend
#
# PRASYARAT
#   1. Backend dev:  APP_ENV=development APP_PORT=8081 scripts/dev-api.sh
#   2. Dev server:   cd frontend/apps/web && pnpm dev
#   3. Playwright:   pnpm add -D playwright   (di frontend/apps/web)
#      Bila sudah ada peramban Chromium di sistem, tunjuk lewat CHROME_PATH
#      supaya Playwright tidak perlu mengunduh sendiri:
#        export CHROME_PATH=/usr/bin/google-chrome
#
# PEMAKAIAN
#   bash scripts/run-e2e.sh              # semua uji
#   bash scripts/run-e2e.sh e2e-wizard   # satu uji saja

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Backend dan dev server harus hidup. Gagal cepat dengan pesan yang jelas jauh
# lebih berguna daripada 12 uji yang semuanya timeout satu per satu.
check_up() {
	local url="$1" label="$2"
	if ! curl -s -o /dev/null -m 5 "$url"; then
		printf '\n%s tidak merespons: %s\n' "$label" "$url" >&2
		printf 'Nyalakan dulu (lihat komentar di kepala skrip ini).\n' >&2
		exit 1
	fi
}

check_up "http://127.0.0.1:8081/healthz" "Backend dev"
check_up "http://127.0.0.1:5173/" "Dev server frontend"

if ! node -e "require.resolve('playwright')" 2>/dev/null; then
	printf '\nplaywright belum terpasang. Jalankan:\n' >&2
	printf '  cd frontend/apps/web && pnpm add -D playwright\n' >&2
	exit 1
fi

# Urutan penting: e2e-auth lebih dulu karena ia memverifikasi fondasi sesi.
# Bila fondasi itu rusak, uji lain akan gagal dengan gejala yang membingungkan.
SUITES=(
	e2e-auth
	e2e-session-persist
	e2e-schedule
	e2e-browser
	e2e-hariini
	e2e-wizard
	e2e-detail
	e2e-riwayat
	e2e-caregiver
	e2e-pengaturan
)

if [[ $# -gt 0 ]]; then
	SUITES=("$@")
fi

pass=0
fail=0
failed_names=()

for suite in "${SUITES[@]}"; do
	file="$SCRIPT_DIR/$suite.mjs"
	if [[ ! -f "$file" ]]; then
		printf '  %sLEWAT%s %s (berkas tidak ada)\n' "$(tput setaf 3 2>/dev/null)" "$(tput sgr0 2>/dev/null)" "$suite"
		continue
	fi

	printf '\n%s── %s ──%s\n' "$(tput bold 2>/dev/null)" "$suite" "$(tput sgr0 2>/dev/null)"
	if node "$file"; then
		pass=$((pass + 1))
	else
		fail=$((fail + 1))
		failed_names+=("$suite")
	fi
done

printf '\n════════════════════════════════════════\n'
printf 'Lulus: %d   Gagal: %d\n' "$pass" "$fail"
if [[ $fail -gt 0 ]]; then
	printf 'Yang gagal:\n'
	for name in "${failed_names[@]}"; do printf '  - %s\n' "$name"; done
	exit 1
fi
printf 'Semua uji lulus.\n'
