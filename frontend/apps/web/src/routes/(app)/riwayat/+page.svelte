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
	import { i18n } from '$lib/i18n.svelte';
	import { computeRatio, formatRatio } from '$lib/dose-logic';
	import type { AdherenceDailyPoint } from '@pantau-obat/api-client';

	let logs = $state<MedicationLog[]>([]);
	let medNames = $state<Record<string, string>>({});
	/**
	 * Rekap kepatuhan harian — SATU-SATUNYA sumber yang bisa menggambarkan
	 * hari yang bolong.
	 *
	 * `missed_doses` di sini berasal dari `medication_dose_events`, bukan dari
	 * status `medication_logs`: dosis terlewat hidup di tabel terpisah dan
	 * tidak ikut dalam daftar catatan. Karena itu filter "Terlewat" pada
	 * daftar catatan akan selalu kosong — bukan bug tampilan, datanya memang
	 * tidak pernah ada di sana.
	 */
	let harian = $state<AdherenceDailyPoint[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let exporting = $state(false);
	let exportError = $state<string | null>(null);

	/**
	 * Ringkasan kepatuhan rentang terpilih.
	 *
	 * Memakai `computeRatio` yang SAMA seperti halaman Hari Ini dan server,
	 * supaya dua layar tidak pernah menampilkan angka berbeda untuk hari yang
	 * sama. Versi lama halaman ini menghitung `(taken + late) / total`,
	 * sementara server hanya memasukkan `taken` — jadi angkanya selalu lebih
	 * tinggi dari laporan server.
	 */
	let ringkasan = $derived.by(() => {
		const ratio = computeRatio(logs);
		const taken = logs.filter((l) => l.status === 'taken').length;
		const late = logs.filter((l) => l.status === 'late').length;
		const skipped = logs.filter((l) => l.status === 'skipped').length;
		return { ratio, taken, late, skipped, total: logs.length, teks: formatRatio(ratio) };
	});

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
			const [list, meds, hari] = await Promise.all([
				api.logs.list({
					from: from || undefined,
					to: to || undefined,
					status: statusFilter || undefined,
					source: sourceFilter || undefined,
					limit: 100
				}),
				api.medications.list({ limit: 100 }),
				// Rekap harian dimuat terpisah dari daftar catatan: ia memuat
				// dosis TERLEWAT yang tidak ada di daftar catatan.
				api.adherence.daily({ from: from || undefined, to: to || undefined })
			]);
			logs = list;
			medNames = Object.fromEntries((meds as Medication[]).map((m) => [m.id, m.name]));
			harian = hari;
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
	<h1 class="text-2xl font-semibold tracking-tight">{i18n.t('nav.riwayat')}</h1>
	<p class="mt-1 text-sm text-muted-foreground">{i18n.t('riwayat.subtitle')}</p>
</div>

<!-- Ringkasan kepatuhan. Angkanya memakai aturan yang sama dengan server,
     sehingga tidak mungkin berbeda dari laporan di tempat lain. -->
{#if !loading && !error && logs.length > 0}
	<Card class="mb-4">
		<CardContent class="py-4">
			<div class="flex items-baseline justify-between gap-3">
				<div>
					<p class="text-2xl font-semibold tabular-nums">{ringkasan.teks}</p>
					<p class="mt-0.5 text-xs text-muted-foreground">{i18n.t('riwayat.adherence_range')}</p>
				</div>
				<p class="text-xs text-muted-foreground tabular-nums">{ringkasan.total} {i18n.t('riwayat.records_count')}</p>
			</div>
			<dl class="mt-3 grid grid-cols-3 gap-3 border-t border-border pt-3 text-center">
				<div>
					<dt class="text-[11px] text-muted-foreground">{i18n.t('riwayat.status_taken')}</dt>
					<dd class="text-sm font-semibold tabular-nums">{ringkasan.taken}</dd>
				</div>
				<div>
					<dt class="text-[11px] text-muted-foreground">{i18n.t('riwayat.status_late')}</dt>
					<dd class="text-sm font-semibold tabular-nums">{ringkasan.late}</dd>
				</div>
				<div>
					<dt class="text-[11px] text-muted-foreground">{i18n.t('riwayat.status_skipped')}</dt>
					<dd class="text-sm font-semibold tabular-nums">{ringkasan.skipped}</dd>
				</div>
			</dl>
			{#if ringkasan.ratio === null}
				<p class="mt-2 text-center text-[11px] text-muted-foreground">
					{i18n.t('riwayat.no_data_range')}
				</p>
			{/if}
		</CardContent>
	</Card>
{/if}

<!-- Bagan kepatuhan harian.
     Sumbernya rekap harian, BUKAN daftar catatan — hanya rekap itu yang
     memuat dosis terlewat. Hari yang bolong tidak akan muncul di daftar
     catatan sama sekali. -->
{#if !loading && !error && harian.length > 0}
	<Card class="mb-4">
		<CardContent class="py-4">
			<div class="flex items-baseline justify-between gap-3">
				<p class="text-sm font-medium">{i18n.t('riwayat.daily_title')}</p>
				<p class="text-xs text-muted-foreground">
					{harian.length} hari
				</p>
			</div>

			<!-- Batang disusun dari kiri (terlama) ke kanan (terbaru).
			     Tinggi = rasio hari itu; `null` digambar sebagai batang kosong
			     bergaris, bukan nol — karena "belum ada data" bukan "gagal". -->
			<div class="mt-3 flex h-24 items-end gap-1" role="img" aria-label="Bagan kepatuhan harian">
				{#each [...harian].reverse() as d (d.date)}
					{@const tinggi = d.adherence_ratio === null ? 0 : Math.round(d.adherence_ratio * 100)}
					<div class="flex flex-1 flex-col items-center gap-1" title="{d.date}: {d.taken_doses}/{d.scheduled_doses}">
						<div class="flex h-20 w-full items-end">
							<div
								class="w-full rounded-t {d.missed_doses > 0
									? 'bg-warn'
									: d.adherence_ratio === null
										? 'bg-muted'
										: 'bg-ok'}"
								style="height: {d.adherence_ratio === null ? 3 : Math.max(3, tinggi)}%"
							></div>
						</div>
					</div>
				{/each}
			</div>

			<dl class="mt-3 grid grid-cols-3 gap-3 border-t border-border pt-3 text-center">
				<div>
					<dt class="text-[11px] text-muted-foreground">{i18n.t('riwayat.scheduled_total')}</dt>
					<dd class="text-sm font-semibold tabular-nums">
						{harian.reduce((a, d) => a + d.scheduled_doses, 0)}
					</dd>
				</div>
				<div>
					<dt class="text-[11px] text-muted-foreground">{i18n.t('riwayat.missed_total')}</dt>
					<dd class="text-sm font-semibold tabular-nums text-warn">
						{harian.reduce((a, d) => a + d.missed_doses, 0)}
					</dd>
				</div>
				<div>
					<dt class="text-[11px] text-muted-foreground">{i18n.t('riwayat.days_without_data')}</dt>
					<dd class="text-sm font-semibold tabular-nums">
						{harian.filter((d) => d.adherence_ratio === null).length}
					</dd>
				</div>
			</dl>

			<p class="mt-2 text-[11px] text-muted-foreground">
				Batang kuning menandai hari dengan dosis terlewat. Batang abu-abu berarti
				belum ada dosis yang bisa dinilai pada hari itu.
			</p>
		</CardContent>
	</Card>
{/if}

<!-- Kalender bulanan: menandai hari yang bolong, yang tidak terlihat di
     daftar catatan karena dosis terlewat tidak pernah menjadi baris catatan. -->
{#if !loading && !error && harian.length > 0}
	<Card class="mb-4">
		<CardContent class="py-4">
			<p class="text-sm font-medium">{i18n.t('riwayat.calendar_title')}</p>
			<div class="mt-3 grid grid-cols-7 gap-1.5">
				{#each ['S', 'S', 'R', 'K', 'J', 'S', 'M'] as h, i (i)}
					<span class="text-center text-[10px] font-medium text-muted-foreground">{h}</span>
				{/each}
				{#each [...harian].reverse() as d (d.date)}
					{@const tgl = Number(d.date.slice(8, 10))}
					<div
						class="grid aspect-square place-items-center rounded-md text-[11px] font-medium tabular-nums {d.missed_doses >
						0
							? 'bg-warn/20 text-warn'
							: d.adherence_ratio === null
								? 'bg-muted text-muted-foreground'
								: 'bg-ok/20 text-ok'}"
						title="{d.date}: {d.taken_doses} diminum, {d.missed_doses} terlewat"
					>
						{tgl}
					</div>
				{/each}
			</div>
			<div class="mt-3 flex flex-wrap gap-3 text-[11px] text-muted-foreground">
				<span class="flex items-center gap-1.5">
					<span class="size-2.5 rounded-sm bg-ok/60"></span> {i18n.t('riwayat.legend_recorded')}
				</span>
				<span class="flex items-center gap-1.5">
					<span class="size-2.5 rounded-sm bg-warn/60"></span> {i18n.t('riwayat.legend_missed')}
				</span>
				<span class="flex items-center gap-1.5">
					<span class="size-2.5 rounded-sm bg-muted"></span> {i18n.t('riwayat.legend_nodata')}
				</span>
			</div>
		</CardContent>
	</Card>
{/if}

<!-- ============ Filter ============ -->
<Card>
	<CardContent class="space-y-3 py-4">
		<div class="grid grid-cols-2 gap-3">
			<div class="space-y-1.5">
				<Label for="f-from">{i18n.t('riwayat.from')}</Label>
				<Input id="f-from" type="date" bind:value={from} />
			</div>
			<div class="space-y-1.5">
				<Label for="f-to">{i18n.t('riwayat.to')}</Label>
				<Input id="f-to" type="date" bind:value={to} />
			</div>
		</div>

		<div class="flex flex-wrap gap-2">
			<Button variant="outline" size="sm" onclick={() => setPreset(7)}>{i18n.t('riwayat.range_7')}</Button>
			<Button variant="outline" size="sm" onclick={() => setPreset(30)}>{i18n.t('riwayat.range_30')}</Button>
			<Button variant="outline" size="sm" onclick={() => setPreset(90)}>{i18n.t('riwayat.range_90')}</Button>
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
				{loading ? i18n.t('riwayat.loading') : i18n.t('riwayat.apply')}
			</Button>
			<Button size="sm" variant="outline" onclick={exportCsv} disabled={exporting || !from || !to}>
				{exporting ? i18n.t('riwayat.preparing') : i18n.t('riwayat.export_csv')}
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
			<p class="text-sm font-medium">{i18n.t('riwayat.empty_title')}</p>
			<p class="mt-1 text-xs text-muted-foreground">
				{i18n.t('riwayat.empty_hint')}
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
