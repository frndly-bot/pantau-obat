<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import { page } from '$app/state';
	import { session } from '$lib/session.svelte';
	import { i18n } from '$lib/i18n.svelte';

	let { children, data } = $props();

	// Kamus dimuat saat load (lihat +layout.ts), sehingga HTML pertama sudah
	// berbahasa benar — tidak ada kedipan teks.
	$effect(() => {
		void i18n.init(data.locale);
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
