<script lang="ts">
	/**
	 * Detail obat: rincian, jam minum, dan stok.
	 *
	 * `GET /medications/{id}` mengembalikan frekuensi, jam, dan stok sekaligus
	 * (`includeDetail=true`), jadi halaman ini hanya perlu satu panggilan.
	 *
	 * Semua angka turunan (`doses_remaining`, `estimated_days_left`,
	 * `needs_refill`) datang dari server. Klien TIDAK menghitungnya sendiri:
	 * `estimated_days_left` memakai laju pemakaian nyata 30 hari terakhir, dan
	 * menghitung ulang di klien akan menghasilkan angka berbeda dari yang
	 * dilihat di tempat lain.
	 */
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import {
		ApiError,
		type Medication,
		type MedicationStock,
		type StockTransaction
	} from '@pantau-obat/api-client';
	import {
		Badge,
		Button,
		Card,
		CardContent,
		Dialog,
		Input,
		Label,
		Skeleton
	} from '@pantau-obat/ui';
	import { api } from '$lib/api';
	import { i18n } from '$lib/i18n.svelte';
	import MedicationEdit from '$lib/components/medication-edit.svelte';

	let med = $state<Medication | null>(null);
	let stock = $state<MedicationStock | null>(null);
	let txns = $state<StockTransaction[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let actionError = $state<string | null>(null);
	let busy = $state(false);

	// Dialog restock
	/** Dialog ubah data obat. Sebelumnya tidak ada sama sekali. */
	let editOpen = $state(false);
	/** Dialog hentikan / arsipkan obat. */
	let stopOpen = $state(false);
	let restockOpen = $state(false);
	let restockQty = $state(30);
	let restockNote = $state('');

	// Dialog koreksi saldo
	let adjustOpen = $state(false);
	let adjustQty = $state(0);
	let adjustNote = $state('');

	const medId = $derived(page.params.id ?? '');

	const mealLabel: Record<string, string> = {
		before_meal: 'Sebelum makan',
		after_meal: 'Setelah makan',
		with_meal: 'Bersama makan',
		not_related: 'Tidak terkait makan'
	};

	const patternLabel: Record<string, string> = {
		daily: 'Harian',
		interval: 'Tiap sekian jam',
		weekly: 'Hari tertentu',
		monthly_cycle: 'Siklus minum–jeda',
		prn: 'Bila perlu (PRN)'
	};

	const kindLabel: Record<string, string> = {
		initial: 'Saldo awal',
		restock: 'Isi ulang',
		consume: 'Dosis diminum',
		adjust: 'Koreksi',
		expire: 'Kedaluwarsa'
	};

	const dayName: Record<string, string> = {
		mon: 'Sen',
		tue: 'Sel',
		wed: 'Rab',
		thu: 'Kam',
		fri: 'Jum',
		sat: 'Sab',
		sun: 'Min'
	};

	/** Menguraikan bitmask hari menjadi label singkat. */
	function daysLabel(mask: number): string {
		const bits: Array<[string, number]> = [
			['mon', 1],
			['tue', 2],
			['wed', 4],
			['thu', 8],
			['fri', 16],
			['sat', 32],
			['sun', 64]
		];
		const on = bits.filter(([, bit]) => (mask & bit) !== 0).map(([k]) => dayName[k]);
		if (on.length === 7) return 'Setiap hari';
		return on.join(', ') || '—';
	}

	async function load() {
		try {
			med = await api.medications.get(medId);
			error = null;
			// Stok diambil terpisah: obat boleh tidak punya stok sama sekali.
			try {
				stock = await api.stock.get(medId);
				txns = await api.stock.transactions(medId, { limit: 20 });
			} catch (err) {
				// 404 = memang tidak melacak stok. Itu keadaan normal.
				if (!(err instanceof ApiError) || !err.isNotFound) throw err;
				stock = null;
				txns = [];
			}
		} catch (err) {
			error = err instanceof ApiError ? err.message : 'Gagal memuat detail obat.';
		} finally {
			loading = false;
		}
	}

	async function doRestock() {
		if (busy) return;
		busy = true;
		actionError = null;
		try {
			await api.stock.restock(medId, {
				qty: restockQty,
				note: restockNote.trim() || null
			});
			restockOpen = false;
			restockNote = '';
			await load();
		} catch (err) {
			actionError = err instanceof ApiError ? err.message : 'Gagal mengisi ulang stok.';
		} finally {
			busy = false;
		}
	}

	/**
	 * Menghentikan sementara atau mengarsipkan obat.
	 *
	 * Dua tindakan berbeda, dan bedanya penting bagi pengguna:
	 *
	 *   nonaktifkan (`is_active = false`) — obat berhenti muncul di jadwal,
	 *   tapi datanya utuh dan bisa diaktifkan lagi kapan saja.
	 *
	 *   arsipkan (`DELETE`) — soft delete. Riwayat TETAP ADA; yang berubah
	 *   hanya bahwa obat tidak lagi dianggap aktif. Server membalas
	 *   `logs_preserved: true` untuk menegaskan itu.
	 *
	 * Keduanya sengaja tidak menghapus riwayat: pada aplikasi kesehatan,
	 * menghapus catatan dosis berarti menghapus bukti kepatuhan yang mungkin
	 * dibutuhkan saat kontrol ke dokter.
	 */
	async function stopOrArchive(mode: 'deactivate' | 'archive') {
		if (busy || !med) return;
		busy = true;
		actionError = null;
		try {
			if (mode === 'archive') {
				await api.medications.remove(med.id);
				await goto('/obat');
				return;
			}
			await api.medications.update(med.id, { is_active: false });
			stopOpen = false;
			await load();
		} catch (err) {
			actionError = err instanceof ApiError ? err.message : 'Gagal mengubah status obat.';
		} finally {
			busy = false;
		}
	}

	/** Mengaktifkan kembali obat yang sebelumnya dihentikan. */
	async function reactivate() {
		if (busy || !med) return;
		busy = true;
		actionError = null;
		try {
			await api.medications.update(med.id, { is_active: true });
			await load();
		} catch (err) {
			actionError = err instanceof ApiError ? err.message : 'Gagal mengaktifkan obat.';
		} finally {
			busy = false;
		}
	}

	async function doAdjust() {
		if (busy) return;
		busy = true;
		actionError = null;
		try {
			// `reason` wajib menurut backend: koreksi saldo harus bisa
			// dipertanggungjawabkan, karena ia memutus hubungan antara saldo
			// dan riwayat transaksi.
			await api.stock.adjust(medId, { qty_delta: adjustQty, note: adjustNote.trim() });
			adjustOpen = false;
			adjustNote = '';
			adjustQty = 0;
			await load();
		} catch (err) {
			actionError = err instanceof ApiError ? err.message : 'Gagal menyesuaikan saldo.';
		} finally {
			busy = false;
		}
	}

	function formatDate(iso: string): string {
		return new Intl.DateTimeFormat('id-ID', {
			day: 'numeric',
			month: 'short',
			year: 'numeric',
			hour: '2-digit',
			minute: '2-digit'
		}).format(new Date(iso));
	}

	onMount(load);
</script>

<svelte:head>
	<title>{med?.name ?? 'Detail obat'} — Pantau Obat</title>
</svelte:head>

<div class="mb-4">
	<Button variant="ghost" size="sm" onclick={() => goto('/obat')}>← Obat</Button>
</div>

{#if error}
	<div class="rounded-lg border border-destructive/30 bg-destructive/8 px-4 py-3 text-sm text-destructive" role="alert">
		{error}
	</div>
{:else if loading}
	<div class="space-y-3">
		<Skeleton class="h-32 w-full" />
		<Skeleton class="h-40 w-full" />
	</div>
{:else if med}
	{#if actionError}
		<div class="mb-4 rounded-lg border border-destructive/30 bg-destructive/8 px-4 py-3 text-sm text-destructive" role="alert">
			{actionError}
		</div>
	{/if}

	<!-- ============ Rincian obat ============ -->
	<Card>
		<CardContent class="space-y-4 py-5">
			<div class="flex items-start justify-between gap-3">
				<div class="min-w-0">
					<h1 class="text-lg font-semibold tracking-tight">{med.name}</h1>
					<p class="mt-1 text-sm text-muted-foreground">
						{med.dose_value} {med.dose_unit_code} · {mealLabel[med.meal_relation] ?? med.meal_relation}
					</p>
				</div>
				<Badge variant={med.is_active ? 'ok' : 'na'}>
					{med.is_active ? i18n.t('common.active') : i18n.t('meds.archive')}
				</Badge>
			</div>

			<!-- Aksi atas obat. Sebelumnya halaman ini TIDAK punya cara apa pun
			     untuk mengubah data obat setelah dibuat: wizard hanya bisa
			     menambah. -->
			<div class="flex flex-wrap gap-2">
				<Button size="sm" variant="outline" onclick={() => (editOpen = true)}>
					{i18n.t('common.edit')}
				</Button>
				{#if med.is_active}
					<Button size="sm" variant="outline" onclick={() => (stopOpen = true)}>
						{i18n.t('medDetail.stop_archive')}
					</Button>
				{:else}
					<Button size="sm" variant="outline" onclick={reactivate} disabled={busy}>
						{i18n.t('medDetail.reactivate')}
					</Button>
				{/if}
			</div>

			<dl class="space-y-2 text-sm">
				<div class="flex justify-between gap-3">
					<dt class="text-muted-foreground">Pola</dt>
					<dd class="font-medium">
						{patternLabel[med.frequency?.pattern ?? ''] ?? med.frequency?.pattern ?? '—'}
					</dd>
				</div>
				{#if med.frequency?.pattern === 'interval' && med.frequency.interval_hours}
					<div class="flex justify-between gap-3">
						<dt class="text-muted-foreground">Interval</dt>
						<dd class="font-medium">tiap {med.frequency.interval_hours} jam</dd>
					</div>
				{/if}
				{#if med.frequency?.pattern === 'weekly' && med.frequency.days_of_week}
					<div class="flex justify-between gap-3">
						<dt class="text-muted-foreground">Hari</dt>
						<dd class="font-medium">{daysLabel(med.frequency.days_of_week)}</dd>
					</div>
				{/if}
				{#if med.frequency?.pattern === 'monthly_cycle'}
					<div class="flex justify-between gap-3">
						<dt class="text-muted-foreground">Siklus</dt>
						<dd class="font-medium">
							{med.frequency.cycle_intake_days} minum / {med.frequency.cycle_pause_days} jeda
						</dd>
					</div>
				{/if}
				{#if med.frequency?.pattern === 'prn' && med.frequency.prn_max_per_day}
					<div class="flex justify-between gap-3">
						<dt class="text-muted-foreground">Batas per hari</dt>
						<dd class="font-medium">maks {med.frequency.prn_max_per_day}×</dd>
					</div>
				{/if}
				<div class="flex justify-between gap-3">
					<dt class="text-muted-foreground">Mulai terapi</dt>
					<dd class="font-medium">{med.start_date}</dd>
				</div>
				<div class="flex justify-between gap-3">
					<dt class="text-muted-foreground">Selesai</dt>
					<dd class="font-medium">{med.end_date ?? 'tanpa batas'}</dd>
				</div>
				{#if med.must_finish}
					<div class="flex justify-between gap-3">
						<dt class="text-muted-foreground">Catatan</dt>
						<dd class="font-medium">Harus dihabiskan</dd>
					</div>
				{/if}
			</dl>

			{#if med.notes}
				<p class="rounded-md border border-border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
					{med.notes}
				</p>
			{/if}
		</CardContent>
	</Card>

	<!-- ============ Jam minum ============ -->
	<Card class="mt-4">
		<CardContent class="py-5">
			<p class="text-sm font-medium">Jam minum</p>
			{#if med.frequency?.pattern === 'prn'}
				<p class="mt-2 text-xs text-muted-foreground">
					PRN tidak punya jam tetap. Dosisnya dicatat manual dari halaman Hari Ini.
				</p>
			{:else if (med.times ?? []).length === 0}
				<p class="mt-2 text-xs text-muted-foreground">Belum ada jam minum.</p>
			{:else}
				<ul class="mt-3 flex flex-wrap gap-2">
					{#each med.times ?? [] as t (t.id)}
						<li>
							<Badge variant="outline" class="gap-1.5 py-1 tabular-nums">
								{t.time_local}
								{#if t.label}
									<span class="text-muted-foreground">· {t.label}</span>
								{/if}
							</Badge>
						</li>
					{/each}
				</ul>
			{/if}
		</CardContent>
	</Card>

	<!-- ============ Stok ============ -->
	<Card class="mt-4">
		<CardContent class="py-5">
			<div class="flex items-center justify-between gap-3">
				<p class="text-sm font-medium">Stok</p>
				{#if stock?.needs_refill}
					<Badge variant="warn">Perlu restock</Badge>
				{/if}
			</div>

			{#if !stock}
				<p class="mt-2 text-xs text-muted-foreground">
					Obat ini tidak melacak stok. Saldo hanya bisa diatur saat obat dibuat.
				</p>
			{:else}
				<div class="mt-3 grid grid-cols-2 gap-3">
					<div class="rounded-lg border border-border p-3">
						<p class="text-xs text-muted-foreground">Sisa</p>
						<p class="mt-0.5 text-lg font-semibold tabular-nums">
							{stock.current_qty} <span class="text-sm font-normal text-muted-foreground">{stock.unit_code}</span>
						</p>
					</div>
					<div class="rounded-lg border border-border p-3">
						<p class="text-xs text-muted-foreground">Perkiraan habis</p>
						<p class="mt-0.5 text-lg font-semibold tabular-nums">
							{#if stock.estimated_days_left === null}
								<span class="text-sm font-normal text-muted-foreground">belum cukup data</span>
							{:else}
								{Math.floor(stock.estimated_days_left)}
								<span class="text-sm font-normal text-muted-foreground">hari</span>
							{/if}
						</p>
					</div>
				</div>

				<p class="mt-2.5 text-xs text-muted-foreground">
					{stock.doses_remaining} dosis tersisa · dipakai {stock.qty_per_dose} {stock.unit_code} per dosis
					{#if stock.refill_threshold_qty !== null}
						· peringatan pada sisa {stock.refill_threshold_qty}
					{/if}
				</p>

				<div class="mt-4 flex gap-2">
					<Button size="sm" class="flex-1" onclick={() => (restockOpen = true)}>Isi ulang</Button>
					<Button size="sm" variant="outline" onclick={() => (adjustOpen = true)}>Koreksi saldo</Button>
				</div>
			{/if}
		</CardContent>
	</Card>

	<!-- ============ Riwayat transaksi ============ -->
	{#if stock && txns.length > 0}
		<Card class="mt-4">
			<CardContent class="py-5">
				<p class="text-sm font-medium">Riwayat stok</p>
				<ul class="mt-3 divide-y divide-border">
					{#each txns as t (t.id)}
						<li class="flex items-center justify-between gap-3 py-2.5">
							<div class="min-w-0">
								<p class="text-sm">{kindLabel[t.kind] ?? t.kind}</p>
								<p class="mt-0.5 text-xs text-muted-foreground">{formatDate(t.created_at)}</p>
								{#if t.note}
									<p class="mt-0.5 truncate text-xs text-muted-foreground">{t.note}</p>
								{/if}
							</div>
							<div class="text-right">
								<p class="text-sm font-medium tabular-nums {t.qty_delta < 0 ? 'text-warn' : 'text-ok'}">
									{t.qty_delta > 0 ? '+' : ''}{t.qty_delta}
								</p>
								<p class="text-xs text-muted-foreground tabular-nums">sisa {t.balance_after}</p>
							</div>
						</li>
					{/each}
				</ul>
			</CardContent>
		</Card>
	{/if}

	<!-- ============ Dialog: isi ulang ============ -->
	<Dialog bind:open={restockOpen} title="Isi ulang stok">
		<div class="space-y-4">
			<div class="space-y-2">
				<Label for="r-qty">Jumlah ditambahkan</Label>
				<Input id="r-qty" type="number" min="1" bind:value={restockQty} />
				<p class="text-xs text-muted-foreground">
					Dalam {stock?.unit_code ?? 'satuan'}. Saldo sekarang {stock?.current_qty ?? 0}.
				</p>
			</div>
			<div class="space-y-2">
				<Label for="r-note">Catatan (opsional)</Label>
				<Input id="r-note" bind:value={restockNote} placeholder="Mis. tebus resep di apotek" />
			</div>
		</div>
		{#snippet footer()}
			<div class="flex justify-end gap-2">
				<Button variant="ghost" onclick={() => (restockOpen = false)}>Batal</Button>
				<Button onclick={doRestock} disabled={busy}>{busy ? 'Menyimpan…' : 'Simpan'}</Button>
			</div>
		{/snippet}
	</Dialog>

	<!-- ============ Dialog: koreksi saldo ============ -->
	<Dialog bind:open={adjustOpen} title="Koreksi saldo">
		<div class="space-y-4">
			<div class="rounded-md border border-warn/40 bg-warn/10 px-3 py-2">
				<p class="text-xs">
					Koreksi memutus hubungan antara saldo dan riwayat transaksi, jadi alasannya wajib diisi
					dan ikut tercatat.
				</p>
			</div>
			<div class="space-y-2">
				<Label for="a-qty">Perubahan saldo</Label>
				<Input id="a-qty" type="number" bind:value={adjustQty} />
				<p class="text-xs text-muted-foreground">
					Negatif mengurangi (mis. tumpah 2), positif menambah. Saldo sekarang {stock?.current_qty ?? 0}.
				</p>
			</div>
			<div class="space-y-2">
				<Label for="a-note">Alasan</Label>
				<Input id="a-note" bind:value={adjustNote} placeholder="Mis. tablet pecah saat disiapkan" />
			</div>
		</div>
		{#snippet footer()}
			<div class="flex justify-end gap-2">
				<Button variant="ghost" onclick={() => (adjustOpen = false)}>Batal</Button>
				<Button onclick={doAdjust} disabled={busy || !adjustNote.trim()}>
					{busy ? 'Menyimpan…' : 'Simpan'}
				</Button>
			</div>
		{/snippet}
	</Dialog>
{/if}

<!-- ============ Dialog: ubah data obat ============ -->
<MedicationEdit
	bind:open={editOpen}
	{med}
	onsaved={load}
	onopenchange={(v) => (editOpen = v)}
/>

<!-- ============ Dialog: hentikan / arsipkan ============ -->
<Dialog open={stopOpen} title={i18n.t('medDetail.stop_remove_medication')} onopenchange={(v) => !v && (stopOpen = false)}>
	<div class="space-y-4">
		<p class="text-sm text-muted-foreground">
			{med?.name}
		</p>

		<!-- Dua pilihan dengan akibat berbeda, dan bedanya disebutkan apa
		     adanya. Pengguna harus tahu bahwa riwayatnya TIDAK hilang —
		     itu justru alasan keduanya aman dipakai. -->
		<div class="space-y-3">
			<button
				type="button"
				class="w-full rounded-lg border border-border px-3.5 py-3 text-left transition-colors hover:bg-accent"
				disabled={busy}
				onclick={() => stopOrArchive('deactivate')}
			>
				<p class="text-sm font-medium">{i18n.t('medDetail.pause_now_deactivate')}</p>
				<p class="mt-0.5 text-xs text-muted-foreground">
					Obat berhenti muncul di jadwal. Data dan riwayatnya tetap utuh, dan bisa
					diaktifkan lagi kapan saja.
				</p>
			</button>

			<button
				type="button"
				class="w-full rounded-lg border border-border px-3.5 py-3 text-left transition-colors hover:bg-accent"
				disabled={busy}
				onclick={() => stopOrArchive('archive')}
			>
				<p class="text-sm font-medium">{i18n.t('medDetail.archive_medication_soft_delete')}</p>
				<p class="mt-0.5 text-xs text-muted-foreground">
					Obat dikeluarkan dari daftar. <b>Riwayat dosis tidak dihapus</b> — catatan
					yang sudah ada tetap bisa dilihat di halaman Riwayat.
				</p>
			</button>
		</div>

		<div class="rounded-lg border border-ok/40 bg-ok/10 px-3.5 py-3 text-xs">
			<b>{i18n.t('medDetail.history_must_never_lost')}</b>
			<p class="mt-0.5 text-muted-foreground">
				Pada aplikasi kesehatan, catatan dosis adalah bukti kepatuhan yang mungkin
				dibutuhkan saat kontrol. Karena itu tidak ada cara menghapusnya dari sini.
			</p>
		</div>

		{#if actionError}
			<p class="rounded-md border border-destructive/30 bg-destructive/8 px-3 py-2 text-sm text-destructive" role="alert">
				{actionError}
			</p>
		{/if}
	</div>
	{#snippet footer()}
		<div class="flex justify-end">
			<Button variant="outline" onclick={() => (stopOpen = false)} disabled={busy}>
				{i18n.t('common.cancel')}
			</Button>
		</div>
	{/snippet}
</Dialog>
