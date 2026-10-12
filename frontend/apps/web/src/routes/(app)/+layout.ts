/**
 * Penjaga rute untuk halaman ber-guard.
 *
 * Halaman di grup `(app)` hanya untuk pengguna yang sudah masuk. Karena
 * sesi dipulihkan di klien (refresh token ada di cookie HttpOnly yang tidak
 * terbaca JS), penjaga ini menunggu pemulihan selesai sebelum memutuskan —
 * sehingga tidak ada kedipan "dialihkan ke login" saat halaman dimuat ulang.
 */
import { redirect } from '@sveltejs/kit';
import { browser } from '$app/environment';
import { session } from '$lib/session.svelte';
import type { LayoutLoad } from './$types';

// Halaman ber-guard dirender di klien: isinya bergantung pada sesi, dan
// tidak butuh SEO. Halaman publik (landing, login, register) tetap
// di-prerender — lihat `+page.ts` masing-masing.
export const ssr = false;

export const load: LayoutLoad = async ({ url }) => {
	if (!browser) return {};

	// Tunggu pemulihan sesi (idempoten; layout akar juga memanggilnya).
	await session.init();

	if (!session.isAuthenticated) {
		const next = url.pathname + url.search;
		throw redirect(307, `/login?next=${encodeURIComponent(next)}`);
	}

	return { user: session.user };
};
