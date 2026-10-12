<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLSelectAttributes } from 'svelte/elements';
	import { cn } from '../../utils';

	type Option = { value: string; label: string };

	type Props = {
		class?: string;
		value?: string;
		/** Daftar pilihan. Boleh dikosongkan bila memakai `children`. */
		options?: Option[];
		placeholder?: string;
		/** Isi `<option>` kustom; dipakai bila pilihan bukan dari `options`. */
		children?: Snippet;
	} & Omit<HTMLSelectAttributes, 'class' | 'value'>;

	let {
		class: className,
		value = $bindable(''),
		options = [],
		placeholder,
		children,
		...rest
	}: Props = $props();
</script>

<div class="relative">
	<select
		class={cn(
			'flex h-9 w-full appearance-none rounded-md border border-input bg-background px-3 py-1 pr-8 text-sm shadow-xs',
			'transition-colors duration-[var(--dur)]',
			'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
			'disabled:cursor-not-allowed disabled:opacity-50',
			'aria-[invalid=true]:border-destructive',
			className
		)}
		bind:value
		{...rest}
	>
		{#if placeholder}
			<option value="" disabled>{placeholder}</option>
		{/if}
		{#if children}
			{@render children()}
		{:else}
			{#each options as opt (opt.value)}
				<option value={opt.value}>{opt.label}</option>
			{/each}
		{/if}
	</select>
	<svg
		class="pointer-events-none absolute right-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
		viewBox="0 0 24 24"
		fill="none"
		stroke="currentColor"
		stroke-width="2"
		stroke-linecap="round"
		stroke-linejoin="round"
		aria-hidden="true"
	>
		<path d="m6 9 6 6 6-6" />
	</svg>
</div>
