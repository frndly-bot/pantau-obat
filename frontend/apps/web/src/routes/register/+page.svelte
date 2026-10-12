<script lang="ts">
	/**
	 * Halaman daftar akun.
 *
 * Validasi kekuatan sandi dilakukan backend; pesannya ditampilkan apa adanya
 * (`VALIDATION_FAILED` menyertakan field yang bermasalah).
 */
import { goto } from '$app/navigation';
import { ApiError } from '@pantau-obat/api-client';
import { Button, Input, Label } from '@pantau-obat/ui';
import { api } from '$lib/api';
import { session } from '$lib/session.svelte';
import { APP_NAME } from '$lib/config';

let email = $state('');
let password = $state('');
let fullName = $state('');
let timezone = $state(
	typeof Intl !== 'undefined' ? Intl.DateTimeFormat().resolvedOptions().timeZone : 'Asia/Jakarta'
);
let error = $state<string | null>(null);
let fieldIssues = $state<Record<string, string>>({});
let submitting = $state(false);

async function onSubmit(event: SubmitEvent) {
	event.preventDefault();
	error = null;
	fieldIssues = {};
	submitting = true;
	try {
		const result = await api.auth.register({
			email,
			password,
			full_name: fullName,
			timezone,
		});
		await session.adopt(result.access_token, result.user);
		await goto('/hari-ini');
	} catch (err) {
		if (err instanceof ApiError) {
			error = err.message;
			fieldIssues = {};
			for (const issue of err.details ?? []) fieldIssues[issue.field] = issue.issue;
		} else {
			error = 'Tidak dapat menghubungi server. Coba lagi.';
		}
	} finally {
		submitting = false;
	}
}
</script>

<svelte:head>
	<title>Daftar — {APP_NAME}</title>
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
			<h1 class="text-xl font-semibold tracking-tight">Buat akun</h1>
			<p class="mt-1.5 text-sm text-muted-foreground">
				Gratis, tanpa iklan, dan data kesehatan Anda tidak dikirim ke pihak ketiga.
			</p>

			<form class="mt-6 space-y-4" onsubmit={onSubmit}>
				<div class="space-y-2">
					<Label for="full_name">Nama lengkap</Label>
					<Input
						id="full_name"
						autocomplete="name"
						required
						bind:value={fullName}
						aria-invalid={fieldIssues.full_name ? 'true' : undefined}
					/>
					{#if fieldIssues.full_name}
						<p class="text-xs text-destructive">{fieldIssues.full_name}</p>
					{/if}
				</div>

				<div class="space-y-2">
					<Label for="email">Email</Label>
					<Input
						id="email"
						type="email"
						autocomplete="username"
						required
						bind:value={email}
						placeholder="nama@contoh.com"
						aria-invalid={fieldIssues.email ? 'true' : undefined}
					/>
					{#if fieldIssues.email}
						<p class="text-xs text-destructive">{fieldIssues.email}</p>
					{/if}
				</div>

				<div class="space-y-2">
					<Label for="password">Sandi</Label>
					<Input
						id="password"
						type="password"
						autocomplete="new-password"
						required
						minlength={8}
						bind:value={password}
						aria-invalid={fieldIssues.password ? 'true' : undefined}
					/>
					<p class="text-xs text-muted-foreground">Minimal 8 karakter.</p>
					{#if fieldIssues.password}
						<p class="text-xs text-destructive">{fieldIssues.password}</p>
					{/if}
				</div>

				{#if error}
					<p class="rounded-md border border-destructive/30 bg-destructive/8 px-3 py-2 text-sm text-destructive" role="alert">
						{error}
					</p>
				{/if}

				<Button type="submit" class="w-full" disabled={submitting}>
					{submitting ? 'Membuat akun…' : 'Daftar'}
				</Button>
			</form>
		</div>

		<p class="mt-6 text-center text-sm text-muted-foreground">
			Sudah punya akun?
			<a href="/login" class="font-medium text-primary underline-offset-4 hover:underline">Masuk</a>
		</p>
	</div>
</div>
