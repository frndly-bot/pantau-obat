/**
 * Server load tingkat akar — hanya berjalan saat PRERENDER (tidak ada server
 * di produksi; lihat ADR-001). Tugasnya menentukan locale awal.
 */
import { resolveLocale, LOCALE_COOKIE, type Locale } from '@pantau-obat/i18n';
import type { LayoutServerLoad } from './$types';

export const load: LayoutServerLoad = ({ cookies, request }) => {
	const locale: Locale = resolveLocale({
		cookie: cookies.get(LOCALE_COOKIE) ?? null,
		acceptLanguage: request.headers.get('accept-language'),
	});
	return { locale };
};
