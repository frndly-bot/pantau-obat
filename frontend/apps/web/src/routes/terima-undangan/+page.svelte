<script lang="ts">
	/**
	 * Halaman terima undangan caregiver.
	 *
	 * Alamat ini adalah tujuan tautan yang dikirim backend
	 * (`{APP_PUBLIC_URL}/terima-undangan?token=...`). Sebelum halaman ini ada,
	 * tautan undangan menuju 404 — alur caregiver dari email buntu.
	 *
	 * Keadaan yang ditangani, semuanya nyata:
	 *
	 * 1. **Belum masuk** — undangan terikat pada satu alamat email, dan
	 *    backend menolak penerimaan oleh akun yang bukan pemilik email itu.
	 *    Jadi orang harus masuk DULU. `?next=` dipakai supaya setelah masuk
	 *    ia kembali ke sini, bukan terdampar di beranda.
	 * 2. **Sudah masuk** — tampilkan tombol Terima/Tolak.
	 * 3. **Email tidak cocok** — backend menjawab 403; halaman menjelaskan
	 *    dan menawarkan berganti akun.
	 * 4. **Token kosong / tidak berlaku** — dijelaskan tanpa menebak-nebak.
	 *
	 * MENGAPA EMAIL TIDAK DIPERIKSA DI AWAL: undangan yang masih `pending`
	 * belum punya `caregiver_id` (kolom itu baru diisi saat diterima), dan
	 * `GET /care-links?as=caregiver` menyaring berdasarkan `caregiver_id`.
	 * Artinya undangan pending TIDAK muncul di daftar mana pun — halaman ini
	 * tidak punya cara mengetahui email tujuan sebelum mencoba. Backend juga
	 * sengaja tidak menyediakan pratinjau: menampilkan nama pemilik sebelum
	 * hak akses diberikan berarti membocorkan data kesehatan ke siapa pun
	 * yang memegang token. Karena itu keputusan email diserahkan ke backend,
	 * dan jawabannya (403) diterjemahkan di sini.
	 */
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { ApiError, ERROR_CODES, type CareLink } from '@pantau-obat/api-client';
	import { Badge, Button } from '@pantau-obat/ui';
	import { api } from '$lib/api';
	import { session } from '$lib/session.svelte';
	import { APP_NAME } from '$lib/config';

	/**
	 * Token dari tautan email.
	 *
	 * Dibaca di `onMount`, bukan saat render: halaman ini di-PRERENDER dan
	 * SvelteKit melarang menyentuh `url.searchParams` saat prerender. Nilai
	 * awalnya kosong, lalu diisi begitu JavaScript jalan.
	 */
	let token = $state('');

	type Stage = 'loading' | 'guest' | 'confirm' | 'email-mismatch' | 'done' | 'error';

	let stage = $state<Stage>('loading');
	let busy = $state<'accept' | 'reject' | null>(null);
	let error = $state<string | null>(null);
	let decided = $state<CareLink | null>(null);
	let decidedAs = $state<'accepted' | 'rejected'>('accepted');

	onMount(async () => {
		token = page.url.searchParams.get('token') ?? '';

		if (!token) {
			stage = 'error';
			error = 'Tautan undangan tidak lengkap — token tidak ditemukan di alamat.';
			return;
		}

		// Tunggu pemulihan sesi sebelum memutuskan tampilan mana yang tepat;
		// tanpa ini, pengguna yang sudah masuk sempat melihat "silakan masuk".
		await session.init();

		stage = session.isAuthenticated ? 'confirm' : 'guest';
	});

	async function decide(accept: boolean) {
		if (busy || !token) return;
		busy = accept ? 'accept' : 'reject';
		error = null;
		try {
			const link = accept
				? await api.care.accept({ token })
				: await api.care.reject({ token });
			decided = link;
			decidedAs = accept ? 'accepted' : 'rejected';
			stage = 'done';
		} catch (err) {
			if (err instanceof ApiError) {
				// 403 berarti undangan ini untuk email lain. Itu keadaan yang
				// layak ditampilkan sebagai layar tersendiri, bukan sekadar
				// pesan error: pengguna perlu tahu bahwa yang salah bukan
				// tautannya, melainkan akun yang sedang masuk.
				if (err.code === ERROR_CODES.FORBIDDEN) {
					stage = 'email-mismatch';
					return;
				}

				// Sesi habis: undangan belum tersentuh, jadi cukup minta masuk
				// lagi. Tautannya masih ada di alamat halaman ini.
				if (err.code === ERROR_CODES.UNAUTHENTICATED) {
					await session.forget();
					stage = 'guest';
					return;
				}

				// Token tidak ditemukan atau sudah kedaluwarsa bersifat
				// TERMINAL: tidak ada percobaan ulang yang bisa berhasil.
				// Menampilkan pesannya sambil membiarkan tombol Terima tetap
				// ada membuat pengguna menekan tombol yang pasti gagal, jadi
				// keadaan ini dipindah ke layar tersendiri tanpa tombol itu.
				if (
					err.code === ERROR_CODES.NOT_FOUND ||
					err.code === ERROR_CODES.VALIDATION_FAILED
				) {
					stage = 'error';
				}

				error = explain(err);
			} else {
				// Gangguan jaringan: undangan belum tentu tidak berlaku, jadi
				// tombolnya dibiarkan supaya bisa dicoba lagi.
				error = 'Tidak dapat menghubungi server. Coba lagi.';
			}
		} finally {
			busy = null;
		}
	}

	/**
	 * Menerjemahkan kode error backend menjadi kalimat yang menolong.
	 *
	 * Backend sengaja menjawab "tidak ditemukan" baik untuk token yang tidak
	 * ada maupun yang sudah dipakai — supaya keberadaan undangan tidak bocor
	 * ke orang yang menebak-nebak token. Pesannya karena itu mencakup kedua
	 * kemungkinan.
	 */
	function explain(err: ApiError): string {
		switch (err.code) {
			case ERROR_CODES.NOT_FOUND:
				return 'Undangan tidak ditemukan. Kemungkinan tautannya sudah pernah dipakai, dibatalkan pengirim, atau salah salin.';
			case ERROR_CODES.VALIDATION_FAILED:
				return err.message || 'Undangan sudah kedaluwarsa atau tidak berlaku lagi.';
			case ERROR_CODES.UNAUTHENTICATED:
				return 'Sesi Anda berakhir. Masuk kembali untuk menerima undangan.';
			default:
				return err.message || 'Gagal memproses undangan.';
		}
	}

	function invitePath(): string {
		return `/terima-undangan?token=${encodeURIComponent(token)}`;
	}

	function signIn() {
		void goto(`/login?next=${encodeURIComponent(invitePath())}`);
	}

	async function switchAccount() {
		await session.signOut();
		signIn();
	}

	function goHome() {
		void goto('/hari-ini');
	}

	function goCaregiver() {
		void goto('/caregiver?as=caregiver');
	}
</script>

<svelte:head>
	<title>Terima undangan — {APP_NAME}</title>
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
			{#if stage === 'loading'}
				<div class="flex items-center gap-3" role="status" aria-live="polite">
					<span
						class="size-4 shrink-0 animate-spin rounded-full border-2 border-muted-foreground/30 border-t-primary"
						aria-hidden="true"
					></span>
					<p class="text-sm text-muted-foreground">Memeriksa undangan…</p>
				</div>
			{:else if stage === 'guest'}
				<h1 class="text-xl font-semibold tracking-tight">Undangan pemantauan</h1>
				<p class="mt-2 text-sm text-muted-foreground">
					Seseorang mengundang Anda untuk memantau jadwal minum obatnya. Masuk dulu untuk
					menerima undangan ini.
				</p>
				<p class="mt-4 rounded-md border border-border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
					Undangan ditujukan ke alamat email tertentu, jadi gunakan akun dengan alamat yang
					menerima undangan.
				</p>
				<div class="mt-6 space-y-2">
					<Button class="w-full" onclick={signIn}>Masuk untuk menerima</Button>
					<a
						href={`/register?next=${encodeURIComponent(invitePath())}`}
						class="block text-center text-sm text-muted-foreground underline-offset-4 hover:underline"
					>
						Belum punya akun? Daftar
					</a>
				</div>
			{:else if stage === 'confirm'}
				<h1 class="text-xl font-semibold tracking-tight">Terima undangan?</h1>
				<p class="mt-2 text-sm text-muted-foreground">
					Setelah diterima, Anda bisa melihat jadwal dan catatan minum obat pengundang,
					sesuai izin yang diberikan.
				</p>

				<div class="mt-4 rounded-md border border-border bg-muted/40 px-3 py-2.5 text-sm">
					<span class="text-muted-foreground">Masuk sebagai</span>
					<span class="mt-0.5 block font-medium">{session.user?.email}</span>
				</div>

				{#if error}
					<p class="mt-4 rounded-md border border-destructive/30 bg-destructive/8 px-3 py-2 text-sm text-destructive" role="alert">
						{error}
					</p>
				{/if}

				<div class="mt-6 space-y-2">
					<Button class="w-full" disabled={busy !== null} onclick={() => decide(true)}>
						{busy === 'accept' ? 'Menerima…' : 'Terima undangan'}
					</Button>
					<Button
						variant="ghost"
						class="w-full"
						disabled={busy !== null}
						onclick={() => decide(false)}
					>
						{busy === 'reject' ? 'Menolak…' : 'Tolak'}
					</Button>
				</div>
			{:else if stage === 'email-mismatch'}
				<h1 class="text-xl font-semibold tracking-tight">Undangan untuk akun lain</h1>
				<p class="mt-2 text-sm text-muted-foreground">
					Undangan ini ditujukan untuk alamat email lain, jadi tidak bisa diterima oleh akun
					yang sedang masuk:
				</p>
				<div class="mt-3 rounded-md border border-border bg-muted/40 px-3 py-2.5 text-sm">
					<span class="font-medium">{session.user?.email}</span>
				</div>
				<p class="mt-4 text-sm text-muted-foreground">
					Masuk dengan akun yang menerima undangan untuk melanjutkan.
				</p>
				<div class="mt-6 space-y-2">
					<Button class="w-full" onclick={switchAccount}>Masuk dengan akun lain</Button>
					<Button variant="ghost" class="w-full" onclick={goHome}>Kembali ke beranda</Button>
				</div>
			{:else if stage === 'done'}
				<h1 class="text-xl font-semibold tracking-tight">
					{decidedAs === 'accepted' ? 'Undangan diterima' : 'Undangan ditolak'}
				</h1>
				{#if decidedAs === 'accepted'}
					<p class="mt-2 text-sm text-muted-foreground">
						Anda sekarang terhubung sebagai pemantau. Jadwal dan catatan yang diizinkan bisa
						dilihat di halaman Caregiver.
					</p>
					{#if decided?.role}
						<div class="mt-3">
							<Badge variant="secondary">Peran: {decided.role}</Badge>
						</div>
					{/if}
					<div class="mt-6 space-y-2">
						<Button class="w-full" onclick={goCaregiver}>Lihat di Caregiver</Button>
						<Button variant="ghost" class="w-full" onclick={goHome}>Ke beranda</Button>
					</div>
				{:else}
					<p class="mt-2 text-sm text-muted-foreground">
						Undangan ini sudah ditolak. Pengundang tidak lagi menunggu jawaban dari Anda.
					</p>
					<div class="mt-6">
						<Button variant="ghost" class="w-full" onclick={goHome}>Ke beranda</Button>
					</div>
				{/if}
			{:else}
				<h1 class="text-xl font-semibold tracking-tight">Undangan tidak dapat dibuka</h1>
				<p class="mt-2 rounded-md border border-destructive/30 bg-destructive/8 px-3 py-2 text-sm text-destructive" role="alert">
					{error}
				</p>
				<p class="mt-4 text-sm text-muted-foreground">
					Minta pengundang mengirim ulang undangan, lalu buka tautan terbaru dari email.
				</p>
				<div class="mt-6">
					<Button variant="ghost" class="w-full" onclick={goHome}>Ke beranda</Button>
				</div>
			{/if}
		</div>

		{#if stage !== 'done'}
			<p class="mt-6 text-center text-xs text-muted-foreground">
				Data kesehatan Anda tidak dibagikan ke pihak ketiga.
			</p>
		{/if}
	</div>
</div>
