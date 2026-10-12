<script lang="ts">
	/**
	 * Halaman daftar obat (kerangka Fase 3).
	 *
	 * Mengambil `GET /medications` sungguhan supaya bentuk datanya terverifikasi
	 * terhadap backend; wizard tambah obat menyusul.
	 */
	import { onMount } from 'svelte';
	import { ApiError, type Medication } from '@pantau-obat/api-client';
	import { Badge, Button, Card, CardContent, Skeleton } from '@pantau-obat/ui';
	import { api } from '$lib/api';

	let meds = $state<Medication[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);

	const mealLabel: Record<string, string> = {
		before_meal: 'Sebelum makan',
		after_meal: 'Setelah makan',
		with_meal: 'Bersama makan',
		not_related: 'Tidak terkait makan'
	};

	onMount(async () => {
		try {
			meds = await api.medications.list();
		} catch (err) {
			error = err instanceof ApiError ? err.message : 'Gagal memuat daftar obat.';
		} finally {
			loading = false;
		}
	});
</script>

<svelte:head>
	<title>Obat — Pantau Obat</title>
</svelte:head>

<div class="mb-5 flex items-center justify-between">
	<div>
		<h1 class="text-2xl font-semibold tracking-tight">Obat</h1>
		<p class="mt-1 text-sm text-muted-foreground">Daftar obat yang Anda pantau.</p>
	</div>
	<Button size="sm" disabled>Tambah obat</Button>
</div>

{#if error}
	<div class="rounded-lg border border-destructive/30 bg-destructive/8 px-4 py-3 text-sm text-destructive" role="alert">
		{error}
	</div>
{:else if loading}
	<div class="space-y-3">
		{#each [1, 2] as n (n)}
			<Skeleton class="h-24 w-full" />
		{/each}
	</div>
{:else if meds.length === 0}
	<Card>
		<CardContent class="flex flex-col items-center gap-2 py-10 text-center">
			<p class="text-sm font-medium">Belum ada obat</p>
			<p class="max-w-xs text-xs text-muted-foreground">
				Wizard tambah obat (5 pola frekuensi) dikerjakan pada tahap berikutnya.
			</p>
		</CardContent>
	</Card>
{:else}
	<ul class="space-y-3">
		{#each meds as med (med.id)}
			<li>
				<Card>
					<CardContent class="py-4">
						<div class="flex items-start justify-between gap-3">
							<div class="min-w-0">
								<p class="truncate text-sm font-medium">{med.name}</p>
								<p class="mt-0.5 text-xs text-muted-foreground">
									{med.dose_value} {med.dose_unit_code} · {mealLabel[med.meal_relation] ?? med.meal_relation}
								</p>
							</div>
							<Badge variant={med.is_active ? 'ok' : 'na'}>
								{med.is_active ? 'Aktif' : 'Arsip'}
							</Badge>
						</div>
					</CardContent>
				</Card>
			</li>
		{/each}
	</ul>
{/if}
