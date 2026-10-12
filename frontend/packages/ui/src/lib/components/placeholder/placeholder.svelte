<script lang="ts">
	import type { Snippet } from 'svelte';
	import { Card, CardContent } from '../card';

	type Props = {
		title: string;
		description?: string;
		children?: Snippet;
	};

	let { title, description, children }: Props = $props();
</script>

<Card>
	<CardContent class="flex flex-col items-center gap-2 py-12 text-center">
		<span class="grid size-11 place-items-center rounded-full bg-muted text-muted-foreground" aria-hidden="true">
			<svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
				<path d="M12 8v4M12 16h.01M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0Z" />
			</svg>
		</span>
		<p class="text-sm font-medium">{title}</p>
		{#if description}
			<p class="max-w-xs text-xs leading-relaxed text-muted-foreground">{description}</p>
		{/if}
		{@render children?.()}
	</CardContent>
</Card>
