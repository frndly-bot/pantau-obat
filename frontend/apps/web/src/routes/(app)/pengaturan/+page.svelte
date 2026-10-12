<script lang="ts">
	/**
	 * Halaman pengaturan (kerangka Fase 3).
	 *
	 * Menampilkan profil sungguhan dari sesi. Ganti sandi, kanal notifikasi,
	 * perangkat, dan audit-log menyusul.
	 */
	import { Badge, Card, CardContent } from '@pantau-obat/ui';
	import { session } from '$lib/session.svelte';

	const user = $derived(session.user);
</script>

<svelte:head>
	<title>Pengaturan — Pantau Obat</title>
</svelte:head>

<div class="mb-5">
	<h1 class="text-2xl font-semibold tracking-tight">Pengaturan</h1>
	<p class="mt-1 text-sm text-muted-foreground">Profil dan preferensi akun Anda.</p>
</div>

<Card>
	<CardContent class="space-y-4 py-5">
		<div class="flex items-center justify-between gap-3">
			<span class="text-sm text-muted-foreground">Nama</span>
			<span class="text-sm font-medium">{user?.full_name ?? '—'}</span>
		</div>
		<div class="flex items-center justify-between gap-3">
			<span class="text-sm text-muted-foreground">Email</span>
			<span class="truncate text-sm font-medium">{user?.email ?? '—'}</span>
		</div>
		<div class="flex items-center justify-between gap-3">
			<span class="text-sm text-muted-foreground">Zona waktu</span>
			<span class="text-sm font-medium">{user?.timezone ?? '—'}</span>
		</div>
		<div class="flex items-center justify-between gap-3">
			<span class="text-sm text-muted-foreground">Peran</span>
			<Badge variant={user?.role === 'admin' ? 'private' : 'outline'}>
				{user?.role === 'admin' ? 'Admin' : 'Pengguna'}
			</Badge>
		</div>
	</CardContent>
</Card>

<!-- Kanal notifikasi: v0.1 hanya in-app (keputusan pemilik proyek). -->
<Card class="mt-4">
	<CardContent class="space-y-3 py-5">
		<p class="text-sm font-medium">Kanal notifikasi</p>
		<div class="flex items-center justify-between gap-3">
			<span class="text-sm text-muted-foreground">Dalam aplikasi</span>
			<Badge variant="ok">Aktif</Badge>
		</div>
		<div class="flex items-center justify-between gap-3">
			<span class="text-sm text-muted-foreground">Push, email, Telegram</span>
			<Badge variant="na">Nanti</Badge>
		</div>
		<p class="text-xs leading-relaxed text-muted-foreground">
			Kanal lain membutuhkan kredensial operator (VAPID/SMTP/token bot) dan ditunda ke versi berikutnya.
		</p>
	</CardContent>
</Card>

<Card class="mt-4">
	<CardContent class="space-y-2 py-5">
		<p class="text-sm font-medium">Belum tersedia di tahap ini</p>
		<ul class="list-inside list-disc space-y-1 text-xs text-muted-foreground">
			<li>Ganti sandi</li>
			<li>Kelola perangkat &amp; sesi</li>
			<li>Riwayat aktivitas (audit-log)</li>
			<li>Hapus akun</li>
		</ul>
	</CardContent>
</Card>
