/**
 * Uji regresi: Pengaturan — edit profil & toggle kanal notifikasi.
 *
 * MASALAH YANG DIKUNCI DI SINI
 *
 * Halaman Pengaturan hanya MENAMPILKAN nilai sebagai teks statis: nama, nomor
 * HP, zona waktu, dan status kanal notifikasi. Semuanya tidak bisa diubah,
 * padahal `users.update` dan `notifications.updatePrefs` sudah tersedia sejak
 * awal. Akibatnya pengguna tidak bisa memperbaiki namanya sendiri atau
 * mematikan kanal yang mengganggu.
 *
 * Yang diuji bukan keberadaan isiannya, melainkan bahwa nilainya benar-benar
 * TERSIMPAN di server — dan bahwa field yang tidak diubah tidak ikut berubah.
 */
import { chromium } from 'playwright';

const API = 'http://127.0.0.1:8081/api/v1';
const BASE = 'http://127.0.0.1:5173';
const ORIGIN = 'http://localhost:5173';
const password = 'SandiUji123!';

let fail = 0;
const check = (label, ok, detail = '') => {
	console.log(`${ok ? 'OK  ' : 'GAGAL'} ${label}${detail ? ' — ' + detail : ''}`);
	if (!ok) fail++;
};

const email = `setting-${Date.now()}@example.com`;
const tok = (
	await (
		await fetch(`${API}/auth/register`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json', Origin: ORIGIN },
			body: JSON.stringify({ email, password, full_name: 'Nama Awal', timezone: 'Asia/Jakarta' })
		})
	).json()
).data.access_token;
const H = { Authorization: `Bearer ${tok}`, 'Content-Type': 'application/json', Origin: ORIGIN };

/** Membaca profil dari server. */
async function profil() {
	return (await (await fetch(`${API}/users/me`, { headers: H })).json()).data;
}

const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH });
const ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, locale: 'id-ID' });
const page = await ctx.newPage();
const errs = [];
page.on('pageerror', (e) => errs.push(e.message));

await page.goto(`${BASE}/login`, { waitUntil: 'networkidle' });
await page.waitForFunction(
	() => {
		const i = document.querySelector('#email');
		return !!i && !i.disabled;
	},
	null,
	{ timeout: 20000 }
);
await page.fill('#email', email);
await page.fill('#password', password);
await page.click('button[type="submit"]');
await page.waitForURL(/hari-ini/, { timeout: 30000 });

await page.goto(`${BASE}/pengaturan`, { waitUntil: 'networkidle' });
await page.waitForTimeout(2500);

// ── Form profil terisi dari data pengguna ──
const isianNama = page.locator('#p-name');
const isianHP = page.locator('#p-phone');
const pilihTZ = page.locator('#p-tz');

check('isian nama ada', (await isianNama.count()) === 1);
check('isian nama terisi nilai awal', (await isianNama.inputValue()) === 'Nama Awal');
check('isian nomor HP ada', (await isianHP.count()) === 1);
check('pemilih zona waktu ada', (await pilihTZ.count()) === 1);
check(
	'zona waktu terisi nilai awal',
	(await pilihTZ.inputValue()) === 'Asia/Jakarta',
	await pilihTZ.inputValue()
);

// ── Tombol Simpan terkunci sampai ada perubahan ──
const tombolSimpan = page.getByRole('button', { name: 'Simpan profil', exact: true });
check('tombol Simpan terkunci tanpa perubahan', await tombolSimpan.isDisabled());

// ── Ubah nama → tombol aktif → simpan ──
await isianNama.fill('Nama Sudah Diubah');
await page.waitForTimeout(300);
check('tombol Simpan aktif setelah diubah', !(await tombolSimpan.isDisabled()));

await tombolSimpan.click();
await page.waitForTimeout(2500);

const p1 = await profil();
check('nama tersimpan di server', p1.full_name === 'Nama Sudah Diubah', String(p1.full_name));
check('pesan konfirmasi tampil', ((await page.textContent('main')) ?? '').includes('Profil tersimpan'));

// ── Nomor HP + zona waktu ──
await isianHP.fill('+628123456789');
await pilihTZ.selectOption('Asia/Makassar');
await page.waitForTimeout(300);
await tombolSimpan.click();
await page.waitForTimeout(2500);

const p2 = await profil();
check('nomor HP tersimpan', p2.phone === '+628123456789', String(p2.phone));
check('zona waktu tersimpan', p2.timezone === 'Asia/Makassar', String(p2.timezone));
check('email TIDAK ikut berubah', p2.email === email, String(p2.email));

// ── Mengosongkan nomor HP harus bisa (jadi null, bukan string kosong) ──
await isianHP.fill('');
await page.waitForTimeout(300);
await tombolSimpan.click();
await page.waitForTimeout(2500);
const p3 = await profil();
check('nomor HP bisa dikosongkan kembali', p3.phone === null, String(p3.phone));

// ── Toggle kanal notifikasi ──
const saklar = page.locator('button[role="switch"]');
const jumlahSaklar = await saklar.count();
check('ada saklar kanal yang bisa diatur', jumlahSaklar >= 1, `${jumlahSaklar} saklar`);

if (jumlahSaklar > 0) {
	const saklarPertama = saklar.first();
	const sebelum = await saklarPertama.getAttribute('aria-checked');

	await saklarPertama.click();
	await page.waitForTimeout(2500);

	const prefs = (await (await fetch(`${API}/notification-prefs`, { headers: H })).json()).data ?? [];
	const kanalInApp = prefs.find((p) => p.channel === 'in_app');
	check(
		'status kanal berubah di server',
		String(kanalInApp?.enabled) !== sebelum,
		`sebelum ${sebelum}, sekarang ${kanalInApp?.enabled}`
	);

	// Kembalikan supaya tidak mengganggu uji lain.
	await page.reload({ waitUntil: 'networkidle' });
	await page.waitForTimeout(2000);
	await page.locator('button[role="switch"]').first().click();
	await page.waitForTimeout(2000);
}

// ── Kotak masuk ──
check('seksi kotak masuk tampil', ((await page.textContent('main')) ?? '').includes('Kotak masuk'));

check('tidak ada error JS', errs.length === 0, errs.slice(0, 2).join(' | '));

await browser.close();
console.log('');
if (fail === 0) console.log('e2e-pengaturan-form: SEMUA LULUS');
else console.log(`e2e-pengaturan-form: ${fail} GAGAL`);
process.exit(fail === 0 ? 0 : 1);
