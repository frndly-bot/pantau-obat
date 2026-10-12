<script lang="ts">
	/**
	 * Halaman "Hari Ini".
	 *
	 * CATATAN PENTING (temuan saat implementasi):
	 * Backend saat ini hanya menyediakan `GET /logs/today`, yang mengembalikan
	 * dosis yang SUDAH dicatat — bukan daftar jadwal dosis hari ini. Generator
	 * jadwal (`OccurrenceKey` di `domain/occurrence.go`) ada, tetapi belum
	 * diekspos lewat endpoint. Jadi halaman ini menampilkan catatan hari ini
	 * secara jujur, dan menandai bagian "belum tercatat" sebagai belum
	 * tersedia — bukan mengarang data.
	 */
	import { onMount } from 'svelte';
	import { ApiError, type MedicationLog } from '@pantau-obat/api-client';
	import { Badge, Card, CardContent, Skeleton } from '@pantau-obat/ui';
	import { api } from '$lib/api';
	import { session } from '$lib/session.svelte';

	let logs = $state<MedicationLog[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);

	const statusLabel: Record<string, string> = {
		taken: 'Diminum',
		late: 'Terlambat',
		skipped: 'Dilewati',
		snoozed: 'Ditunda'
	};

	const statusVariant: Record<string, 'ok' | 'warn' | 'na' | 'info'> = {
		taken: 'ok',
		late: 'warn',
		skipped: 'na',
		snoozed: 'info'
	};

	let today = $derived(
		new Intl.DateTimeFormat('id-ID', {
			weekday: 'long',
			day: 'numeric',
			month: 'long',
			year: 'numeric'
		}).format(new Date())
	);

	onMount(async () => {
		try {
			logs = await api.logs.today();
		} catch (err) {
			error = err instanceof ApiError ? err.message : 'Gagal memuat data hari ini.';
		} finally {
			loading = false;
		}
	});
</script>

<svelte:head>
	<title>Hari Ini — Pantau Obat</title>
</svelte:head>

<header class="mb-5">
	<p class="text-xs font-medium uppercase tracking-wide text-muted-foreground">{today}</p>
	<h1 class="mt-1 text-2xl font-semibold tracking-tight">
		Halo{session.user?.full_name ? `, ${session.user.full_name.split(' ')[0]}` : ''}
	</h1>
	<p class="mt-1 text-sm text-muted-foreground">Berikut catatan dosis Anda hari ini.</p>
</header>

{#if error}
	<div class="rounded-lg border border-destructive/30 bg-destructive/8 px-4 py-3 text-sm text-destructive" role="alert">
		{error}
	</div>
{:else if loading}
	<div class="space-y-3">
		{#each [1, 2, 3] as n (n)}
			<Skeleton class="h-20 w-full" />
		{/each}
	</div>
{:else if logs.length === 0}
	<Card>
		<CardContent class="flex flex-col items-center gap-2 py-10 text-center">
			<span class="grid size-11 place-items-center rounded-full bg-muted text-muted-foreground" aria-hidden="true">
				<svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
					<path d="M8 2v4M16 2v4M3 10h18M5 4h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2Z" />
				</svg>
			</span>
			<p class="text-sm font-medium">Belum ada catatan dosis hari ini</p>
			<p class="max-w-xs text-xs text-muted-foreground">
				Catatan muncul di sini setelah Anda menandai dosis diminum, ditunda, atau dilewati.
			</p>
		</CardContent>
	</Card>
{:else}
	<ul class="space-y-3">
		{#each logs as log (log.id)}
			<li>
				<Card>
					<CardContent class="flex items-center justify-between gap-3 py-4">
						<div class="min-w-0">
							<p class="truncate text-sm font-medium">
								{log.scheduled_time ?? 'Tanpa jam jadwal'}
							</p>
							<p class="mt-0.5 text-xs text-muted-foreground">
								{log.note ?? (log.source === 'caregiver' ? 'Dicatat oleh caregiver' : 'Dicatat sendiri')}
							</p>
						</div>
						<Badge variant={statusVariant[log.status] ?? 'outline'}>
							{statusLabel[log.status] ?? log.status}
						</Badge>
					</CardContent>
				</Card>
			</li>
		{/each}
	</ul>
{/if}

<!-- Keterbatasan yang disampaikan terbuka, bukan disembunyikan. -->
<div class="mt-6 rounded-lg border border-border bg-muted/40 px-4 py-3">
	<p class="text-xs leading-relaxed text-muted-foreground">
		<strong class="font-medium text-foreground">Catatan pengembangan:</strong>
		daftar dosis yang <em>belum</em> tercatat hari ini belum bisa ditampilkan karena backend
		belum mengekspos endpoint jadwal dosis (generatornya sudah ada di
		<code class="rounded bg-background px-1 py-0.5 font-mono text-[11px]">domain/occurrence.go</code>).
		Halaman ini akan menyusul begitu endpoint tersebut tersedia.
	</p>
</div>
