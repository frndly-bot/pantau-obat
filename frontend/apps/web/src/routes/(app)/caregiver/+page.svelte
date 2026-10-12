<script lang="ts">
	/**
	 * Halaman caregiver (kerangka Fase 3).
	 *
	 * Mengambil `GET /care-links` sungguhan — daftar relasi caregiver milik
	 * pengguna. Alur undang/terima dan pengelolaan obat pasien (peran manager)
	 * menyusul.
	 */
	import { onMount } from 'svelte';
	import { ApiError, type CareLink } from '@pantau-obat/api-client';
	import { Badge, Button, Card, CardContent, Skeleton } from '@pantau-obat/ui';
	import { api } from '$lib/api';

	let links = $state<CareLink[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);

	const roleLabel: Record<string, string> = {
		viewer: 'Hanya melihat',
		helper: 'Membantu mencatat',
		manager: 'Mengelola obat'
	};

	const statusLabel: Record<string, string> = {
		pending: 'Menunggu',
		accepted: 'Aktif',
		rejected: 'Ditolak',
		revoked: 'Dicabut',
		expired: 'Kedaluwarsa'
	};

	const statusVariant: Record<string, 'ok' | 'warn' | 'na' | 'info'> = {
		pending: 'info',
		accepted: 'ok',
		rejected: 'na',
		revoked: 'na',
		expired: 'na'
	};

	onMount(async () => {
		try {
			links = await api.care.list();
		} catch (err) {
			error = err instanceof ApiError ? err.message : 'Gagal memuat data caregiver.';
		} finally {
			loading = false;
		}
	});
</script>

<svelte:head>
	<title>Caregiver — Pantau Obat</title>
</svelte:head>

<div class="mb-5 flex items-center justify-between">
	<div>
		<h1 class="text-2xl font-semibold tracking-tight">Caregiver</h1>
		<p class="mt-1 text-sm text-muted-foreground">Orang yang membantu memantau pengobatan Anda.</p>
	</div>
	<Button size="sm" disabled>Undang</Button>
</div>

{#if error}
	<div class="rounded-lg border border-destructive/30 bg-destructive/8 px-4 py-3 text-sm text-destructive" role="alert">
		{error}
	</div>
{:else if loading}
	<div class="space-y-3">
		{#each [1, 2] as n (n)}
			<Skeleton class="h-20 w-full" />
		{/each}
	</div>
{:else if links.length === 0}
	<Card>
		<CardContent class="py-10 text-center">
			<p class="text-sm font-medium">Belum ada caregiver</p>
			<p class="mt-1 text-xs text-muted-foreground">
				Alur undang caregiver (peran viewer, helper, atau manager) dikerjakan pada tahap berikutnya.
			</p>
		</CardContent>
	</Card>
{:else}
	<ul class="space-y-3">
		{#each links as link (link.id)}
			<li>
				<Card>
					<CardContent class="py-4">
						<div class="flex items-start justify-between gap-3">
							<div class="min-w-0">
								<p class="truncate text-sm font-medium">{link.invite_email ?? 'Pengguna terdaftar'}</p>
								<p class="mt-0.5 text-xs text-muted-foreground">{roleLabel[link.role] ?? link.role}</p>
							</div>
							<Badge variant={statusVariant[link.status] ?? 'outline'}>
								{statusLabel[link.status] ?? link.status}
							</Badge>
						</div>
					</CardContent>
				</Card>
			</li>
		{/each}
	</ul>
{/if}
