/**
 * Verifikasi manual dengan AKUN SAMPEL yang diberikan pemilik proyek.
 *
 * Berbeda dari suite lain yang membuat akun baru, uji ini memakai akun
 * sungguhan supaya terlihat apa yang benar-benar dialami pengguna: data yang
 * sudah ada, bukan data bersih.
 *
 * Karena akun ini dipakai berulang, uji ini TIDAK mengubah apa pun —
 * hanya membaca dan memeriksa tampilan. Tindakan yang mengubah data
 * (mencatat dosis, menambah obat) sengaja tidak dilakukan di sini.
 */
import { chromium } from 'playwright';

const BASE = 'http://127.0.0.1:5173';
const EMAIL = process.env.SAMPLE_EMAIL;
const PASSWORD = process.env.SAMPLE_PASSWORD;

if (!EMAIL || !PASSWORD) {
	console.error('SAMPLE_EMAIL dan SAMPLE_PASSWORD harus diisi lewat environment.');
	process.exit(2);
}

let fail = 0;
const check = (l, ok, d = '') => {
	console.log(`${ok ? 'OK  ' : 'GAGAL'} ${l}${d ? ' — ' + d : ''}`);
	if (!ok) fail++;
};

const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH });
const ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 2 });
const page = await ctx.newPage();

const errs = [];
page.on('pageerror', (e) => errs.push(e.message));
page.on('console', (m) => {
	if (m.type() === 'error' && !/401|Failed to load resource/i.test(m.text())) errs.push(m.text());
});

// ── Masuk ──
console.log('── Masuk dengan akun sampel ──');
await page.goto(`${BASE}/login`);
await page.waitForFunction(
	() => {
		const i = document.querySelector('#email');
		const b = document.querySelector('button[type="submit"]');
		return !!i && !i.disabled && !!b && !b.disabled;
	},
	null,
	{ timeout: 20000 }
);
await page.fill('#email', EMAIL);
await page.fill('#password', PASSWORD);
await page.click('button[type="submit"]');
await page.waitForURL(/hari-ini/, { timeout: 25000 });
check('masuk berhasil', true);

// ── Hari Ini ──
console.log('\n── Hari Ini ──');
await page.waitForTimeout(1500);
const hariIni = await page.locator('body').innerText();
check('halaman Hari Ini terbuka', /hari ini/i.test(hariIni));
const adaSlot = /minum|dosis|obat|tidak ada|belum/i.test(hariIni);
check('konten Hari Ini tergambar', adaSlot, hariIni.slice(0, 80).replace(/\n+/g, ' | '));

// ── Refresh: sesi harus bertahan ──
console.log('\n── Muat ulang halaman (uji sesi) ──');
await page.reload({ waitUntil: 'networkidle' });
await page.waitForTimeout(1500);
check('sesi bertahan setelah refresh (tidak kembali ke login)', /hari-ini/.test(page.url()), page.url());

// ── Obat ──
console.log('\n── Obat ──');
await page.goto(`${BASE}/obat`, { waitUntil: 'networkidle' });
await page.waitForTimeout(1200);
const obat = await page.locator('body').innerText();
check('halaman Obat terbuka', /obat/i.test(obat));
check('tidak ada pesan error memuat', !/gagal memuat|tidak dapat menghubungi/i.test(obat), '');

// ── Riwayat ──
console.log('\n── Riwayat ──');
await page.goto(`${BASE}/riwayat`, { waitUntil: 'networkidle' });
await page.waitForTimeout(1200);
const riwayat = await page.locator('body').innerText();
check('halaman Riwayat terbuka', /riwayat/i.test(riwayat));

// ── Caregiver ──
console.log('\n── Caregiver ──');
await page.goto(`${BASE}/caregiver`, { waitUntil: 'networkidle' });
await page.waitForTimeout(1200);
const caregiver = await page.locator('body').innerText();
check('halaman Caregiver terbuka', /caregiver/i.test(caregiver));
check(
	'seksi "Undangan untuk Anda" tampil (perbaikan baru)',
	/undangan untuk anda/i.test(caregiver),
	caregiver.slice(0, 100).replace(/\n+/g, ' | ')
);

// ── Pengaturan ──
console.log('\n── Pengaturan ──');
await page.goto(`${BASE}/pengaturan`, { waitUntil: 'networkidle' });
await page.waitForTimeout(1200);
const pengaturan = await page.locator('body').innerText();
check('halaman Pengaturan terbuka', /pengaturan|profil|sandi/i.test(pengaturan));
check(
	'email akun sampel tampil',
	pengaturan.toLowerCase().includes(EMAIL.toLowerCase()),
	''
);

// ── Navigasi bawah ──
console.log('\n── Navigasi bawah ──');
const navItems = await page.locator('nav a, [class*="bottomnav"] a').count();
check('navigasi bawah tersedia', navItems >= 4, `${navItems} tautan`);

// ── Halaman publik ──
console.log('\n── Halaman publik ──');
for (const [jalur, harapan] of [
	['/', /pantau obat/i],
	['/login', /masuk/i],
	['/register', /daftar|buat akun/i]
]) {
	await page.goto(`${BASE}${jalur}`, { waitUntil: 'networkidle' });
	const t = await page.locator('body').innerText();
	check(`${jalur} tampil`, harapan.test(t));
}

// ── Sesi bertahan setelah keluar-masuk ──
console.log('\n── Keluar dan masuk lagi ──');
await page.goto(`${BASE}/pengaturan`, { waitUntil: 'networkidle' });
const keluar = page.locator('button:has-text("Keluar"), a:has-text("Keluar")').first();
if (await keluar.count()) {
	await keluar.click();
	await page.waitForURL(/\/$|\/login/, { timeout: 20000 });
	check('keluar berhasil', !/pengaturan/.test(page.url()), page.url());
} else {
	check('tombol keluar ditemukan', false);
}

check('tidak ada error JS', errs.length === 0, errs.slice(0, 2).join(' | '));

await browser.close();
console.log('');
if (fail === 0) console.log('verifikasi-akun-sampel: SEMUA LULUS');
else console.log(`verifikasi-akun-sampel: ${fail} GAGAL`);
process.exit(fail === 0 ? 0 : 1);
