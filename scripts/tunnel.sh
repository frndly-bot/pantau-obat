#!/usr/bin/env bash
#
# Menjalankan Cloudflare Tunnel untuk Pantau Obat.
#
# KENAPA TUNNEL MENUNJUK KE `web` DAN BUKAN KE `api` ATAU VITE
#
# Halaman dan API harus berada di SATU origin:
#   - Cookie refresh memakai SameSite=Strict; peramban menolak cookie
#     antar-situs, sehingga sesi hilang setiap halaman dimuat ulang.
#   - CSP aplikasi memakai connect-src 'self', sehingga permintaan ke origin
#     lain diblokir peramban.
#
# Caddy (service `web`) menyajikan berkas statis DAN mem-proxy `/api` di
# origin yang sama, jadi tidak ada CORS yang perlu diatur. Menunjuk tunnel
# langsung ke Vite (:5173) juga tidak cukup: itu dev server, dan header CSP
# serta perilaku produksinya berbeda.
#
# PEMAKAIAN
#   bash scripts/tunnel.sh              # tunnel cepat (URL acak)
#   bash scripts/tunnel.sh --stop       # hentikan tunnel yang berjalan
#   bash scripts/tunnel.sh --update-env <URL>
#                                       # tulis APP_PUBLIC_URL lalu recreate api
#
# Setelah URL muncul, JANGAN LUPA memperbarui APP_PUBLIC_URL. Tautan undangan
# caregiver disusun dari nilai itu; bila tidak diperbarui, undangan menunjuk
# alamat lama dan penerimanya mendapat 404.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
DEPLOY_DIR="$REPO_DIR/deploy"
NAME="po-tunnel"
NETWORK="po-internal"

case "${1:-}" in
	--stop)
		if docker ps -a --format '{{.Names}}' | grep -qx "$NAME"; then
			docker rm -f "$NAME" >/dev/null && echo "tunnel dihentikan"
		else
			echo "tidak ada tunnel yang berjalan"
		fi
		exit 0
		;;
	--update-env)
		URL="${2:-}"
		if [[ -z "$URL" ]]; then
			echo "pemakaian: bash scripts/tunnel.sh --update-env <URL>" >&2
			exit 2
		fi
		ENV_FILE="$DEPLOY_DIR/.env"
		if [[ ! -f "$ENV_FILE" ]]; then
			echo "deploy/.env tidak ditemukan — salin dari .env.example dulu" >&2
			exit 1
		fi
		# Ganti baris yang ada, atau tambahkan bila belum ada.
		if grep -q '^APP_PUBLIC_URL=' "$ENV_FILE"; then
			# `|` dipakai sebagai pemisah supaya URL berisi `/` tidak merusak.
			sed -i "s|^APP_PUBLIC_URL=.*|APP_PUBLIC_URL=$URL|" "$ENV_FILE"
		else
			printf '\nAPP_PUBLIC_URL=%s\n' "$URL" >>"$ENV_FILE"
		fi
		echo "APP_PUBLIC_URL=$URL"
		(cd "$DEPLOY_DIR" && docker compose up -d --force-recreate api)
		echo
		echo "Selanjutnya: uji dari luar dengan"
		echo "  TUNNEL_URL=$URL node frontend/apps/web/scripts/e2e-tunnel.mjs"
		exit 0
		;;
esac

# Caddy harus sudah jalan; tunnel menunjuk ke sana.
if ! docker ps --format '{{.Names}}' | grep -qx 'po-web'; then
	echo "po-web belum jalan. Jalankan dulu:" >&2
	echo "  cd deploy && docker compose up -d postgres api web" >&2
	exit 1
fi

docker rm -f "$NAME" >/dev/null 2>&1 || true

echo "Menjalankan tunnel (Ctrl-C untuk berhenti)…"
echo "Setelah URL muncul, jalankan: bash scripts/tunnel.sh --update-env <URL>"
echo

# `-it` dilewati bila bukan terminal (mis. dijalankan dari skrip), supaya
# perintah tidak gagal dengan "the input device is not a TTY".
TTY_FLAG=""
[[ -t 1 ]] && TTY_FLAG="-it"

exec docker run --rm $TTY_FLAG --name "$NAME" --network "$NETWORK" \
	cloudflare/cloudflared:latest tunnel --url http://web:80
