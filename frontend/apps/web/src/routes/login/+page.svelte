<script lang="ts">
	/**
	 * Halaman masuk.
 *
 * Alur kredensial ditangani lewat `api.auth.login` dari klien. Refresh token
 * TIDAK pernah menyentuh JavaScript — backend mengirimnya sebagai cookie
 * HttpOnly (`po_refresh`), dan browser menyimpannya sendiri.
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
let error = $state<string | null>(null);
let submitting = $state(false);

/**
 * Benar setelah JavaScript selesai memuat (hidrasi).
 *
 * Halaman ini di-PRERENDER, jadi form sudah tampil dan bisa diisi sebelum
 * JS selesai dimuat — itu memang tujuannya. Masalahnya, selama JS belum
 * jalan belum ada penangan `onsubmit`, sehingga menekan tombol mengirim
 * form secara NATIVE (GET ke halaman ini sendiri) dan isian pengguna hilang
 * tanpa pesan apa pun.
 *
 * Di ponsel lambat — perangkat utama aplikasi ini — jeda itu nyata. Karena
 * itu tombol submit baru aktif setelah hidrasi. Tanpa JS aplikasi ini memang
 * tidak bisa memanggil API sama sekali, jadi menahan tombol sebentar tidak
 * menghilangkan kemampuan, sedangkan membiarkannya aktif menghilangkan isian
 * orang tanpa penjelasan.
 */
let hydrated = $state(false);

onMount(() => {
	hydrated = true;
	next = safeNext(page.url.searchParams.get('next'));
});

/**
 * Tujuan setelah masuk berhasil.
 *
 * Penjaga rute mengirim pengguna ke sini dengan `?next=<halaman asal>`
 * (mis. tautan undangan). Tanpa menghormatinya, orang yang mengklik tautan
 * undangan lalu diminta masuk akan terdampar di beranda dan kehilangan
 * tautannya — alur yang seharusnya mulus jadi buntu.
 *
 * Hanya jalur internal yang diterima. Nilai dari query bisa diisi siapa pun,
 * dan mengalihkan ke alamat luar berarti halaman ini bisa dipakai untuk
 * mengirim orang ke situs lain sambil membawa kesan "baru saja masuk".
 *
 * Dibaca di `onMount`, bukan saat render: halaman ini di-PRERENDER, dan
 * SvelteKit melarang menyentuh `url.searchParams` saat prerender karena
 * query string tidak ada di berkas HTML hasil build. Nilai awalnya `/hari-ini`
 * supaya HTML statis tetap benar; nilai sebenarnya dipasang begitu JS jalan.
 */
let next = $state('/hari-ini');

function safeNext(raw: string | null): string {
	if (!raw) return '/hari-ini';
	// Harus diawali satu garis miring, dan TIDAK boleh "//" atau "/\" yang
	// oleh peramban dibaca sebagai alamat absolut (protokol-relative).
	if (!raw.startsWith('/') || raw.startsWith('//') || raw.startsWith('/\\')) return '/hari-ini';
	return raw;
}

async function onSubmit(event: SubmitEvent) {
	event.preventDefault();
	error = null;
	submitting = true;
	try {
		const result = await api.auth.login({ email, password });
		await session.adopt(result.access_token, result.user);
		await goto(next);
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
			<h1 class="text-xl font-semibold tracking-tight">{i18n.t('common.sign')}</h1>
			<p class="mt-1.5 text-sm text-muted-foreground">{i18n.t('auth.continue_to_schedule')}</p>

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

				<div class="space-y-2">
					<Label for="password">{i18n.t('auth.password')}</Label>
					<Input
						id="password"
						type="password"
						autocomplete="current-password"
						required
						disabled={!hydrated}
						bind:value={password}
						aria-invalid={error ? 'true' : undefined}
					/>
				</div>

				<div class="text-right">
					<a href="/lupa-sandi" class="text-sm font-medium text-primary underline-offset-4 hover:underline">
						{i18n.t('auth.forgot_link')}
					</a>
				</div>

				{#if error}
					<p class="rounded-md border border-destructive/30 bg-destructive/8 px-3 py-2 text-sm text-destructive" role="alert">
						{error}
					</p>
				{/if}

				<Button type="submit" class="w-full" disabled={submitting || !hydrated}>
					{submitting ? i18n.t('common.saving') : i18n.t('common.sign')}
				</Button>
			</form>
		</div>

		<p class="mt-6 text-center text-sm text-muted-foreground">
			Belum punya akun?
			<a
				href={`/register${next === '/hari-ini' ? '' : `?next=${encodeURIComponent(next)}`}`}
				class="font-medium text-primary underline-offset-4 hover:underline"
			>
				Daftar
			</a>
		</p>
	</div>
</div>
