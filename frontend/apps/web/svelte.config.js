import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/**
 * Konfigurasi SvelteKit.
 *
 * `adapter-static` dipakai sesuai ADR-001 (tanpa runtime Node di VPS) dan
 * karena frontend ini akan dibungkus Capacitor — Capacitor hanya bisa
 * menjalankan bundel statis, bukan server.
 *
 * Halaman publik (landing, login, register) di-PRERENDER menjadi HTML terisi
 * saat build, sehingga tetap SEO dan muat cepat. Halaman ber-guard dirender
 * di klien (`ssr = false` di `(app)/+layout.ts`) karena butuh sesi.
 */
/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		adapter: adapter({
			pages: 'build',
			assets: 'build',
			// Fallback untuk rute aplikasi (CSR). Sengaja BUKAN `index.html`:
			// halaman publik di-prerender dan `/` harus tetap menjadi landing
			// hasil prerender. Server (Caddy) menyajikan `spa.html` untuk path
			// yang tidak punya berkas sendiri.
			fallback: 'spa.html',
			precompress: false,
			strict: false,
		}),
		alias: {
			$lib: './src/lib',
		},
	},
};

export default config;
