<script lang="ts">
	/**
	 * Halaman "Hari Ini" — daftar dosis hari ini, termasuk yang BELUM dicatat.
	 *
	 * Sumber: `GET /schedule` (bukan `GET /logs/today`). Bedanya menentukan:
	 * `/logs/today` hanya mengembalikan dosis yang sudah dicatat, sehingga
	 * layar kosong sampai user mencatat sesuatu — justru saat pengingat paling
	 * dibutuhkan. `/schedule` menghitung slot dari aturan frekuensi, jadi dosis
	 * yang jatuh tempo selalu terlihat.
	 */
	import { onMount } from 'svelte';
	import { ApiError, type DaySlot } from '@pantau-obat/api-client';
	import { Badge, Button, Card, CardContent, Skeleton } from '@pantau-obat/ui';
	import { api } from '$lib/api';
	import { session } from '$lib/session.svelte';

	let slots = $state<DaySlot[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	/** schedule_id yang sedang diproses, supaya tombolnya tidak diklik dua kali. */
	let busy = $state<string | null>(null);
	let actionError = $state<string | null>(null);

	const statusLabel: Record<string, string> = {
		pending: 'Belum waktunya',
		due: 'Belum dicatat',
		taken: 'Diminum',
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

	const mealLabel: Record<string, string> = {
		before_meal: 'sebelum makan',
		after_meal: 'setelah makan',
		with_meal: 'bersama makan',
		not_related: 'bebas'
	};

	/** Dosis yang perlu ditindak lebih dulu: `due`, lalu `pending`, lalu sisanya. */
	let ordered = $derived(
		[...slots].sort((a, b) => {
			const rank = (s: DaySlot) => (s.status === 'due' ? 0 : s.status === 'pending' ? 1 : 2);
			const diff = rank(a) - rank(b);
			return diff !== 0 ? diff : a.time_local.localeCompare(b.time_local);
		})
	);

	let dueCount = $derived(slots.filter((s) => s.status === 'due').length);
	let doneCount = $derived(
		slots.filter((s) => ['taken', 'late', 'skipped'].includes(s.status)).length
	);

	let today = $derived(
		new Intl.DateTimeFormat('id-ID', {
			weekday: 'long',
			day: 'numeric',
			month: 'long',
			year: 'numeric'
		}).format(new Date())
	);

	async function load() {
		try {
			slots = await api.schedule.day();
			error = null;
		} catch (err) {
			error = err instanceof ApiError ? err.message : 'Gagal memuat jadwal hari ini.';
		} finally {
			loading = false;
		}
	}

	/** Mencatat satu dosis, lalu menyegarkan daftar dari server. */
	async function record(slot: DaySlot, status: 'taken' | 'skipped') {
		if (busy) return;
		busy = slot.schedule_id;
		actionError = null;
		try {
			await api.logs.create(slot.medication_id, {
				schedule_id: slot.schedule_id,
				slot_date: slot.slot_date,
				scheduled_hour: slot.scheduled_hour,
				scheduled_minute: slot.scheduled_minute,
				scheduled_tz: slot.scheduled_tz,
				status
			});
			await load();
		} catch (err) {
			actionError = err instanceof ApiError ? err.message : 'Gagal menyimpan catatan.';
		} finally {
			busy = null;
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>Hari Ini — Pantau Obat</title>
</svelte:head>

<header class="mb-5">
	<p class="text-xs font-medium uppercase tracking-wide text-muted-foreground">{today}</p>
	<h1 class="mt-1 text-2xl font-semibold tracking-tight">
		Halo{session.user?.full_name ? `, ${session.user.full_name.split(' ')[0]}` : ''}
	</h1>
	{#if !loading && !error && slots.length > 0}
		<p class="mt-1 text-sm text-muted-foreground">
			{#if dueCount > 0}<span class="font-medium text-foreground">{dueCount} dosis</span> menunggu dicatat ·{/if}
			{doneCount} dari {slots.length} selesai
		</p>
	{:else}
		<p class="mt-1 text-sm text-muted-foreground">Berikut jadwal dosis Anda hari ini.</p>
	{/if}
</header>

{#if actionError}
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
{:else if slots.length === 0}
	<Card>
		<CardContent class="flex flex-col items-center gap-2 py-10 text-center">
			<span class="grid size-11 place-items-center rounded-full bg-muted text-muted-foreground" aria-hidden="true">
				<svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
					<path d="M8 2v4M16 2v4M3 10h18M5 4h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2Z" />
				</svg>
			</span>
			<p class="text-sm font-medium">Tidak ada dosis terjadwal hari ini</p>
			<p class="max-w-xs text-xs text-muted-foreground">
				Tambahkan obat beserta jadwalnya untuk mulai memantau dosis harian.
			</p>
			<Button size="sm" variant="outline" href="/obat" class="mt-2">Kelola obat</Button>
		</CardContent>
	</Card>
{:else}
	<ul class="space-y-3">
		{#each ordered as slot (slot.schedule_id + slot.slot_date)}
			{@const actionable = slot.status === 'due' || slot.status === 'pending'}
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
							</div>
							<Badge variant={statusVariant[slot.status] ?? 'outline'}>
								{statusLabel[slot.status] ?? slot.status}
							</Badge>
						</div>

						{#if actionable}
							<div class="mt-3.5 flex gap-2">
								<Button
									size="sm"
									class="flex-1"
									disabled={busy === slot.schedule_id}
									onclick={() => record(slot, 'taken')}
								>
									{busy === slot.schedule_id ? 'Menyimpan…' : 'Diminum'}
								</Button>
								<Button
									size="sm"
									variant="outline"
									disabled={busy === slot.schedule_id}
									onclick={() => record(slot, 'skipped')}
								>
									Lewati
								</Button>
							</div>
						{/if}
					</CardContent>
				</Card>
			</li>
		{/each}
	</ul>
{/if}
