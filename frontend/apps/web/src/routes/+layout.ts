/**
 * Load layout tingkat akar.
 *
 * Locale ditentukan di server load (`+layout.server.ts`) dari cookie atau
 * header `Accept-Language`. Karena berjalan saat PRERENDER, teks halaman
 * publik sudah berbahasa benar di HTML hasil build.
 */
import type { LayoutLoad } from './$types';

export const load: LayoutLoad = ({ data }) => {
	return { locale: data.locale };
};
