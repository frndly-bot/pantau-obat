# Pantau Obat — Frontend

Status: **belum ada kode.** Folder ini masih kosong.
Dokumen arsitektur: `docs/design-v2/architecture-v2.md` bagian 4.
Keputusan dua aplikasi: `docs/decisions-v2/ADR-003-frontend-dua-app.md`.

Dua aplikasi SvelteKit dalam satu workspace:

| App | Perangkat utama | Navigasi | Tujuan |
|---|---|---|---|
| `apps/user` | HP 360px | bottom nav | catat dosis, riwayat, caregiver |
| `apps/admin` | desktop ≥1280px | sidebar | kelola user, kurasi katalog, agregat |

Komponen dasar dan klien API dipakai bersama, bukan diduplikasi.

```
frontend/
  pnpm-workspace.yaml
  package.json
  packages/
    ui/              # komponen bersama (shadcn-svelte)
    api-client/      # tipe + fetch wrapper, generate dari OpenAPI backend
    config/          # preset tailwind, tsconfig, eslint
  apps/
    user/            # SvelteKit, adapter-static, mobile-first
    admin/           # SvelteKit, adapter-static, desktop-first
```

## Toolchain

| Alat | Versi |
|---|---|
| Node | 24 LTS (Krypton) |
| npm | bawaan Node |
| pnpm | 12.x (stabil terbaru) |
| TypeScript | 7.x (stabil terbaru) |
| Svelte | 5.x |
| SvelteKit | 3.x |

Kalau belum ada, dari root repo:

```bash
make setup-toolchain     # pasang Go + nvm + Node + pnpm + TypeScript
make check-tools         # hanya periksa
```

Node dipasang lewat **nvm**, tidak mengubah Node sistem.

## Perintah

Dari **root repo**:

```bash
make fe-install          # pnpm install seluruh workspace
make fe-dev-user         # dev server aplikasi user
make fe-dev-admin        # dev server aplikasi admin
make fe-build            # build produksi kedua app (statis)
make fe-test             # vitest
make fe-check            # svelte-check + tsc
make fe-clean            # hapus artefak build
```

Kalau lebih suka langsung di folder ini:

```bash
cd frontend
pnpm install
pnpm --filter user dev
pnpm --filter admin dev
pnpm -r build
pnpm -r check
```

## Menyiapkan project dari nol

Workspace root:

```bash
cd frontend
corepack enable
# buat pnpm-workspace.yaml:
#   packages:
#     - "packages/*"
#     - "apps/*"
```

Setiap aplikasi:

```bash
pnpm dlx sv create apps/user --template minimal --types ts
pnpm dlx sv create apps/admin --template minimal --types ts
```

shadcn-svelte dan dependency utama (jalankan di dalam tiap app):

```bash
pnpm dlx shadcn-svelte@latest init
pnpm dlx shadcn-svelte@latest add button input dialog table toast badge card form
pnpm add -D tailwindcss @tailwindcss/vite
pnpm add lucide-svelte
pnpm add -D @vite-pwa/sveltekit
pnpm add -D vitest @playwright/test
pnpm add sveltekit-superforms zod
pnpm add -D @tanstack/table-core
```

## Konfigurasi

| Variabel | Dipakai di | Isi |
|---|---|---|
| `PUBLIC_API_BASE_URL` | dua app | base URL API, mis. `/api/v1` |

Keduanya di-prerender, jadi tidak ada secret di frontend. Jangan pernah
menaruh kunci rahasia di variabel `PUBLIC_*`.

## Catatan implementasi yang wajib dipatuhi

1. **`adapter-static` dengan prerender penuh.** Tidak ada server Node di
   produksi. Hasil build diserve Caddy.

2. **Terjemahan enum dilakukan di frontend.** Database menyimpan bahasa Inggris
   (`after_meal`, `morning`, `child`). Frontend yang memetakannya ke Bahasa
   Indonesia. Jangan mengirim nilai Indonesia ke API.

3. **Guard admin hanya untuk pengalaman pengguna, bukan keamanan.** Periksa
   `role === 'admin'` dan `admin_scope` untuk menyembunyikan menu, tapi
   pemeriksaan sebenarnya tetap di backend. Rute admin selalu diverifikasi ulang.

4. **`apps/admin` tidak boleh mengakses isi obat pasien.** Kalau UI admin
   menampilkan data agregat, itu memang desainnya, bukan kekurangan.
   Rincian: `docs/design-v2/api-v2.md` bagian 11.

5. **Mobile responsive adalah requirement, bukan polish.** Semua halaman user
   harus terbaca di 360px tanpa zoom. Target sentuh minimal 44×44 px.

6. **Tipe API satu sumber.** `packages/api-client` digenerate dari OpenAPI
   backend, jangan ditulis manual. Kalau kontrak berubah, build harus gagal.

## Yang belum ada

- `pnpm-workspace.yaml` dan seluruh kode.
- `packages/ui`, `packages/api-client`, `packages/config`.
- Konfigurasi Tailwind v4, shadcn-svelte, PWA, Vitest, Playwright.
- Halaman wireframe HTML (prinsip repositori: wireframe per halaman sebelum
  implementasi, bukan ASCII).
