<script lang="ts">
	/**
	 * Halaman "Buat sandi baru" — tujuan tautan yang dikirim lewat email.
	 *
	 * Token dibaca dari query (`?token=…`), lalu dikirim ke backend bersama
	 * sandi baru. Halaman ini PUBLIK: pemakainya justru orang yang tidak bisa
	 * masuk, jadi tidak ada penjaga rute di sini.
	 *
	 * Berbeda dari halaman "lupa sandi", halaman ini MEMBERI TAHU bila
	 * tokennya tidak sah. Itu bukan kebocoran: pengguna sudah memegang
	 * tokennya, jadi tidak ada informasi baru yang terungkap — sedangkan
	 * menyembunyikannya hanya membuat orang menunggu surat yang tidak akan
	 * datang. Yang dijaga di sini adalah: jangan sampai orang mengira
	 * sandinya sudah berganti padahal belum.
	 */
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { ApiError } from '@pantau-obat/api-client';
	import { Button, Input, Label } from '@pantau-obat/ui';
	import { api } from '$lib/api';
	import { i18n } from '$lib/i18n.svelte';
	import { APP_NAME } from '$lib/config';

	let password = $state('');
	let konfirmasi = $state('');
	let error = $state<string | null>(null);
	let submitting = $state(false);
	let selesai = $state(false);
	let token = $state('');
	let tokenDibaca = $state(false);

	/**
	 * Benar setelah JavaScript selesai memuat.
	 *
	 * Halaman ini butuh JS untuk membaca token dari query, jadi sebelum
	 * hidrasi form belum bisa mengirim apa pun. Tanpa penanda ini, tombol
	 * akan tampak aktif tetapi tidak melakukan apa-apa — dan pengguna
	 * menyimpulkan tautannya rusak.
	 */
	let hydrated = $state(false);
	onMount(() => {
		hydrated = true;
		token = page.url.searchParams.get('token') ?? '';
		tokenDibaca = true;
	});

	async function onSubmit(e: SubmitEvent) {
		e.preventDefault();
		error = null;

		// Diperiksa di sini, bukan hanya di server: mengirim dua sandi yang
		// tidak sama hanya membuang waktu dan menghasilkan pesan yang kurang
		// jelas daripada yang bisa ditampilkan langsung.
		if (password !== konfirmasi) {
			error = i18n.t('auth.password_mismatch');
			return;
		}

		submitting = true;
		try {
			await api.auth.resetPassword({ token, password });
			selesai = true;
			// Dialihkan setelah jeda singkat supaya pengguna sempat membaca
			// bahwa sandinya benar-benar berganti — tanpa itu, perpindahan
			// halaman yang mendadak membuatnya ragu apakah berhasil.
			setTimeout(() => void goto('/login'), 2500);
		} catch (err) {
			if (err instanceof ApiError) {
				error = err.message;
			} else {
				error = i18n.t('common.error_generic');
			}
		} finally {
			submitting = false;
		}
	}
</script>

<div class="grid min-h-dvh place-items-center bg-muted/30 px-5 py-12">
	<div class="w-full max-w-sm">
		<a href="/" class="mb-8 flex items-center justify-center gap-2.5">
			<span class="grid size-8 place-items-center rounded-lg bg-primary text-primary-foreground" aria-hidden="true">
				<svg viewBox="0 0 24 24" class="size-4.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
					<path d="M10.5 20.5a5 5 0 0 1-7-7l7-7a5 5 0 0 1 7 7Z" />
					<path d="m8.5 8.5 7 7" />
				</svg>
			</span>
			<span class="text-[15px] font-semibold tracking-tight">{APP_NAME}</span>
		</a>

		<div class="rounded-xl border border-border bg-card p-6 shadow-sm">
			{#if selesai}
				<h1 class="text-xl font-semibold tracking-tight">{i18n.t('auth.reset_done_title')}</h1>
				<p class="mt-1.5 text-sm text-muted-foreground" role="status">{i18n.t('auth.reset_done_body')}</p>
				<a
					href="/login"
					class="mt-6 block w-full rounded-md border border-border px-4 py-2 text-center text-sm font-medium hover:bg-muted/50"
				>
					{i18n.t('auth.to_login')}
				</a>
			{:else if tokenDibaca && !token}
				<h1 class="text-xl font-semibold tracking-tight">{i18n.t('auth.reset_title')}</h1>
				<p class="mt-1.5 text-sm text-destructive" role="alert">{i18n.t('auth.reset_no_token')}</p>
				<a
					href="/lupa-sandi"
					class="mt-6 block w-full rounded-md border border-border px-4 py-2 text-center text-sm font-medium hover:bg-muted/50"
				>
					{i18n.t('auth.forgot_link')}
				</a>
			{:else}
				<h1 class="text-xl font-semibold tracking-tight">{i18n.t('auth.reset_title')}</h1>
				<p class="mt-1.5 text-sm text-muted-foreground">{i18n.t('auth.reset_intro')}</p>

				<form class="mt-6 space-y-4" onsubmit={onSubmit}>
					<div class="space-y-2">
						<Label for="password">{i18n.t('auth.password_new')}</Label>
						<Input
							id="password"
							type="password"
							autocomplete="new-password"
							required
							disabled={!hydrated}
							bind:value={password}
							aria-invalid={error ? 'true' : undefined}
						/>
					</div>

					<div class="space-y-2">
						<Label for="konfirmasi">{i18n.t('auth.password_confirm')}</Label>
						<Input
							id="konfirmasi"
							type="password"
							autocomplete="new-password"
							required
							disabled={!hydrated}
							bind:value={konfirmasi}
							aria-invalid={error ? 'true' : undefined}
						/>
					</div>

					{#if error}
						<p class="rounded-md border border-destructive/30 bg-destructive/8 px-3 py-2 text-sm text-destructive" role="alert">
							{error}
						</p>
					{/if}

					<Button type="submit" class="w-full" disabled={submitting || !hydrated}>
						{submitting ? i18n.t('common.saving') : i18n.t('auth.reset_submit')}
					</Button>
				</form>
			{/if}
		</div>
	</div>
</div>
