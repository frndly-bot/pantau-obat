<script lang="ts">
	/**
	 * Halaman masuk.
 *
 * Alur kredensial ditangani lewat `api.auth.login` dari klien. Refresh token
 * TIDAK pernah menyentuh JavaScript — backend mengirimnya sebagai cookie
 * HttpOnly (`po_refresh`), dan browser menyimpannya sendiri.
 */
import { goto } from '$app/navigation';
import { ApiError } from '@pantau-obat/api-client';
import { Button, Input, Label } from '@pantau-obat/ui';
import { api } from '$lib/api';
import { session } from '$lib/session.svelte';
import { i18n } from '$lib/i18n.svelte';
import { APP_NAME } from '$lib/config';

let email = $state('');
let password = $state('');
let error = $state<string | null>(null);
let submitting = $state(false);

async function onSubmit(event: SubmitEvent) {
	event.preventDefault();
	error = null;
	submitting = true;
	try {
		const result = await api.auth.login({ email, password });
		await session.adopt(result.access_token, result.user);
		await goto('/hari-ini');
	} catch (err) {
		if (err instanceof ApiError) {
			// Pesan dari backend sengaja disamakan untuk email tidak dikenal
			// dan sandi salah — jangan bedakan di UI.
			error = err.message;
		} else {
			error = 'Tidak dapat menghubungi server. Coba lagi.';
		}
	} finally {
		submitting = false;
	}
}
</script>

<svelte:head>
	<title>Masuk — {APP_NAME}</title>
	<meta name="robots" content="noindex" />
</svelte:head>

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
			<h1 class="text-xl font-semibold tracking-tight">Masuk</h1>
			<p class="mt-1.5 text-sm text-muted-foreground">Lanjutkan ke jadwal obat Anda.</p>

			<form class="mt-6 space-y-4" onsubmit={onSubmit}>
				<div class="space-y-2">
					<Label for="email">Email</Label>
					<Input
						id="email"
						type="email"
						autocomplete="username"
						required
						bind:value={email}
						placeholder="nama@contoh.com"
						aria-invalid={error ? 'true' : undefined}
					/>
				</div>

				<div class="space-y-2">
					<Label for="password">Sandi</Label>
					<Input
						id="password"
						type="password"
						autocomplete="current-password"
						required
						bind:value={password}
						aria-invalid={error ? 'true' : undefined}
					/>
				</div>

				{#if error}
					<p class="rounded-md border border-destructive/30 bg-destructive/8 px-3 py-2 text-sm text-destructive" role="alert">
						{error}
					</p>
				{/if}

				<Button type="submit" class="w-full" disabled={submitting}>
					{submitting ? 'Memproses…' : 'Masuk'}
				</Button>
			</form>
		</div>

		<p class="mt-6 text-center text-sm text-muted-foreground">
			Belum punya akun?
			<a href="/register" class="font-medium text-primary underline-offset-4 hover:underline">Daftar</a>
		</p>
	</div>
</div>
