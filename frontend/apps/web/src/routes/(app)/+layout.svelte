<script lang="ts">
	/**
	 * Kerangka aplikasi setelah masuk: bilah atas + navigasi bawah.
	 *
	 * Nav bawah 5 item dipertahankan dari prototipe desain (Hari Ini · Obat ·
	 * Riwayat · Caregiver · Atur) karena sudah disetujui pemilik proyek.
	 */
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { session } from '$lib/session.svelte';
	import { APP_NAME } from '$lib/config';

	let { children } = $props();

	const navItems = [
		{ href: '/hari-ini', label: 'Hari Ini', icon: 'M8 2v4M16 2v4M3 10h18M5 4h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2Z' },
		{ href: '/obat', label: 'Obat', icon: 'M10.5 20.5a5 5 0 0 1-7-7l7-7a5 5 0 0 1 7 7ZM8.5 8.5l7 7' },
		{ href: '/riwayat', label: 'Riwayat', icon: 'M3 12a9 9 0 1 0 3-6.7L3 8M3 3v5h5M12 7v5l3 2' },
		{ href: '/caregiver', label: 'Caregiver', icon: 'M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8ZM22 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75' },
		{ href: '/pengaturan', label: 'Atur', icon: 'M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6ZM19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.9.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.9 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1A1.7 1.7 0 0 0 4.6 9a1.7 1.7 0 0 0-.3-1.9l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.9.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.9-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.9V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1Z' }
	];

	let current = $derived(page.url.pathname);

	async function signOut() {
		await session.signOut();
		await goto('/login');
	}
</script>

<div class="min-h-dvh bg-background text-foreground">
	<header class="sticky top-0 z-40 border-b border-border bg-background/90 backdrop-blur">
		<div class="mx-auto flex h-14 max-w-[var(--wrap-user)] items-center justify-between px-4">
			<a href="/hari-ini" class="flex items-center gap-2">
				<span class="grid size-7 place-items-center rounded-md bg-primary text-primary-foreground" aria-hidden="true">
					<svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
						<path d="M10.5 20.5a5 5 0 0 1-7-7l7-7a5 5 0 0 1 7 7Z" />
						<path d="m8.5 8.5 7 7" />
					</svg>
				</span>
				<span class="text-sm font-semibold tracking-tight">{APP_NAME}</span>
			</a>

			<div class="flex items-center gap-3">
				<span class="hidden text-xs text-muted-foreground sm:inline">
					{session.user?.full_name ?? ''}
				</span>
				<button
					type="button"
					class="rounded-md px-2.5 py-1.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
					onclick={signOut}
				>
					Keluar
				</button>
			</div>
		</div>
	</header>

	<main class="mx-auto max-w-[var(--wrap-user)] px-4 pt-5 pb-28">
		{@render children()}
	</main>

	<!-- Navigasi bawah: tinggi mengikuti token --nav-h dan aman dari safe-area iOS. -->
	<nav
		class="fixed inset-x-0 bottom-0 z-40 border-t border-border bg-background/95 backdrop-blur"
		style="padding-bottom: env(safe-area-inset-bottom)"
	>
		<ul class="mx-auto flex max-w-[var(--wrap-user)]">
			{#each navItems as item (item.href)}
				{@const active = current === item.href || current.startsWith(item.href + '/')}
				<li class="flex-1">
					<a
						href={item.href}
						class="flex flex-col items-center gap-1 py-2 text-[11px] font-medium transition-colors {active
							? 'text-primary'
							: 'text-muted-foreground hover:text-foreground'}"
						aria-current={active ? 'page' : undefined}
					>
						<svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
							<path d={item.icon} />
						</svg>
						{item.label}
					</a>
				</li>
			{/each}
		</ul>
	</nav>
</div>
