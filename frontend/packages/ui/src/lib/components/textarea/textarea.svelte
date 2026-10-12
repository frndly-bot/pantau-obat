<script lang="ts">
	import type { HTMLTextareaAttributes } from 'svelte/elements';
	import { cn } from '../../utils';

	type Props = { class?: string; value?: string } & Omit<HTMLTextareaAttributes, 'class'>;

	let { class: className, value = $bindable(''), ...rest }: Props = $props();
</script>

<textarea
	class={cn(
		'flex min-h-[72px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-xs',
		'transition-colors duration-[var(--dur)] placeholder:text-muted-foreground',
		'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
		'disabled:cursor-not-allowed disabled:opacity-50',
		className
	)}
	bind:value
	{...rest}
></textarea>
