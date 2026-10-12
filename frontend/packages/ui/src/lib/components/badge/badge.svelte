<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { cn } from '../../utils';

	type Variant = 'default' | 'secondary' | 'outline' | 'ok' | 'warn' | 'err' | 'info' | 'na' | 'private';

	type Props = {
		variant?: Variant;
		class?: string;
		children: Snippet;
	} & Omit<HTMLAttributes<HTMLSpanElement>, 'class' | 'children'>;

	let { variant = 'default', class: className, children, ...rest }: Props = $props();

	const base =
		'inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium whitespace-nowrap';

	/* Warna status memakai token domain dari tokens.css. */
	const variants: Record<Variant, string> = {
		default: 'border-transparent bg-primary text-primary-foreground',
		secondary: 'border-transparent bg-secondary text-secondary-foreground',
		outline: 'border-border text-foreground',
		ok: 'border-transparent bg-ok/15 text-ok',
		warn: 'border-transparent bg-warn/15 text-warn',
		err: 'border-transparent bg-err/15 text-err',
		info: 'border-transparent bg-info/15 text-info',
		na: 'border-transparent bg-na/15 text-na',
		private: 'border-transparent bg-private/15 text-private',
	};
</script>

<span class={cn(base, variants[variant], className)} {...rest}>
	{@render children()}
</span>
