/**
 * i18n aplikasi web (Svelte 5 runes).
 *
 * "Dihandle SSR" (keputusan 4) diwujudkan begini: kamus dimuat di
 * `+layout.ts`, sehingga saat halaman publik di-PRERENDER, teks sudah benar
 * di HTML hasil build — tidak ada kedipan teks. Di klien, kamus yang sama
 * dipakai ulang dari `data` layout tanpa permintaan tambahan.
 */
import {
	createTranslator,
	loadDictionary,
	DEFAULT_LOCALE,
	FALLBACK_LOCALE,
	isLocale,
	LOCALE_COOKIE,
	type Dictionary,
	type Locale,
} from '@pantau-obat/i18n';
import { browser } from '$app/environment';
// Kamus cadangan dimuat SEKARANG, bukan lewat `await`.
//
// `createTranslator` mengembalikan NAMA KUNCI bila kunci tidak ditemukan.
// Sebelumnya kamus diisi lewat `await loadDictionary(...)` di dalam
// `init()`, sehingga pada render pertama `dict` masih kosong dan halaman
// sempat menampilkan `settings.appearance.theme` alih-alih "Tema".
//
// Impor statis ini membuat kamus cadangan sudah tersedia sejak render
// pertama, jadi kunci selalu punya terjemahan. Bundelnya tidak membengkak
// berarti: `id.json` toh selalu ikut sebagai cadangan.
import idDict from '@pantau-obat/i18n/locales/id.json';

class I18nState {
	locale = $state<Locale>(DEFAULT_LOCALE);
	private dict = $state<Dictionary>(idDict as Dictionary);
	private fallback = $state<Dictionary>(idDict as Dictionary);

	/**
	 * Menetapkan locale aktif dan memuat kamusnya.
	 *
	 * Bila locale sama dengan cadangan, tidak ada permintaan tambahan —
	 * kamus yang sudah diimpor statis itu yang dipakai.
	 */
	async init(locale: Locale) {
		this.locale = locale;
		if (locale === FALLBACK_LOCALE) {
			this.dict = this.fallback;
			return;
		}
		this.dict = await loadDictionary(locale);
	}

	/** Terjemahkan satu kunci. */
	t(key: string, vars?: Record<string, string | number>): string {
		return createTranslator(this.dict, this.fallback).t(key, vars);
	}

	/** Terjemahkan daftar kunci sekaligus. */
	ta(keys: readonly string[]): string[] {
		return createTranslator(this.dict, this.fallback).ta(keys);
	}

	/** Ganti bahasa, simpan pilihan, lalu muat kamus baru. */
	async setLocale(locale: Locale) {
		if (!isLocale(locale)) return;
		if (browser) {
			document.cookie = `${LOCALE_COOKIE}=${locale}; Path=/; Max-Age=${60 * 60 * 24 * 365}; SameSite=Lax`;
			document.documentElement.lang = locale;
		}
		await this.init(locale);
	}

	/** Kode locale aktif. */
	get current(): Locale {
		return this.locale;
	}
}

export const i18n = new I18nState();
