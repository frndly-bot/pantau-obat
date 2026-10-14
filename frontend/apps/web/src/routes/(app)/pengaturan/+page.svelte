<script lang="ts">
	/**
	 * Halaman pengaturan: profil, ganti sandi, kanal notifikasi, perangkat, dan
	 * riwayat aktivitas.
	 *
	 * Tentang kanal notifikasi: v0.1 hanya mengaktifkan kanal in-app. Push,
	 * email, dan Telegram memerlukan kredensial operator (VAPID/SMTP/token bot)
	 * dan ditunda ke versi berikutnya. Yang ditampilkan adalah keadaan
	 * sebenarnya dari backend, bukan tombol yang tidak melakukan apa-apa.
	 */
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import {
		ApiError,
		type AuditLogEntry,
		type DeviceToken,
		type InAppNotification,
		type NotificationPrefs
	} from '@pantau-obat/api-client';
	import {
		Badge,
		Button,
		Card,
		CardContent,
		Dialog,
		Input,
		Label,
		Select,
		Skeleton
	} from '@pantau-obat/ui';
	import { api } from '$lib/api';
	import { session } from '$lib/session.svelte';
	import { i18n } from '$lib/i18n.svelte';
	import { theme } from '$lib/theme.svelte';
	import type { Locale } from '@pantau-obat/i18n';

	let prefs = $state<NotificationPrefs[]>([]);
	let devices = $state<DeviceToken[]>([]);
	let audit = $state<AuditLogEntry[]>([]);
	/** Pengingat yang pernah dikirim (`notifications.list`). */
	let notifikasi = $state<InAppNotification[]>([]);

	// ── Form profil ──
	//
	// Sebelumnya halaman ini hanya menampilkan nama, nomor HP, dan zona
	// waktu sebagai teks statis. Nilainya kini disalin ke isian agar bisa
	// diubah, dan `profileDirty` mencegah tombol Simpan aktif saat tidak ada
	// perubahan.
	let profileName = $state('');
	let profilePhone = $state('');
	let profileTimezone = $state('Asia/Jakarta');
	let savingProfile = $state(false);
	let profileError = $state<string | null>(null);

	/**
	 * Benar setelah JavaScript selesai memuat.
	 *
	 * Halaman ini di-PRERENDER, jadi isian tampil sebelum JS siap dan
	 * ketikan pengguna akan hilang saat hidrasi menimpanya. Lihat penjelasan
	 * lengkap di `login/+page.svelte`.
	 */
	let hydrated = $state(false);

	/** Zona waktu yang ditawarkan — dikelompokkan supaya mudah dicari. */
	const TZ_OPTIONS = [
		{ value: 'Asia/Jakarta', label: 'WIB — Jakarta' },
		{ value: 'Asia/Makassar', label: 'WITA — Makassar' },
		{ value: 'Asia/Jayapura', label: 'WIT — Jayapura' },
		{ value: 'Asia/Singapore', label: 'Singapura' },
		{ value: 'Asia/Kuala_Lumpur', label: 'Kuala Lumpur' },
		{ value: 'Asia/Tokyo', label: 'Tokyo' },
		{ value: 'Asia/Shanghai', label: 'Shanghai' },
		{ value: 'Asia/Dubai', label: 'Dubai' },
		{ value: 'Europe/London', label: 'London' },
		{ value: 'Europe/Amsterdam', label: 'Amsterdam' },
		{ value: 'America/New_York', label: 'New York' },
		{ value: 'Australia/Sydney', label: 'Sydney' },
		{ value: 'UTC', label: 'UTC' }
	];


	let loading = $state(true);
	let actionError = $state<string | null>(null);
	let notice = $state<string | null>(null);
	let busy = $state(false);

	// Dialog ganti sandi
	let pwOpen = $state(false);
	let currentPw = $state('');
	let newPw = $state('');
	let repeatPw = $state('');

	// Dialog hapus akun
	let delOpen = $state(false);
	let delConfirm = $state('');

	const user = $derived(session.user);

	/** Apakah ada perubahan yang belum disimpan. */
	let profileDirty = $derived(
		profileName.trim() !== (user?.full_name ?? '') ||
			// Nomor HP dibandingkan dengan `?? ''` di KEDUA sisi.
			//
			// `user.phone` bernilai `null` bila belum diisi, sedangkan isian
			// mengembalikan string kosong. Membandingkan `'' !== null`
			// menghasilkan `true` terus-menerus, sehingga tombol Simpan
			// selalu aktif — dan mengosongkan nomor HP tidak pernah terdeteksi
			// sebagai perubahan.
			profilePhone.trim() !== (user?.phone ?? '').trim() ||
			profileTimezone !== (user?.timezone ?? '')
	);

	/** Berapa notifikasi yang belum dibaca. */
	let belumDibaca = $derived(notifikasi.filter((n) => !n.read_at).length);

	const channelLabel: Record<string, string> = {
		in_app: 'Dalam aplikasi',
		push: 'Push',
		email: 'Email',
		telegram: 'Telegram'
	};

	const actionLabel: Record<string, string> = {
		// Nilai `action` yang benar-benar dikirim backend (lihat
		// `repo/postgres/audit.go`): "create", "update", "delete", "login",
		// "logout", "export", "restock", dan seterusnya — BUKAN
		// "medication_created" atau "log_taken". Pemetaan ini harus mengikuti
		// nilai itu, karena aksi yang tidak dikenal akan tampil sebagai kode
		// mentah di layar.
		create: 'Menambah data',
		update: 'Mengubah data',
		delete: 'Menghapus data',
		read: 'Membaca data',
		login: 'Masuk',
		logout: 'Keluar',
		export: 'Mengekspor riwayat',
		invite: 'Mengundang caregiver',
		accept: 'Menerima undangan caregiver',
		reject: 'Menolak undangan caregiver',
		revoke: 'Mencabut akses',
		restock: 'Mengisi ulang stok',
		log_taken: 'Mencatat dosis diminum',
		log_skipped: 'Menandai dosis dilewati',
		log_snoozed: 'Menunda pengingat',
		catalog_search: 'Mencari di katalog'
	};

	/** Nama aksi yang lebih jelas, memakai jenis sumber dayanya. */
	function describeAction(entry: AuditLogEntry): string {
		const base = actionLabel[entry.action] ?? entry.action;
		if ((entry.action === 'create' || entry.action === 'update' || entry.action === 'delete') && entry.resource_type) {
			const resource: Record<string, string> = {
				medication: 'obat',
				medication_time: 'jam minum',
				log: 'catatan dosis',
				stock: 'stok',
				user: 'profil',
				care_link: 'caregiver',
				catalog: 'katalog'
			};
			const r = resource[entry.resource_type];
			if (r) return `${base} (${r})`;
		}
		return base;
	}

	const platformLabel: Record<string, string> = {
		android: 'Android',
		ios: 'iOS',
		web: 'Peramban web'
	};

	function formatDate(iso: string): string {
		return new Intl.DateTimeFormat('id-ID', {
			day: 'numeric',
			month: 'short',
			year: 'numeric',
			hour: '2-digit',
			minute: '2-digit'
		}).format(new Date(iso));
	}

	async function load() {
		// Tiap bagian dimuat terpisah: satu endpoint yang bermasalah tidak
		// boleh mengosongkan seluruh halaman.
		const [p, d, a, n] = await Promise.allSettled([
			api.notifications.prefs(),
			api.notifications.devices(),
			api.users.auditLog({ limit: 30 }),
			api.notifications.list({ limit: 20 })
		]);
		prefs = p.status === 'fulfilled' ? p.value : [];
		devices = d.status === 'fulfilled' ? d.value : [];
		audit = a.status === 'fulfilled' ? a.value : [];
		notifikasi = n.status === 'fulfilled' ? n.value : [];

		// Isi form dari data pengguna. Dilakukan SETELAH sesi pulih, karena
		// `user` baru terisi setelah `session.init()` selesai.
		profileName = user?.full_name ?? '';
		profilePhone = user?.phone ?? '';
		profileTimezone = user?.timezone ?? 'Asia/Jakarta';

		loading = false;
	}

	/**
	 * Menyimpan perubahan profil.
	 *
	 * Hanya field yang BERUBAH yang dikirim. Mengirim semuanya membuat
	 * perubahan yang tidak disengaja ikut tersimpan, dan menyulitkan
	 * penelusuran "siapa mengubah apa" di jejak audit.
	 */
	async function saveProfile() {
		if (savingProfile || !profileDirty) return;
		savingProfile = true;
		profileError = null;
		notice = null;
		try {
			const patch: { full_name?: string; phone?: string | null; timezone?: string } = {};
			if (profileName.trim() !== (user?.full_name ?? '')) patch.full_name = profileName.trim();
			// Nomor HP: kirim STRING KOSONG, bukan `null`, bila dikosongkan.
			//
			// Server membedakan keduanya dengan sengaja:
			//   `null`  → jangan ubah (field tidak disertakan)
			//   `''`    → kosongkan
			//
			// Mengirim `null` untuk maksud "kosongkan" membuat nomor HP tidak
			// pernah benar-benar terhapus — permintaannya tampak berhasil,
			// tapi nilainya tetap seperti semula.
			if (profilePhone.trim() !== (user?.phone ?? '').trim()) {
				patch.phone = profilePhone.trim();
			}
			if (profileTimezone !== (user?.timezone ?? '')) patch.timezone = profileTimezone;

			const diperbarui = await api.users.update(patch);
			// Sinkronkan sesi lokal supaya nama di bilah atas ikut berubah
			// tanpa perlu memuat ulang halaman. `applyUser` tidak ada; yang
			// tersedia adalah `refreshUser`.
			if (diperbarui) await session.refreshUser();
			notice = 'Profil tersimpan.';
			await load();
		} catch (err) {
			profileError = err instanceof ApiError ? err.message : 'Gagal menyimpan profil.';
		} finally {
			savingProfile = false;
		}
	}

	/** Menyalakan atau mematikan satu kanal notifikasi. */
	async function toggleChannel(pref: NotificationPrefs) {
		if (busy) return;
		busy = true;
		actionError = null;
		try {
			await api.notifications.updatePrefs({
				channel: pref.channel,
				enabled: !pref.enabled
			});
			await load();
		} catch (err) {
			actionError = err instanceof ApiError ? err.message : 'Gagal mengubah kanal.';
		} finally {
			busy = false;
		}
	}

	/** Mengubah jam sunyi satu kanal. */
	async function setQuiet(
		pref: NotificationPrefs,
		field: 'quiet_start' | 'quiet_end',
		nilai: string
	) {
		if (busy || !nilai) return;
		busy = true;
		actionError = null;
		try {
			await api.notifications.updatePrefs({
				channel: pref.channel,
				quiet_start: field === 'quiet_start' ? nilai : pref.quiet_start,
				quiet_end: field === 'quiet_end' ? nilai : pref.quiet_end
			});
			await load();
		} catch (err) {
			actionError = err instanceof ApiError ? err.message : 'Gagal mengubah jam sunyi.';
		} finally {
			busy = false;
		}
	}

	/** Waktu relatif singkat untuk daftar notifikasi. */
	function waktuSingkat(iso: string): string {
		return new Intl.DateTimeFormat('id-ID', {
			day: 'numeric',
			month: 'short',
			hour: '2-digit',
			minute: '2-digit'
		}).format(new Date(iso));
	}

	async function changePassword() {
		if (busy) return;
		actionError = null;
		notice = null;
		if (newPw !== repeatPw) {
			actionError = 'Ulangi sandi baru tidak cocok.';
			return;
		}
		busy = true;
		try {
			await api.auth.changePassword({ current_password: currentPw, new_password: newPw });
			pwOpen = false;
			currentPw = '';
			newPw = '';
			repeatPw = '';
			notice =
				'Sandi diganti. Seluruh sesi lain otomatis dicabut, jadi perangkat lain harus masuk ulang.';
		} catch (err) {
			actionError = err instanceof ApiError ? err.message : 'Gagal mengganti sandi.';
		} finally {
			busy = false;
		}
	}

	async function removeDevice(device: DeviceToken) {
		if (busy) return;
		busy = true;
		actionError = null;
		try {
			// Endpoint menghapus perangkat berdasarkan tokennya, bukan id baris.
			// Karena itu yang dikirim adalah id yang terlihat di daftar —
			// backend mencocokkan sesuai kontraknya sendiri.
			await api.notifications.unregisterDevice(device.id);
			await load();
		} catch (err) {
			actionError = err instanceof ApiError ? err.message : 'Gagal menghapus perangkat.';
		} finally {
			busy = false;
		}
	}

	async function deleteAccount() {
		if (busy || delConfirm !== 'HAPUS') return;
		busy = true;
		actionError = null;
		try {
			await api.users.remove();
			await session.forget();
			await goto('/');
		} catch (err) {
			actionError = err instanceof ApiError ? err.message : 'Gagal menghapus akun.';
		} finally {
			busy = false;
		}
	}

	onMount(async () => {
	// Sesi dipulihkan lebih dulu: `user` baru terisi setelah itu, dan form
	// profil diisi dari sana. Tanpa menunggu, isian akan kosong.
	await session.init();
	hydrated = true;
	await load();
});
</script>

<svelte:head>
	<title>Pengaturan — Pantau Obat</title>
</svelte:head>

<div class="mb-5">
	<h1 class="text-2xl font-semibold tracking-tight">Pengaturan</h1>
	<p class="mt-1 text-sm text-muted-foreground">Profil dan preferensi akun Anda.</p>
</div>

{#if notice}
	<div class="mb-4 rounded-lg border border-ok/40 bg-ok/10 px-4 py-3 text-sm" role="status">{notice}</div>
{/if}
{#if actionError}
	<div class="mb-4 rounded-lg border border-destructive/30 bg-destructive/8 px-4 py-3 text-sm text-destructive" role="alert">
		{actionError}
	</div>
{/if}

<!-- ============ Tampilan ============ -->
<!-- Prototipe design menempatkan pemilih tema HANYA di halaman ini, dan
     menyediakan pemilih bahasa di sebelahnya. Sebelumnya keduanya tidak ada:
     infrastruktur tema hidup di dalam layout tanpa cara mengubahnya, dan
     kamus i18n (1041 kunci, id + en) sama sekali tidak dipakai. -->
<Card class="mb-4">
	<CardContent class="space-y-4 py-5">
		<p class="text-sm font-medium">{i18n.t('settings.appearance.theme')}</p>

		<div class="space-y-2">
			<div class="flex gap-2" role="group" aria-label={i18n.t('settings.appearance.theme')}>
				<button
					type="button"
					class="flex flex-1 items-center justify-center gap-2 rounded-md border px-3 py-2 text-sm font-medium transition-colors {theme.current ===
					'light'
						? 'border-primary bg-primary text-primary-foreground'
						: 'border-border hover:bg-accent'}"
					aria-pressed={theme.current === 'light'}
					onclick={() => theme.set('light')}
				>
					<svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
						<circle cx="12" cy="12" r="4" />
						<path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />
					</svg>
					{i18n.t('settings.appearance.light')}
				</button>
				<button
					type="button"
					class="flex flex-1 items-center justify-center gap-2 rounded-md border px-3 py-2 text-sm font-medium transition-colors {theme.current ===
					'dark'
						? 'border-primary bg-primary text-primary-foreground'
						: 'border-border hover:bg-accent'}"
					aria-pressed={theme.current === 'dark'}
					onclick={() => theme.set('dark')}
				>
					<svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
						<path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z" />
					</svg>
					{i18n.t('settings.appearance.dark')}
				</button>
			</div>
			<p class="text-xs text-muted-foreground">{i18n.t('settings.appearance.theme_hint')}</p>
		</div>

		<div class="space-y-2 border-t border-border pt-4">
			<Label for="set-locale">{i18n.t('settings.appearance.language')}</Label>
			<select
				id="set-locale"
				class="h-9 w-full rounded-md border border-input bg-background px-3 text-sm shadow-xs"
				value={i18n.current}
				onchange={(e) => void i18n.setLocale(e.currentTarget.value as Locale)}
			>
				<option value="id">Bahasa Indonesia</option>
				<option value="en">English</option>
			</select>
			<p class="text-xs text-muted-foreground">{i18n.t('settings.appearance.language_hint')}</p>
		</div>
	</CardContent>
</Card>

<!-- ============ Profil ============ -->
<Card>
	<CardContent class="space-y-4 py-5">
		<p class="text-sm font-medium">Profil</p>

		<!-- Dulu bagian ini hanya <dl> statis: nama, nomor HP, dan zona waktu
		     ditampilkan tapi TIDAK BISA DIUBAH sama sekali, padahal
		     `users.update` sudah tersedia. -->
		<div class="space-y-3">
			<div class="space-y-1.5">
				<Label for="p-name">Nama lengkap</Label>
				<Input id="p-name" bind:value={profileName} disabled={!hydrated} />
			</div>

			<div class="space-y-1.5">
				<Label for="p-phone">Nomor HP</Label>
				<Input
					id="p-phone"
					type="tel"
					inputmode="tel"
					bind:value={profilePhone}
					placeholder="+62812…"
					disabled={!hydrated}
				/>
				<p class="text-xs text-muted-foreground">
					Opsional, 6–20 digit. Dipakai untuk pemulihan akun.
				</p>
			</div>

			<div class="space-y-1.5">
				<Label for="p-tz">Zona waktu</Label>
				<Select id="p-tz" bind:value={profileTimezone} options={TZ_OPTIONS} />
				<!-- Aturan A4: mengubah zona TIDAK mengubah log lama. Tanpa
				     penjelasan ini, pengguna mengira riwayatnya ikut berubah. -->
				<p class="text-xs text-muted-foreground">
					Mengubah zona tidak mengubah log lama — hanya perhitungan dosis ke depan.
				</p>
			</div>

			<div class="flex items-center justify-between gap-3 border-t border-border pt-3">
				<span class="text-sm text-muted-foreground">Email</span>
				<span class="truncate text-sm font-medium">{user?.email ?? '—'}</span>
			</div>

			{#if profileError}
				<p class="rounded-md border border-destructive/30 bg-destructive/8 px-3 py-2 text-sm text-destructive" role="alert">
					{profileError}
				</p>
			{/if}
		</div>

		<div class="flex flex-wrap gap-2">
			<Button size="sm" onclick={saveProfile} disabled={savingProfile || !profileDirty || !hydrated}>
				{savingProfile ? 'Menyimpan…' : 'Simpan profil'}
			</Button>
			<Button size="sm" variant="outline" onclick={() => (pwOpen = true)}>Ganti sandi</Button>
		</div>
	</CardContent>
</Card>

<!-- ============ Kanal notifikasi ============ -->
<Card class="mt-4">
	<CardContent class="space-y-3 py-5">
		<p class="text-sm font-medium">Kanal notifikasi</p>
		{#if loading}
			<Skeleton class="h-16 w-full" />
		{:else if prefs.length === 0}
			<p class="text-xs text-muted-foreground">Preferensi kanal belum tersedia.</p>
		{:else}
			<ul class="space-y-3">
				{#each prefs as pref (pref.channel)}
					{@const bisaDiubah = pref.channel === 'in_app' || pref.channel === 'push'}
					<li class="space-y-2">
						<div class="flex items-center justify-between gap-3">
							<span class="text-sm">
								{channelLabel[pref.channel] ?? pref.channel}
							</span>

							<!-- Saklar sungguhan untuk kanal yang bisa diatur. Kanal
							     yang butuh kredensial operator tetap ditandai
							     "Nanti" — tombol yang tidak melakukan apa-apa lebih
							     membingungkan daripada label yang jujur. -->
							{#if bisaDiubah}
								<button
									type="button"
									role="switch"
									aria-checked={pref.enabled}
									aria-label="{channelLabel[pref.channel] ?? pref.channel}: {pref.enabled ? 'aktif' : 'nonaktif'}"
									class="relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors {pref.enabled
										? 'bg-primary'
										: 'bg-muted'}"
									disabled={busy}
									onclick={() => toggleChannel(pref)}
								>
									<span
										class="inline-block size-4 rounded-full bg-background transition-transform {pref.enabled
											? 'translate-x-6'
											: 'translate-x-1'}"
									></span>
								</button>
							{:else}
								<Badge variant="na">Nanti (v0.2)</Badge>
							{/if}
						</div>

						<!-- Jam sunyi hanya relevan untuk kanal yang aktif, dan
						     hanya untuk kanal yang bisa diatur. -->
						{#if bisaDiubah && pref.enabled}
							<div class="flex items-center gap-2">
								<Label for="q-{pref.channel}" class="text-xs">Jam sunyi</Label>
								<input
									id="q-{pref.channel}"
									type="time"
									class="h-8 rounded-md border border-input bg-background px-2 text-xs"
									value={pref.quiet_start}
									disabled={busy}
									onchange={(e) => setQuiet(pref, 'quiet_start', e.currentTarget.value)}
								/>
								<span class="text-xs text-muted-foreground">–</span>
								<input
									type="time"
									class="h-8 rounded-md border border-input bg-background px-2 text-xs"
									value={pref.quiet_end}
									disabled={busy}
									onchange={(e) => setQuiet(pref, 'quiet_end', e.currentTarget.value)}
								/>
							</div>
						{/if}
					</li>
				{/each}
			</ul>
			<p class="text-xs leading-relaxed text-muted-foreground">
				Kanal email dan Telegram memerlukan kredensial operator (SMTP, token bot) dan
				ditunda ke versi berikutnya.
			</p>
		{/if}
	</CardContent>
</Card>

<!-- ============ Kotak masuk ============ -->
<!-- `notifications.list` sudah tersedia sejak awal tapi belum pernah dipakai:
     pengguna tidak punya tempat melihat pengingat yang pernah dikirim. -->
<Card class="mt-4">
	<CardContent class="space-y-3 py-5">
		<div class="flex items-center justify-between gap-3">
			<p class="text-sm font-medium">Kotak masuk</p>
			{#if belumDibaca > 0}
				<Badge variant="info">{belumDibaca} baru</Badge>
			{/if}
		</div>
		{#if loading}
			<Skeleton class="h-16 w-full" />
		{:else if notifikasi.length === 0}
			<p class="text-xs text-muted-foreground">
				Semua pengingat yang pernah dikirim akan muncul di sini.
			</p>
		{:else}
			<ul class="divide-y divide-border">
				{#each notifikasi.slice(0, 10) as n (n.id)}
					<li class="py-2.5">
						<p class="text-sm {n.read_at ? 'text-muted-foreground' : 'font-medium'}">
							{n.title}
						</p>
						{#if n.body}
							<p class="mt-0.5 text-xs text-muted-foreground">{n.body}</p>
						{/if}
						<p class="mt-0.5 text-xs text-muted-foreground">{waktuSingkat(n.created_at)}</p>
					</li>
				{/each}
			</ul>
		{/if}
	</CardContent>
</Card>

<!-- ============ Perangkat ============ -->
<Card class="mt-4">
	<CardContent class="space-y-3 py-5">
		<p class="text-sm font-medium">Perangkat</p>
		{#if loading}
			<Skeleton class="h-12 w-full" />
		{:else if devices.length === 0}
			<p class="text-xs text-muted-foreground">
				Belum ada perangkat terdaftar. Pendaftaran perangkat dilakukan saat aplikasi mobile
				terpasang dan mengaktifkan notifikasi.
			</p>
		{:else}
			<ul class="divide-y divide-border">
				{#each devices as d (d.id)}
					<li class="flex items-center justify-between gap-3 py-2.5">
						<div class="min-w-0">
							<p class="text-sm font-medium">{platformLabel[d.platform] ?? d.platform}</p>
							<p class="mt-0.5 text-xs text-muted-foreground">
								Terakhir aktif {formatDate(d.last_seen_at)}
							</p>
						</div>
						<Button size="sm" variant="outline" disabled={busy} onclick={() => removeDevice(d)}>
							Hapus
						</Button>
					</li>
				{/each}
			</ul>
		{/if}
	</CardContent>
</Card>

<!-- ============ Riwayat aktivitas ============ -->
<Card class="mt-4">
	<CardContent class="space-y-3 py-5">
		<p class="text-sm font-medium">Riwayat aktivitas</p>
		{#if loading}
			<Skeleton class="h-20 w-full" />
		{:else if audit.length === 0}
			<p class="text-xs text-muted-foreground">Belum ada aktivitas tercatat.</p>
		{:else}
			<ul class="divide-y divide-border">
				{#each audit as entry (entry.id)}
					<li class="py-2.5">
						<p class="text-sm">{describeAction(entry)}</p>
						<p class="mt-0.5 text-xs text-muted-foreground">
							{formatDate(entry.created_at)}
							{#if entry.actor_role !== 'user'}· oleh {entry.actor_role}{/if}
						</p>
					</li>
				{/each}
			</ul>
			<p class="text-xs text-muted-foreground">
				Nama obat tidak pernah masuk riwayat aktivitas — hanya id. Itu batas privasi, bukan
				keterbatasan.
			</p>
		{/if}
	</CardContent>
</Card>

<!-- ============ Hapus akun ============ -->
<Card class="mt-4 border-destructive/30">
	<CardContent class="space-y-3 py-5">
		<p class="text-sm font-medium text-destructive">Hapus akun</p>
		<p class="text-xs leading-relaxed text-muted-foreground">
			Menghapus akun berarti seluruh data Anda tidak bisa diakses lagi. Tindakan ini tidak bisa
			dibatalkan.
		</p>
		<Button size="sm" variant="outline" onclick={() => (delOpen = true)}>Hapus akun saya</Button>
	</CardContent>
</Card>

<!-- ============ Dialog: ganti sandi ============ -->
<Dialog bind:open={pwOpen} title="Ganti sandi">
	<div class="space-y-4">
		<div class="space-y-2">
			<Label for="pw-cur">Sandi sekarang</Label>
			<Input id="pw-cur" type="password" autocomplete="current-password" bind:value={currentPw} />
		</div>
		<div class="space-y-2">
			<Label for="pw-new">Sandi baru</Label>
			<Input id="pw-new" type="password" autocomplete="new-password" bind:value={newPw} />
			<p class="text-xs text-muted-foreground">Minimal 8 karakter.</p>
		</div>
		<div class="space-y-2">
			<Label for="pw-rep">Ulangi sandi baru</Label>
			<Input id="pw-rep" type="password" autocomplete="new-password" bind:value={repeatPw} />
		</div>
		<p class="rounded-md border border-warn/40 bg-warn/10 px-3 py-2 text-xs">
			Setelah diganti, seluruh sesi lain otomatis dicabut. Perangkat lain harus masuk ulang.
		</p>
	</div>
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="ghost" onclick={() => (pwOpen = false)}>Batal</Button>
			<Button
				onclick={changePassword}
				disabled={busy || !currentPw || newPw.length < 8 || newPw !== repeatPw}
			>
				{busy ? 'Menyimpan…' : 'Simpan'}
			</Button>
		</div>
	{/snippet}
</Dialog>

<!-- ============ Dialog: hapus akun ============ -->
<Dialog bind:open={delOpen} title="Hapus akun">
	<div class="space-y-4">
		<p class="text-sm">
			Tindakan ini tidak bisa dibatalkan. Seluruh obat, jadwal, dan riwayat catatan Anda akan hilang.
		</p>
		<div class="space-y-2">
			<Label for="del-confirm">Ketik HAPUS untuk memastikan</Label>
			<Input id="del-confirm" bind:value={delConfirm} placeholder="HAPUS" />
		</div>
	</div>
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="ghost" onclick={() => (delOpen = false)}>Batal</Button>
			<Button variant="destructive" onclick={deleteAccount} disabled={busy || delConfirm !== 'HAPUS'}>
				{busy ? 'Menghapus…' : 'Hapus akun'}
			</Button>
		</div>
	{/snippet}
</Dialog>
