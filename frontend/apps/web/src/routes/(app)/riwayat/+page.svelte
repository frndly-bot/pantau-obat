<script lang="ts">
	/**
	 * Halaman riwayat (kerangka Fase 3).
	 *
	 * Mengambil `GET /logs` sungguhan. Filter rentang tanggal dan tombol
	 * ekspor CSV menyusul — endpoint ekspor sudah tersedia di backend
	 * (`GET /logs/export`, CSV RFC 4180 + BOM).
	 */
	import { onMount } from 'svelte';
	import { ApiError, type MedicationLog } from '@pantau-obat/api-client';
	import { Badge, Card, CardContent, Skeleton } from '@pantau-obat/ui';
	import { api } from '$lib/api';

	let logs = $state<MedicationLog[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);

	const statusLabel: Record<string, string> = {
		taken: 'Diminum',
		late: 'Terlambat',
		skipped: 'Dilewati',
		snoozed: 'Ditunda'
	};

	function formatDate(iso: string): string {
		return new Intl.DateTimeFormat('id-ID', { day: 'numeric', month: 'short', year: 'numeric' }).format(
			new Date(iso + 'T00:00:00')
		);
	}

	onMount(async () => {
		try {
			logs = await api.logs.list({ limit: 50 });
		} catch (err) {
			error = err instanceof ApiError ? err.message : 'Gagal memuat riwayat.';
		} finally {
			loading = false;
		}
	});
</script>

<svelte:head>
	<title>Riwayat — Pantau Obat</title>
</svelte:head>

<div class="mb-5">
	<h1 class="text-2xl font-semibold tracking-tight">Riwayat</h1>
	<p class="mt-1 text-sm text-muted-foreground">Catatan dosis terbaru Anda.</p>
</div>

{#if error}
	<div class="rounded-lg border border-destructive/30 bg-destructive/8 px-4 py-3 text-sm text-destructive" role="alert">
		{error}
	</div>
{:else if loading}
	<div class="space-y-3">
		{#each [1, 2, 3] as n (n)}
			<Skeleton class="h-16 w-full" />
		{/each}
	</div>
{:else if logs.length === 0}
	<Card>
		<CardContent class="py-10 text-center">
			<p class="text-sm font-medium">Belum ada riwayat</p>
			<p class="mt-1 text-xs text-muted-foreground">
				Catatan akan muncul setelah Anda mulai menandai dosis.
			</p>
		</CardContent>
	</Card>
{:else}
	<ul class="space-y-2.5">
		{#each logs as log (log.id)}
			<li>
				<Card>
					<CardContent class="flex items-center justify-between gap-3 py-3.5">
						<div class="min-w-0">
							<p class="text-sm font-medium">{formatDate(log.slot_date)}</p>
							<p class="mt-0.5 text-xs text-muted-foreground">
								{log.scheduled_time ?? 'Tanpa jam jadwal'}
								{log.source === 'caregiver' ? ' · oleh caregiver' : ''}
							</p>
						</div>
						<Badge variant={log.status === 'taken' ? 'ok' : log.status === 'late' ? 'warn' : 'na'}>
							{statusLabel[log.status] ?? log.status}
						</Badge>
					</CardContent>
				</Card>
			</li>
		{/each}
	</ul>
{/if}
