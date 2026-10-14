<script lang="ts">
	/**
	 * Halaman "Lupa sandi".
	 *
	 * Satu hal yang menentukan bentuk halaman ini: backend SELALU membalas
	 * sukses, termasuk untuk alamat yang tidak terdaftar. Itu disengaja —
	 * kalau endpoint membedakan keduanya, siapa pun bisa memakainya untuk
	 * memeriksa apakah seseorang punya akun di aplikasi kesehatan ini.
	 *
	 * Karena itu halaman ini TIDAK BOLEH menyimpulkan apa pun dari balasan
	 * server. Pesan setelah mengirim sengaja berbunyi "jika email itu
	 * terdaftar" — bukan "tautan sudah dikirim" — supaya layarnya tidak
	 * membocorkan hal yang justru ditutup oleh backend. Menuliskan "email
	 * tidak ditemukan" di sini akan membatalkan seluruh perlindungannya.
	 */
	import { onMount } from 'svelte';
	import { ApiError } from '@pantau-obat/api-client';
	import { Button, Input, Label } from '@pantau-obat/ui';
	import { api } from '$lib/api';
	import { i18n } from '$lib/i18n.svelte';
	import { APP_NAME } from '$lib/config';

	let email = $state('');
	let error = $state<string | null>(null);
	let submitting = $state(false);
	let terkirim = $state(false);

	/**
	 * Benar setelah JavaScript selesai memuat.
	 *
	 * Sama seperti halaman masuk: halaman ini di-PRERENDER, jadi form sudah
	 * tampil sebelum JS siap. Selama itu belum ada penangan `onsubmit`,
	 * sehingga menekan tombol mengirim form secara native dan isian
	 * pengguna hilang tanpa pesan. Di ponsel lambat jeda itu nyata.
	 */
	let hydrated = $state(false);
	onMount(() => {
		hydrated = true;
	});

	async function onSubmit(e: SubmitEvent) {
		e.preventDefault();
		error = null;
		submitting = true;
		try {
			await api.auth.forgotPassword({ email });
			// Balasan apa pun diperlakukan sama — lihat catatan di atas.
			terkirim = true;
		} catch (err) {
			// Yang bisa dilaporkan hanyalah kegagalan yang memang tentang
			// permintaannya: alamat kosong, jeda antar permintaan, atau
			// server yang belum dikonfigurasi untuk mengirim email.
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
			{#if terkirim}
				<h1 class="text-xl font-semibold tracking-tight">{i18n.t('auth.forgot_sent_title')}</h1>
				<p class="mt-1.5 text-sm text-muted-foreground" role="status">
					{i18n.t('auth.forgot_sent_body')}
				</p>
				<p class="mt-3 text-sm text-muted-foreground">{i18n.t('auth.forgot_sent_hint')}</p>

				<a
					href="/login"
					class="mt-6 block w-full rounded-md border border-border px-4 py-2 text-center text-sm font-medium hover:bg-muted/50"
				>
					{i18n.t('auth.forgot_back')}
				</a>
			{:else}
				<h1 class="text-xl font-semibold tracking-tight">{i18n.t('auth.forgot_title')}</h1>
				<p class="mt-1.5 text-sm text-muted-foreground">{i18n.t('auth.forgot_intro')}</p>

				<form class="mt-6 space-y-4" onsubmit={onSubmit}>
					<div class="space-y-2">
						<Label for="email">{i18n.t('auth.email')}</Label>
						<Input
							id="email"
							type="email"
							autocomplete="username"
							required
							disabled={!hydrated}
							bind:value={email}
							placeholder={i18n.t('misc.name_example_com')}
							aria-invalid={error ? 'true' : undefined}
						/>
					</div>

					{#if error}
						<p class="rounded-md border border-destructive/30 bg-destructive/8 px-3 py-2 text-sm text-destructive" role="alert">
							{error}
						</p>
					{/if}

					<Button type="submit" class="w-full" disabled={submitting || !hydrated}>
						{submitting ? i18n.t('common.saving') : i18n.t('auth.forgot_submit')}
					</Button>
				</form>
			{/if}
		</div>

		<p class="mt-6 text-center text-sm text-muted-foreground">
			<a href="/login" class="font-medium text-primary underline-offset-4 hover:underline">
				{i18n.t('auth.forgot_back')}
			</a>
		</p>
	</div>
</div>
