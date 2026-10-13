/**
 * Load layout tingkat akar.
 *
 * KENAPA TIDAK ADA `+layout.server.ts`
 *
 * Versi sebelumnya memakai server load untuk membaca cookie/`Accept-Language`
 * saat prerender. Dua masalah nyata muncul dari situ:
 *
 * 1. **Tidak berguna.** Halaman publik dirender SEKALI saat build, di mesin
 *    build. `Accept-Language` yang terbaca adalah milik mesin itu, bukan
 *    pengunjung — jadi HTML hasil build selalu berbahasa yang sama untuk
 *    semua orang. Cookie pun belum ada saat build.
 *
 * 2. **Merusak rute yang dirender di klien.** Adanya server load tingkat
 *    akar membuat SvelteKit meminta `<rute>/__data.json` untuk SEMUA rute,
 *    termasuk rute ber-guard yang `ssr = false` dan karenanya tidak punya
 *    berkas data itu. Permintaan itu dijawab 404, dan SvelteKit
 *    menafsirkannya sebagai "rute tidak ada" — halaman ber-guard menampilkan
 *    "404 Not Found" alih-alih isinya. Gejala ini hanya muncul pada build
 *    statis (produksi), tidak pada Vite dev server.
 *
 * Karena itu locale ditentukan di KLIEN (lihat `+layout.svelte`): cookie
 * pilihan pengguna, lalu preferensi peramban, lalu `DEFAULT_LOCALE`. Teks
 * berubah sepersekian detik setelah halaman tampil, dan itu memang harga
 * yang wajar untuk prerender statis — kamusnya sendiri sudah ikut di bundel,
 * jadi tidak ada permintaan jaringan tambahan.
 */
import type { LayoutLoad } from './$types';

export const load: LayoutLoad = () => {
	return {};
};
