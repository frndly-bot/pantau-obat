<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLLabelAttributes } from 'svelte/elements';
	import { cn } from '../../utils';

	type Props = { class?: string; children: Snippet } & Omit<HTMLLabelAttributes, 'class' | 'children'>;

	let { class: className, children, ...rest }: Props = $props();
</script>

<label
	class={cn(
		'text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70',
		className
	)}
	{...rest}
>
	{@render children()}
</label>
