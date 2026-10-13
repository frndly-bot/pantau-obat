/**
 * Uji regresi aksesibilitas: manajemen fokus dialog.
 *
 * MASALAH YANG DIKUNCI DI SINI
 *
 * Komponen Dialog dulu hanya menangani tombol Escape. Tiga hal tidak ada,
 * dan ketiganya hanya terasa bagi pengguna keyboard atau pembaca layar —
 * di layar tampak baik-baik saja:
 *
 *   1. Fokus tidak dipindahkan ke dalam dialog saat dibuka. Pengguna harus
 *      menekan Tab berkali-kali menembus elemen di belakang dialog.
 *   2. Fokus tidak ditahan di dalam dialog. Tab bisa keluar ke halaman yang
 *      secara visual tertutup tirai.
 *   3. Fokus tidak dikembalikan saat dialog ditutup. Setiap selesai memakai
 *      satu modal, navigasi harus diulang dari awal — dan modal dipakai
 *      berulang kali untuk mencatat dosis.
 *
 * Uji ini memakai keyboard sungguhan (Tab, Shift+Tab, Escape) lalu membaca
 * `document.activeElement`, bukan memeriksa atribut di markup.
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

const email = `a11y-${Date.now()}@example.com`;
await fetch(`${API}/auth/register`, {
	method: 'POST',
	headers: { 'Content-Type': 'application/json', Origin: ORIGIN },
	body: JSON.stringify({ email, password, full_name: 'Uji Fokus', timezone: 'Asia/Jakarta' })
});

const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH });
const ctx = await browser.newContext({ viewport: { width: 390, height: 844 } });
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

// ── Buka dialog dari halaman Obat (wizard tambah obat) ──
await page.goto(`${BASE}/obat`, { waitUntil: 'networkidle' });
await page.waitForTimeout(1000);

const trigger = page.locator('button:has-text("Tambah obat")').first();
await trigger.focus();
const triggerText = (await trigger.textContent())?.trim() ?? '';

await trigger.click();
await page.waitForTimeout(800);

/** Membaca keadaan fokus: apakah di dalam dialog, dan pada elemen apa. */
const focusState = () =>
	page.evaluate(() => {
		const dialog = document.querySelector('[role="dialog"]');
		const active = document.activeElement;
		if (!dialog) return { dialogAda: false, diDalam: false, tag: '', teks: '' };
		return {
			dialogAda: true,
			diDalam: dialog.contains(active),
			tag: active?.tagName.toLowerCase() ?? '',
			teks: (active?.textContent ?? active?.getAttribute?.('aria-label') ?? '')
				.toString()
				.trim()
				.slice(0, 30)
		};
	});

// 1. Fokus harus pindah ke dalam dialog
let s = await focusState();
check('dialog terbuka', s.dialogAda);
check('fokus berpindah KE DALAM dialog saat dibuka', s.diDalam, `${s.tag} "${s.teks}"`);

// 2. Tab harus tetap di dalam dialog (keliling beberapa kali)
let stayedInside = true;
const visited = [];
for (let i = 0; i < 8; i++) {
	await page.keyboard.press('Tab');
	await page.waitForTimeout(80);
	const f = await focusState();
	visited.push(f.teks || f.tag);
	if (!f.diDalam) {
		stayedInside = false;
		break;
	}
}
check('fokus TERTAHAN di dalam dialog setelah 8× Tab', stayedInside, `dilewati: ${visited.join(' → ')}`);

// 3. Shift+Tab juga harus tertahan (arah sebaliknya)
let stayedBack = true;
for (let i = 0; i < 4; i++) {
	await page.keyboard.press('Shift+Tab');
	await page.waitForTimeout(80);
	const f = await focusState();
	if (!f.diDalam) {
		stayedBack = false;
		break;
	}
}
check('fokus TERTAHAN saat Shift+Tab (arah sebaliknya)', stayedBack);

// 4. Escape menutup dialog DAN mengembalikan fokus ke tombol pemicu
await page.keyboard.press('Escape');
await page.waitForTimeout(500);

const after = await page.evaluate(() => {
	const active = document.activeElement;
	return {
		dialogAda: !!document.querySelector('[role="dialog"]'),
		teks: (active?.textContent ?? '').toString().trim().slice(0, 30),
		tag: active?.tagName.toLowerCase() ?? ''
	};
});

check('Escape menutup dialog', !after.dialogAda);
check(
	'fokus DIKEMBALIKAN ke tombol pemicu',
	after.teks.includes('Tambah obat') || after.teks === triggerText,
	`fokus sekarang: ${after.tag} "${after.teks}" (pemicu: "${triggerText}")`
);

check('tidak ada error JS', errs.length === 0, errs.slice(0, 2).join(' | '));

await browser.close();
console.log('');
if (fail === 0) console.log('e2e-a11y-dialog: SEMUA LULUS');
else console.log(`e2e-a11y-dialog: ${fail} GAGAL`);
process.exit(fail === 0 ? 0 : 1);
