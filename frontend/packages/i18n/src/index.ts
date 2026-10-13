/**
 * Mesin i18n Pantau Obat.
 *
 * Prinsip:
 * - Kamus dimuat per-locale lewat `dynamic import` — hanya satu bahasa yang
 *   masuk bundle awal.
 * - Fallback berantai: locale aktif → `id` → nama kunci (agar kunci yang
 *   belum dipetakan langsung terlihat saat mengembangkan bahasa baru).
 * - Interpolasi `{nama}` dipertahankan (dipakai oleh kamus yang ada).
 * - Tanpa state global: pemanggil memegang kamusnya sendiri. Di Svelte,
 *   kamus datang dari layout (`data.dict`) sehingga HTML pertama sudah
 *   berbahasa benar tanpa kedipan teks.
 */
import { DEFAULT_LOCALE, FALLBACK_LOCALE, isLocale, type Dictionary, type Locale, type TranslationKey } from './types';

/** Memuat kamus satu locale. Dipakai di server (saat prerender) dan di klien. */
export async function loadDictionary(locale: Locale): Promise<Dictionary> {
	const mod = await import(`../locales/${locale}.json`);
	return mod.default as Dictionary;
}

/** Memuat kamus beberapa locale sekaligus (mis. aktif + cadangan). */
export async function loadDictionaries(
	...locales: Locale[]
): Promise<Record<string, Dictionary>> {
	const out: Record<string, Dictionary> = {};
	for (const loc of locales) out[loc] = await loadDictionary(loc);
	return out;
}

/**
 * Menerjemahkan kunci pada kamus yang diberikan.
 *
 * @param dict   kamus locale aktif
 * @param key    kunci `domain.key`
 * @param vars   nilai interpolasi, mis. `{ name: 'Budi' }` untuk `{name}`
 * @param fallback kamus cadangan (biasanya `id`)
 */
export function translate(
	dict: Dictionary,
	key: TranslationKey | string,
	vars?: Record<string, string | number>,
	fallback?: Dictionary
): string {
	const value = lookup(dict, key) ?? (fallback ? lookup(fallback, key) : undefined);
	if (value === undefined) return key;
	return interpolate(value, vars);
}

/**
 * Menelusuri kunci bertitik, mis. `settings.appearance.theme`.
 *
 * Menelusuri SATU TINGKAT PER SATU, bukan mencocokkan sisa jalur sebagai
 * satu nama kunci. Perbedaan itu penting: kamus berisi kunci bersarang
 * (`settings.appearance` adalah objek berisi `theme`, `light`, `dark`, …),
 * sehingga `bucket['appearance.theme']` tidak pernah ada — yang ada adalah
 * `bucket['appearance']['theme']`.
 *
 * Kesalahan itu sempat terjadi dan gejalanya menyesatkan: halaman
 * menampilkan `settings.appearance.theme` alih-alih "Tema", yang terlihat
 * seperti terjemahan yang belum ditambahkan, padahal terjemahannya ADA.
 */
function lookup(dict: Dictionary, key: string): string | undefined {
	const parts = key.split('.');
	if (parts.length < 2) return undefined;

	const bucket = dict[parts[0]];
	if (!bucket || typeof bucket !== 'object') return undefined;

	// Mulai dari elemen kedua, turun satu tingkat per satu.
	let nilai: unknown = bucket;
	for (const bagian of parts.slice(1)) {
		if (typeof nilai !== 'object' || nilai === null) return undefined;
		nilai = (nilai as Record<string, unknown>)[bagian];
	}

	return typeof nilai === 'string' ? nilai : undefined;
}

/** Mengganti `{nama}` dengan nilai dari `vars`. */
export function interpolate(text: string, vars?: Record<string, string | number>): string {
	if (!vars) return text;
	return text.replace(/\{(\w+)\}/g, (match, name: string) =>
		Object.prototype.hasOwnProperty.call(vars, name) ? String(vars[name]) : match
	);
}

/**
 * Membuat fungsi `t` yang terikat pada satu kamus — bentuk yang dipakai
 * komponen Svelte.
 */
export function createTranslator(dict: Dictionary, fallback?: Dictionary) {
	return {
		/** Terjemahkan satu kunci. */
		t: (key: TranslationKey | string, vars?: Record<string, string | number>) =>
			translate(dict, key, vars, fallback),
		/** Daftar nilai terjemahan untuk kunci majemuk (mis. daftar pilihan). */
		ta: (keys: readonly string[]) => keys.map((k) => translate(dict, k, undefined, fallback)),
	};
}

export { DEFAULT_LOCALE, FALLBACK_LOCALE, isLocale, type Dictionary, type Locale, type TranslationKey } from './types';
export { LOCALE_COOKIE, resolveLocale, fromAcceptLanguage } from './detect';
