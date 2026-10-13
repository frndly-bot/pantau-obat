<script lang="ts">
	import type { Snippet } from 'svelte';
	import { cn } from '../../utils';

	type Props = {
		open?: boolean;
		title?: string;
		class?: string;
		children: Snippet;
		footer?: Snippet;
		/** Dipanggil saat backdrop atau tombol tutup ditekan. */
		onclose?: () => void;
		/**
		 * Bila nilainya berubah, isi dialog digulir kembali ke ATAS.
		 *
		 * Dipakai wizard bertahap: setiap langkah punya isi yang sama sekali
		 * berbeda, dan tanpa pengaturan ulang posisi gulir lama ikut terbawa
		 * lalu terpotong ke batas konten baru — pengguna mendarat di tengah
		 * atau di dasar form langkah berikutnya, bukan di awalnya.
		 *
		 * Sengaja berupa prop, bukan perilaku otomatis: dialog yang isinya
		 * diperbarui berkala (mis. daftar yang memuat ulang) justru harus
		 * MEMPERTAHANKAN posisi gulir, dan meresetnya di sana akan terasa
		 * seperti halaman yang melompat sendiri.
		 */
		scrollResetKey?: unknown;
	};

	let {
		open = $bindable(false),
		title,
		class: className,
		children,
		footer,
		onclose,
		scrollResetKey
	}: Props = $props();

	/** Wadah gulir dialog; dipakai untuk mengatur ulang posisi. */
	let scrollEl = $state<HTMLDivElement | null>(null);

	/** Panel dialog; dipakai untuk memindahkan dan menahan fokus. */
	let panelEl = $state<HTMLDivElement | null>(null);

	/**
	 * Elemen yang sedang fokus SEBELUM dialog dibuka.
	 *
	 * Disimpan supaya fokus bisa dikembalikan ke tempat asalnya saat dialog
	 * ditutup. Tanpa ini, pengguna keyboard harus mengulang navigasi dari
	 * awal setiap kali selesai memakai satu modal — dan pada halaman ini
	 * modal dipakai berulang kali untuk mencatat dosis.
	 */
	let previousFocus: HTMLElement | null = null;

	/** Elemen yang bisa menerima fokus di dalam dialog. */
	function focusable(): HTMLElement[] {
		if (!panelEl) return [];
		const selector = [
			'a[href]',
			'button:not([disabled])',
			'input:not([disabled])',
			'select:not([disabled])',
			'textarea:not([disabled])',
			'[tabindex]:not([tabindex="-1"])'
		].join(',');
		return Array.from(panelEl.querySelectorAll<HTMLElement>(selector)).filter(
			// Elemen tersembunyi tidak bisa menerima fokus.
			(el) => el.offsetParent !== null || el === document.activeElement
		);
	}

	/**
	 * Saat dialog dibuka: simpan fokus lama, lalu pindahkan fokus ke kontrol
	 * pertama di dalam dialog.
	 *
	 * Kenapa harus dipindahkan: tanpa itu fokus tetap di tombol di belakang
	 * dialog. Pengguna keyboard yang menekan Tab akan berkeliling ke elemen
	 * di baliknya, dan pembaca layar masih boleh membacakan konten luar yang
	 * seharusnya tidak lagi relevan.
	 *
	 * Fokus diletakkan di wadah bila tidak ada kontrol yang bisa difokus,
	 * supaya tombol Tab berikutnya tetap masuk ke dalam dialog.
	 */
	$effect(() => {
		if (open) {
			previousFocus = document.activeElement as HTMLElement | null;
			// Menunggu DOM terpasang sebelum mencari kontrolnya.
			queueMicrotask(() => {
				const first = focusable()[0];
				if (first) first.focus();
				else panelEl?.focus();
			});
		} else if (previousFocus) {
			// Dialog ditutup: kembalikan fokus ke elemen asalnya.
			previousFocus.focus();
			previousFocus = null;
		}
	});

	/**
	 * Mengembalikan posisi gulir ke atas saat kunci berubah.
	 *
	 * `$effect` berjalan SETELAH DOM diperbarui, jadi konten langkah baru
	 * sudah terpasang ketika posisi direset.
	 */
	$effect(() => {
		// Dibaca supaya effect ikut berjalan ketika nilainya berubah.
		void scrollResetKey;
		if (scrollEl) scrollEl.scrollTop = 0;
	});

	/**
	 * Esc menutup, Tab ditahan di dalam dialog.
	 *
	 * Penahan Tab mengulang dari kontrol terakhir ke kontrol pertama, dan
	 * sebaliknya. Tanpa ini fokus bisa keluar dari dialog ke halaman di
	 * belakangnya, padahal halaman itu secara visual tertutup tirai.
	 */
	function onKeydown(event: KeyboardEvent) {
		if (!open) return;

		if (event.key === 'Escape') {
			open = false;
			onclose?.();
			return;
		}

		if (event.key !== 'Tab') return;

		const items = focusable();
		if (items.length === 0) {
			event.preventDefault();
			panelEl?.focus();
			return;
		}

		const first = items[0];
		const last = items[items.length - 1];
		const active = document.activeElement;

		if (event.shiftKey && (active === first || active === panelEl)) {
			event.preventDefault();
			last.focus();
		} else if (!event.shiftKey && active === last) {
			event.preventDefault();
			first.focus();
		}
	}
</script>

<svelte:window onkeydown={onKeydown} />

{#if open}
	<!-- Backdrop -->
	<div
		class="fixed inset-0 z-50 bg-[var(--overlay)] backdrop-blur-[2px]"
		role="presentation"
		onclick={() => {
			open = false;
			onclose?.();
		}}
	></div>

	<!-- Panel: di layar kecil menempel ke bawah seperti sheet; di layar besar
	     mengambang di tengah. `tabindex="-1"` supaya wadahnya sendiri bisa
	     menerima fokus ketika dialog tidak punya kontrol yang bisa difokus. -->
	<div
		bind:this={panelEl}
		tabindex="-1"
		class="fixed inset-x-0 bottom-0 z-50 flex max-h-[92dvh] flex-col rounded-t-xl border border-border bg-card shadow-lg outline-none sm:inset-x-auto sm:left-1/2 sm:bottom-auto sm:top-1/2 sm:w-full sm:max-w-lg sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-xl"
		role="dialog"
		aria-modal="true"
		aria-label={title}
	>
		{#if title}
			<div class="flex items-center justify-between gap-3 border-b border-border px-4 py-3">
				<h2 class="text-sm font-semibold tracking-tight">{title}</h2>
				<button
					type="button"
					class="grid size-8 place-items-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
					aria-label="Tutup"
					onclick={() => {
						open = false;
						onclose?.();
					}}
				>
					<svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
						<path d="M18 6 6 18M6 6l12 12" />
					</svg>
				</button>
			</div>
		{/if}

		<div bind:this={scrollEl} class={cn('min-h-0 flex-1 overflow-y-auto px-4 py-4', className)}>
			{@render children()}
		</div>

		{#if footer}
			<div class="border-t border-border px-4 py-3" style="padding-bottom: max(0.75rem, env(safe-area-inset-bottom))">
				{@render footer()}
			</div>
		{/if}
	</div>
{/if}
