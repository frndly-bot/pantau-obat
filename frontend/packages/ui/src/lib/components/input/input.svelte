<script lang="ts">
	import type { HTMLInputAttributes } from 'svelte/elements';
	import { cn } from '../../utils';

	/**
	 * `value` menerima string ATAU angka.
	 *
	 * Formulir obat banyak memakai input angka (dosis, stok, interval), dan
	 * memaksa pemanggil menyimpan semuanya sebagai string berarti setiap
	 * pemakaian harus mengonversi bolak-balik — di situlah kesalahan ketik
	 * terjadi (mis. "3" + 1 menjadi "31"). Dengan nilai yang sadar tipe, input
	 * `type="number"` mengisi angka dan `type="text"` mengisi string.
	 */
	type Value = string | number | null;
	type Props = { class?: string; value?: Value } & Omit<HTMLInputAttributes, 'class' | 'value'>;

	let { class: className, value = $bindable(), type = 'text', ...rest }: Props = $props();

	/** Nilai yang ditampilkan di DOM selalu string. */
	let shown = $derived(value === null || value === undefined ? '' : String(value));

	function onInput(event: Event) {
		const raw = (event.currentTarget as HTMLInputElement).value;
		if (type === 'number') {
			// Kosong berarti null, bukan 0 — "belum diisi" dan "nol" berbeda,
			// dan kolom seperti ambang restock memang boleh kosong.
			value = raw === '' ? null : Number(raw);
		} else {
			value = raw;
		}
	}
</script>

<input
	{type}
	value={shown}
	oninput={onInput}
	class={cn(
		'flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-xs',
		'transition-colors duration-[var(--dur)] file:border-0 file:bg-transparent file:text-sm file:font-medium',
		'placeholder:text-muted-foreground',
		'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
		'disabled:cursor-not-allowed disabled:opacity-50',
		'aria-[invalid=true]:border-destructive aria-[invalid=true]:ring-destructive',
		className
	)}
	{...rest}
/>
