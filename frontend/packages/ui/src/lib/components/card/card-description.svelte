<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { cn } from '../../utils';

	type Props = { class?: string; children: Snippet } & Omit<HTMLAttributes<HTMLDivElement>, 'class' | 'children'>;

	let { class: className, children, ...rest }: Props = $props();
</script>

<p class={cn('text-sm text-muted-foreground', className)} {...rest}>
	{@render children()}
</p>
