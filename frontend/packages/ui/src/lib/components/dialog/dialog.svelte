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

	/**
	 * Mengembalikan posisi gulir ke atas saat kunci berubah.
	 *
	 * `$effect` berjalan SETELAH DOM diperbarui, jadi konten langkah baru
	 * sudah terpasang ketika posisi direset. Fokus tidak perlu dipindah:
	 * tombol navigasi ada di footer, di luar wadah gulir, sehingga peramban
	 * tidak menggulir balik untuk mengikuti fokus.
	 */
	$effect(() => {
		// Dibaca supaya effect ikut berjalan ketika nilainya berubah.
		void scrollResetKey;
		if (scrollEl) scrollEl.scrollTop = 0;
	});

	/** Menutup lewat tombol Esc. */
	function onKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape' && open) {
			open = false;
			onclose?.();
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
	     mengambang di tengah. -->
	<div
		class="fixed inset-x-0 bottom-0 z-50 flex max-h-[92dvh] flex-col rounded-t-xl border border-border bg-card shadow-lg sm:inset-x-auto sm:left-1/2 sm:bottom-auto sm:top-1/2 sm:w-full sm:max-w-lg sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-xl"
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
