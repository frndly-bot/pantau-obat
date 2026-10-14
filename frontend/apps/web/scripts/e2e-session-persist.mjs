/**
 * Uji: apakah sesi bertahan setelah halaman di-refresh?
 *
 * Diuji untuk DUA kombinasi alamat, karena cookie refresh memakai
 * SameSite=Strict dan "localhost" berbeda situs dengan "127.0.0.1" menurut
 * browser:
 *   A. frontend 127.0.0.1:5173 → API 127.0.0.1:8081  (sesama situs)
 *   B. frontend localhost:5173 → API 127.0.0.1:8081  (beda situs)
 *
 * Yang diuji:
 *   1. login → refresh halaman → masih masuk?
 *   2. tutup konteks, buka konteks baru (seperti buka ulang browser) → masih masuk?
 *   3. access token memang hilang (di memori) tapi sesi pulih lewat cookie
 */
import { chromium } from 'playwright';

const API = 'http://127.0.0.1:8081/api/v1';
const ORIGIN_FOR_API = 'http://127.0.0.1:5173';
const password = 'SandiUji123!';

let failures = 0;
function check(label, ok, detail = '') {
	console.log(`  ${ok ? 'OK  ' : 'GAGAL'} ${label}${detail ? ' — ' + detail : ''}`);
	if (!ok) failures++;
}

/** Membuat satu akun uji lewat API. */
async function makeAccount(tag) {
	const email = `${tag}-${Date.now()}-${Math.random().toString(36).slice(2, 7)}@example.com`;
	const r = await fetch(`${API}/auth/register`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json', Origin: ORIGIN_FOR_API },
		body: JSON.stringify({ email, password, full_name: 'Uji Sesi', timezone: 'Asia/Jakarta' }),
	});
	if (r.status !== 201) throw new Error(`register gagal: ${r.status}`);
	return email;
}

async function testCase(label, webBase) {
	console.log(`\n=== ${label} (${webBase}) ===`);
	const email = await makeAccount('sesi');
	const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH });
	const context = await browser.newContext({ viewport: { width: 390, height: 844 }, locale: 'id-ID' });
	const page = await context.newPage();

	// Login lewat UI.
	await page.goto(`${webBase}/login`, { waitUntil: 'networkidle' });
	await page.fill('#email', email);
	await page.fill('#password', password);
	await page.click('button[type="submit"]');
	await page.waitForURL(/hari-ini/, { timeout: 20000 }).catch(() => {});
	check('login berhasil masuk /hari-ini', page.url().includes('hari-ini'), page.url());

	// 1. Refresh halaman.
	await page.reload({ waitUntil: 'networkidle' });
	await page.waitForTimeout(1500);
	const afterReload = page.url().includes('hari-ini');
	check('setelah REFRESH masih di /hari-ini', afterReload, page.url());

	// 2. Cookie apa saja yang dipegang browser.
	const cookies = await context.cookies();
	const refresh = cookies.find((c) => c.name === 'po_refresh');
	const marker = cookies.find((c) => c.name === 'po_session');
	check('cookie po_refresh ada', !!refresh, refresh ? `SameSite=${refresh.sameSite} Secure=${refresh.secure}` : 'tidak ada');
	check('cookie po_refresh HttpOnly', refresh?.httpOnly === true);
	check('penanda po_session ada', !!marker);

	// 3. Konteks baru = seperti menutup lalu membuka ulang browser.
	//    Storage (cookie) disalin, memori JavaScript TIDAK.
	const state = await context.storageState();
	const ctx2 = await browser.newContext({ viewport: { width: 390, height: 844 }, storageState: state });
	const page2 = await ctx2.newPage();
	await page2.goto(`${webBase}/hari-ini`, { waitUntil: 'networkidle' });
	await page2.waitForTimeout(1800);
	check('setelah buka ulang browser masih masuk', page2.url().includes('hari-ini'), page2.url());

	// 4. Bukti access token memang hilang dari memori, sesi tetap pulih.
	const ls = await page2.evaluate(() => JSON.stringify(Object.entries(localStorage)));
	check('tidak ada token di localStorage', !/eyJ/.test(ls), ls.slice(0, 40));

	await browser.close();
}

await testCase('A. frontend 127.0.0.1 (sesama situs dengan API)', 'http://127.0.0.1:5173');
await testCase('B. frontend localhost (BEDA situs dengan API)', 'http://localhost:5173');

console.log(failures === 0 ? '\nSEMUA LULUS' : `\n${failures} GAGAL`);
process.exit(failures === 0 ? 0 : 1);
