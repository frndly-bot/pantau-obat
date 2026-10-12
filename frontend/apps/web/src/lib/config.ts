/**
 * Konfigurasi runtime aplikasi.
 *
 * Memakai `import.meta.env` (Vite) dengan prefiks `VITE_` — bukan `$env/*`
 * SvelteKit — supaya nilai yang sama bekerja di dev, build statis, prerender,
 * dan nanti di dalam Capacitor. Bisa ditimpa lewat berkas `.env`.
 */

function readEnv(key: string, fallback: string): string {
	const value = (import.meta.env as Record<string, string | undefined>)[key];
	return value && value.length > 0 ? value : fallback;
}

/**
 * Basis URL API.
 *
 * Bawaannya RELATIF (`/api/v1`), dan itu disengaja:
 *
 * - **Dev**: dev server mem-proxy `/api` ke backend (lihat `vite.config.ts`),
 *   sehingga browser hanya bicara ke satu origin. Cookie refresh ber-
 *   `SameSite=Strict` hanya mau disimpan bila permintaannya sesama situs.
 *   Kalau di sini dipakai alamat absolut ke port backend, halaman yang dibuka
 *   di `localhost` akan menganggap API di `127.0.0.1` sebagai situs lain dan
 *   cookie-nya ditolak — sesi hilang setiap kali halaman di-refresh.
 * - **Produksi**: Caddy menyajikan berkas statis dan mem-proxy `/api` ke
 *   backend pada origin yang sama. Jadi nilai relatif ini juga yang benar.
 * - **Capacitor** (nanti): di sana tidak ada origin web, jadi alamat absolut
 *   diisi lewat `VITE_API_BASE_URL`.
 */
export const API_BASE_URL = readEnv('VITE_API_BASE_URL', '/api/v1');

/** URL repositori, dipakai di landing page. */
export const GITHUB_URL = readEnv(
	'VITE_GITHUB_URL',
	'https://github.com/frndly-bot/pantau-obat'
);

/** Nama produk. */
export const APP_NAME = 'Pantau Obat';
