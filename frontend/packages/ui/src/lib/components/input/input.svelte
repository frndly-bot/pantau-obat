<script lang="ts">
	import type { HTMLInputAttributes } from 'svelte/elements';
	import { cn } from '../../utils';

	type Props = { class?: string; value?: string } & Omit<HTMLInputAttributes, 'class'>;

	// `value` harus bindable supaya pemanggil bisa memakai `bind:value`.
	let { class: className, value = $bindable(''), ...rest }: Props = $props();
</script>

<input
	class={cn(
		'flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-xs',
		'transition-colors duration-[var(--dur)] file:border-0 file:bg-transparent file:text-sm file:font-medium',
		'placeholder:text-muted-foreground',
		'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
		'disabled:cursor-not-allowed disabled:opacity-50',
		'aria-[invalid=true]:border-destructive aria-[invalid=true]:ring-destructive',
		className
	)}
	bind:value
	{...rest}
/>
