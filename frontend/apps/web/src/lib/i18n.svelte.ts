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
	FALLBACK_LOCALE,
	isLocale,
	LOCALE_COOKIE,
	type Dictionary,
	type Locale,
} from '@pantau-obat/i18n';
import { browser } from '$app/environment';

class I18nState {
	locale = $state<Locale>('id');
	private dict = $state<Dictionary>({});
	private fallback = $state<Dictionary>({});

	/** Dipanggil dari +layout.ts saat load. */
	async init(locale: Locale) {
		this.locale = locale;
		const fallback = await loadDictionary(FALLBACK_LOCALE);
		this.fallback = fallback;
		this.dict = locale === FALLBACK_LOCALE ? fallback : await loadDictionary(locale);
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
