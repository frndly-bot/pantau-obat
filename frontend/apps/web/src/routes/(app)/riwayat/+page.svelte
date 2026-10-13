<script lang="ts">
	/**
	 * Halaman riwayat: catatan dosis dengan filter rentang dan ekspor CSV.
	 *
	 * Dua hal yang perlu diperhatikan di sini:
	 *
	 * 1. `GET /logs` hanya mengembalikan `medication_id`, bukan nama obat. Nama
	 *    diambil dari daftar obat dan dipetakan di klien — satu panggilan,
	 *    bukan satu panggilan per baris.
	 *
	 * 2. Unduhan CSV TIDAK bisa memakai tautan biasa. Access token hidup di
	 *    memori dan dikirim sebagai header `Authorization`, sedangkan
	 *    `<a href>` tidak membawa header apa pun. Jadi berkasnya diambil lewat
	 *    klien API, lalu diserahkan ke browser sebagai blob.
	 */
	import { onMount } from 'svelte';
	import { ApiError, type Medication, type MedicationLog } from '@pantau-obat/api-client';
	import { Badge, Button, Card, CardContent, Input, Label, Skeleton } from '@pantau-obat/ui';
	import { api } from '$lib/api';
	import { session } from '$lib/session.svelte';

	let logs = $state<MedicationLog[]>([]);
	let medNames = $state<Record<string, string>>({});
	let loading = $state(true);
	let error = $state<string | null>(null);
	let exporting = $state(false);
	let exportError = $state<string | null>(null);

	/** Rentang bawaan: 30 hari terakhir. */
	let to = $state('');
	let from = $state('');
	let statusFilter = $state('');
	let sourceFilter = $state('');

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

	/**
	 * Sumber catatan. `system` ada di sini karena server punya TIGA nilai
	 * (self, caregiver, system) sedangkan prototipe design hanya menawarkan
	 * dua. Akibatnya catatan yang lahir otomatis — mis. dari alur koreksi
	 * atau penandaan pekerja latar — tidak bisa disaring dan tidak bisa
	 * dibedakan dari catatan yang dibuat manusia: pengguna melihat baris tak
	 * dikenal tanpa penjelasan siapa pembuatnya.
	 */
	const SOURCE_OPTIONS: ReadonlyArray<readonly [string, string]> = [
		['', 'Semua sumber'],
		['self', 'Saya sendiri'],
		['caregiver', 'Caregiver'],
		['system', 'Sistem']
	];

	const sourceLabel: Record<string, string> = {
		self: 'saya sendiri',
		caregiver: 'dicatat caregiver',
		system: 'diperbarui otomatis'
	};

	/** Tanggal hari ini menurut zona waktu pengguna, bukan UTC. */
	function todayLocal(): string {
		return new Intl.DateTimeFormat('en-CA', {
			timeZone: session.user?.timezone ?? 'Asia/Jakarta'
		}).format(new Date());
	}

	/** Menggeser tanggal sejumlah hari (negatif = ke belakang). */
	function shiftDays(iso: string, days: number): string {
		const d = new Date(iso + 'T00:00:00');
		d.setDate(d.getDate() + days);
		return new Intl.DateTimeFormat('en-CA').format(d);
	}

	function formatDate(iso: string): string {
		return new Intl.DateTimeFormat('id-ID', {
			weekday: 'long',
			day: 'numeric',
			month: 'long',
			year: 'numeric'
		}).format(new Date(iso + 'T00:00:00'));
	}

	/** Catatan dikelompokkan per tanggal supaya mudah dibaca. */
	let grouped = $derived.by(() => {
		const map = new Map<string, MedicationLog[]>();
		for (const log of logs) {
			const list = map.get(log.slot_date) ?? [];
			list.push(log);
			map.set(log.slot_date, list);
		}
		return [...map.entries()].sort((a, b) => b[0].localeCompare(a[0]));
	});

	async function load() {
		loading = true;
		try {
			const [list, meds] = await Promise.all([
				api.logs.list({
					from: from || undefined,
					to: to || undefined,
					status: statusFilter || undefined,
					source: sourceFilter || undefined,
					limit: 100
				}),
				api.medications.list({ limit: 100 })
			]);
			logs = list;
			medNames = Object.fromEntries((meds as Medication[]).map((m) => [m.id, m.name]));
			error = null;
		} catch (err) {
			error = err instanceof ApiError ? err.message : 'Gagal memuat riwayat.';
		} finally {
			loading = false;
		}
	}

	/** Mengunduh CSV lewat klien API, lalu menyerahkannya ke browser. */
	async function exportCsv() {
		exporting = true;
		exportError = null;
		try {
			const blob = await api.logs.exportCsv({ from, to });
			const url = URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = `pantau-obat-riwayat-${from}-${to}.csv`;
			document.body.appendChild(a);
			a.click();
			a.remove();
			// Objek URL harus dilepas, kalau tidak berkasnya tertahan di memori.
			URL.revokeObjectURL(url);
		} catch (err) {
			exportError = err instanceof ApiError ? err.message : 'Gagal mengunduh CSV.';
		} finally {
			exporting = false;
		}
	}

	function setPreset(days: number) {
		to = todayLocal();
		from = shiftDays(to, -(days - 1));
		void load();
	}

	onMount(() => {
		to = todayLocal();
		from = shiftDays(to, -29);
		void load();
	});
</script>

<svelte:head>
	<title>Riwayat — Pantau Obat</title>
</svelte:head>

<div class="mb-5">
	<h1 class="text-2xl font-semibold tracking-tight">Riwayat</h1>
	<p class="mt-1 text-sm text-muted-foreground">Catatan dosis Anda.</p>
</div>

<!-- ============ Filter ============ -->
<Card>
	<CardContent class="space-y-3 py-4">
		<div class="grid grid-cols-2 gap-3">
			<div class="space-y-1.5">
				<Label for="f-from">Dari</Label>
				<Input id="f-from" type="date" bind:value={from} />
			</div>
			<div class="space-y-1.5">
				<Label for="f-to">Sampai</Label>
				<Input id="f-to" type="date" bind:value={to} />
			</div>
		</div>

		<div class="flex flex-wrap gap-2">
			<Button variant="outline" size="sm" onclick={() => setPreset(7)}>7 hari</Button>
			<Button variant="outline" size="sm" onclick={() => setPreset(30)}>30 hari</Button>
			<Button variant="outline" size="sm" onclick={() => setPreset(90)}>90 hari</Button>
		</div>

		<div class="flex flex-wrap gap-1.5">
			{#each [['', 'Semua'], ['taken', 'Diminum'], ['late', 'Terlambat'], ['skipped', 'Dilewati'], ['snoozed', 'Ditunda']] as [val, label] (val)}
				<button
					type="button"
					class="rounded-md border px-2.5 py-1 text-xs font-medium transition-colors {statusFilter === val
						? 'border-primary bg-primary text-primary-foreground'
						: 'border-border hover:bg-accent'}"
					aria-pressed={statusFilter === val}
					onclick={() => {
						statusFilter = val;
						void load();
					}}
				>
					{label}
				</button>
			{/each}
		</div>

		<!-- Filter sumber. Dipisah dari filter status karena keduanya menjawab
		     pertanyaan berbeda: "apa yang tercatat" vs "siapa yang mencatat".
		     Tanpa filter sumber, catatan otomatis tidak bisa dipisahkan dari
		     catatan yang dibuat manusia. -->
		<div class="flex flex-wrap gap-1.5">
			{#each SOURCE_OPTIONS as [val, label] (val)}
				<button
					type="button"
					class="rounded-md border px-2.5 py-1 text-xs font-medium transition-colors {sourceFilter === val
						? 'border-primary bg-primary text-primary-foreground'
						: 'border-border hover:bg-accent'}"
					aria-pressed={sourceFilter === val}
					onclick={() => {
						sourceFilter = val;
						void load();
					}}
				>
					{label}
				</button>
			{/each}
		</div>

		<div class="flex gap-2 pt-1">
			<Button size="sm" class="flex-1" onclick={load} disabled={loading}>
				{loading ? 'Memuat…' : 'Terapkan'}
			</Button>
			<Button size="sm" variant="outline" onclick={exportCsv} disabled={exporting || !from || !to}>
				{exporting ? 'Menyiapkan…' : 'Ekspor CSV'}
			</Button>
		</div>
	</CardContent>
</Card>

{#if exportError}
	<div class="mt-4 rounded-lg border border-destructive/30 bg-destructive/8 px-4 py-3 text-sm text-destructive" role="alert">
		{exportError}
	</div>
{/if}

{#if error}
	<div class="mt-4 rounded-lg border border-destructive/30 bg-destructive/8 px-4 py-3 text-sm text-destructive" role="alert">
		{error}
	</div>
{:else if loading}
	<div class="mt-4 space-y-3">
		{#each [1, 2, 3] as n (n)}
			<Skeleton class="h-16 w-full" />
		{/each}
	</div>
{:else if logs.length === 0}
	<Card class="mt-4">
		<CardContent class="py-10 text-center">
			<p class="text-sm font-medium">Tidak ada catatan pada rentang ini</p>
			<p class="mt-1 text-xs text-muted-foreground">
				Coba ubah rentang tanggal, atau catat dosis dari halaman Hari Ini.
			</p>
		</CardContent>
	</Card>
{:else}
	<div class="mt-5 space-y-5">
		{#each grouped as [date, items] (date)}
			<section>
				<h2 class="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
					{formatDate(date)}
				</h2>
				<ul class="space-y-2">
					{#each items as log (log.id)}
						<li>
							<Card>
								<CardContent class="flex items-center justify-between gap-3 py-3">
									<div class="min-w-0">
										<p class="truncate text-sm font-medium">
											{medNames[log.medication_id] ?? 'Obat'}
										</p>
										<p class="mt-0.5 text-xs text-muted-foreground">
											{log.scheduled_time ?? 'tanpa jam'}
											{#if log.source && log.source !== 'self'}· {sourceLabel[log.source]}{/if}
											{#if log.note}· {log.note}{/if}
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
			</section>
		{/each}
	</div>
{/if}
