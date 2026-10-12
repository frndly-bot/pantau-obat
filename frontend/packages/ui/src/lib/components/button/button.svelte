<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAnchorAttributes, HTMLButtonAttributes } from 'svelte/elements';
	import { cn } from '../../utils';

	type Variant = 'default' | 'secondary' | 'outline' | 'ghost' | 'destructive' | 'link';
	type Size = 'sm' | 'default' | 'lg' | 'icon';

	type Props = {
		variant?: Variant;
		size?: Size;
		href?: string;
		class?: string;
		children: Snippet;
	} & Omit<HTMLButtonAttributes, 'class' | 'children'> &
		Omit<HTMLAnchorAttributes, 'class' | 'children'>;

	let {
		variant = 'default',
		size = 'default',
		href,
		class: className,
		children,
		...rest
	}: Props = $props();

	const base =
		'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md font-medium ' +
		'transition-[color,background-color,border-color,box-shadow] duration-[var(--dur)] ' +
		'disabled:pointer-events-none disabled:opacity-50 [&_svg]:size-4 [&_svg]:shrink-0 ' +
		'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 ' +
		'focus-visible:ring-offset-background';

	const variants: Record<Variant, string> = {
		default: 'bg-primary text-primary-foreground shadow-xs hover:bg-primary/90',
		secondary: 'bg-secondary text-secondary-foreground hover:bg-secondary/80',
		outline: 'border border-border bg-background hover:bg-accent hover:text-accent-foreground',
		ghost: 'hover:bg-accent hover:text-accent-foreground',
		destructive: 'bg-destructive text-destructive-foreground shadow-xs hover:bg-destructive/90',
		link: 'text-primary underline-offset-4 hover:underline',
	};

	/* Kontrol mengikuti skala shadcn: h-8 / h-9 / h-10 (lihat tokens.css). */
	const sizes: Record<Size, string> = {
		sm: 'h-8 px-3 text-sm',
		default: 'h-9 px-4 text-sm',
		lg: 'h-10 px-6 text-base',
		icon: 'h-9 w-9',
	};

	let classes = $derived(cn(base, variants[variant], sizes[size], className));
</script>

{#if href}
	<a {href} class={classes} {...rest}>
		{@render children()}
	</a>
{:else}
	<button class={classes} {...rest}>
		{@render children()}
	</button>
{/if}
