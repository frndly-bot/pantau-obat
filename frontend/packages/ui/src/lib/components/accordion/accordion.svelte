<script lang="ts">
	import type { Snippet } from 'svelte';
	import { cn } from '../../utils';

	type Item = { id: string; question: string; answer: string };

	type Props = {
		items: Item[];
		class?: string;
		/** Kunci yang terbuka secara bawaan (opsional). */
		defaultOpen?: string;
	};

	let { items, class: className, defaultOpen }: Props = $props();

	let open = $state<string | null>(null);

	// `defaultOpen` dibaca sebagai nilai awal; dipisah agar Svelte tidak
	// mengira referensinya reaktif (peringatan state_referenced_locally).
	$effect(() => {
		if (defaultOpen !== undefined) open = defaultOpen;
	});

	function toggle(id: string) {
		open = open === id ? null : id;
	}
</script>

<div class={cn('divide-y divide-border rounded-lg border border-border bg-card', className)}>
	{#each items as item (item.id)}
		{@const isOpen = open === item.id}
		<div>
			<h3>
				<button
					type="button"
					class="flex w-full items-center justify-between gap-4 px-4 py-3.5 text-left text-sm font-medium transition-colors duration-[var(--dur)] hover:bg-accent/60"
					aria-expanded={isOpen}
					aria-controls="faq-{item.id}"
					onclick={() => toggle(item.id)}
				>
					<span>{item.question}</span>
					<svg
						class={cn(
							'size-4 shrink-0 text-muted-foreground transition-transform duration-[var(--dur)]',
							isOpen && 'rotate-45'
						)}
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="2"
						stroke-linecap="round"
						aria-hidden="true"
					>
						<path d="M12 5v14M5 12h14" />
					</svg>
				</button>
			</h3>
			{#if isOpen}
				<div id="faq-{item.id}" class="px-4 pb-4 text-sm text-muted-foreground">
					{item.answer}
				</div>
			{/if}
		</div>
	{/each}
</div>
