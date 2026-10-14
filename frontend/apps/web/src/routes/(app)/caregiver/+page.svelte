<script lang="ts">
	/**
	 * Halaman caregiver: undang pendamping, kelola relasi, lihat undangan.
	 *
	 * Ada dua peran di halaman ini dan keduanya ditampilkan TERPISAH:
	 *
	 * - **{i18n.t('caregiver.section_mine')}** — saya pemilik data; saya mengatur siapa yang boleh
	 *   melihat atau mencatat.
	 * - **{i18n.t('caregiver.section_invites')}** — saya pendamping; saya menunggu/berperan.
	 *
	 * Menggabungkannya dalam satu daftar membingungkan: "menunggu" pada baris
	 * yang saya kirim berarti menunggu ORANG LAIN bertindak, sedangkan pada
	 * baris yang saya terima berarti menunggu SAYA.
	 *
	 * Menerima undangan memerlukan TOKEN dari tautan email, dan token itu tidak
	 * pernah dikirim di daftar (memang rahasia). Karena itu halaman ini tidak
	 * menyediakan tombol "Terima" palsu — ia mengarahkan ke tautan di email.
	 */
	import { onMount } from 'svelte';
	import {
		ApiError,
		type CareLink,
		type CareRelation,
		type CareRole
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

	let links = $state<CareLink[]>([]);
	/** Relasi tempat SAYA pendamping (dari `?as=caregiver`). */
	let asCaregiver = $state<CareLink[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let actionError = $state<string | null>(null);
	let busy = $state<string | null>(null);

	let inviteOpen = $state(false);
	let inviteEmail = $state('');
	let inviteRelation = $state<CareRelation>('family');
	let inviteRole = $state<CareRole>('viewer');
	/** Tautan undangan terakhir; hanya tersedia sekali dari backend. */
	let lastInviteUrl = $state<string | null>(null);
	let copied = $state(false);

	const relationLabel: Record<string, string> = {
		family: 'Keluarga',
		child: 'Anak',
		parent: 'Orang tua',
		spouse: 'Pasangan',
		friend: 'Teman',
		nurse: 'Perawat',
		other: 'Lainnya'
	};

	const roleLabel: Record<string, string> = {
		viewer: 'Hanya melihat',
		helper: 'Membantu mencatat',
		manager: 'Mengelola obat'
	};

	const roleHint: Record<string, string> = {
		viewer: 'Bisa melihat jadwal dan riwayat. Tidak bisa mengubah apa pun.',
		helper: 'Bisa mencatat dosis atas nama Anda. Tidak bisa mengubah obat atau jadwal.',
		manager: 'Bisa mencatat dosis sekaligus mengelola obat dan stok.'
	};

	const statusLabel: Record<string, string> = {
		pending: 'Menunggu',
		accepted: 'Aktif',
		rejected: 'Ditolak',
		revoked: 'Dicabut',
		expired: 'Kedaluwarsa'
	};

	const statusVariant: Record<string, 'ok' | 'warn' | 'na' | 'info'> = {
		pending: 'info',
		accepted: 'ok',
		rejected: 'na',
		revoked: 'na',
		expired: 'na'
	};

	/**
	 * Relasi tempat SAYA pemilik data.
	 *
	 * `GET /care-links` mengembalikan SATU sisi saja, dipilih lewat `?as=`.
	 * Tanpa parameter itu, yang dikembalikan adalah "siapa yang memantau saya".
	 * Sisi sebaliknya ("siapa yang saya pantau") harus diminta terpisah dengan
	 * `?as=caregiver`. Keduanya diperlukan halaman ini, jadi keduanya diambil.
	 */
	let sent = $derived(links.filter((l) => l.owner_id === session.user?.id));

	/** Relasi tempat SAYA pendamping. */
	let received = $derived(asCaregiver);

	async function load() {
		try {
			const [ownerSide, caregiverSide] = await Promise.all([
				api.care.list(),
				api.care.listAsCaregiver()
			]);
			links = ownerSide;
			asCaregiver = caregiverSide;
			error = null;
		} catch (err) {
			error = err instanceof ApiError ? err.message : 'Gagal memuat data caregiver.';
		} finally {
			loading = false;
		}
	}

	async function sendInvite() {
		if (busy) return;
		busy = 'invite';
		actionError = null;
		try {
			const res = await api.care.invite({
				invite_email: inviteEmail.trim(),
				relation: inviteRelation,
				role: inviteRole
			});
			// Tautan undangan hanya ditampilkan SEKALI — backend tidak
			// menyimpan token dalam bentuk yang bisa diambil ulang. Jadi
			// tautannya ditampilkan sekarang, saat masih ada.
			lastInviteUrl = (res as unknown as { invite_url?: string }).invite_url ?? null;
			inviteOpen = false;
			inviteEmail = '';
			await load();
		} catch (err) {
			actionError = err instanceof ApiError ? err.message : 'Gagal mengirim undangan.';
		} finally {
			busy = null;
		}
	}

	async function revoke(link: CareLink) {
		if (busy) return;
		busy = link.id;
		actionError = null;
		try {
			await api.care.revoke(link.id);
			await load();
		} catch (err) {
			actionError = err instanceof ApiError ? err.message : 'Gagal mencabut akses.';
		} finally {
			busy = null;
		}
	}

	async function copyInviteUrl() {
		if (!lastInviteUrl) return;
		try {
			await navigator.clipboard.writeText(lastInviteUrl);
			copied = true;
			setTimeout(() => (copied = false), 2500);
		} catch {
			// Clipboard bisa ditolak (izin/konteks). Tautannya tetap terlihat
			// dan bisa disalin manual, jadi ini bukan kegagalan fatal.
			copied = false;
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>Caregiver — Pantau Obat</title>
</svelte:head>

<div class="mb-5 flex items-center justify-between gap-3">
	<div>
		<h1 class="text-2xl font-semibold tracking-tight">{i18n.t('caregiver.page_title')}</h1>
		<p class="mt-1 text-sm text-muted-foreground">{i18n.t('caregiver.page_sub')}</p>
	</div>
	<Button size="sm" onclick={() => (inviteOpen = true)}>{i18n.t('caregiver.invite')}</Button>
</div>

{#if actionError}
	<div class="mb-4 rounded-lg border border-destructive/30 bg-destructive/8 px-4 py-3 text-sm text-destructive" role="alert">
		{actionError}
	</div>
{/if}

<!-- Tautan undangan: hanya tersedia SEKALI, tepat setelah undangan dibuat. -->
{#if lastInviteUrl}
	<div class="mb-4 rounded-lg border border-info/40 bg-info/10 px-4 py-3">
		<p class="text-sm font-medium">{i18n.t('caregiver.invite_sent')}</p>
		<p class="mt-1 text-xs">
			Kirim tautan ini ke orang yang Anda undang. Demi keamanan, tautan hanya ditampilkan
			sekali dan tidak bisa dilihat lagi setelah Anda meninggalkan halaman ini.
		</p>
		<p class="mt-2 break-all rounded-md border border-border bg-background px-2.5 py-2 font-mono text-[11px]">
			{lastInviteUrl}
		</p>
		<div class="mt-2.5 flex gap-2">
			<Button size="sm" variant="outline" onclick={copyInviteUrl}>
				{copied ? i18n.t('caregiver.copied') : i18n.t('caregiver.copy_link')}
			</Button>
			<Button size="sm" variant="ghost" onclick={() => (lastInviteUrl = null)}>{i18n.t('common.close')}</Button>
		</div>
	</div>
{/if}

{#if error}
	<div class="rounded-lg border border-destructive/30 bg-destructive/8 px-4 py-3 text-sm text-destructive" role="alert">
		{error}
	</div>
{:else if loading}
	<div class="space-y-3">
		{#each [1, 2] as n (n)}
			<Skeleton class="h-20 w-full" />
		{/each}
	</div>
{:else}
	<!-- ============ Undangan / relasi tempat SAYA pendamping ============ -->
	<!--
		Seksi ini SELALU tampil, termasuk saat kosong. Sebelumnya ia hanya
		mucul bila ada isinya, sehingga pengguna baru tidak pernah tahu bahwa
		orang lain bisa mengundangnya — dan tidak ada tempat untuk menjelaskan
		bagaimana undangan itu sampai.

		Catatan penting soal isi: undangan yang masih `pending` TIDAK muncul
		di sini. Kolom `caregiver_id` baru diisi saat undangan diterima atau
		ditolak, sedangkan endpoint `?as=caregiver` menyaring berdasarkan
		kolom itu. Karena itu tidak ada tombol "Terima" di halaman ini — token
		undangan hanya ada di tautan email, dan memang rahasia.
	-->
	<section class="mb-6">
		<h2 class="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
			{i18n.t('caregiver.section_invites')}
		</h2>
		{#if received.length === 0}
			<Card>
				<CardContent class="flex flex-col items-center gap-2 py-10 text-center">
					<p class="text-sm font-medium">{i18n.t('caregiver.no_watch_title')}</p>
					<p class="max-w-xs text-xs text-muted-foreground">
						Anda bisa memantau seseorang setelah mereka mengundang email Anda. Undangan
						datang lewat email berisi tautan khusus — buka tautan itu untuk menerima.
					</p>
				</CardContent>
			</Card>
		{:else}
			<ul class="space-y-3">
				{#each received as link (link.id)}
					<li>
						<!-- Kartu pasien bisa DIKLIK menuju halaman detailnya.
						     Sebelumnya daftar ini tidak bisa diklik sama sekali,
						     sehingga caregiver hanya bisa melihat nama tanpa
						     bisa melakukan pekerjaannya. -->
						<a href="/caregiver/{link.owner_id}" class="block transition-opacity hover:opacity-90">
							<Card>
								<CardContent class="py-4">
									<div class="flex items-start justify-between gap-3">
										<div class="min-w-0">
											<p class="truncate text-sm font-medium">
												{relationLabel[link.relation] ?? link.relation}
											</p>
											<p class="mt-0.5 text-xs text-muted-foreground">
												{roleLabel[link.role] ?? link.role}
											</p>
										</div>
										<Badge variant={statusVariant[link.status] ?? 'outline'}>
											{statusLabel[link.status] ?? link.status}
										</Badge>
									</div>
									{#if link.status === 'accepted'}
										<p class="mt-3 text-xs font-medium text-primary">
											{i18n.t('caregiver.open_patient')}
										</p>
									{/if}
								</CardContent>
							</Card>
						</a>
					</li>
				{/each}
			</ul>
		{/if}
	</section>

	<!-- ============ Caregiver yang saya undang ============ -->
	<section>
		<h2 class="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
			{i18n.t('caregiver.section_mine')}
		</h2>
		{#if sent.length === 0}
			<Card>
				<CardContent class="flex flex-col items-center gap-2 py-10 text-center">
					<p class="text-sm font-medium">{i18n.t('caregiver.no_caregiver_title')}</p>
					<p class="max-w-xs text-xs text-muted-foreground">
						Undang keluarga atau pendamping agar mereka bisa ikut memantau, tanpa perlu menyerahkan
						kata sandi Anda.
					</p>
					<Button size="sm" class="mt-2" onclick={() => (inviteOpen = true)}>{i18n.t('caregiver.invite_caregiver')}</Button>
				</CardContent>
			</Card>
		{:else}
			<ul class="space-y-3">
				{#each sent as link (link.id)}
					<li>
						<Card>
							<CardContent class="py-4">
								<div class="flex items-start justify-between gap-3">
									<div class="min-w-0">
										<p class="truncate text-sm font-medium">
											{link.invite_email ?? i18n.t('caregiver.registered_user')}
										</p>
										<p class="mt-0.5 text-xs text-muted-foreground">
											{relationLabel[link.relation] ?? link.relation} · {roleLabel[link.role] ?? link.role}
										</p>
									</div>
									<Badge variant={statusVariant[link.status] ?? 'outline'}>
										{statusLabel[link.status] ?? link.status}
									</Badge>
								</div>
								{#if link.status !== 'revoked' && link.status !== 'expired'}
									<div class="mt-3 flex justify-end">
										<Button
											size="sm"
											variant="outline"
											disabled={busy === link.id}
											onclick={() => revoke(link)}
										>
											{busy === link.id ? i18n.t('caregiver.processing') : i18n.t('caregiver.revoke')}
										</Button>
									</div>
								{/if}
							</CardContent>
						</Card>
					</li>
				{/each}
			</ul>
		{/if}
	</section>
{/if}

<!-- ============ Dialog: undang ============ -->
<Dialog bind:open={inviteOpen} title={i18n.t('caregiver.invite_title')}>
	<div class="space-y-4">
		<div class="space-y-2">
			<Label for="i-email">Email</Label>
			<Input id="i-email" type="email" bind:value={inviteEmail} placeholder="nama@contoh.com" />
			<p class="text-xs text-muted-foreground">
				{i18n.t('caregiver.invite_hint')}
			</p>
		</div>

		<div class="space-y-2">
			<Label for="i-rel">{i18n.t('caregiver.relation')}</Label>
			<Select
				id="i-rel"
				bind:value={inviteRelation}
				options={[
					{ value: 'family', label: 'Keluarga' },
					{ value: 'child', label: 'Anak' },
					{ value: 'parent', label: 'Orang tua' },
					{ value: 'spouse', label: 'Pasangan' },
					{ value: 'friend', label: 'Teman' },
					{ value: 'nurse', label: 'Perawat' },
					{ value: 'other', label: 'Lainnya' }
				]}
			/>
		</div>

		<div class="space-y-2">
			<Label for="i-role">{i18n.t('caregiver.role')}</Label>
			<Select
				id="i-role"
				bind:value={inviteRole}
				options={[
					{ value: 'viewer', label: 'Hanya melihat' },
					{ value: 'helper', label: 'Membantu mencatat' },
					{ value: 'manager', label: 'Mengelola obat' }
				]}
			/>
			<p class="text-xs text-muted-foreground">{roleHint[inviteRole]}</p>
		</div>
	</div>

	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="ghost" onclick={() => (inviteOpen = false)}>Batal</Button>
			<Button onclick={sendInvite} disabled={busy === 'invite' || !inviteEmail.trim()}>
				{busy === 'invite' ? i18n.t('caregiver.sending') : i18n.t('caregiver.send_invite')}
			</Button>
		</div>
	{/snippet}
</Dialog>
