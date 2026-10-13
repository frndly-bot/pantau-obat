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
		/**
		 * CSP dengan HASH OTOMATIS.
		 *
		 * SvelteKit menyisipkan satu skrip inline kecil di setiap halaman
		 * (bootstrap `__sveltekit_*`) untuk menghidrasi halaman. Tanpa
		 * pengecualian, `script-src 'self'` memblokirnya — dan yang terjadi
		 * bukan sekadar peringatan di konsol: SELURUH JavaScript aplikasi
		 * tidak pernah dimuat, sehingga form terkunci dan aplikasi tidak bisa
		 * dipakai sama sekali.
		 *
		 * Ini pernah lolos ke produksi karena seluruh pengujian sebelumnya
		 * berjalan lewat Vite dev server, yang tidak mengirim header CSP.
		 * Header CSP hanya ada di Caddy.
		 *
		 * `mode: 'hash'` membuat SvelteKit menghitung hash skrip inline itu
		 * saat build dan menyisipkannya ke header `Content-Security-Policy`
		 * pada HTML hasil prerender. Karena yang diizinkan adalah hash isi
		 * skrip (bukan `'unsafe-inline'`), skrip yang disisipkan penyerang
		 * tetap diblokir — perlindungan XSS tidak berkurang.
		 *
		 * `directives` sengaja hanya mendefinisikan `script-src`; sisanya
		 * tetap diatur Caddy (lihat `deploy/caddy/Caddyfile`). SvelteKit
		 * menggabungkan keduanya, dan header dari Caddy tidak ditimpa.
		 */
		csp: {
			mode: 'hash',
			directives: {
				'script-src': ['self'],
			},
		},
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
