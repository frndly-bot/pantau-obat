<script lang="ts">
	/**
	 * Landing page — dirender saat build (prerender), tanpa server Node.
	 *
	 * Sesuai keputusan pemilik proyek:
	 * - simple, mengikuti sistem desain di `design/` (token teal, gaya shadcn);
	 * - TANPA data dummy / angka statistik karangan;
	 * - hanya pengenalan produk + tautan ke GitHub.
	 *
	 * Rujukan tata letak: https://www.makevisible.com/ (bagian panjang, banyak
	 * jeda, tipografi besar, satu gagasan per bagian).
	 */
	import { APP_NAME, GITHUB_URL } from '$lib/config';
	import { Accordion, Badge, Button } from '@pantau-obat/ui';
	import { reveal } from '@pantau-obat/ui';

	const features = [
	{
		title: 'Pengingat tepat waktu',
		body: 'Jadwal dosis disusun dari pola yang Anda pilih — harian, interval, mingguan, siklus bulanan, atau sesuai kebutuhan. Pengingat mengikuti zona waktu Anda.'
	},
	{
		title: 'Catat sekali ketuk',
		body: 'Tandai dosis sebagai diminum, tunda, atau lewati langsung dari layar Hari Ini. Koreksi jam tetap bisa dilakukan bila Anda baru mencatatnya belakangan.'
	},
	{
		title: 'Stok & pengingat isi ulang',
		body: 'Sisa obat dihitung dari setiap dosis yang tercatat. Ketika saldo menyentuh ambang yang Anda tentukan, aplikasi mengingatkan untuk menebus resep.'
	},
	{
		title: 'Pantauan caregiver',
		body: 'Undang keluarga atau pendamping dengan peran yang jelas: melihat saja, membantu mencatat, atau ikut mengelola obat dan stok.'
	}
];

const steps = [
	{
		title: 'Tambahkan obat',
		body: 'Isi nama, dosis, dan pola minum. Katalog obat membantu mengisi rincian umum.'
	},
	{
		title: 'Ikuti jadwal harian',
		body: 'Layar Hari Ini menampilkan dosis yang jatuh tempo beserta sisa stok, tanpa perlu menyusun ulang apa pun.'
	},
	{
		title: 'Lihat riwayat & kepatuhan',
		body: 'Riwayat catatan dan ringkasan kepatuhan tersedia kapan saja, dan bisa diekspor sebagai CSV.'
	}
];

const privacyPoints = [
	'Data kesehatan Anda bukan komoditas. Tidak ada iklan, tidak ada penjualan data.',
	'Token sesi disimpan sebagai cookie HttpOnly — JavaScript tidak dapat membacanya.',
	'Kata sandi disimpan sebagai hash, dan setiap sesi dapat dicabut dari perangkat mana pun.',
	'Riwayat aktivitas dicatat agar Anda bisa menelusuri sendiri siapa mengubah apa.'
];

const faq = [
	{
		id: 'gratis',
		question: `Apakah ${APP_NAME} gratis?`,
		answer:
			'Ya. Proyek ini terbuka dan bisa Anda jalankan sendiri. Kode sumbernya tersedia di GitHub.'
	},
	{
		id: 'data',
		question: 'Di mana data saya disimpan?',
		answer:
			'Di server yang Anda jalankan sendiri. Karena proyek ini open source, tidak ada pihak lain yang menyimpan salinan data Anda.'
	},
	{
		id: 'caregiver',
		question: 'Bisakah keluarga ikut memantau?',
		answer:
			'Bisa. Anda dapat mengundang caregiver dengan peran yang Anda tentukan — hanya melihat, membantu mencatat, atau ikut mengelola obat dan stok.'
	},
	{
		id: 'pola',
		question: 'Pola minum seperti apa yang didukung?',
		answer:
			'Lima pola: harian, interval (setiap N jam), mingguan (hari tertentu), siklus bulanan (minum–jeda), dan sesuai kebutuhan (PRN).'
	},
	{
		id: 'pwa',
		question: 'Apakah bisa dipakai di ponsel?',
		answer:
			'Ya. Antarmukanya dirancang untuk layar kecil terlebih dahulu, dan dapat dipasang sebagai aplikasi (PWA).'
	}
];
</script>

<svelte:head>
	<title>{APP_NAME} — pengingat & pencatat obat harian</title>
	<meta
		name="description"
		content="Pantau Obat membantu Anda mengingat jadwal minum obat, mencatat setiap dosis, dan memantau sisa stok — dengan dukungan caregiver."
	/>
	<meta property="og:title" content="{APP_NAME} — pengingat & pencatatan obat" />
	<meta
		property="og:description"
		content="Pengingat tepat waktu, catat sekali ketuk, stok terkendali. Open source."
	/>
	<meta property="og:type" content="website" />
</svelte:head>

<div class="min-h-dvh bg-background text-foreground">
	<!-- ===================== Navigasi ===================== -->
	<header class="sticky top-0 z-40 border-b border-border/60 bg-background/85 backdrop-blur">
		<nav class="mx-auto flex h-16 max-w-6xl items-center justify-between px-5">
			<a href="#atas" class="flex items-center gap-2.5">
				<span
					class="grid size-8 place-items-center rounded-lg bg-primary text-primary-foreground"
					aria-hidden="true"
				>
					<svg viewBox="0 0 24 24" class="size-4.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
						<path d="M10.5 20.5a5 5 0 0 1-7-7l7-7a5 5 0 0 1 7 7Z" />
						<path d="m8.5 8.5 7 7" />
					</svg>
				</span>
				<span class="text-[15px] font-semibold tracking-tight">{APP_NAME}</span>
			</a>

			<div class="hidden items-center gap-7 text-sm text-muted-foreground md:flex">
				<a class="transition-colors hover:text-foreground" href="#fitur">Fitur</a>
				<a class="transition-colors hover:text-foreground" href="#cara-kerja">Cara kerja</a>
				<a class="transition-colors hover:text-foreground" href="#privasi">Privasi</a>
				<a class="transition-colors hover:text-foreground" href="#faq">FAQ</a>
			</div>

			<div class="flex items-center gap-2">
				<Button variant="ghost" size="sm" href="/login">Masuk</Button>
				<Button size="sm" href="/register">Daftar</Button>
			</div>
		</nav>
	</header>

	<main id="atas">
		<!-- ===================== Hero ===================== -->
		<section class="mx-auto max-w-6xl px-5 pt-20 pb-16 sm:pt-28 sm:pb-24">
			<div class="max-w-3xl">
				<Badge variant="outline" class="mb-6 gap-1.5 py-1">
					<span class="size-1.5 rounded-full bg-ok" aria-hidden="true"></span>
					Open source · dapat Anda jalankan sendiri
				</Badge>

				<h1 class="text-balance text-4xl font-semibold leading-[1.1] tracking-tight sm:text-5xl lg:text-6xl">
					Jangan biarkan jadwal minum obat bergantung pada ingatan.
				</h1>

				<p class="mt-6 max-w-2xl text-pretty text-lg leading-relaxed text-muted-foreground">
					{APP_NAME} mengingatkan dosis pada waktunya, mencatat setiap kali Anda minum, dan
					menjaga sisa stok tetap terlihat — supaya Anda bisa fokus pada pemulihan, bukan pada
					catatan.
				</p>

				<div class="mt-9 flex flex-wrap items-center gap-3">
					<Button size="lg" href="/register">Mulai sekarang</Button>
					<Button size="lg" variant="outline" href={GITHUB_URL} rel="noopener noreferrer">
						<svg viewBox="0 0 24 24" class="size-4" fill="currentColor" aria-hidden="true">
							<path d="M12 2C6.48 2 2 6.58 2 12.25c0 4.53 2.87 8.37 6.84 9.73.5.09.68-.22.68-.49v-1.7c-2.78.62-3.37-1.37-3.37-1.37-.45-1.18-1.11-1.5-1.11-1.5-.91-.63.07-.62.07-.62 1 .07 1.53 1.06 1.53 1.06.9 1.57 2.36 1.12 2.94.86.09-.67.35-1.12.63-1.38-2.22-.26-4.56-1.14-4.56-5.06 0-1.12.39-2.03 1.03-2.75-.1-.26-.45-1.3.1-2.7 0 0 .84-.28 2.75 1.05a9.36 9.36 0 0 1 5 0c1.91-1.33 2.75-1.05 2.75-1.05.55 1.4.2 2.44.1 2.7.64.72 1.03 1.63 1.03 2.75 0 3.93-2.35 4.8-4.58 5.05.36.32.68.94.68 1.9v2.82c0 .27.18.59.69.49A10.02 10.02 0 0 0 22 12.25C22 6.58 17.52 2 12 2Z" />
						</svg>
						Lihat di GitHub
					</Button>
				</div>
			</div>

			<!-- Pratinjau antarmuka: murni dekoratif, tanpa data contoh. -->
			<div
				class="mt-16 overflow-hidden rounded-xl border border-border bg-card shadow-lg"
				use:reveal
			>
				<div class="flex items-center gap-1.5 border-b border-border bg-muted/40 px-4 py-3">
					<span class="size-2.5 rounded-full bg-border" aria-hidden="true"></span>
					<span class="size-2.5 rounded-full bg-border" aria-hidden="true"></span>
					<span class="size-2.5 rounded-full bg-border" aria-hidden="true"></span>
					<span class="ml-3 text-xs text-muted-foreground">Hari Ini</span>
				</div>
				<div class="grid gap-3 p-5 sm:grid-cols-3">
					{#each [1, 2, 3] as n (n)}
						<div class="rounded-lg border border-border bg-background p-4">
							<div class="flex items-center justify-between">
								<span class="h-2.5 w-16 rounded-full bg-muted"></span>
								<span
									class="size-2 rounded-full {n === 1 ? 'bg-ok' : n === 2 ? 'bg-info' : 'bg-na'}"
									aria-hidden="true"
								></span>
							</div>
							<div class="mt-3 space-y-2">
								<div class="h-2 w-full rounded-full bg-muted"></div>
								<div class="h-2 w-2/3 rounded-full bg-muted"></div>
							</div>
							<div class="mt-4 flex gap-2">
								<div class="h-7 flex-1 rounded-md bg-primary/90"></div>
								<div class="h-7 w-7 rounded-md border border-border"></div>
							</div>
						</div>
					{/each}
				</div>
			</div>
		</section>

		<!-- ===================== Masalah ===================== -->
		<section class="border-y border-border bg-muted/30">
			<div class="mx-auto max-w-6xl px-5 py-20 sm:py-24" use:reveal>
				<p class="max-w-3xl text-balance text-2xl font-medium leading-snug tracking-tight sm:text-3xl">
					Lupa satu dosis mungkin terdengar sepele. Tetapi pada pengobatan jangka panjang —
					tekanan darah, diabetes, tuberkulosis — keberhasilan bergantung pada
					<span class="text-primary">konsistensi setiap hari</span>, bukan pada niat baik.
				</p>
				<p class="mt-6 max-w-2xl text-pretty leading-relaxed text-muted-foreground">
					Mencatat di buku mudah hilang, dan mengingat-ingat jadwal di tengah kesibukan
					tidak berkelanjutan. Yang dibutuhkan bukan tekad lebih besar, melainkan alat yang
					tepat.
				</p>
			</div>
		</section>

		<!-- ===================== Fitur ===================== -->
		<section id="fitur" class="mx-auto max-w-6xl scroll-mt-20 px-5 py-20 sm:py-28">
			<h2 class="text-3xl font-semibold tracking-tight sm:text-4xl">Yang Anda dapatkan</h2>
			<p class="mt-4 max-w-2xl text-muted-foreground">
				Empat hal inti, dikerjakan dengan baik. Tanpa fitur yang tidak Anda butuhkan.
			</p>

			<div class="mt-12 grid gap-5 sm:grid-cols-2">
				{#each features as feature, i (feature.title)}
					<article
						class="rounded-xl border border-border bg-card p-6 transition-shadow duration-[var(--dur)] hover:shadow-md"
						use:reveal={{ delay: i * 60 }}
					>
						<h3 class="text-base font-semibold tracking-tight">{feature.title}</h3>
						<p class="mt-2.5 text-pretty text-sm leading-relaxed text-muted-foreground">
							{feature.body}
						</p>
					</article>
				{/each}
			</div>
		</section>

		<!-- ===================== Cara kerja ===================== -->
		<section id="cara-kerja" class="scroll-mt-20 border-y border-border bg-muted/30">
			<div class="mx-auto max-w-6xl px-5 py-20 sm:py-28">
				<h2 class="text-3xl font-semibold tracking-tight sm:text-4xl">Cara kerjanya</h2>
				<p class="mt-4 max-w-2xl text-muted-foreground">Tiga langkah, tanpa masa belajar.</p>

				<ol class="mt-12 grid gap-8 sm:grid-cols-3">
					{#each steps as step, i (step.title)}
						<li class="flex flex-col gap-3" use:reveal={{ delay: i * 80 }}>
							<span
								class="grid size-9 place-items-center rounded-full border border-border bg-background text-sm font-semibold tabular-nums"
								aria-hidden="true"
							>
								{i + 1}
							</span>
							<h3 class="text-base font-semibold tracking-tight">{step.title}</h3>
							<p class="text-pretty text-sm leading-relaxed text-muted-foreground">
								{step.body}
							</p>
						</li>
					{/each}
				</ol>
			</div>
		</section>

		<!-- ===================== Privasi & keamanan ===================== -->
		<section id="privasi" class="mx-auto max-w-6xl scroll-mt-20 px-5 py-20 sm:py-28">
			<div class="grid gap-12 lg:grid-cols-2 lg:gap-16">
				<div>
					<h2 class="text-3xl font-semibold tracking-tight sm:text-4xl">
						Data kesehatan Anda, di tangan Anda
					</h2>
					<p class="mt-5 text-pretty leading-relaxed text-muted-foreground">
						Ini bukan aplikasi yang menjadikan kebiasaan Anda sebagai bahan jualan. Proyeknya
						terbuka, dan Anda yang menentukan di mana datanya tinggal.
					</p>
					<Button variant="outline" class="mt-7" href={GITHUB_URL} rel="noopener noreferrer">
						Baca kodenya
					</Button>
				</div>

				<ul class="space-y-5">
					{#each privacyPoints as point, i (point)}
						<li class="flex gap-3.5" use:reveal={{ delay: i * 60 }}>
							<span
								class="mt-0.5 grid size-5 shrink-0 place-items-center rounded-full bg-primary/12 text-primary"
								aria-hidden="true"
							>
								<svg viewBox="0 0 24 24" class="size-3.5" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round">
									<path d="m5 12.5 4.5 4.5L19 7" />
								</svg>
							</span>
							<span class="text-pretty text-sm leading-relaxed">{point}</span>
						</li>
					{/each}
				</ul>
			</div>
		</section>

		<!-- ===================== FAQ ===================== -->
		<section id="faq" class="scroll-mt-20 border-t border-border">
			<div class="mx-auto max-w-3xl px-5 py-20 sm:py-28">
				<h2 class="text-3xl font-semibold tracking-tight sm:text-4xl">Pertanyaan umum</h2>
				<div class="mt-10" use:reveal>
					<Accordion items={faq} />
				</div>
			</div>
		</section>

		<!-- ===================== CTA penutup ===================== -->
		<section class="border-t border-border bg-muted/30">
			<div class="mx-auto max-w-6xl px-5 py-20 text-center sm:py-28">
				<h2 class="text-balance text-3xl font-semibold tracking-tight sm:text-4xl">
					Mulai hari ini, satu dosis lebih tertib.
				</h2>
				<p class="mx-auto mt-5 max-w-xl text-pretty text-muted-foreground">
					Buat akun, tambahkan obat pertama Anda, dan biarkan jadwalnya bekerja untuk Anda.
				</p>
				<div class="mt-9 flex flex-wrap items-center justify-center gap-3">
					<Button size="lg" href="/register">Daftar gratis</Button>
					<Button size="lg" variant="ghost" href="/login">Sudah punya akun? Masuk</Button>
				</div>
			</div>
		</section>
	</main>

	<!-- ===================== Footer ===================== -->
	<footer class="border-t border-border">
		<div class="mx-auto flex max-w-6xl flex-col gap-6 px-5 py-12 sm:flex-row sm:items-center sm:justify-between">
			<div class="flex items-center gap-2.5">
				<span
					class="grid size-7 place-items-center rounded-md bg-primary text-primary-foreground"
					aria-hidden="true"
				>
					<svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
						<path d="M10.5 20.5a5 5 0 0 1-7-7l7-7a5 5 0 0 1 7 7Z" />
						<path d="m8.5 8.5 7 7" />
					</svg>
				</span>
				<span class="text-sm font-medium">{APP_NAME}</span>
			</div>

			<nav class="flex flex-wrap gap-x-6 gap-y-2 text-sm text-muted-foreground">
				<a class="transition-colors hover:text-foreground" href={GITHUB_URL} rel="noopener noreferrer">
					GitHub
				</a>
				<a class="transition-colors hover:text-foreground" href="/login">Masuk</a>
				<a class="transition-colors hover:text-foreground" href="/register">Daftar</a>
			</nav>

			<p class="text-xs text-muted-foreground">
				Perangkat lunak bebas. Tanpa iklan, tanpa penjualan data.
			</p>
		</div>
	</footer>
</div>
