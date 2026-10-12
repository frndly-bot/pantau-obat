# ============================================================================
# Frontend Pantau Obat — build statis, disajikan Caddy
# ============================================================================
#
# Multi-stage: Node hanya dipakai untuk membangun, hasilnya berkas statis.
# Yang masuk ke image akhir hanya Caddy + hasil build — bukan Node, bukan
# node_modules. Alasannya sama dengan backend: semakin sedikit runtime yang
# ikut, semakin sedikit yang bisa rusak saat image dasar diperbarui.
#
# Build dijalankan dari ROOT repo (lihat `context` di docker-compose.yml),
# karena workspace pnpm mencakup beberapa paket.

# ---------------------------------------------------------------------------
# Tahap 1 — build
# ---------------------------------------------------------------------------
FROM node:22-alpine AS build

WORKDIR /src

# corepack menyediakan pnpm sesuai `packageManager` di package.json, supaya
# versi pnpm di CI dan di mesin pengembang tidak berbeda.
RUN corepack enable

# Manifes dulu, kode belakangan: lapisan instalasi tidak ikut dibuang setiap
# kali ada perubahan kode.
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./
COPY frontend/apps/web/package.json ./apps/web/
COPY frontend/packages/api-client/package.json ./packages/api-client/
COPY frontend/packages/i18n/package.json ./packages/i18n/
COPY frontend/packages/ui/package.json ./packages/ui/

# `--frozen-lockfile` menolak lockfile yang tidak sinkron dengan manifest.
# Tanpa itu, build bisa "berhasil" dengan dependensi yang berbeda dari yang
# diuji.
RUN pnpm install --frozen-lockfile

COPY frontend/ ./

# VITE_API_BASE_URL sengaja DIBIARKAN KOSONG: bawaannya relatif (`/api/v1`),
# dan Caddy mem-proxy `/api` di origin yang sama. Mengisinya dengan alamat
# absolut akan membuat cookie refresh (SameSite=Strict) ditolak peramban dan
# sesi hilang setiap halaman di-refresh.
RUN pnpm --filter @pantau-obat/web build

# ---------------------------------------------------------------------------
# Tahap 2 — sajikan
# ---------------------------------------------------------------------------
FROM caddy:2-alpine

COPY --from=build /src/apps/web/build /srv
COPY deploy/caddy/Caddyfile /etc/caddy/Caddyfile

# Caddy membaca $SITE_ADDRESS dari environment (lihat Caddyfile).
ENV SITE_ADDRESS=:80

EXPOSE 80 443

# Caddy punya healthcheck sendiri lewat `caddy validate`, tetapi image
# scratch-like ini tidak punya shell; jadi tidak dipasang di sini dan
# kesehatan diperiksa dari luar bila diperlukan.
