<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { cn } from '../../utils';

	type Props = { class?: string; children: Snippet } & Omit<HTMLAttributes<HTMLDivElement>, 'class' | 'children'>;

	let { class: className, children, ...rest }: Props = $props();
</script>

<div class={cn('flex items-center p-6 pt-0', className)} {...rest}>
	{@render children()}
</div>
