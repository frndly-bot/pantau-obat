<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { cn } from '../../utils';

	type Props = { class?: string; children: Snippet } & Omit<HTMLAttributes<HTMLDivElement>, 'class' | 'children'>;

	let { class: className, children, ...rest }: Props = $props();
</script>

<h3 class={cn('text-lg font-semibold leading-none tracking-tight', className)} {...rest}>
	{@render children()}
</h3>
