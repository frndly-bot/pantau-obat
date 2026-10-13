<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import { page } from '$app/state';
	import { session } from '$lib/session.svelte';
	import { i18n } from '$lib/i18n.svelte';
	import { resolveLocale, LOCALE_COOKIE, DEFAULT_LOCALE, type Locale } from '@pantau-obat/i18n';

	/**
	 * Menentukan bahasa di peramban: cookie pilihan pengguna lebih dulu,
	 * lalu preferensi peramban, lalu bawaan.
	 *
	 * Aman dipanggil saat SSR/prerender: di sana `document` tidak ada,
	 * sehingga langsung memakai `DEFAULT_LOCALE` dan HTML hasil build tetap
	 * bisa dibuat. Nilai sebenarnya dipasang begitu halaman jalan di klien.
	 */
	function detectClientLocale(): Locale {
		if (typeof document === 'undefined') return DEFAULT_LOCALE;

		const cookie = document.cookie
			.split('; ')
			.find((c) => c.startsWith(`${LOCALE_COOKIE}=`))
			?.split('=')[1];

		return resolveLocale({
			cookie: cookie ?? null,
			acceptLanguage: navigator.language ?? null
		});
	}

	let { children } = $props();

	// Locale ditentukan di klien: cookie pilihan pengguna, lalu preferensi
	// peramban, lalu bawaan. Tidak lewat server load karena halaman publik
	// di-prerender saat build — `Accept-Language` yang terbaca di sana milik
	// mesin build, bukan pengunjung. Lihat penjelasan lengkap di `+layout.ts`.
	$effect(() => {
		void i18n.init(detectClientLocale());
	});

	onMount(() => {
		// Pulihkan sesi sekali saat aplikasi dimuat (memakai cookie HttpOnly).
		void session.init();

		const onExpired = () => {
			void session.forget();
			if (page.url.pathname.startsWith('/hari-ini')) {
				// Penjaga rute di (app) yang akan mengalihkan.
			}
		};
		window.addEventListener('po:session-expired', onExpired);

		// Transisi halaman halus (View Transitions API) — tanpa pustaka.
		document.documentElement.dataset.theme =
			document.documentElement.dataset.theme ?? 'light';

		// Daftarkan service worker (PWA). Hanya di produksi: di dev, cache
		// akan menyajikan berkas lama dan membuat perubahan tidak terlihat.
		if ('serviceWorker' in navigator && import.meta.env.PROD) {
			navigator.serviceWorker.register('/service-worker.js').catch(() => {
				// PWA bersifat tambahan; kegagalannya tidak boleh mengganggu
				// aplikasi.
			});
		}

		return () => window.removeEventListener('po:session-expired', onExpired);
	});

	// Tema awal dari preferensi sistem (dihormati, bukan dipaksa).
	$effect(() => {
		if (!browser) return;
		const stored = localStorage.getItem('po_theme');
		if (stored === 'dark' || stored === 'light') {
			document.documentElement.dataset.theme = stored;
		} else if (window.matchMedia('(prefers-color-scheme: dark)').matches) {
			document.documentElement.dataset.theme = 'dark';
		}
	});
</script>

{@render children()}
