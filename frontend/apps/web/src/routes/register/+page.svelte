<script lang="ts">
	/**
	 * Halaman daftar akun.
 *
 * Validasi kekuatan sandi dilakukan backend; pesannya ditampilkan apa adanya
 * (`VALIDATION_FAILED` menyertakan field yang bermasalah).
 */
import { onMount } from 'svelte';
import { goto } from '$app/navigation';
import { page } from '$app/state';
import { ApiError } from '@pantau-obat/api-client';
import { Button, Input, Label } from '@pantau-obat/ui';
import { api } from '$lib/api';
import { session } from '$lib/session.svelte';
	import { i18n } from '$lib/i18n.svelte';
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

/**
 * Persetujuan bahwa aplikasi ini alat bantu, bukan pengganti nasihat dokter.
 *
 * Prototipe design mewajibkan centang ini sebelum tombol daftar aktif, dan
 * kamus i18n sudah memuat teksnya. Tanpa ini, pengguna bisa mendaftar tanpa
 * pernah diberi tahu batas kemampuan aplikasi — padahal isinya menyangkut
 * keputusan pengobatan, dan aplikasi ini memang tidak memberi saran dosis
 * maupun memeriksa interaksi obat.
 *
 * Bobotnya kepatuhan, bukan kosmetik: persetujuan yang tidak pernah
 * ditampilkan tidak bisa disebut persetujuan.
 */
let agreed = $state(false);

/**
 * Benar setelah JavaScript selesai memuat (hidrasi).
 *
 * Halaman ini di-PRERENDER, jadi form tampil sebelum JS siap. Selama itu
 * belum ada penangan `onsubmit`, sehingga menekan tombol mengirim form
 * secara native (GET) dan isian hilang tanpa penjelasan. Lihat penjelasan
 * lengkap di `login/+page.svelte`.
 */
let hydrated = $state(false);

onMount(() => {
	hydrated = true;
	next = safeNext(page.url.searchParams.get('next'));
});

/**
 * Tujuan setelah pendaftaran berhasil.
 *
 * Sama seperti di halaman masuk: tautan undangan mengarah ke sini lewat
 * `?next=`, dan mengabaikannya membuat pengguna baru terdampar di beranda
 * tanpa tahu bahwa undangannya belum diterima. Hanya jalur internal yang
 * diterima — lihat penjelasan lengkap di `login/+page.svelte`.
 *
 * Dibaca di `onMount`, bukan saat render, karena halaman ini di-PRERENDER
 * dan SvelteKit melarang menyentuh `url.searchParams` saat prerender.
 */
let next = $state('/hari-ini');

function safeNext(raw: string | null): string {
	if (!raw) return '/hari-ini';
	if (!raw.startsWith('/') || raw.startsWith('//') || raw.startsWith('/\\')) return '/hari-ini';
	return raw;
}

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
		await goto(next);
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
					<Label for="full_name">{i18n.t('auth.full_name')}</Label>
					<Input
						id="full_name"
						autocomplete="name"
						required
						disabled={!hydrated}
						bind:value={fullName}
						aria-invalid={fieldIssues.full_name ? 'true' : undefined}
					/>
					{#if fieldIssues.full_name}
						<p class="text-xs text-destructive">{fieldIssues.full_name}</p>
					{/if}
				</div>

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
						aria-invalid={fieldIssues.email ? 'true' : undefined}
					/>
					{#if fieldIssues.email}
						<p class="text-xs text-destructive">{fieldIssues.email}</p>
					{/if}
				</div>

				<div class="space-y-2">
					<Label for="password">{i18n.t('auth.password')}</Label>
					<Input
						id="password"
						type="password"
						autocomplete="new-password"
						required
						minlength={8}
						disabled={!hydrated}
						bind:value={password}
						aria-invalid={fieldIssues.password ? 'true' : undefined}
					/>
					<p class="text-xs text-muted-foreground">{i18n.t('auth.at_least_8_characters')}.</p>
					{#if fieldIssues.password}
						<p class="text-xs text-destructive">{fieldIssues.password}</p>
					{/if}
				</div>

				{#if error}
					<p class="rounded-md border border-destructive/30 bg-destructive/8 px-3 py-2 text-sm text-destructive" role="alert">
						{error}
					</p>
				{/if}

				<!-- Persetujuan medis. Tombol daftar dikunci sampai dicentang,
				     karena isinya menyangkut keputusan pengobatan. -->
				<label class="flex cursor-pointer items-start gap-2.5 text-xs text-muted-foreground">
					<input
						type="checkbox"
						class="mt-0.5 size-4 shrink-0 accent-[var(--primary)]"
						disabled={!hydrated}
						bind:checked={agreed}
					/>
					<span>
						{i18n.t('auth.i_understand_this_app')} <b class="font-semibold text-foreground">{i18n.t('auth.helper_tool')}</b>, {i18n.t('auth.not_replacement')}
						<span class="mt-0.5 block text-[11px]">
							{i18n.t('auth.it_gives_no_dosing_advice_performs_no_drug_interacti')}
						</span>
					</span>
				</label>

				<Button type="submit" class="w-full" disabled={submitting || !hydrated || !agreed}>
					{submitting ? i18n.t('auth.creating_account') : i18n.t('common.register')}
				</Button>
			</form>
		</div>

		<p class="mt-6 text-center text-sm text-muted-foreground">
			{i18n.t('auth.already_have_account')}
			<a
				href={`/login${next === '/hari-ini' ? '' : `?next=${encodeURIComponent(next)}`}`}
				class="font-medium text-primary underline-offset-4 hover:underline"
			>
				{i18n.t('common.sign')}
			</a>
		</p>
	</div>
</div>
