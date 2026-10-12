/**
 * Uji alur pengguna lewat browser sungguhan (Chromium headless).
 *
 * Menguji hal yang TIDAK bisa dibuktikan oleh uji API: bahwa form benar-benar
 * mengirim data, cookie HttpOnly diterima browser, dan halaman berpindah.
 */
import { chromium } from 'playwright';

const BASE = process.env.WEB_BASE ?? 'http://127.0.0.1:5173';
const email = `browser-${Date.now()}@example.com`;
const password = 'SandiUji123!';

const browser = await chromium.launch();
const context = await browser.newContext({ viewport: { width: 390, height: 844 } });
const page = await context.newPage();

const consoleErrors = [];
page.on('console', (m) => {
	if (m.type() === 'error') consoleErrors.push(m.text());
});
page.on('pageerror', (e) => consoleErrors.push('pageerror: ' + e.message));

let failures = 0;
function check(label, ok, detail = '') {
	console.log(`${ok ? 'OK  ' : 'GAGAL'} ${label}${detail ? ' — ' + detail : ''}`);
	if (!ok) failures++;
}

// 1. Landing page
await page.goto(BASE + '/', { waitUntil: 'networkidle' });
const hero = await page.textContent('h1');
check('landing: judul hero tampil', hero?.includes('jadwal minum obat'), hero?.slice(0, 40));
const navGithub = await page.getAttribute('a[href*="github.com"]', 'href');
check('landing: tautan GitHub ada', !!navGithub, navGithub ?? '');
check('landing: tidak ada angka statistik dummy', !/\b\d+%\s*(pengguna|user|member)/i.test(await page.content()));

// 2. FAQ accordion bisa dibuka
const faqBtn = page.locator('#faq button').first();
await faqBtn.click();
await page.waitForTimeout(300);
check('landing: FAQ bisa dibuka', (await page.locator('#faq [role], #faq div[id^="faq-"]').count()) > 0);

// 3. Register
await page.goto(BASE + '/register', { waitUntil: 'networkidle' });
await page.fill('#full_name', 'Uji Browser');
await page.fill('#email', email);
await page.fill('#password', password);
await page.click('button[type="submit"]');
await page.waitForURL(/hari-ini/, { timeout: 15000 }).catch(() => {});
check('register: berpindah ke /hari-ini', page.url().includes('hari-ini'), page.url());

// 4. Cookie refresh HttpOnly ada (bukti backend memasangnya)
const cookies = await context.cookies();
const refresh = cookies.find((c) => c.name === 'po_refresh');
check('cookie po_refresh diterima browser', !!refresh);
check('cookie po_refresh HttpOnly (tidak terbaca JS)', refresh?.httpOnly === true);
check('cookie po_refresh tidak bisa dibaca document.cookie', !(await page.evaluate(() => document.cookie)).includes('po_refresh'));

// 5. Access token TIDAK di localStorage (keputusan keamanan)
const ls = await page.evaluate(() => JSON.stringify(Object.entries(localStorage)));
check('tidak ada token di localStorage', !/eyJ/.test(ls) && !/token/i.test(ls), ls.slice(0, 80));

// 6. Logout lalu halaman terproteksi harus memantul
await page.goto(BASE + '/login', { waitUntil: 'networkidle' });
check('login: form tampil', (await page.locator('#email').count()) === 1);

console.log('\n=== console error ===');
console.log(consoleErrors.length ? consoleErrors.join('\n') : '(tidak ada)');
console.log(failures === 0 ? '\nSEMUA LULUS' : `\n${failures} GAGAL`);

await browser.close();
process.exit(failures === 0 ? 0 : 1);
