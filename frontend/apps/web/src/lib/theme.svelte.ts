/**
 * Tema aplikasi (terang/gelap) — satu sumber untuk seluruh aplikasi.
 *
 * KENAPA ADA BERKAS INI
 *
 * Sebelumnya logika tema hanya hidup di dalam `+layout.svelte`: membaca
 * `localStorage.po_theme` lalu menulis `data-theme` ke `<html>`. Itu cukup
 * untuk menerapkan tema saat halaman dimuat, tapi tidak menyediakan cara
 * MENGUBAHNYA dari halaman lain — sehingga halaman Pengaturan tidak bisa
 * menawarkan pilihan tema sama sekali, padahal prototipe design
 * menempatkannya di sana.
 *
 * Modul ini memindahkan logika itu ke satu tempat, sehingga layout dan
 * halaman Pengaturan memakai aturan yang sama dan tidak ada dua salinan yang
 * bisa saling menyimpang.
 *
 * URUTAN PENENTUAN TEMA
 *
 *   1. Pilihan pengguna yang tersimpan (`po_theme`)
 *   2. Preferensi sistem (`prefers-color-scheme`)
 *   3. Terang
 *
 * Pilihan sistem DIHORMATI, bukan dipaksa — pengguna yang perangkatnya
 * bertema gelap langsung mendapat tema gelap tanpa perlu mengubah apa pun.
 * Begitu ia memilih sendiri, pilihannya yang berlaku.
 */
import { browser } from '$app/environment';

export type Theme = 'light' | 'dark';

const STORAGE_KEY = 'po_theme';

class ThemeState {
	/** Tema yang sedang aktif. */
	current = $state<Theme>('light');

	/**
	 * Menerapkan tema ke `<html>` dan menyimpannya.
	 *
	 * `data-theme` dibaca `theme.css`; `.dark` ikut dipasang karena itu
	 * konvensi shadcn dan beberapa komponen memakainya.
	 */
	set(theme: Theme) {
		this.current = theme;
		if (!browser) return;
		document.documentElement.dataset.theme = theme;
		document.documentElement.classList.toggle('dark', theme === 'dark');
		localStorage.setItem(STORAGE_KEY, theme);
	}

	/**
	 * Menentukan tema awal dan menerapkannya.
	 *
	 * Dipanggil sekali dari layout saat aplikasi dimuat. Tidak menimpa
	 * pilihan tersimpan — kalau ada, itu yang dipakai.
	 */
	init() {
		if (!browser) return;

		const stored = localStorage.getItem(STORAGE_KEY);
		if (stored === 'dark' || stored === 'light') {
			this.set(stored);
			return;
		}

		// Belum pernah memilih: hormati preferensi sistem, dan ikuti
		// perubahannya supaya tema tidak "tertinggal" saat pengguna
		// berpindah mode di perangkatnya.
		const media = window.matchMedia('(prefers-color-scheme: dark)');
		this.set(media.matches ? 'dark' : 'light');

		media.addEventListener('change', (e) => {
			// Hanya ikut bila pengguna belum memilih sendiri.
			if (!localStorage.getItem(STORAGE_KEY)) this.set(e.matches ? 'dark' : 'light');
		});
	}

	/** Membalik tema; dipakai tombol cepat. */
	toggle() {
		this.set(this.current === 'dark' ? 'light' : 'dark');
	}
}

export const theme = new ThemeState();
