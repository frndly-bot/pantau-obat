/**
 * Uji alur LUPA SANDI lewat peramban sungguhan.
 *
 * Yang diuji di sini adalah hal-hal yang tidak terlihat dari uji API:
 * halaman benar-benar ada (bukan 404), tautan dari EMAIL bisa dipakai
 * sampai sandi benar-benar berganti, dan layarnya tidak membocorkan
 * apakah sebuah email terdaftar.
 *
 * Dijalankan terhadap server yang sudah jalan; alamatnya dari env BASE_URL.
 */
import { chromium } from 'playwright';

const BASE = process.env.BASE_URL ?? 'http://127.0.0.1:5173';
const MAILHOG = process.env.MAILHOG_URL ?? 'http://127.0.0.1:8025';
const email = process.env.UJI_EMAIL ?? `pulih-${Date.now()}@example.com`;
const sandiLama = 'SandiLama123!';
const sandiBaru = 'SandiBaru456!';

const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH });
const ctx = await browser.newContext({ locale: 'id-ID' });
const page = await ctx.newPage();

let lulus = 0;
let gagal = 0;
function cek(nama, syarat, info = '') {
	if (syarat) {
		lulus++;
		console.log(`  ✓ ${nama}`);
	} else {
		gagal++;
		console.log(`  ✗ ${nama}${info ? ` — ${info}` : ''}`);
	}
}

/** Membuat akun lewat API supaya uji ini fokus pada alur pemulihan. */
async function daftar(email) {
	const r = await fetch(`${BASE}/api/v1/auth/register`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({
			email,
			password: sandiLama,
			full_name: 'Uji Pemulihan',
			timezone: 'Asia/Jakarta'
		})
	});
	return r.ok;
}

/** Mengambil tautan reset terbaru untuk satu alamat dari MailHog. */
async function tautanReset(email) {
	for (let i = 0; i < 20; i++) {
		const r = await fetch(`${MAILHOG}/api/v2/messages`);
		const d = await r.json();
		for (const m of d.items ?? []) {
			const ke = `${m.To[0].Mailbox}@${m.To[0].Domain}`;
			if (ke !== email) continue;
			const mt = /https?:\/\/[^\s]+\/reset-sandi\?token=[A-Za-z0-9_-]+/.exec(
				m.Content?.Body ?? ''
			);
			if (mt) return mt[0];
		}
		await new Promise((r) => setTimeout(r, 500));
	}
	return null;
}

try {
	console.log(`\n=== Persiapan: akun ${email} ===`);
	cek('akun uji terdaftar', await daftar(email));

	console.log('\n=== 1. Halaman /lupa-sandi ada (bukan 404) ===');
	await page.goto(`${BASE}/lupa-sandi`, { waitUntil: 'networkidle' });
	const teksLupa = (await page.locator('body').innerText()).replace(/\s+/g, ' ');
	cek('bukan halaman 404', !/404/.test(teksLupa), teksLupa.slice(0, 80));
	cek('judul tampil', /Lupa sandi/i.test(teksLupa));
	cek('field email ada', (await page.locator('#email').count()) === 1);

	console.log('\n=== 2. Tautan dari halaman login menuju /lupa-sandi ===');
	await page.goto(`${BASE}/login`, { waitUntil: 'networkidle' });
	const adaTautan = (await page.locator('a[href="/lupa-sandi"]').count()) > 0;
	cek('tautan "Lupa sandi?" ada di halaman masuk', adaTautan);
	if (adaTautan) {
		await page.locator('a[href="/lupa-sandi"]').first().click();
		await page.waitForURL('**/lupa-sandi', { timeout: 15000 });
		cek('klik tautan membuka halaman lupa sandi', true);
	}

	console.log('\n=== 3. Kirim permintaan pemulihan ===');
	await page.locator('#email').fill(email);
	await page.getByRole('button', { name: /kirim tautan/i }).click();
	await page.waitForTimeout(2500);
	const teksTerkirim = (await page.locator('body').innerText()).replace(/\s+/g, ' ');
	cek('pesan "periksa email" tampil', /periksa email/i.test(teksTerkirim));
	// Ini yang menjaga anti-enumerasi: layar tidak boleh menyatakan alamatnya
	// ada maupun tidak ada.
	cek(
		'tidak membocorkan apakah email terdaftar',
		/tidak terdaftar|tidak ditemukan/i.test(teksTerkirim) === false,
		teksTerkirim.slice(0, 120)
	);

	console.log('\n=== 4. Alamat yang TIDAK terdaftar diperlakukan sama ===');
	await page.goto(`${BASE}/lupa-sandi`, { waitUntil: 'networkidle' });
	await page.locator('#email').fill(`hantu-${Date.now()}@example.com`);
	await page.getByRole('button', { name: /kirim tautan/i }).click();
	await page.waitForTimeout(2500);
	const teksHantu = (await page.locator('body').innerText()).replace(/\s+/g, ' ');
	cek('tampilan sama untuk email tak terdaftar', /periksa email/i.test(teksHantu));
	cek(
		'tidak menyatakan email tidak ditemukan',
		/tidak terdaftar|tidak ditemukan/i.test(teksHantu) === false
	);

	console.log('\n=== 5. Tautan dari email bisa dipakai ===');
	const tautan = await tautanReset(email);
	cek('tautan reset diterima di MailHog', Boolean(tautan), tautan ? '' : 'tidak ada surat');

	if (tautan) {
		// Tautan menunjuk ke APP_PUBLIC_URL; di uji lokal kita arahkan ke BASE.
		const jalur = new URL(tautan);
		await page.goto(`${BASE}${jalur.pathname}${jalur.search}`, { waitUntil: 'networkidle' });
		const teksReset = (await page.locator('body').innerText()).replace(/\s+/g, ' ');
		cek('halaman /reset-sandi terbuka', !/404/.test(teksReset), teksReset.slice(0, 80));
		cek('form sandi baru tampil', (await page.locator('#password').count()) === 1);
		cek('field ulangi sandi ada', (await page.locator('#konfirmasi').count()) === 1);

		console.log('\n=== 6. Sandi yang tidak sama ditolak di klien ===');
		await page.locator('#password').fill(sandiBaru);
		await page.locator('#konfirmasi').fill('Berbeda123!');
		await page.getByRole('button', { name: /simpan sandi baru/i }).click();
		await page.waitForTimeout(1200);
		const teksBeda = (await page.locator('body').innerText()).replace(/\s+/g, ' ');
		cek('pesan "tidak sama" tampil', /tidak sama/i.test(teksBeda));

		console.log('\n=== 7. Sandi baru benar-benar tersimpan ===');
		await page.locator('#password').fill(sandiBaru);
		await page.locator('#konfirmasi').fill(sandiBaru);
		await page.getByRole('button', { name: /simpan sandi baru/i }).click();
		await page.waitForTimeout(2500);
		const teksSukses = (await page.locator('body').innerText()).replace(/\s+/g, ' ');
		cek('konfirmasi sandi berganti tampil', /sandi sudah diganti/i.test(teksSukses));

		// Dibuktikan lewat API: masuk dengan sandi baru harus berhasil.
		const masukBaru = await fetch(`${BASE}/api/v1/auth/login`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ email, password: sandiBaru })
		});
		cek('masuk dengan sandi BARU berhasil', masukBaru.ok, `status ${masukBaru.status}`);

		const masukLama = await fetch(`${BASE}/api/v1/auth/login`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ email, password: sandiLama })
		});
		cek('masuk dengan sandi LAMA ditolak', !masukLama.ok);

		console.log('\n=== 8. Token tidak bisa dipakai dua kali ===');
		await page.goto(`${BASE}${jalur.pathname}${jalur.search}`, { waitUntil: 'networkidle' });
		await page.locator('#password').fill('SandiLain789!');
		await page.locator('#konfirmasi').fill('SandiLain789!');
		await page.getByRole('button', { name: /simpan sandi baru/i }).click();
		await page.waitForTimeout(2500);
		const teksUlang = (await page.locator('body').innerText()).replace(/\s+/g, ' ');
		cek(
			'token bekas ditolak dengan pesan yang jelas',
			/kedaluwarsa|tidak berlaku/i.test(teksUlang),
			teksUlang.slice(0, 120)
		);
	}
} catch (e) {
	gagal++;
	console.log(`\n  ✗ uji terhenti: ${e.message}`);
} finally {
	await browser.close();
}

console.log(`\n${'='.repeat(50)}`);
console.log(`LULUS: ${lulus}   GAGAL: ${gagal}`);
process.exit(gagal === 0 ? 0 : 1);
