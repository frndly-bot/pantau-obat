# Pantau Obat — Frontend

Status: **Fase 0–2 selesai** (fondasi, landing page, login/register).
Fase 3 (halaman user lengkap) sedang berjalan.
Dokumen arsitektur: `docs/design-v2/architecture-v2.md` bagian 4.
Keputusan dua aplikasi: `docs/decisions-v2/ADR-003-frontend-dua-app.md`.
Rencana lengkap: `.hermes/plans/2026-10-12_implementasi-sveltekit-user.md`.

## Struktur

```
frontend/
  pnpm-workspace.yaml
  package.json
  packages/
    ui/              # komponen bersama (Svelte 5 + Tailwind v4)
    api-client/      # tipe kontrak + klien fetch + adapter sesi
    i18n/            # kamus JSON + mesin terjemahan
  apps/
    web/             # landing + aplikasi user (satu app)
```

**Satu app (`apps/web`), bukan dua.** Landing → login → aplikasi adalah satu
alur dengan domain dan sesi yang sama; memisahkannya berarti dua build, dua
deploy, dan dua salinan klien API hanya untuk tiga halaman publik. Bundel
statis yang sama juga menjadi sumber pembungkusan Capacitor nanti.
`apps/admin` tetap terpisah sesuai ADR-003 (belum dikerjakan).

## Toolchain

| Alat | Versi terpasang |
|---|---|
| Node | v26.7.0 |
| pnpm | 11.17.0 (lewat `corepack enable pnpm`) |
| SvelteKit | 2.70.3 |
| Svelte | 5.57.2 (runes) |
| Tailwind CSS | 4.x (`@theme`, CSS-first) |
| adapter | `@sveltejs/adapter-static` |

## Perintah

```bash
cd frontend
pnpm install
pnpm --filter @pantau-obat/web dev      # dev server di :5173
pnpm --filter @pantau-obat/web check    # svelte-check + tsc
pnpm --filter @pantau-obat/web build    # build statis ke apps/web/build
```

## Menjalankan secara lokal

Backend harus jalan dalam mode **development**, bukan production. Ini penting
di localhost: pada mode production cookie refresh memakai atribut `Secure`
(diabaikan browser pada `http://`) dan CORS menolak origin localhost —
akibatnya login selalu gagal.

```bash
# Terminal 1 — backend di :8081 (development)
APP_ENV=development APP_PORT=8081 scripts/dev-api.sh

# Terminal 2 — frontend
cd frontend/apps/web && pnpm dev
```

Buka <http://localhost:5173> (atau <http://127.0.0.1:5173> — keduanya sama).

### Kenapa API dipanggil lewat dev server, bukan langsung ke backend

`VITE_API_BASE_URL` bawaannya **relatif** (`/api/v1`), dan dev server mem-proxy
`/api` ke backend (`vite.config.ts`). Alasannya bukan kenyamanan:

Cookie refresh memakai `SameSite=Strict`. Bila halaman dibuka di
`localhost:5173` sementara API dipanggil ke `127.0.0.1:8081`, browser
menganggapnya **dua situs berbeda** dan menolak menyimpan cookie itu. Gejalanya
menipu: login tampak berhasil (access token ada di memori), tetapi **begitu
halaman di-refresh, sesi hilang** dan tidak bisa dipulihkan.

Dengan proxy, browser hanya pernah bicara ke satu origin. Ini juga meniru cara
produksi bekerja: Caddy menyajikan berkas statis dan mem-proxy `/api`.

Bila backend berjalan di port lain, arahkan proxy tanpa mengubah kode:

```bash
PO_API_TARGET=http://127.0.0.1:9000 pnpm dev
```

**Untuk Capacitor nanti**, alamat absolut diisi lewat `VITE_API_BASE_URL`
(di sana tidak ada origin web, jadi proxy tidak berlaku).

## Keputusan arsitektur yang wajib dipatuhi

### 1. `adapter-static`, tanpa server Node

Sesuai ADR-001. Halaman publik (`/`, `/login`, `/register`) di-**prerender**
menjadi HTML terisi saat build — SEO baik, muat cepat, tanpa runtime Node.
Halaman ber-guard dirender di klien (`ssr = false` di `(app)/+layout.ts`)
karena isinya bergantung pada sesi.

> Fallback SPA bernama **`spa.html`**, bukan `index.html`. Kalau memakai
> `index.html`, adapter menimpanya dan landing page hasil prerender hilang
> (sudah pernah terjadi). Server harus menyajikan `spa.html` untuk path yang
> tidak punya berkas sendiri.

### 2. Sesi: token di memori + refresh di cookie HttpOnly

Frontend ini akan dibungkus **Capacitor** menjadi aplikasi mobile. Capacitor
tidak bisa menjalankan server Node, jadi pola BFF (proxy sisi server) tidak
bisa dipakai. Karena itu:

| | Tempat |
|---|---|
| Refresh token | **Tidak pernah di JavaScript.** Web: cookie `po_refresh` (HttpOnly, SameSite=Strict, Path=/api/v1/auth). Mobile nanti: secure storage native. |
| Access token | Memori saja (`ApiClient`), umur pendek (~15 menit) |
| localStorage | **Tidak dipakai** untuk token apa pun |

`packages/api-client/src/session/` berisi `SessionAdapter` — satu-satunya titik
yang perlu ditukar saat Capacitor ditambahkan (`native.ts` sudah disiapkan
sebagai kerangka). UI dan pemanggilan API tidak berubah.

Penanda `po_session=1` (bukan token) dipakai supaya JS tahu kapan perlu
memanggil `/auth/refresh`; tanpa itu setiap kunjungan tamu memicu refresh gagal.

### 3. Backend membungkus balasan dalam envelope

Semua balasan sukses berbentuk `{ "data": ..., "page"?: ... }`, error
`{ "error": { "code", "message", "details"? } }`. `ApiClient` membuka envelope
ini; `requestPage()` dipakai bila informasi paginasi dibutuhkan.

Rincian error validasi ada di `error.details` (array `{field, issue}`), diakses
lewat `ApiError.issueFor('password')`.

### 4. Terjemahan enum dilakukan di frontend

Database menyimpan bahasa Inggris (`after_meal`, `morning`, `child`). Frontend
yang memetakannya ke Bahasa Indonesia. Jangan mengirim nilai Indonesia ke API.

### 5. Mobile-first adalah requirement

Semua halaman user harus terbaca di 360px tanpa zoom. Target sentuh minimal
44×44 px (token `--touch`).

## i18n

Kamus ada di `packages/i18n/locales/{id,en}.json` — 13 domain, 747 kunci per
bahasa, hasil konversi dari `design/assets/i18n.js`.

```ts
import { createTranslator } from '@pantau-obat/i18n';
const { t } = createTranslator(dict, fallback);
t('today.title');
```

- **JSON, bukan .js** — bisa dipakai alat terjemahan apa pun.
- **Nested per domain** (`today.*`, `meds.*`, `settings.*`).
- **Fallback berantai**: locale aktif → `id` → nama kunci.
- **Interpolasi** `{nama}` didukung.
- **Type-safe key**: `TranslationKey` menangkap salah ketik saat `tsc`.

### Perbaikan mojibake (penting)

Sumber `design/assets/i18n.js` mengandung **mojibake**: karakter UTF-8 tersimpan
seolah latin-1/Windows-1252, lalu di-encode UTF-8 lagi. Contoh: `"6–20 digit"`
menjadi `"6â€"20 digit"`, `"akun…"` menjadi `"akunâ€¦"` (59 entri).

`packages/i18n/scripts/repair-mojibake.mjs` memulihkannya dengan memetakan
kembali lewat tabel **cp1252** (bukan latin-1 — byte 0x80–0x9F di cp1252
memetakan ke `€`, `"`, `…`, bukan karakter kontrol). Jalankan ulang bila kamus
diregenerasi dari sumber:

```bash
cd frontend/packages/i18n
node scripts/repair-mojibake.mjs
```

**Sumber aslinya sebaiknya diperbaiki juga**, supaya regenerasi berikutnya
tidak memerlukan langkah ini.

## Pengujian

```bash
# Alur auth terhadap backend nyata (register → me → refresh → logout)
node apps/web/scripts/e2e-auth.mjs

# Sesi bertahan setelah refresh (termasuk kasus lintas-situs)
node apps/web/scripts/e2e-session-persist.mjs

# Jadwal harian: slot, status due/pending, anti-ganda
node apps/web/scripts/e2e-schedule.mjs

# Alur browser (Playwright): register lewat UI, cookie HttpOnly,
# tidak ada token di localStorage, navigasi, pemulihan sesi, logout
node apps/web/scripts/e2e-browser.mjs
```

`e2e-auth.mjs` membutuhkan backend di `http://127.0.0.1:8081`.
`e2e-browser.mjs` membutuhkan dev server di `:5173` dan Playwright tersedia
(`CHROME_PATH` dapat diarahkan ke Chromium yang sudah terpasang).

## Yang belum dikerjakan

- **Fase 3**: wizard tambah obat (5 pola frekuensi), detail obat + stok,
  filter riwayat + ekspor CSV, alur undang/terima caregiver, ganti sandi,
  kanal notifikasi, perangkat, audit-log.
- **Fase 4**: PWA (manifest, service worker), vitest untuk util, konfigurasi
  Caddy.
- **`apps/admin`** — plan terpisah.
- **Capacitor** — hanya kerangka adapter; CORS perlu origin
  `capacitor://localhost` (iOS) dan `http://localhost` (Android).

## Keterbatasan backend yang ditemukan

1. **Tidak ada endpoint jadwal dosis.** `GET /logs/today` hanya mengembalikan
   dosis yang **sudah dicatat**, bukan daftar dosis hari ini. Generator jadwal
   (`domain/occurrence.go`) ada tetapi belum diekspos. Halaman "Hari Ini"
   menampilkan apa yang tersedia dan menyatakan keterbatasan ini terbuka,
   bukan mengarang data.

2. **Backend belum punya `/openapi.json`.** Tipe di
   `packages/api-client/src/types.ts` ditulis tangan dari
   `docs/design-v2/api-v2.md` dan bentuk balasan handler.
