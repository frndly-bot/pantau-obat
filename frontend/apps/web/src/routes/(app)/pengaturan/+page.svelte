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
		const [p, d, a] = await Promise.allSettled([
			api.notifications.prefs(),
			api.notifications.devices(),
			api.users.auditLog({ limit: 30 })
		]);
		prefs = p.status === 'fulfilled' ? p.value : [];
		devices = d.status === 'fulfilled' ? d.value : [];
		audit = a.status === 'fulfilled' ? a.value : [];
		loading = false;
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

	onMount(load);
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
		<dl class="space-y-2 text-sm">
			<div class="flex items-center justify-between gap-3">
				<dt class="text-muted-foreground">Nama</dt>
				<dd class="font-medium">{user?.full_name ?? '—'}</dd>
			</div>
			<div class="flex items-center justify-between gap-3">
				<dt class="text-muted-foreground">Email</dt>
				<dd class="truncate font-medium">{user?.email ?? '—'}</dd>
			</div>
			<div class="flex items-center justify-between gap-3">
				<dt class="text-muted-foreground">Zona waktu</dt>
				<dd class="font-medium">{user?.timezone ?? '—'}</dd>
			</div>
			<div class="flex items-center justify-between gap-3">
				<dt class="text-muted-foreground">Peran</dt>
				<dd>
					<Badge variant={user?.role === 'admin' ? 'private' : 'outline'}>
						{user?.role === 'admin' ? 'Admin' : 'Pengguna'}
					</Badge>
				</dd>
			</div>
		</dl>
		<Button size="sm" variant="outline" onclick={() => (pwOpen = true)}>Ganti sandi</Button>
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
			<ul class="space-y-2">
				{#each prefs as pref (pref.channel)}
					<li class="flex items-center justify-between gap-3">
						<span class="text-sm text-muted-foreground">
							{channelLabel[pref.channel] ?? pref.channel}
							{#if pref.quiet_start && pref.quiet_end}
								<span class="block text-xs">jam sunyi {pref.quiet_start}–{pref.quiet_end}</span>
							{/if}
						</span>
						<Badge variant={pref.enabled ? 'ok' : 'na'}>
							{pref.enabled ? 'Aktif' : 'Nanti'}
						</Badge>
					</li>
				{/each}
			</ul>
			<p class="text-xs leading-relaxed text-muted-foreground">
				Kanal selain dalam aplikasi memerlukan kredensial operator (VAPID, SMTP, token bot) dan
				ditunda ke versi berikutnya. Badge "Nanti" menandai kanal yang belum aktif.
			</p>
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
