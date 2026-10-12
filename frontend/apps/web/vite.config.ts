import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

/**
 * Alamat backend untuk pengembangan.
 *
 * Diambil dari `PO_API_TARGET` bila ada, supaya port backend tidak perlu
 * diubah di kode saat backend berjalan di port lain.
 */
const API_TARGET = process.env.PO_API_TARGET ?? 'http://127.0.0.1:8081';

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	server: {
		// Dijalankan dengan --host 0.0.0.0 supaya bisa dibuka dari browser.
		port: 5173,
		strictPort: false,
		proxy: {
			/**
			 * Meneruskan /api ke backend.
			 *
			 * KENAPA INI PERLU, bukan sekadar kenyamanan:
			 *
			 * Cookie refresh (`po_refresh`) memakai `SameSite=Strict`. Kalau
			 * halaman dibuka di `localhost:5173` sementara API ada di
			 * `127.0.0.1:8081`, browser menganggapnya DUA SITUS BERBEDA dan
			 * menolak menyimpan cookie itu — persis seperti yang terjadi pada
			 * `SameSite=None` tanpa `Secure`. Akibatnya login "berhasil" (access
			 * token ada di memori) tetapi begitu halaman di-refresh, token
			 * hilang dan sesi tidak bisa dipulihkan.
			 *
			 * Dengan proxy, browser hanya pernah bicara ke SATU origin
			 * (localhost:5173), jadi cookie selalu pihak-pertama. Ini juga
			 * meniru cara produksi bekerja: Caddy menyajikan berkas statis dan
			 * mem-proxy /api ke backend.
			 */
			'/api': {
				target: API_TARGET,
				changeOrigin: false,
				// Teruskan cookie apa adanya, termasuk atribut Path dan SameSite.
				cookieDomainRewrite: '',
			},
		},
	},
});
