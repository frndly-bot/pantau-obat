/**
 * Uji regresi: persetujuan medis di halaman daftar.
 *
 * MASALAH YANG DIKUNCI DI SINI
 *
 * Prototipe design mewajibkan pengguna mencentang "Saya mengerti aplikasi ini
 * alat bantu, bukan pengganti nasihat dokter" sebelum bisa mendaftar.
 * Implementasi Svelte tidak punya checkbox itu sama sekali, sehingga siapa
 * pun bisa membuat akun tanpa pernah diberi tahu batas kemampuan aplikasi —
 * padahal isinya menyangkut keputusan pengobatan, dan aplikasi ini memang
 * tidak memberi saran dosis maupun memeriksa interaksi obat.
 *
 * Yang diuji bukan keberadaan checkbox di markup, melainkan bahwa tombol
 * daftar benar-benar TERKUNCI sebelum dicentang. Checkbox yang ada tapi tidak
 * mengunci apa pun tidak memenuhi maksudnya.
 */
import { chromium } from 'playwright';

const BASE = 'http://127.0.0.1:5173';

let fail = 0;
const check = (label, ok, detail = '') => {
	console.log(`${ok ? 'OK  ' : 'GAGAL'} ${label}${detail ? ' — ' + detail : ''}`);
	if (!ok) fail++;
};

const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH });
const ctx = await browser.newContext({ viewport: { width: 390, height: 844 } });
const page = await ctx.newPage();
const errs = [];
page.on('pageerror', (e) => errs.push(e.message));

await page.goto(`${BASE}/register`, { waitUntil: 'networkidle' });
await page.waitForFunction(
	() => {
		const i = document.querySelector('#email');
		return !!i && !i.disabled;
	},
	null,
	{ timeout: 20000 }
);

// Checkbox persetujuan harus ada dan punya teks yang menyebut batas aplikasi.
const kotak = page.locator('input[type="checkbox"]').first();
check('checkbox persetujuan ada', (await kotak.count()) === 1);

const teksHalaman = (await page.textContent('body')) ?? '';
check('teks menyebut "alat bantu"', teksHalaman.includes('alat bantu'));
check('teks menyebut bukan pengganti nasihat dokter', teksHalaman.includes('bukan pengganti'));
check(
	'teks menyebut tidak ada saran dosis / interaksi obat',
	teksHalaman.includes('Tidak ada saran dosis')
);

// Tombol daftar harus TERKUNCI sebelum dicentang.
const tombol = page.locator('button[type="submit"]').first();
check('tombol daftar terkunci sebelum dicentang', await tombol.isDisabled());

// Isi form lengkap, tombol harus tetap terkunci.
await page.fill('#full_name', 'Uji Persetujuan');
await page.fill('#email', `setuju-${Date.now()}@example.com`);
await page.fill('#password', 'SandiUji123!');
await page.waitForTimeout(300);
check(
	'tombol tetap terkunci walau form sudah lengkap (belum centang)',
	await tombol.isDisabled()
);

// Centang → tombol harus aktif.
await kotak.check();
await page.waitForTimeout(300);
check('tombol aktif setelah dicentang', !(await tombol.isDisabled()));

// Batal centang → terkunci lagi.
await kotak.uncheck();
await page.waitForTimeout(300);
check('tombol terkunci lagi setelah centang dibatalkan', await tombol.isDisabled());

// Centang lagi lalu daftar sungguhan harus berhasil.
await kotak.check();
await page.waitForTimeout(200);
await tombol.click();
await page.waitForURL(/hari-ini/, { timeout: 25000 }).catch(() => {});
check('pendaftaran berhasil setelah dicentang', page.url().includes('hari-ini'), page.url());

check('tidak ada error JS', errs.length === 0, errs.slice(0, 2).join(' | '));

await browser.close();
console.log('');
if (fail === 0) console.log('e2e-persetujuan: SEMUA LULUS');
else console.log(`e2e-persetujuan: ${fail} GAGAL`);
process.exit(fail === 0 ? 0 : 1);
