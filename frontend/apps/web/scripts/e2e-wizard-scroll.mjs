/**
 * Uji regresi: posisi gulir saat berpindah langkah di wizard tambah obat.
 *
 * MASALAH YANG DIKUNCI DI SINI
 *
 * Dialog mempertahankan posisi gulir saat isinya berganti. Pada wizard,
 * setiap langkah punya isi yang sama sekali berbeda. Bila pengguna menggulir
 * ke bawah di langkah 2 lalu menekan "Lanjut", posisi gulir itu terbawa ke
 * langkah 3 — dan karena isi langkah 3 lebih pendek, posisi itu terpotong ke
 * batas bawahnya. Hasilnya: pengguna mendarat di DASAR form langkah baru,
 * bukan di awalnya. Diukur sebelum perbaikan: langkah 2 di `scrollTop 118`,
 * langkah 3 mulai di `scrollTop 12` (batas maksimum kontennya).
 *
 * Yang diperiksa: setelah pindah langkah, posisi gulir HARUS 0. Diuji untuk
 * maju, mundur, dan melompat lewat penunjuk langkah — karena ketiganya jalur
 * berbeda yang bisa lupa diperbaiki satu per satu.
 */
import { chromium } from 'playwright';

const API = 'http://127.0.0.1:8081/api/v1';
const BASE = 'http://127.0.0.1:5173';
const ORIGIN = 'http://localhost:5173';
const password = 'SandiUji123!';

let fail = 0;
const check = (l, ok, d = '') => {
	console.log(`${ok ? 'OK  ' : 'GAGAL'} ${l}${d ? ' — ' + d : ''}`);
	if (!ok) fail++;
};

const email = `scroll-${Date.now()}@example.com`;
await fetch(`${API}/auth/register`, {
	method: 'POST',
	headers: { 'Content-Type': 'application/json', Origin: ORIGIN },
	body: JSON.stringify({ email, password, full_name: 'Uji Scroll', timezone: 'Asia/Jakarta' })
});

const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH });
const ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 2 });
const page = await ctx.newPage();
const errs = [];
page.on('pageerror', (e) => errs.push(e.message));

await page.goto(`${BASE}/login`, { waitUntil: 'networkidle' });
await page.waitForFunction(
	() => {
		const i = document.querySelector('#email');
		const b = document.querySelector('button[type="submit"]');
		return !!i && !i.disabled && !!b && !b.disabled;
	},
	null,
	{ timeout: 20000 }
);
await page.fill('#email', email);
await page.fill('#password', password);
await page.click('button[type="submit"]');
await page.waitForURL(/hari-ini/, { timeout: 25000 });

await page.goto(`${BASE}/obat`, { waitUntil: 'networkidle' });
await page.waitForTimeout(1000);
await page.locator('button:has-text("Tambah obat")').first().click();
await page.waitForTimeout(1200);

/** Membaca keadaan wadah gulir di dalam dialog. */
function baca() {
	return page.evaluate(() => {
		const dialog = document.querySelector('[role="dialog"]');
		if (!dialog) return { ada: false };
		let sc = null;
		for (const el of dialog.querySelectorAll('*')) {
			const s = getComputedStyle(el);
			if (s.overflowY === 'auto' || s.overflowY === 'scroll') {
				sc = el;
				break;
			}
		}
		if (!sc) return { ada: true, wadahGulir: false };
		return {
			ada: true,
			wadahGulir: true,
			scrollTop: Math.round(sc.scrollTop),
			bisaGulir: sc.scrollHeight > sc.clientHeight + 1,
			langkah: dialog.querySelector('p')?.textContent?.trim().slice(0, 30) ?? ''
		};
	});
}

/** Menggulir wadah dialog sampai dasar, meniru pengguna mengisi kolom bawah. */
function gulirKeBawah() {
	return page.evaluate(() => {
		const dialog = document.querySelector('[role="dialog"]');
		for (const el of dialog.querySelectorAll('*')) {
			const s = getComputedStyle(el);
			if (s.overflowY === 'auto' || s.overflowY === 'scroll') {
				el.scrollTop = el.scrollHeight;
				return Math.round(el.scrollTop);
			}
		}
		return -1;
	});
}

const lanjut = () => page.locator('button:has-text("Lanjut")').click();
const kembali = () => page.locator('button:has-text("Kembali")').click();

// ── Langkah 1 → 2 ──
await page.fill('#m-name', 'Obat Uji Scroll');
await lanjut();
await page.waitForTimeout(500);
let s = await baca();
check('langkah 2 mulai dari atas', s.scrollTop === 0, `scrollTop ${s.scrollTop}`);

// ── Langkah 2 digulir ke dasar, lalu 2 → 3 (kasus yang dilaporkan) ──
const dasar = await gulirKeBawah();
await page.waitForTimeout(300);
s = await baca();
check('langkah 2 benar-benar bisa digulir (prasyarat uji)', dasar > 0, `scrollTop ${s.scrollTop}`);

await lanjut();
await page.waitForTimeout(600);
s = await baca();
check(
	'langkah 3 mulai dari ATAS setelah digulir di langkah 2',
	s.scrollTop === 0,
	`scrollTop ${s.scrollTop} dari maksimum ~${s.bisaGulir ? 'lebih dari 0' : '0'}`
);

// ── Mundur: 3 → 2 harus mulai dari atas juga ──
await gulirKeBawah();
await page.waitForTimeout(300);
await kembali();
await page.waitForTimeout(600);
s = await baca();
check('kembali ke langkah 2 juga mulai dari atas', s.scrollTop === 0, `scrollTop ${s.scrollTop}`);

// ── Melompat lewat penunjuk langkah (jalur ketiga) ──
await gulirKeBawah();
await page.waitForTimeout(300);
// Penunjuk langkah punya aria-label "Langkah N: judul".
await page.getByRole('button', { name: /^Langkah 4:/ }).click();
await page.waitForTimeout(600);
s = await baca();
check('lompat lewat penunjuk langkah juga mulai dari atas', s.scrollTop === 0, `scrollTop ${s.scrollTop}`);

check('tidak ada error JS', errs.length === 0, errs.slice(0, 2).join(' | '));

await browser.close();
console.log('');
if (fail === 0) console.log('e2e-wizard-scroll: SEMUA LULUS');
else console.log(`e2e-wizard-scroll: ${fail} GAGAL`);
process.exit(fail === 0 ? 0 : 1);
