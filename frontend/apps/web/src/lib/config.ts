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

/** Basis URL API. Di dev mengarah ke backend lokal di :8080. */
export const API_BASE_URL = readEnv('VITE_API_BASE_URL', 'http://127.0.0.1:8080/api/v1');

/** URL repositori, dipakai di landing page. */
export const GITHUB_URL = readEnv(
	'VITE_GITHUB_URL',
	'https://github.com/frndly-bot/pantau-obat'
);

/** Nama produk. */
export const APP_NAME = 'Pantau Obat';
