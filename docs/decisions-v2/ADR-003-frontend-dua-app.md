# ADR-003 v2: Frontend Dua Aplikasi

- Status: Accepted
- Tanggal: 2026-10-10
- Terkait: `ADR-001-stack-v2`, `docs/design-v2/architecture-v2.md` bagian 4

## Context

Aplikasi punya dua permukaan dengan kebutuhan yang berbeda jauh:

| | User (pasien + caregiver) | Admin |
|---|---|---|
| Perangkat utama | HP Android kelas menengah, 360px | desktop, 1280px ke atas |
| Tugas utama | satu ketukan: catat dosis | banyak baris: cari, filter, ubah |
| Navigasi | bottom nav, 4 item | sidebar tetap + breadcrumb |
| Kerapatan | longgar, target sentuh 44px | padat, tabel banyak kolom |
| Animasi | penting, memberi umpan balik | minim, jangan mengalihkan |
| Frekuensi pakai | beberapa kali sehari | sesekali |

Pertanyaannya: satu aplikasi SvelteKit dengan rute `/admin`, dua aplikasi yang
berbagi paket, atau dua aplikasi yang sepenuhnya mandiri?

## Decision

**Dua aplikasi SvelteKit dalam satu workspace, berbagi paket komponen dan
klien API.**

```
frontend/
  pnpm-workspace.yaml
  packages/
    ui/                # shadcn-svelte + komponen bersama
    api-client/        # tipe + fetch wrapper dari OpenAPI
    config/            # preset tailwind, tsconfig, eslint
  apps/
    user/              # mobile-first
    admin/             # desktop-first
```

Yang **dibagi**: komponen dasar (Button, Input, Dialog, Table, Toast, Badge),
token tema, klien API, tipe hasil generate, konfigurasi lint/TS.

Yang **dipisah**: rute, shell/layout, navigasi, prioritas responsif, dan
guard otorisasi.

Keduanya tetap memakai `adapter-static` (sesuai ADR-001). Hasil build:
`apps/user/build` → `/srv/user`, `apps/admin/build` → `/srv/admin`.

## Consequences

### Positive

1. **Komponen dasar tidak diduplikasi.** Sekitar 15 komponen dipakai dua app.
2. **Tipe API satu sumber.** `api-client` digenerate dari OpenAPI backend,
   sehingga perubahan kontrak terdeteksi saat build, bukan saat produksi.
3. **Shell tidak bertabrakan.** Bottom nav mobile dan sidebar admin tidak perlu
   hidup berdampingan.
4. **Bundle user tetap kecil.** Kode admin tidak ikut terbawa.
5. **Guard lebih sederhana.** `apps/user` tidak perlu tahu soal `admin_scope`;
   `apps/admin` tidak perlu tahu soal bottom nav.
6. **Deploy tetap statis.** Dua folder build, tidak ada runtime Node.

### Negative

1. **Setup lebih rumit.** Perlu `pnpm workspace` dan pemahaman monorepo.
2. **Satu langkah build tambahan.** `packages/ui` harus dibangun sebelum app.
3. **Dev server dua proses.** Perlu dijalankan sekaligus (mis. `pnpm -r dev`).
4. **Versi Svelte harus disamakan.** Kalau `packages/ui` dan app memakai versi
   berbeda, komponen bisa gagal.

### Neutral

1. **shadcn-svelte komponen masuk ke repo.** Bukan dependency npm, jadi bebas
   diubah. Konsekuensi: pembaruan upstream manual.
2. **Tailwind v4 satu konfigurasi.** Dipakai dua app lewat preset bersama.

## Alternatives Considered

| Alternatif | Kenapa Ditolak |
|---|---|
| **Satu app dengan rute `/admin`** | Bundle user membawa kode admin. Shell bertabrakan. Guard mudah salah karena satu layout harus melayani dua pola navigasi. |
| **Dua app sepenuhnya mandiri (dua repo)** | ~15 komponen dasar dan klien API diduplikasi. Komponen yang berubah harus disalin dua kali. Setup paling sederhana, perawatan paling mahal. |
| **Satu app, layout bersyarat `if (isAdmin)`** | Masalah yang sama dengan opsi pertama, ditambah logika bersyarat di banyak tempat. |
| **Micro-frontend** | Terlalu berat untuk dua permukaan dan tim 1–2 orang. |
| **SSR untuk admin** | Tidak perlu. Admin dipakai di desktop dengan koneksi stabil, prerender sudah cukup. |

## Catatan Implementasi

- `packages/api-client` digenerate dari OpenAPI yang di-export backend. Backend
  mengekspos `GET /openapi.json` hanya di environment non-produksi.
- `packages/ui` memakai komponen shadcn-svelte yang disalin ke repo, bukan
  di-import dari registry saat build.
- Guard di `apps/admin` memeriksa `role === 'admin'` **dan** `admin_scope` untuk
  rute tertentu. Pemeriksaan tetap diulang di backend; guard frontend hanya
  untuk pengalaman pengguna, bukan keamanan.
- Tema diambil dari `packages/ui/src/lib/styles/theme.css`, dengan variabel
  yang bisa di-override per app.

## Yang Belum Diputuskan

- Apakah `packages/ui` perlu versi/publish internal, atau cukup dipakai lewat
  workspace saja. Rekomendasi: cukup workspace, belum perlu publish.
- Apakah admin butuh dark mode. Rekomendasi: tidak untuk v1.
