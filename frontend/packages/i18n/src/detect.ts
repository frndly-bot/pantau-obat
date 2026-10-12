/**
 * Penentuan locale — dipakai bersama server (prerender) dan klien.
 *
 * Urutan prioritas:
 * 1. Cookie `po_locale` (pilihan eksplisit pengguna)
 * 2. Header `Accept-Language` (hanya tersedia di server)
 * 3. `DEFAULT_LOCALE`
 */
import { DEFAULT_LOCALE, isLocale, type Locale } from './types';

export const LOCALE_COOKIE = 'po_locale';

/** Mengurai header `Accept-Language` dan memilih locale yang didukung. */
export function fromAcceptLanguage(header: string | null | undefined): Locale | null {
	if (!header) return null;
	const parts = header
		.split(',')
		.map((part) => {
			const [tag, q] = part.trim().split(';');
			const quality = q?.startsWith('q=') ? Number.parseFloat(q.slice(2)) : 1;
			return { tag: tag.trim().toLowerCase(), q: Number.isNaN(quality) ? 1 : quality };
		})
		.filter((p) => p.tag.length > 0)
		.sort((a, b) => b.q - a.q);

	for (const { tag } of parts) {
		// `id-ID` → `id`, `en-US` → `en`
		const base = tag.split('-')[0];
		if (isLocale(base)) return base;
	}
	return null;
}

/** Memilih locale dari sumber yang tersedia. */
export function resolveLocale(input: {
	cookie?: string | null;
	acceptLanguage?: string | null;
}): Locale {
	if (isLocale(input.cookie)) return input.cookie;
	return fromAcceptLanguage(input.acceptLanguage) ?? DEFAULT_LOCALE;
}
