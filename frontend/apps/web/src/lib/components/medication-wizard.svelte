<script lang="ts">
	/**
	 * Wizard tambah obat — 5 langkah sesuai desain (`design/user/obat.html`):
	 * Sumber obat · Detail obat · Pola minum · Jam & hari · Stok & simpan.
	 *
	 * Aturan validasi ada di `$lib/medication-draft.ts` (teruji vitest), dan
	 * meniru validasi backend supaya kesalahan ketahuan sebelum dikirim —
	 * bukan setelah menunggu balasan 400.
	 *
	 * Pilihan satuan/rute/bentuk sediaan diambil dari `GET /refs`, BUKAN
	 * ditulis di klien: kolom-kolom itu punya foreign key, jadi kode yang tidak
	 * ada di database akan berakhir sebagai 500.
	 */
	import { ApiError, type CatalogSuggestion, type Refs, type TimeLabel } from '@pantau-obat/api-client';
	import {
		Badge,
		Button,
		Dialog,
		Input,
		Label,
		Select,
		Textarea
	} from '@pantau-obat/ui';
	import { api } from '$lib/api';
	import {
		blankDraft,
		bitmaskToDays,
		daysToBitmask,
		EVERY_DAY,
		patternHasTimes,
		syncTimes,
		TIME_LABELS,
		toPayload,
		validateDraft,
		WEEKDAYS,
		type MedicationDraft
	} from '$lib/medication-draft';

	type Props = {
		open?: boolean;
		timezone: string;
		today: string;
		/** Dipanggil setelah obat berhasil disimpan. */
		onsaved?: () => void;
	};

	let { open = $bindable(false), timezone, today, onsaved }: Props = $props();

	const STEP_TITLES = ['Sumber obat', 'Detail obat', 'Pola minum', 'Jam & hari', 'Stok & simpan'];

	let step = $state(0);
	// Draf TIDAK diisi dari props di sini: nilai props pada inisialisasi hanya
	// menangkap keadaan awal, sedangkan tanggal "hari ini" bisa berubah bila
	// aplikasi dibiarkan terbuka melewati tengah malam. Pengisian dilakukan
	// saat wizard dibuka (lihat $effect di bawah).
	let draft = $state<MedicationDraft>(blankDraft());
	let refs = $state<Refs | null>(null);
	let saving = $state(false);
	let serverError = $state<string | null>(null);
	let fieldIssues = $state<Record<string, string>>({});

	// Pencarian katalog (langkah 1).
	let searchQuery = $state('');
	let suggestions = $state<CatalogSuggestion[]>([]);
	let searching = $state(false);
	let searchError = $state<string | null>(null);

	const validation = $derived(validateDraft(draft));
	let stepIssues = $derived(validation.byStep[step]);

	const patternLabels: Record<string, { title: string; hint: string }> = {
		daily: { title: 'Sekian kali sehari', hint: '1–24 kali per hari, jam diisi satu per satu' },
		interval: { title: 'Tiap sekian jam', hint: 'Mis. tiap 8 jam mulai 06:00' },
		weekly: { title: 'Hari tertentu', hint: 'Mis. Senin dan Kamis saja' },
		monthly_cycle: { title: 'Siklus minum–jeda', hint: 'Mis. 21 hari minum lalu 7 hari jeda' },
		prn: { title: 'Bila perlu (PRN)', hint: 'Tanpa jadwal tetap; dicatat saat dipakai' }
	};

	const labelNames: Record<string, string> = {
		morning: 'Pagi',
		afternoon: 'Siang',
		evening: 'Malam',
		dawn: 'Subuh',
		other: 'Lainnya'
	};

	const unitOptions = $derived((refs?.units ?? []).map((u) => ({ value: u.code, label: u.name })));
	const routeOptions = $derived((refs?.routes ?? []).map((r) => ({ value: r.code, label: r.name })));
	const formOptions = $derived(
		(refs?.dosage_forms ?? []).map((f) => ({ value: f.code, label: f.name }))
	);

	// Saat wizard dibuka: isi tanggal mulai dari nilai terkini dan muat tabel
	// referensi sekali. Tanggal diambil di sini, bukan saat komponen dipasang,
	// supaya aplikasi yang dibiarkan terbuka melewati tengah malam tetap
	// memakai tanggal yang benar.
	$effect(() => {
		if (!open) return;
		draft.start_date = today;
		if (!refs) {
			void api.refs
				.all()
				.then((r) => (refs = r))
				.catch(() => (refs = { units: [], routes: [], dosage_forms: [] }));
		}
	});

	/** Saat pola berubah: sesuaikan jam agar draf tidak dalam keadaan pasti ditolak. */
	function choosePattern(pattern: MedicationDraft['pattern']) {
		draft.pattern = pattern;
		syncTimes(draft);
	}

	function toggleDay(key: string) {
		const days = bitmaskToDays(draft.days_of_week);
		const next = days.includes(key) ? days.filter((d) => d !== key) : [...days, key];
		draft.days_of_week = daysToBitmask(next);
	}

	function addTime() {
		draft.times.push({ time_local: '12:00', label: 'other' });
	}

	function removeTime(index: number) {
		draft.times.splice(index, 1);
	}

	async function searchCatalog() {
		const q = searchQuery.trim();
		if (q.length < 2) {
			suggestions = [];
			return;
		}
		searching = true;
		searchError = null;
		try {
			suggestions = await api.catalog.suggest({ q, limit: 8 });
		} catch (err) {
			searchError = err instanceof ApiError ? err.message : 'Pencarian katalog gagal.';
		} finally {
			searching = false;
		}
	}

	/** Mengisi draf dari saran katalog; pengguna tetap bisa mengubahnya. */
	function applySuggestion(s: CatalogSuggestion) {
		draft.name = s.name;
		if (s.generic_name) draft.notes = s.generic_name;
		if (s.form_code) draft.dosage_form_code = s.form_code;
		if (s.route_code) draft.route_code = s.route_code;
		if (s.strength_note) draft.strength_note = s.strength_note;
		suggestions = [];
		searchQuery = '';
	}

	async function save() {
		serverError = null;
		fieldIssues = {};
		if (validation.all.length > 0) {
			// Arahkan ke langkah pertama yang bermasalah, jangan biarkan
			// pengguna menekan Simpan berulang tanpa tahu apa yang kurang.
			const firstBad = validation.byStep.findIndex((issues) => issues.length > 0);
			if (firstBad >= 0) step = firstBad;
			return;
		}
		saving = true;
		try {
			await api.medications.create(toPayload(draft, timezone) as never);
			open = false;
			draft = blankDraft(today, timezone);
			step = 0;
			onsaved?.();
		} catch (err) {
			if (err instanceof ApiError) {
				serverError = err.message;
				for (const d of err.details ?? []) fieldIssues[d.field] = d.issue;
			} else {
				serverError = 'Tidak dapat menghubungi server. Coba lagi.';
			}
		} finally {
			saving = false;
		}
	}

	function close() {
		open = false;
		step = 0;
		draft = blankDraft(today, timezone);
		serverError = null;
	}
</script>

<Dialog bind:open title="Tambah obat" onclose={close}>
	<!-- Penunjuk langkah -->
	<div class="mb-5 flex items-center gap-1.5">
		{#each STEP_TITLES as title, i (title)}
			<button
				type="button"
				class="flex-1 text-left"
				aria-label="Langkah {i + 1}: {title}"
				aria-current={i === step ? 'step' : undefined}
				onclick={() => (step = i)}
			>
				<span
					class="block h-1 rounded-full transition-colors duration-[var(--dur)] {i === step
						? 'bg-primary'
						: i < step
							? 'bg-primary/50'
							: 'bg-border'}"
				></span>
			</button>
		{/each}
	</div>
	<p class="mb-4 text-xs text-muted-foreground">
		Langkah {step + 1} dari {STEP_TITLES.length} · <span class="text-foreground">{STEP_TITLES[step]}</span>
	</p>

	{#if stepIssues.length > 0}
		<div class="mb-4 rounded-md border border-warn/40 bg-warn/10 px-3 py-2">
			<ul class="space-y-0.5 text-xs text-foreground">
				{#each stepIssues as issue (issue)}
					<li>• {issue}</li>
				{/each}
			</ul>
		</div>
	{/if}

	{#if serverError}
		<div class="mb-4 rounded-md border border-destructive/30 bg-destructive/8 px-3 py-2 text-xs text-destructive" role="alert">
			{serverError}
		</div>
	{/if}

	<!-- ============ Langkah 1: Sumber obat ============ -->
	{#if step === 0}
		<div class="space-y-4">
			<div class="space-y-2">
				<Label for="cat-search">Cari di katalog</Label>
				<div class="flex gap-2">
					<Input
						id="cat-search"
						bind:value={searchQuery}
						placeholder="Nama dagang, generik, ATC…"
						onkeydown={(e) => e.key === 'Enter' && searchCatalog()}
					/>
					<Button type="button" variant="outline" onclick={searchCatalog} disabled={searching}>
						{searching ? '…' : 'Cari'}
					</Button>
				</div>
				<p class="text-xs text-muted-foreground">
					Preset publik dari KFA/BPOM. Boleh dilewati dan isi manual.
				</p>
			</div>

			{#if searchError}
				<p class="text-xs text-destructive">{searchError}</p>
			{/if}

			{#if suggestions.length > 0}
				<ul class="divide-y divide-border rounded-md border border-border">
					{#each suggestions as s (s.name + (s.strength_note ?? ''))}
						<li>
							<button
								type="button"
								class="w-full px-3 py-2.5 text-left transition-colors hover:bg-accent"
								onclick={() => applySuggestion(s)}
							>
								<span class="block text-sm font-medium">{s.name}</span>
								<span class="mt-0.5 block text-xs text-muted-foreground">
									{[s.generic_name, s.form_code, s.strength_note].filter(Boolean).join(' · ')}
								</span>
							</button>
						</li>
					{/each}
				</ul>
			{/if}

			<div class="space-y-2">
				<Label for="m-name">Nama obat</Label>
				<Input id="m-name" bind:value={draft.name} placeholder="Mis. Amlodipine 5mg" />
				{#if fieldIssues.name}
					<p class="text-xs text-destructive">{fieldIssues.name}</p>
				{/if}
			</div>
		</div>

		<!-- ============ Langkah 2: Detail obat ============ -->
	{:else if step === 1}
		<div class="space-y-4">
			<div class="grid grid-cols-2 gap-3">
				<div class="space-y-2">
					<Label for="m-dose">Jumlah per dosis</Label>
					<Input id="m-dose" type="number" min="0.01" step="0.01" bind:value={draft.dose_value} />
				</div>
				<div class="space-y-2">
					<Label for="m-unit">Satuan dosis</Label>
					<Select id="m-unit" bind:value={draft.dose_unit_code} options={unitOptions} />
				</div>
			</div>

			<div class="space-y-2">
				<Label for="m-route">Rute</Label>
				<Select id="m-route" bind:value={draft.route_code} options={routeOptions} />
			</div>

			<div class="space-y-2">
				<Label for="m-form">Bentuk sediaan</Label>
				<Select id="m-form" bind:value={draft.dosage_form_code} options={formOptions} />
			</div>

			<div class="space-y-2">
				<Label for="m-meal">Waktu makan</Label>
				<Select
					id="m-meal"
					bind:value={draft.meal_relation}
					options={[
						{ value: 'before_meal', label: 'Sebelum makan' },
						{ value: 'after_meal', label: 'Sesudah makan' },
						{ value: 'with_meal', label: 'Bersama makan' },
						{ value: 'not_related', label: 'Tidak terkait makan' }
					]}
				/>
			</div>

			<div class="grid grid-cols-2 gap-3">
				<div class="space-y-2">
					<Label for="m-start">Mulai terapi</Label>
					<Input id="m-start" type="date" bind:value={draft.start_date} />
				</div>
				<div class="space-y-2">
					<Label for="m-end">Selesai (opsional)</Label>
					<Input id="m-end" type="date" bind:value={draft.end_date} />
				</div>
			</div>
			<p class="text-xs text-muted-foreground">Kosongkan tanggal selesai bila terapi berjalan terus.</p>

			<label class="flex items-start gap-2.5">
				<input type="checkbox" class="mt-0.5" bind:checked={draft.must_finish} />
				<span class="text-sm">
					Harus dihabiskan
					<span class="block text-xs text-muted-foreground">
						Untuk antibiotik dan sejenisnya. Tidak boleh dihentikan walau sudah membaik.
					</span>
				</span>
			</label>

			<div class="space-y-2">
				<Label for="m-notes">Catatan pribadi</Label>
				<Textarea id="m-notes" bind:value={draft.notes} placeholder="Opsional" />
				<p class="text-xs text-muted-foreground">Disimpan untuk diri sendiri; tidak terlihat caregiver.</p>
			</div>
		</div>

		<!-- ============ Langkah 3: Pola minum ============ -->
	{:else if step === 2}
		<div class="space-y-3">
			{#each Object.entries(patternLabels) as [key, info] (key)}
				{@const active = draft.pattern === key}
				<button
					type="button"
					class="w-full rounded-lg border px-3.5 py-3 text-left transition-colors duration-[var(--dur)] {active
						? 'border-primary bg-primary/8'
						: 'border-border hover:bg-accent'}"
					aria-pressed={active}
					onclick={() => choosePattern(key as MedicationDraft['pattern'])}
				>
					<span class="flex items-center justify-between gap-2">
						<span class="text-sm font-medium">{info.title}</span>
						{#if active}<Badge variant="ok">Dipilih</Badge>{/if}
					</span>
					<span class="mt-0.5 block text-xs text-muted-foreground">{info.hint}</span>
				</button>
			{/each}

			<!-- Parameter khusus tiap pola -->
			<div class="rounded-lg border border-border bg-muted/30 p-3.5">
				{#if draft.pattern === 'daily'}
					<div class="space-y-2">
						<Label for="p-tpd">Berapa kali sehari</Label>
						<Input id="p-tpd" type="number" min="1" max="24" bind:value={draft.times_per_day} />
						<p class="text-xs text-muted-foreground">
							Jumlah jam minum di langkah berikutnya akan disesuaikan otomatis.
						</p>
					</div>
				{:else if draft.pattern === 'interval'}
					<div class="space-y-2">
						<Label for="p-int">Interval (jam)</Label>
						<Input id="p-int" type="number" min="1" max="168" bind:value={draft.interval_hours} />
						<p class="text-xs text-muted-foreground">Mis. 8 untuk tiga kali sehari.</p>
					</div>
				{:else if draft.pattern === 'weekly'}
					<div class="space-y-2">
						<Label>Berlaku pada hari</Label>
						<div class="flex flex-wrap gap-1.5">
							{#each WEEKDAYS as day (day.key)}
								{@const on = (draft.days_of_week & day.bit) !== 0}
								<button
									type="button"
									class="min-w-11 rounded-md border px-2.5 py-1.5 text-xs font-medium transition-colors {on
										? 'border-primary bg-primary text-primary-foreground'
										: 'border-border hover:bg-accent'}"
									aria-pressed={on}
									onclick={() => toggleDay(day.key)}
								>
									{day.key === 'mon'
										? 'Sen'
										: day.key === 'tue'
											? 'Sel'
											: day.key === 'wed'
												? 'Rab'
												: day.key === 'thu'
													? 'Kam'
													: day.key === 'fri'
														? 'Jum'
														: day.key === 'sat'
															? 'Sab'
															: 'Min'}
								</button>
							{/each}
						</div>
						<Button
							type="button"
							variant="ghost"
							size="sm"
							onclick={() => (draft.days_of_week = draft.days_of_week === EVERY_DAY ? 0 : EVERY_DAY)}
						>
							{draft.days_of_week === EVERY_DAY ? 'Kosongkan' : 'Pilih semua'}
						</Button>
					</div>
				{:else if draft.pattern === 'monthly_cycle'}
					<div class="space-y-3">
						<div class="grid grid-cols-2 gap-3">
							<div class="space-y-2">
								<Label for="p-in">Hari minum</Label>
								<Input id="p-in" type="number" min="1" max="365" bind:value={draft.cycle_intake_days} />
							</div>
							<div class="space-y-2">
								<Label for="p-pa">Hari jeda</Label>
								<Input id="p-pa" type="number" min="0" max="365" bind:value={draft.cycle_pause_days} />
							</div>
						</div>
						<div class="space-y-2">
							<Label for="p-an">Titik mulai siklus</Label>
							<Select
								id="p-an"
								bind:value={draft.cycle_anchor}
								options={[
									{ value: 'from_start', label: 'Dari tanggal mulai' },
									{ value: 'dom', label: 'Tanggal tertentu (DOM)' },
									{ value: 'end_of_month', label: 'Akhir bulan' }
								]}
							/>
						</div>
						{#if draft.cycle_anchor === 'dom'}
							<div class="space-y-2">
								<Label for="p-dom">Tanggal</Label>
								<Input id="p-dom" type="number" min="1" max="31" bind:value={draft.cycle_anchor_dom} />
							</div>
						{/if}
						<p class="text-xs text-muted-foreground">
							Selama masa jeda tidak ada pengingat dan tidak dihitung terlewat.
						</p>
					</div>
				{:else if draft.pattern === 'prn'}
					<div class="space-y-2">
						<Label for="p-prn">Batas peringatan per hari</Label>
						<Input id="p-prn" type="number" min="1" max="24" bind:value={draft.prn_max_per_day} />
						<p class="text-xs text-muted-foreground">
							PRN tidak punya jam tetap. Dosisnya dicatat manual dari halaman Hari Ini.
						</p>
					</div>
				{/if}
			</div>
		</div>

		<!-- ============ Langkah 4: Jam & hari ============ -->
	{:else if step === 3}
		{#if patternHasTimes(draft.pattern)}
			<div class="space-y-3">
				<div class="flex items-center justify-between">
					<Label>Jam minum</Label>
					<Button type="button" variant="outline" size="sm" onclick={addTime}>Tambah jam</Button>
				</div>

				{#each draft.times as t, i (i)}
					<div class="flex items-end gap-2">
						<div class="flex-1 space-y-1.5">
							<Label for="t-{i}">Jam {i + 1}</Label>
							<Input id="t-{i}" type="time" bind:value={t.time_local} />
						</div>
						<div class="flex-1 space-y-1.5">
							<Label for="tl-{i}">Waktu</Label>
							<Select
								id="tl-{i}"
								value={t.label}
								onchange={(e) => (t.label = (e.currentTarget as HTMLSelectElement).value as TimeLabel)}
							>
								{#each TIME_LABELS as l (l)}
									<option value={l}>{labelNames[l]}</option>
								{/each}
							</Select>
						</div>
						<Button
							type="button"
							variant="ghost"
							size="icon"
							aria-label="Hapus jam {i + 1}"
							onclick={() => removeTime(i)}
						>
							<svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
								<path d="M3 6h18M8 6V4h8v2M19 6l-1 14H6L5 6" />
							</svg>
						</Button>
					</div>
				{/each}

				<p class="text-xs text-muted-foreground">Mengubah jam hanya berlaku untuk dosis ke depan.</p>
			</div>
		{:else}
			<div class="rounded-lg border border-border bg-muted/30 px-3.5 py-3">
				<p class="text-sm font-medium">PRN tidak punya jam tetap</p>
				<p class="mt-1 text-xs text-muted-foreground">
					Bila diperlukan, dosis dicatat manual dari halaman Hari Ini.
				</p>
			</div>
		{/if}

		<!-- ============ Langkah 5: Stok & simpan ============ -->
	{:else if step === 4}
		<div class="space-y-4">
			<label class="flex items-start gap-2.5">
				<input type="checkbox" class="mt-0.5" bind:checked={draft.track_stock} />
				<span class="text-sm">
					Lacak stok obat ini
					<span class="block text-xs text-muted-foreground">
						Setiap dosis yang dicatat akan memotong saldo.
					</span>
				</span>
			</label>

			{#if draft.track_stock}
				<div class="space-y-4 rounded-lg border border-border bg-muted/30 p-3.5">
					<div class="grid grid-cols-2 gap-3">
						<div class="space-y-2">
							<Label for="s-qty">Stok awal</Label>
							<Input id="s-qty" type="number" min="0" step="0.5" bind:value={draft.initial_qty} />
						</div>
						<div class="space-y-2">
							<Label for="s-unit">Satuan stok</Label>
							<Select id="s-unit" bind:value={draft.stock_unit_code} options={unitOptions} />
						</div>
					</div>
					<div class="grid grid-cols-2 gap-3">
						<div class="space-y-2">
							<Label for="s-per">Dipakai per dosis</Label>
							<Input id="s-per" type="number" min="0.01" step="0.01" bind:value={draft.qty_per_dose} />
						</div>
						<div class="space-y-2">
							<Label for="s-th">Peringatkan bila sisa ≤</Label>
							<Input id="s-th" type="number" min="0" bind:value={draft.refill_threshold_qty} />
						</div>
					</div>
					<label class="flex items-center gap-2.5">
						<input type="checkbox" bind:checked={draft.refill_reminder} />
						<span class="text-sm">Kirim pengingat restock</span>
					</label>
				</div>
			{/if}

			<!-- Ringkasan sebelum menyimpan -->
			<div class="rounded-lg border border-border p-3.5">
				<p class="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">Ringkasan</p>
				<dl class="space-y-1.5 text-sm">
					<div class="flex justify-between gap-3">
						<dt class="text-muted-foreground">Obat</dt>
						<dd class="truncate font-medium">{draft.name || '—'}</dd>
					</div>
					<div class="flex justify-between gap-3">
						<dt class="text-muted-foreground">Dosis</dt>
						<dd class="font-medium">{draft.dose_value} {draft.dose_unit_code}</dd>
					</div>
					<div class="flex justify-between gap-3">
						<dt class="text-muted-foreground">Pola</dt>
						<dd class="font-medium">{patternLabels[draft.pattern]?.title}</dd>
					</div>
					<div class="flex justify-between gap-3">
						<dt class="text-muted-foreground">Jam</dt>
						<dd class="font-medium">
							{draft.times.length > 0
								? draft.times.map((t) => t.time_local).join(', ')
								: 'tidak dijadwalkan'}
						</dd>
					</div>
					<div class="flex justify-between gap-3">
						<dt class="text-muted-foreground">Stok</dt>
						<dd class="font-medium">
							{draft.track_stock ? `${draft.initial_qty} ${draft.stock_unit_code}` : 'tidak dilacak'}
						</dd>
					</div>
				</dl>
			</div>
		</div>
	{/if}

	{#snippet footer()}
		<div class="flex items-center justify-between gap-2">
			<Button type="button" variant="ghost" onclick={() => (step > 0 ? step-- : close())}>
				{step === 0 ? 'Batal' : 'Kembali'}
			</Button>
			{#if step < STEP_TITLES.length - 1}
				<Button type="button" onclick={() => step++}>Lanjut</Button>
			{:else}
				<Button type="button" onclick={save} disabled={saving}>
					{saving ? 'Menyimpan…' : 'Simpan obat'}
				</Button>
			{/if}
		</div>
	{/snippet}
</Dialog>
