<script lang="ts">
	/**
	 * Halaman daftar obat + wizard tambah obat.
	 *
	 * Daftar diambil dari `GET /medications` sungguhan. Wizard (5 langkah, 5
	 * pola frekuensi) ada di `$lib/components/medication-wizard.svelte`.
	 */
	import { onMount } from 'svelte';
	import { ApiError, type Medication } from '@pantau-obat/api-client';
	import { Badge, Button, Card, CardContent, Skeleton } from '@pantau-obat/ui';
	import { api } from '$lib/api';
	import { session } from '$lib/session.svelte';
	import MedicationWizard from '$lib/components/medication-wizard.svelte';

	let meds = $state<Medication[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let wizardOpen = $state(false);
	let justSaved = $state(false);

	const mealLabel: Record<string, string> = {
		before_meal: 'Sebelum makan',
		after_meal: 'Setelah makan',
		with_meal: 'Bersama makan',
		not_related: 'Tidak terkait makan'
	};

	// CATATAN: pola frekuensi TIDAK ditampilkan di daftar. `GET /medications`
	// sengaja tidak menyertakan frekuensi (menghindari N+1 query pada daftar
	// panjang), jadi menampilkannya di sini berarti mengarang data. Pola
	// tersedia di halaman detail obat.

	/** Tanggal hari ini menurut zona waktu pengguna, bukan UTC. */
	let today = $derived(
		new Intl.DateTimeFormat('en-CA', {
			timeZone: session.user?.timezone ?? 'Asia/Jakarta'
		}).format(new Date())
	);

	async function load() {
		try {
			meds = await api.medications.list();
			error = null;
		} catch (err) {
			error = err instanceof ApiError ? err.message : 'Gagal memuat daftar obat.';
		} finally {
			loading = false;
		}
	}

	async function onSaved() {
		justSaved = true;
		await load();
		setTimeout(() => (justSaved = false), 4000);
	}

	onMount(load);
</script>

<svelte:head>
	<title>Obat — Pantau Obat</title>
</svelte:head>

<div class="mb-5 flex items-center justify-between gap-3">
	<div>
		<h1 class="text-2xl font-semibold tracking-tight">Obat</h1>
		<p class="mt-1 text-sm text-muted-foreground">Daftar obat yang Anda pantau.</p>
	</div>
	<Button size="sm" onclick={() => (wizardOpen = true)}>Tambah obat</Button>
</div>

{#if justSaved}
	<div class="mb-4 rounded-lg border border-ok/40 bg-ok/10 px-4 py-3 text-sm" role="status">
		Obat disimpan. Jadwalnya langsung muncul di halaman Hari Ini.
	</div>
{/if}

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
			<span class="grid size-11 place-items-center rounded-full bg-muted text-muted-foreground" aria-hidden="true">
				<svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
					<path d="M10.5 20.5a5 5 0 0 1-7-7l7-7a5 5 0 0 1 7 7Z" />
					<path d="m8.5 8.5 7 7" />
				</svg>
			</span>
			<p class="text-sm font-medium">Belum ada obat</p>
			<p class="max-w-xs text-xs text-muted-foreground">
				Tambahkan obat pertama Anda beserta jadwalnya. Setelah itu, dosisnya muncul di halaman
				Hari Ini.
			</p>
			<Button size="sm" class="mt-2" onclick={() => (wizardOpen = true)}>Tambah obat</Button>
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
								{#if med.must_finish}
									<p class="mt-1 text-xs text-muted-foreground">Harus dihabiskan</p>
								{/if}
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

<MedicationWizard
	bind:open={wizardOpen}
	timezone={session.user?.timezone ?? 'Asia/Jakarta'}
	today={today}
	onsaved={onSaved}
/>
