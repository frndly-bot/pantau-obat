<script lang="ts">
	/**
	 * Halaman pasien yang saya pantau — inti peran caregiver.
	 *
	 * KENAPA HALAMAN INI ADA
	 *
	 * Sebelumnya halaman Caregiver hanya menampilkan DAFTAR pasien tanpa satu
	 * pun yang bisa diklik. Akibatnya seseorang bisa menerima undangan jadi
	 * pendamping, lalu tidak bisa melakukan pekerjaannya sama sekali — hanya
	 * melihat nama. Semua API-nya sudah tersedia sejak awal, hanya belum
	 * dipakai.
	 *
	 * APA YANG BOLEH DILAKUKAN
	 *
	 * Batasnya diambil dari server, bukan ditebak dari peran:
	 * `GET /care/{ownerId}/overview` mengirim blok `permissions` berisi
	 * `can_read`, `can_write_log`, dan `can_manage_meds`. UI hanya
	 * menyembunyikan tombol berdasarkan itu.
	 *
	 * Menyembunyikan tombol BUKAN penegakan aturan — server tetap memeriksa
	 * setiap permintaan. Kalau tombolnya disembunyikan padahal server
	 * mengizinkan, pengguna kehilangan fungsi; kalau ditampilkan padahal
	 * server menolak, pengguna mendapat pesan galat. Karena itu sumbernya
	 * harus server.
	 *
	 * PRIVASI
	 *
	 * `prescription_photo_url` dan catatan pribadi pasien TIDAK pernah
	 * dikirim server di jalur ini (disaring di `carelinks.go`, bukan di UI),
	 * jadi halaman ini tidak perlu menyembunyikan apa pun — datanya memang
	 * tidak pernah sampai.
	 */
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import {
		ApiError,
		type CareOverview,
		type DaySlot,
		type MedicationLog,
		type Medication
	} from '@pantau-obat/api-client';
	import {
		Badge,
		Button,
		Card,
		CardContent,
		Dialog,
		Label,
		Skeleton
	} from '@pantau-obat/ui';
	import { api } from '$lib/api';
	import { i18n } from '$lib/i18n.svelte';
	import { formatRatio, rankSlot, summarizeDay } from '$lib/dose-logic';

	const ownerId = $derived(page.params.id ?? '');

	let overview = $state<CareOverview | null>(null);
	let slots = $state<DaySlot[]>([]);
	let medications = $state<Medication[]>([]);
	/**
	 * Catatan dosis terakhir pasien.
	 *
	 * Diambil dari `care.logs`, bukan dari `overview`: endpoint overview
	 * tidak mengembalikan catatan (hanya ringkasan angka). Tipe kliennya
	 * sempat mendeklarasikan `recent_logs` yang tidak pernah dikirim server —
	 * dan itu menghasilkan `undefined` tanpa peringatan tipe.
	 */
	let recentLogs = $state<MedicationLog[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let actionError = $state<string | null>(null);
	let notice = $state<string | null>(null);
	let busy = $state<string | null>(null);

	// Dialog catat dosis atas nama pasien.
	let recordOpen = $state(false);
	let target = $state<DaySlot | null>(null);
	let recordStatus = $state<'taken' | 'late' | 'skipped'>('taken');
	let recordNote = $state('');

	const mealLabel: Record<string, string> = {
		before_meal: 'sebelum makan',
		after_meal: 'setelah makan',
		with_meal: 'bersama makan',
		not_related: 'bebas'
	};

	const statusLabel: Record<string, string> = {
		pending: 'Belum waktunya',
		due: 'Belum dicatat',
		taken: 'Sudah diminum',
		late: 'Terlambat',
		skipped: 'Dilewati',
		snoozed: 'Ditunda'
	};

	const statusVariant: Record<string, 'ok' | 'warn' | 'na' | 'info' | 'outline'> = {
		pending: 'outline',
		due: 'info',
		taken: 'ok',
		late: 'warn',
		skipped: 'na',
		snoozed: 'info'
	};

	/** Izin dari server; `null` selama belum dimuat. */
	let can = $derived(overview?.permissions ?? null);
	let canWriteLog = $derived(can?.can_write_log === true);
	let canManageMeds = $derived(can?.can_manage_meds === true);

	/** Dosis yang perlu ditindak lebih dulu. */
	let ordered = $derived(
		[...slots].sort((a, b) => {
			const diff = rankSlot(a.status) - rankSlot(b.status);
			return diff !== 0 ? diff : a.time_local.localeCompare(b.time_local);
		})
	);

	let ringkasan = $derived(summarizeDay(slots));
	let actionable = $derived(slots.filter((s) => s.status === 'due' || s.status === 'pending'));

	async function load() {
		if (!ownerId) return;
		loading = true;
		try {
			// TIGA sumber, masing-masing untuk hal berbeda:
			//   overview    → kepatuhan + izin (blok `permissions`)
			//   schedule    → dosis hari ini TERMASUK yang belum dicatat
			//   medications → daftar obat pasien
			//
			// `schedule` tidak bisa diganti `logs`: logs hanya berisi dosis
			// yang sudah dicatat, sehingga dosis yang perlu ditindak tidak
			// akan terlihat.
			const [ov, jadwal, meds, logs] = await Promise.all([
				api.care.overview(ownerId),
				api.care.schedule(ownerId),
				api.care.medications(ownerId, { limit: 100 }),
				api.care.logs(ownerId, { limit: 10 })
			]);
			overview = ov;
			slots = jadwal;
			medications = meds as Medication[];
			recentLogs = logs;
			error = null;
		} catch (err) {
			error = err instanceof ApiError ? err.message : 'Gagal memuat data pasien.';
		} finally {
			loading = false;
		}
	}

	/** Mencatat dosis atas nama pasien. */
	async function record() {
		const slot = target;
		if (!slot || busy) return;
		busy = slot.schedule_id;
		actionError = null;
		try {
			await api.care.createLog(ownerId, {
				schedule_id: slot.schedule_id,
				slot_date: slot.slot_date,
				scheduled_hour: slot.scheduled_hour,
				scheduled_minute: slot.scheduled_minute,
				scheduled_tz: slot.scheduled_tz,
				status: recordStatus,
				note: recordNote.trim() || null
			});
			recordOpen = false;
			target = null;
			recordNote = '';
			notice = 'Dosis dicatat atas nama pasien.';
			await load();
		} catch (err) {
			actionError = err instanceof ApiError ? err.message : 'Gagal mencatat dosis.';
		} finally {
			busy = null;
		}
	}

	function openRecord(slot: DaySlot) {
		target = slot;
		recordStatus = 'taken';
		recordNote = '';
		actionError = null;
		recordOpen = true;
	}

	/** Waktu relatif sederhana untuk catatan terakhir. */
	function waktuSingkat(iso: string): string {
		const d = new Date(iso);
		return new Intl.DateTimeFormat('id-ID', {
			day: 'numeric',
			month: 'short',
			hour: '2-digit',
			minute: '2-digit'
		}).format(d);
	}

	onMount(load);
</script>

<svelte:head>
	<title>{overview?.owner_name ?? 'Pasien'} — Pantau Obat</title>
</svelte:head>

<div class="mb-4">
	<Button variant="ghost" size="sm" onclick={() => goto('/caregiver')}>
		← {i18n.t('nav.caregiver')}
	</Button>
</div>

{#if error}
	<div class="rounded-lg border border-destructive/30 bg-destructive/8 px-4 py-3 text-sm text-destructive" role="alert">
		{error}
	</div>
{:else if loading}
	<div class="space-y-3">
		<Skeleton class="h-28 w-full" />
		<Skeleton class="h-40 w-full" />
	</div>
{:else if overview}
	{#if actionError}
		<div class="mb-4 rounded-lg border border-destructive/30 bg-destructive/8 px-4 py-3 text-sm text-destructive" role="alert">
			{actionError}
		</div>
	{/if}
	{#if notice}
		<div class="mb-4 rounded-lg border border-ok/40 bg-ok/10 px-4 py-3 text-sm" role="status">{notice}</div>
	{/if}

	<!-- ============ Identitas & izin ============ -->
	<Card class="mb-4">
		<CardContent class="space-y-3 py-5">
			<div class="flex items-start justify-between gap-3">
				<div class="min-w-0">
					<h1 class="text-lg font-semibold tracking-tight">{overview.owner_name}</h1>
					<p class="mt-0.5 text-xs text-muted-foreground">{overview.timezone}</p>
				</div>
				<Badge variant={canManageMeds ? 'private' : canWriteLog ? 'info' : 'outline'}>
					{canManageMeds
						? i18n.t('caregiver.manager')
						: canWriteLog
							? i18n.t('caregiver.helper')
							: i18n.t('caregiver.viewer')}
				</Badge>
			</div>

			<!-- Izin dinyatakan apa adanya, supaya pengguna tahu batasnya tanpa
			     harus mencoba dan gagal. -->
			<p class="text-xs text-muted-foreground">
				{#if canManageMeds}
					Anda bisa mencatat dosis sekaligus mengelola obat dan stok.
				{:else if canWriteLog}
					Anda bisa mencatat dosis atas nama pasien. Obat dan jadwal hanya bisa dilihat.
				{:else}
					Anda hanya bisa melihat. Mencatat dosis memerlukan izin yang lebih tinggi.
				{/if}
			</p>
		</CardContent>
	</Card>

	<!-- ============ Kepatuhan ============ -->
	<Card class="mb-4">
		<CardContent class="py-4">
			<p class="text-xs font-medium uppercase tracking-wide text-muted-foreground">
				{i18n.t('today.today_s_adherence')}
			</p>
			<div class="mt-2 grid grid-cols-3 gap-3 text-center">
				<div>
					<p class="text-lg font-semibold tabular-nums">
						{overview.summary.taken_doses}/{overview.summary.scheduled_doses}
					</p>
					<p class="text-[11px] text-muted-foreground">{i18n.t('today.recorded')}</p>
				</div>
				<div>
					<p class="text-lg font-semibold tabular-nums">{overview.summary.pending_doses}</p>
					<p class="text-[11px] text-muted-foreground">{i18n.t('today.waiting')}</p>
				</div>
				<div>
					<p class="text-lg font-semibold tabular-nums">
						{formatRatio(overview.summary.adherence_ratio)}
					</p>
					<p class="text-[11px] text-muted-foreground">{i18n.t('today.adherence')}</p>
				</div>
			</div>
			{#if overview.summary.missed_doses > 0}
				<p class="mt-2 text-center text-[11px] text-warn">
					{overview.summary.missed_doses} dosis terlewat pada rentang ini.
				</p>
			{/if}
		</CardContent>
	</Card>

	<!-- ============ Perlu ditindak ============ -->
	{#if actionable.length > 0}
		<section class="mb-5">
			<h2 class="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
				Perlu ditindak
			</h2>
			<ul class="space-y-3">
				{#each actionable as slot (slot.schedule_id + slot.slot_date)}
					<li>
						<Card class={slot.status === 'due' ? 'border-info/40' : ''}>
							<CardContent class="py-4">
								<div class="flex items-start justify-between gap-3">
									<div class="min-w-0">
										<div class="flex items-baseline gap-2">
											<span class="text-sm font-semibold tabular-nums">{slot.time_local}</span>
											<span class="truncate text-sm">{slot.medication_name}</span>
										</div>
										<p class="mt-1 text-xs text-muted-foreground">
											{slot.dose_override ?? `${slot.dose_value} ${slot.dose_unit_code}`}
											· {mealLabel[slot.meal_relation] ?? slot.meal_relation}
										</p>
									</div>
									<Badge variant={statusVariant[slot.status] ?? 'outline'}>
										{statusLabel[slot.status] ?? slot.status}
									</Badge>
								</div>

								<!-- Tombol hanya tampil bila server MENGIZINKAN.
								     Menyembunyikan tombol bukan penegakan aturan;
								     server tetap memeriksa setiap permintaan. -->
								{#if canWriteLog}
									<div class="mt-3.5 flex gap-2">
										<Button
											size="sm"
											class="flex-1"
											disabled={busy === slot.schedule_id}
											onclick={() => openRecord(slot)}
										>
											Catat dosis atas nama pasien
										</Button>
									</div>
								{/if}
							</CardContent>
						</Card>
					</li>
				{/each}
			</ul>
		</section>
	{/if}

	<!-- ============ Obat pasien ============ -->
	<section class="mb-5">
		<h2 class="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
			{i18n.t('nav.obat')} ({medications.length})
		</h2>
		{#if medications.length === 0}
			<Card>
				<CardContent class="py-8 text-center">
					<p class="text-sm text-muted-foreground">Pasien belum menambahkan obat.</p>
				</CardContent>
			</Card>
		{:else}
			<ul class="space-y-2">
				{#each medications as m (m.id)}
					<li>
						<Card>
							<CardContent class="flex items-center justify-between gap-3 py-3">
								<div class="min-w-0">
									<p class="truncate text-sm font-medium">{m.name}</p>
									<p class="mt-0.5 text-xs text-muted-foreground">
										{m.dose_value} {m.dose_unit_code}
										{#if m.as_needed}· PRN{/if}
									</p>
								</div>
								<Badge variant={m.is_active ? 'ok' : 'na'}>
									{m.is_active ? i18n.t('common.active') : i18n.t('meds.archive')}
								</Badge>
							</CardContent>
						</Card>
					</li>
				{/each}
			</ul>
		{/if}
	</section>

	<!-- ============ Catatan terakhir ============ -->
	<section>
		<h2 class="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
			{i18n.t('nav.riwayat')}
		</h2>
		{#if recentLogs.length === 0}
			<Card>
				<CardContent class="py-8 text-center">
					<p class="text-sm text-muted-foreground">Belum ada catatan dosis.</p>
				</CardContent>
			</Card>
		{:else}
			<ul class="space-y-2">
				{#each recentLogs as log (log.id)}
					<li>
						<Card>
							<CardContent class="flex items-center justify-between gap-3 py-3">
								<div class="min-w-0">
									<p class="truncate text-sm">{log.scheduled_time ?? '—'}</p>
									<p class="mt-0.5 text-xs text-muted-foreground">
										{waktuSingkat(log.taken_at ?? log.created_at)}
										{#if log.source === 'caregiver'}· dicatat pendamping{/if}
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
	</section>
{/if}

<!-- ============ Dialog: catat dosis atas nama pasien ============ -->
<Dialog open={recordOpen} title="Catat dosis atas nama pasien" onopenchange={(v) => !v && (recordOpen = false)}>
	<div class="space-y-4">
		<p class="text-sm text-muted-foreground">
			{target?.medication_name} · {target?.time_local}
		</p>

		<!-- Jejaknya harus jelas: pasien berhak tahu siapa yang mencatat
		     atas namanya. Server menyimpan actor_id = Anda, owner_id = pasien. -->
		<p class="rounded-lg border border-border bg-muted/40 px-3.5 py-3 text-xs text-muted-foreground">
			Catatan ini akan tersimpan atas nama <b>{overview?.owner_name}</b>, dan
			riwayatnya menunjukkan bahwa Anda yang mencatat.
		</p>

		<div class="space-y-2">
			<Label>Status</Label>
			<div class="flex gap-2">
				{#each [['taken', 'Sudah diminum'], ['late', 'Terlambat'], ['skipped', 'Dilewati']] as [val, label] (val)}
					<button
						type="button"
						class="flex-1 rounded-md border px-3 py-2 text-sm font-medium transition-colors {recordStatus === val
							? 'border-primary bg-primary text-primary-foreground'
							: 'border-border hover:bg-accent'}"
						aria-pressed={recordStatus === val}
						onclick={() => (recordStatus = val as 'taken' | 'late' | 'skipped')}
					>
						{label}
					</button>
				{/each}
			</div>
		</div>

		<div class="space-y-2">
			<Label for="c-note">Catatan (opsional)</Label>
			<input
				id="c-note"
				class="h-9 w-full rounded-md border border-input bg-background px-3 text-sm shadow-xs"
				bind:value={recordNote}
				placeholder="Misalnya: diminum setelah makan siang"
			/>
		</div>

		{#if actionError}
			<p class="rounded-md border border-destructive/30 bg-destructive/8 px-3 py-2 text-sm text-destructive" role="alert">
				{actionError}
			</p>
		{/if}
	</div>
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={() => (recordOpen = false)}>Batal</Button>
			<Button onclick={record} disabled={busy !== null}>
				{busy ? 'Menyimpan…' : 'Simpan catatan'}
			</Button>
		</div>
	{/snippet}
</Dialog>
