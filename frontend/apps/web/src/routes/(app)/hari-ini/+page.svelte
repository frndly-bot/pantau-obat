<script lang="ts">
	/**
	 * Halaman "Hari Ini" — daftar dosis hari ini, termasuk yang BELUM dicatat.
	 *
	 * Sumber: `GET /schedule` (bukan `GET /logs/today`). Bedanya menentukan:
	 * `/logs/today` hanya mengembalikan dosis yang sudah dicatat, sehingga
	 * layar kosong sampai user mencatat sesuatu — justru saat pengingat paling
	 * dibutuhkan. `/schedule` menghitung slot dari aturan frekuensi, jadi dosis
	 * yang jatuh tempo selalu terlihat.
	 *
	 * AKSI PER STATUS (prototipe design mendefinisikan ini; sebelumnya hanya
	 * ada dua tombol untuk dua status):
	 *
	 *   pending / due  → [Diminum] [Tunda] [Lewati]
	 *   snoozed        → [Diminum] [Tunda lagi]
	 *   taken / late   → [Koreksi]
	 *   skipped        → [Diminum]
	 *
	 * `Tunda` menggeser pengingat milik JAM jadwal, bukan catatan dosis —
	 * catatannya memang belum ada saat dosis ditunda. Karena itu endpoint-nya
	 * menerima `schedule_id`, dan `medication_id` dikirim lewat query.
	 */
	import { onMount } from 'svelte';
	import { ApiError, type DaySlot, type Medication } from '@pantau-obat/api-client';
	import { Badge, Button, Card, CardContent, Dialog, Label, Skeleton } from '@pantau-obat/ui';
	import { api } from '$lib/api';
	import { session } from '$lib/session.svelte';
	import { i18n } from '$lib/i18n.svelte';
	import { summarizeDay, rankSlot } from '$lib/dose-logic';

	let slots = $state<DaySlot[]>([]);
	let medications = $state<Medication[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	/** schedule_id yang sedang diproses, supaya tombolnya tidak diklik dua kali. */
	let busy = $state<string | null>(null);
	let actionError = $state<string | null>(null);
	let notice = $state<string | null>(null);

	// ── Dialog ──
	/** Slot yang sedang ditindak di dialog (tunda / lewati / koreksi / PRN). */
	let target = $state<DaySlot | null>(null);
	let dialog = $state<'snooze' | 'skip' | 'correct' | 'prn' | null>(null);

	// Isian dialog
	let snoozeMinutes = $state(10);
	let skipReason = $state('');
	let correctStatus = $state<'taken' | 'late' | 'skipped'>('taken');
	let correctTime = $state('');
	let correctNote = $state('');
	let prnMedId = $state('');
	let prnNote = $state('');

	/**
	 * Label status memakai KUNCI kamus, bukan teks jadi.
	 *
	 * Kuncinya sengaja ditulis lengkap (bukan dirakit dari nama status),
	 * supaya penggantian nama status di backend tidak diam-diam menghasilkan
	 * label kosong.
	 */
	const STATUS_KEY: Record<string, string> = {
		pending: 'today.waiting',
		due: 'today.due',
		taken: 'today.taken_2',
		late: 'history.late_2',
		skipped: 'history.skipped_2',
		snoozed: 'today.snooze'
	};

	function statusText(status: string): string {
		const key = STATUS_KEY[status];
		return key ? i18n.t(key) : status;
	}

	const statusVariant: Record<string, 'ok' | 'warn' | 'na' | 'info' | 'outline'> = {
		pending: 'outline',
		due: 'info',
		taken: 'ok',
		late: 'warn',
		skipped: 'na',
		snoozed: 'info'
	};

	const mealLabel: Record<string, string> = {
		before_meal: 'sebelum makan',
		after_meal: 'setelah makan',
		with_meal: 'bersama makan',
		not_related: 'bebas'
	};

	/** Pilihan menit tunda; cocok dengan nilai yang diterima server. */
	const SNOOZE_OPTIONS = [5, 10, 30] as const;

	/** Obat PRN (tanpa jadwal tetap) yang bisa dicatat kapan saja. */
	let prnMeds = $derived(medications.filter((m) => m.as_needed));

	/** Dosis yang perlu ditindak lebih dulu: `due`, lalu `pending`, lalu sisanya. */
	let ordered = $derived(
		[...slots].sort((a, b) => {
			const diff = rankSlot(a.status) - rankSlot(b.status);
			return diff !== 0 ? diff : a.time_local.localeCompare(b.time_local);
		})
	);

	/** Ringkasan memakai aturan bersama — angka yang sama dengan server. */
	let summary = $derived(summarizeDay(slots));

	let today = $derived(
		new Intl.DateTimeFormat('id-ID', {
			weekday: 'long',
			day: 'numeric',
			month: 'long',
			year: 'numeric'
		}).format(new Date())
	);

	/** Jam lokal pemilik dalam `HH:MM`, untuk mengisi isian koreksi. */
	function nowLocalHHMM(): string {
		return new Intl.DateTimeFormat('en-GB', {
			hour: '2-digit',
			minute: '2-digit',
			hour12: false,
			timeZone: session.user?.timezone ?? 'Asia/Jakarta'
		}).format(new Date());
	}

	async function load() {
		try {
			const [hariIni, meds] = await Promise.all([
				api.schedule.day(),
				api.medications.list({ limit: 100 })
			]);
			slots = hariIni;
			medications = meds as Medication[];
			error = null;
		} catch (err) {
			error = err instanceof ApiError ? err.message : 'Gagal memuat jadwal hari ini.';
		} finally {
			loading = false;
		}
	}

	/** Menjalankan satu aksi tulis, lalu menyegarkan daftar dari server. */
	async function run(scheduleId: string, fn: () => Promise<unknown>, sukses: string) {
		if (busy) return;
		busy = scheduleId;
		actionError = null;
		notice = null;
		try {
			await fn();
			closeDialog();
			notice = sukses;
			await load();
		} catch (err) {
			actionError = err instanceof ApiError ? err.message : 'Gagal menyimpan catatan.';
		} finally {
			busy = null;
		}
	}

	/** Mencatat dosis apa adanya (diminum / dilewati). */
	function record(slot: DaySlot, status: 'taken' | 'skipped') {
		return run(
			slot.schedule_id,
			() =>
				api.logs.create(slot.medication_id, {
					schedule_id: slot.schedule_id,
					slot_date: slot.slot_date,
					scheduled_hour: slot.scheduled_hour,
					scheduled_minute: slot.scheduled_minute,
					scheduled_tz: slot.scheduled_tz,
					status
				}),
			status === 'taken' ? 'Dosis dicatat.' : 'Dosis dilewati.'
		);
	}

	/** Menunda pengingat dosis ini. */
	function snooze() {
		const slot = target;
		if (!slot) return;
		return run(
			slot.schedule_id,
			() =>
				api.logs.snooze(slot.schedule_id, { minutes: snoozeMinutes }, slot.medication_id),
			`Pengingat ditunda ${snoozeMinutes} menit.`
		);
	}

	/** Melewati dosis dengan alasan opsional. */
	function skip() {
		const slot = target;
		if (!slot) return;
		return run(
			slot.schedule_id,
			() =>
				api.logs.create(slot.medication_id, {
					schedule_id: slot.schedule_id,
					slot_date: slot.slot_date,
					scheduled_hour: slot.scheduled_hour,
					scheduled_minute: slot.scheduled_minute,
					scheduled_tz: slot.scheduled_tz,
					status: 'skipped',
					note: skipReason.trim() || null
				}),
			'Dosis dilewati.'
		);
	}

	/**
	 * Mengoreksi catatan yang sudah ada.
	 *
	 * Koreksi BUKAN nilai status pada pencatatan baru — server hanya menerima
	 * taken/late/skipped/snoozed. Koreksi adalah permintaan tersendiri
	 * (`PATCH /logs/{id}`), dan itu sebabnya ia jadi aksi terpisah, bukan
	 * pilihan di dalam daftar status.
	 */
	function correct() {
		const slot = target;
		if (!slot?.log_id) return;
		// `taken_at` dikirim sebagai waktu lengkap: server menerima RFC3339,
		// sedangkan pengguna mengisi `HH:MM` saja.
		const takenAt = correctTime
			? `${slot.slot_date}T${correctTime}:00${offsetOf(slot.scheduled_tz)}`
			: null;
		return run(
			slot.schedule_id,
			() =>
				api.logs.update(slot.log_id!, {
					status: correctStatus,
					taken_at: takenAt,
					note: correctNote.trim() || null
				}),
			'Catatan diperbarui.'
		);
	}

	/** Menghitung offset zona waktu (`+07:00`) untuk tanggal tertentu. */
	function offsetOf(tz: string): string {
		const d = new Date();
		const utc = new Date(d.toLocaleString('en-US', { timeZone: 'UTC' }));
		const local = new Date(d.toLocaleString('en-US', { timeZone: tz }));
		const menit = Math.round((local.getTime() - utc.getTime()) / 60000);
		const tanda = menit < 0 ? '-' : '+';
		const abs = Math.abs(menit);
		return `${tanda}${String(Math.floor(abs / 60)).padStart(2, '0')}:${String(abs % 60).padStart(2, '0')}`;
	}

	/** Mencatat dosis obat PRN — tanpa jadwal, boleh berkali-kali sehari. */
	function recordPrn() {
		if (!prnMedId) return;
		const slotDate = slots[0]?.slot_date ?? new Intl.DateTimeFormat('en-CA').format(new Date());
		return run(
			'prn',
			() =>
				api.logs.createPrn(prnMedId, {
					slot_date: slotDate,
					status: 'taken',
					taken_at: new Date().toISOString(),
					note: prnNote.trim() || null
				}),
			'Dosis PRN dicatat.'
		);
	}

	function openDialog(kind: typeof dialog, slot: DaySlot) {
		target = slot;
		dialog = kind;
		actionError = null;
		if (kind === 'snooze') snoozeMinutes = 10;
		if (kind === 'skip') skipReason = '';
		if (kind === 'correct') {
			correctStatus = (slot.status === 'late' ? 'late' : 'taken') as 'taken' | 'late' | 'skipped';
			correctTime = slot.taken_at
				? new Intl.DateTimeFormat('en-GB', {
						hour: '2-digit',
						minute: '2-digit',
						hour12: false,
						timeZone: slot.scheduled_tz
					}).format(new Date(slot.taken_at))
				: nowLocalHHMM();
			correctNote = slot.note ?? '';
		}
	}

	function closeDialog() {
		dialog = null;
		target = null;
	}

	onMount(load);
</script>

<svelte:head>
	<title>{i18n.t('nav.hari_ini')} — Pantau Obat</title>
</svelte:head>

<header class="mb-5">
	<p class="text-xs font-medium uppercase tracking-wide text-muted-foreground">{today}</p>
	<h1 class="mt-1 text-2xl font-semibold tracking-tight">
		{i18n.t('today.hello')}{session.user?.full_name ? `, ${session.user.full_name.split(' ')[0]}` : ''}
	</h1>
	{#if !loading && !error && slots.length > 0}
		<p class="mt-1 text-sm text-muted-foreground">{summary.text}</p>
	{/if}
</header>

<!-- Ringkasan hari ini. Menampilkan rasio dengan aturan yang SAMA seperti
     server (lihat `dose-logic.ts`), dan membedakan "belum ada data" dari
     "0%" — dua keadaan yang dulu tampak sama. -->
{#if !loading && !error && slots.length > 0}
	<Card class="mb-4">
		<CardContent class="py-4">
			<div class="grid grid-cols-3 gap-3 text-center">
				<div>
					<p class="text-lg font-semibold tabular-nums">{summary.done}/{summary.total}</p>
					<p class="text-[11px] text-muted-foreground">{i18n.t('today.recorded')}</p>
				</div>
				<div>
					<p class="text-lg font-semibold tabular-nums">{summary.waiting}</p>
					<p class="text-[11px] text-muted-foreground">{i18n.t('today.waiting')}</p>
				</div>
				<div>
					<p class="text-lg font-semibold tabular-nums">
						{summary.ratio === null ? '—' : `${Math.round(summary.ratio * 100)}%`}
					</p>
					<p class="text-[11px] text-muted-foreground">{i18n.t('today.adherence')}</p>
				</div>
			</div>
			{#if summary.ratio === null}
				<p class="mt-2 text-center text-[11px] text-muted-foreground">
					{i18n.t('today.no_data_today')}
				</p>
			{/if}
		</CardContent>
	</Card>
{/if}

{#if notice}
	<div class="mb-4 rounded-lg border border-ok/40 bg-ok/10 px-4 py-3 text-sm" role="status">
		{notice}
	</div>
{/if}

{#if actionError && !dialog}
	<div class="mb-4 rounded-lg border border-destructive/30 bg-destructive/8 px-4 py-3 text-sm text-destructive" role="alert">
		{actionError}
	</div>
{/if}

{#if error}
	<div class="rounded-lg border border-destructive/30 bg-destructive/8 px-4 py-3 text-sm text-destructive" role="alert">
		{error}
	</div>
{:else if loading}
	<div class="space-y-3">
		{#each [1, 2, 3] as n (n)}
			<Skeleton class="h-24 w-full" />
		{/each}
	</div>
{:else if slots.length === 0 && prnMeds.length === 0}
	<Card>
		<CardContent class="flex flex-col items-center gap-2 py-10 text-center">
			<span class="grid size-11 place-items-center rounded-full bg-muted text-muted-foreground" aria-hidden="true">
				<svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
					<path d="M8 2v4M16 2v4M3 10h18M5 4h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2Z" />
				</svg>
			</span>
			<p class="text-sm font-medium">{i18n.t('today.no_doses_title')}</p>
			<p class="max-w-xs text-xs text-muted-foreground">
				{i18n.t('today.no_doses_hint')}
			</p>
			<Button size="sm" variant="outline" href="/obat" class="mt-2">{i18n.t('meds.manage_medications')}</Button>
		</CardContent>
	</Card>
{:else}
	<ul class="space-y-3">
		{#each ordered as slot (slot.schedule_id + slot.slot_date)}
			{@const needAction = slot.status === 'due' || slot.status === 'pending'}
			{@const snoozed = slot.status === 'snoozed'}
			{@const recorded = slot.status === 'taken' || slot.status === 'late'}
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
								{#if slot.stock_remaining !== null}
									<p class="mt-1 text-xs {slot.stock_remaining <= 5 ? 'text-warn' : 'text-muted-foreground'}">
										Sisa stok: {slot.stock_remaining} {slot.stock_unit_code ?? ''}
									</p>
								{/if}
								{#if slot.note}
									<p class="mt-1 text-xs text-muted-foreground">{slot.note}</p>
								{/if}
							</div>
							<Badge variant={statusVariant[slot.status] ?? 'outline'}>
								{statusText(slot.status)}
							</Badge>
						</div>

						<!-- Aksi menyesuaikan status, seperti yang didefinisikan prototipe. -->
						<div class="mt-3.5 flex flex-wrap gap-2">
							{#if needAction}
								<Button
									size="sm"
									class="flex-1"
									disabled={busy === slot.schedule_id}
									onclick={() => record(slot, 'taken')}
								>
									{busy === slot.schedule_id ? i18n.t('common.save') + '…' : i18n.t('today.taken_2')}
								</Button>
								<Button
									size="sm"
									variant="outline"
									disabled={busy === slot.schedule_id}
									onclick={() => openDialog('snooze', slot)}
								>
									{i18n.t('today.snooze')}
								</Button>
								<Button
									size="sm"
									variant="outline"
									disabled={busy === slot.schedule_id}
									onclick={() => openDialog('skip', slot)}
								>
									{i18n.t('today.skip')}
								</Button>
							{:else if snoozed}
								<Button
									size="sm"
									class="flex-1"
									disabled={busy === slot.schedule_id}
									onclick={() => record(slot, 'taken')}
								>
									{busy === slot.schedule_id ? i18n.t('common.save') + '…' : i18n.t('today.taken_2')}
								</Button>
								<Button
									size="sm"
									variant="outline"
									disabled={busy === slot.schedule_id}
									onclick={() => openDialog('snooze', slot)}
								>
									{i18n.t('today.snooze')}
								</Button>
							{:else if recorded}
								<Button
									size="sm"
									variant="outline"
									disabled={busy === slot.schedule_id}
									onclick={() => openDialog('correct', slot)}
								>
									{i18n.t('today.correct')}
								</Button>
							{:else if slot.status === 'skipped'}
								<Button
									size="sm"
									variant="outline"
									disabled={busy === slot.schedule_id}
									onclick={() => record(slot, 'taken')}
								>
									{i18n.t('today.taken_2')}
								</Button>
							{/if}
						</div>
					</CardContent>
				</Card>
			</li>
		{/each}
	</ul>

	<!-- Seksi PRN: obat "bila perlu" tidak punya jadwal, jadi tidak akan
	     pernah muncul di daftar di atas. Tanpa seksi ini, dosis PRN tidak
	     bisa dicatat sama sekali dari aplikasi. -->
	{#if prnMeds.length > 0}
		<Card class="mt-5">
			<CardContent class="space-y-3 py-5">
				<div>
					<p class="text-sm font-medium">{i18n.t('today.as_needed_prn')}</p>
					<p class="mt-0.5 text-xs text-muted-foreground">
						{i18n.t('today.prn_unscheduled_hint')}
					</p>
				</div>
				<Button size="sm" variant="outline" onclick={() => (dialog = 'prn')}>
					{i18n.t('today.log_as_needed_medication')}
				</Button>
			</CardContent>
		</Card>
	{/if}
{/if}

<!-- ============ Dialog Tunda ============ -->
<Dialog open={dialog === 'snooze'} onopenchange={(v) => { if (!v) closeDialog(); }} title={i18n.t('today.snooze_reminder')} onclose={closeDialog}>
	<div class="space-y-4">
		<p class="text-sm text-muted-foreground">
			{target?.medication_name} · {target?.time_local}
		</p>
		<div class="space-y-2">
			<Label>{i18n.t('today.how_long')}</Label>
			<div class="flex gap-2">
				{#each SNOOZE_OPTIONS as m (m)}
					<button
						type="button"
						class="flex-1 rounded-md border px-3 py-2 text-sm font-medium transition-colors {snoozeMinutes === m
							? 'border-primary bg-primary text-primary-foreground'
							: 'border-border hover:bg-accent'}"
						aria-pressed={snoozeMinutes === m}
						onclick={() => (snoozeMinutes = m)}
					>
						{m} {i18n.t('today.minutes')}
					</button>
				{/each}
			</div>
		</div>
		<!-- Naskah ini MENYEBUT batas tiga kali tunda, dan itu baru benar
		     sekarang: sebelumnya kalimatnya sengaja dihilangkan karena
		     batasnya hanya didefinisikan di kode tanpa pernah diperiksa,
		     sehingga mengumumkannya akan menjadi janji kosong. Server kini
		     menegakkannya, jadi pengguna justru perlu tahu lebih dulu —
		     penolakan pada percobaan keempat akan membingungkan bila
		     aturannya baru muncul saat itu. -->
		<div class="rounded-lg border border-border bg-muted/40 px-3.5 py-3 text-xs text-muted-foreground">
			{i18n.t('today.snooze_note')}
		</div>
		{#if actionError}
			<p class="rounded-md border border-destructive/30 bg-destructive/8 px-3 py-2 text-sm text-destructive" role="alert">
				{actionError}
			</p>
		{/if}
	</div>
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={closeDialog}>{i18n.t('common.cancel')}</Button>
			<Button onclick={snooze} disabled={busy !== null}>
				{busy ? i18n.t('common.save') + '…' : i18n.t('today.snooze')}
			</Button>
		</div>
	{/snippet}
</Dialog>

<!-- ============ Dialog Lewati ============ -->
<Dialog open={dialog === 'skip'} onopenchange={(v) => { if (!v) closeDialog(); }} title={i18n.t('today.skip_this_dose')} onclose={closeDialog}>
	<div class="space-y-4">
		<p class="text-sm text-muted-foreground">
			{target?.medication_name} · {target?.time_local}
		</p>
		<div class="space-y-2">
			<Label for="skip-reason">{i18n.t('today.reason_optional')}</Label>
			<input
				id="skip-reason"
				class="h-9 w-full rounded-md border border-input bg-background px-3 text-sm shadow-xs"
				bind:value={skipReason}
				placeholder={i18n.t('today.reason_example')}
			/>
		</div>
		<p class="text-xs text-muted-foreground">
			{i18n.t('today.skip_note')}
		</p>
		{#if actionError}
			<p class="rounded-md border border-destructive/30 bg-destructive/8 px-3 py-2 text-sm text-destructive" role="alert">
				{actionError}
			</p>
		{/if}
	</div>
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={closeDialog}>{i18n.t('common.cancel')}</Button>
			<Button variant="destructive" onclick={skip} disabled={busy !== null}>
				{busy ? i18n.t('common.save') + '…' : i18n.t('today.skip_dose')}
			</Button>
		</div>
	{/snippet}
</Dialog>

<!-- ============ Dialog Koreksi ============ -->
<Dialog open={dialog === 'correct'} onopenchange={(v) => { if (!v) closeDialog(); }} title={i18n.t('today.correct_note')} onclose={closeDialog}>
	<div class="space-y-4">
		<p class="text-sm text-muted-foreground">
			{target?.medication_name} · {target?.time_local}
		</p>
		<p class="rounded-lg border border-border bg-muted/40 px-3.5 py-3 text-xs text-muted-foreground">
			{i18n.t('today.correct_note_body')}
		</p>
		<div class="space-y-2">
			<Label>{i18n.t('today.correct_status')}</Label>
			<div class="flex gap-2">
				{#each [['taken', 'Diminum'], ['late', 'Terlambat'], ['skipped', 'Dilewati']] as [val, label] (val)}
					<button
						type="button"
						class="flex-1 rounded-md border px-3 py-2 text-sm font-medium transition-colors {correctStatus === val
							? 'border-primary bg-primary text-primary-foreground'
							: 'border-border hover:bg-accent'}"
						aria-pressed={correctStatus === val}
						onclick={() => (correctStatus = val as 'taken' | 'late' | 'skipped')}
					>
						{label}
					</button>
				{/each}
			</div>
		</div>
		<div class="space-y-2">
			<Label for="correct-time">{i18n.t('today.taken_at_time')}</Label>
			<input
				id="correct-time"
				type="time"
				class="h-9 w-full rounded-md border border-input bg-background px-3 text-sm shadow-xs"
				bind:value={correctTime}
			/>
		</div>
		<div class="space-y-2">
			<Label for="correct-note">{i18n.t('today.additional_note')}</Label>
			<input
				id="correct-note"
				class="h-9 w-full rounded-md border border-input bg-background px-3 text-sm shadow-xs"
				bind:value={correctNote}
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
			<Button variant="outline" onclick={closeDialog}>{i18n.t('common.cancel')}</Button>
			<Button onclick={correct} disabled={busy !== null}>
				{busy ? i18n.t('common.save') + '…' : i18n.t('today.save_correction')}
			</Button>
		</div>
	{/snippet}
</Dialog>

<!-- ============ Dialog PRN ============ -->
<Dialog open={dialog === 'prn'} onopenchange={(v) => { if (!v) closeDialog(); }} title={i18n.t('today.log_as_needed_medication')} onclose={closeDialog}>
	<div class="space-y-4">
		<div class="space-y-2">
			<Label for="prn-med">{i18n.t('today.medication')}</Label>
			<select
				id="prn-med"
				class="h-9 w-full rounded-md border border-input bg-background px-3 text-sm shadow-xs"
				bind:value={prnMedId}
			>
				<option value="">Pilih obat…</option>
				{#each prnMeds as m (m.id)}
					<option value={m.id}>{m.name}</option>
				{/each}
			</select>
		</div>
		<div class="space-y-2">
			<Label for="prn-note">{i18n.t('today.reason_use')} (opsional)</Label>
			<input
				id="prn-note"
				class="h-9 w-full rounded-md border border-input bg-background px-3 text-sm shadow-xs"
				bind:value={prnNote}
				placeholder={i18n.t('today.prn_reason_example')}
			/>
		</div>
		<p class="rounded-lg border border-border bg-muted/40 px-3.5 py-3 text-xs text-muted-foreground">
			{i18n.t('today.prn_hint')}
		</p>
		{#if actionError}
			<p class="rounded-md border border-destructive/30 bg-destructive/8 px-3 py-2 text-sm text-destructive" role="alert">
				{actionError}
			</p>
		{/if}
	</div>
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={closeDialog}>{i18n.t('common.cancel')}</Button>
			<Button onclick={recordPrn} disabled={busy !== null || !prnMedId}>
				{busy ? i18n.t('common.save') + '…' : i18n.t('today.log')}
			</Button>
		</div>
	{/snippet}
</Dialog>
