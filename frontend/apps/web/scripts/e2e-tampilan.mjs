/**
 * Uji regresi: pemilih tema dan bahasa di halaman Pengaturan.
 *
 * MASALAH YANG DIKUNCI DI SINI
 *
 * Dua infrastruktur sudah ada tapi tidak bisa dijangkau pengguna:
 *
 *   1. Tema. Logika `po_theme` + `data-theme` hidup di dalam `+layout.svelte`
 *      tanpa cara MENGUBAHNYA dari halaman lain, sehingga halaman Pengaturan
 *      tidak bisa menawarkan pilihan tema sama sekali — padahal prototipe
 *      design menempatkannya tepat di sana.
 *
 *   2. Bahasa. Kamus i18n berisi 1041 kunci lengkap untuk `id` dan `en`,
 *      tapi NOL pemakaian di seluruh aplikasi. Aplikasi sepenuhnya
 *      hardcoded Indonesia.
 *
 * Yang diuji bukan keberadaan tombolnya, melainkan AKIBATNYA: tema benar-
 * benar berubah di `<html>`, pilihannya bertahan setelah halaman dimuat
 * ulang, dan bahasa benar-benar berganti.
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

const email = `tampilan-${Date.now()}@example.com`;
await fetch(`${API}/auth/register`, {
	method: 'POST',
	headers: { 'Content-Type': 'application/json', Origin: ORIGIN },
	body: JSON.stringify({ email, password, full_name: 'Uji Tampilan', timezone: 'Asia/Jakarta' })
});

const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH });
// `locale: 'id-ID'` penting: aplikasi menghormati bahasa peramban, sedangkan
// Playwright bawaannya `en-US`. Tanpa ini, tombolnya berbunyi "Light"/"Dark"
// dan uji mencari "Terang"/"Gelap" — gagal karena alasan yang salah.
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
await page.waitForURL(/hari-ini/, { timeout: 25000 });

await page.goto(`${BASE}/pengaturan`, { waitUntil: 'networkidle' });
await page.waitForTimeout(1200);

// ── TEMA ──
const temaSekarang = () => page.evaluate(() => document.documentElement.dataset.theme);

const tombolTerang = page.getByRole('button', { name: 'Terang', exact: true });
const tombolGelap = page.getByRole('button', { name: 'Gelap', exact: true });

check('tombol tema "Terang" ada', (await tombolTerang.count()) === 1);
check('tombol tema "Gelap" ada', (await tombolGelap.count()) === 1);

await tombolGelap.click();
await page.waitForTimeout(400);
check('tema benar-benar berubah ke gelap', (await temaSekarang()) === 'dark', await temaSekarang());

const tersimpan = await page.evaluate(() => localStorage.getItem('po_theme'));
check('pilihan tema tersimpan di perangkat', tersimpan === 'dark', String(tersimpan));

// Muat ulang: pilihan harus bertahan (bukan kembali ke preferensi sistem).
await page.reload({ waitUntil: 'networkidle' });
await page.waitForTimeout(900);
check('tema bertahan setelah halaman dimuat ulang', (await temaSekarang()) === 'dark', await temaSekarang());

await page.getByRole('button', { name: 'Terang', exact: true }).click();
await page.waitForTimeout(400);
check('bisa kembali ke tema terang', (await temaSekarang()) === 'light', await temaSekarang());

// ── BAHASA ──
const pilihBahasa = page.locator('#set-locale');
check('pemilih bahasa ada', (await pilihBahasa.count()) === 1);

const opsi = await pilihBahasa.locator('option').allTextContents();
check('menawarkan Indonesia dan English', opsi.length === 2, opsi.join(' / '));

const sebelum = await page.locator('h1').first().textContent();
check('judul berbahasa Indonesia sebelum diganti', sebelum?.includes('Pengaturan'), String(sebelum));

// Penanda terjemahan: label kartu "Tampilan". Halaman ini baru sebagian
// diterjemahkan (kartu Tampilan), jadi menguji elemen yang MEMANG lewat
// kamus — bukan `<h1>` yang masih hardcoded.
const labelTema = page.locator('main p.text-sm.font-medium').first();
check('label tema berbahasa Indonesia', (await labelTema.textContent())?.includes('Tema'), String(await labelTema.textContent()));

await pilihBahasa.selectOption('en');
await page.waitForTimeout(900);

const cookieLocale = (await ctx.cookies()).find((c) => c.name === 'po_locale');
check('pilihan bahasa tersimpan di cookie', cookieLocale?.value === 'en', String(cookieLocale?.value));
check('atribut lang halaman ikut berubah', (await page.evaluate(() => document.documentElement.lang)) === 'en');

const labelEn = await labelTema.textContent();
check('label tema langsung berubah ke Inggris', labelEn?.includes('Theme'), String(labelEn));

// Muat ulang: bahasa harus bertahan. Inilah yang membedakan "pilihan
// tersimpan" dari "pilihan hanya berlaku selama halaman terbuka".
await page.reload({ waitUntil: 'networkidle' });
await page.waitForTimeout(1200);
const labelSetelahMuatUlang = await page.locator('main p.text-sm.font-medium').first().textContent();
check(
	'bahasa bertahan setelah dimuat ulang',
	labelSetelahMuatUlang?.includes('Theme'),
	String(labelSetelahMuatUlang)
);

// Kembalikan ke Indonesia supaya suite lain tidak terpengaruh.
await page.locator('#set-locale').selectOption('id');
await page.waitForTimeout(900);
const labelKembali = await page.locator('main p.text-sm.font-medium').first().textContent();
check('bisa dikembalikan ke Indonesia', labelKembali?.includes('Tema'), String(labelKembali));

check('tidak ada error JS', errs.length === 0, errs.slice(0, 2).join(' | '));

await browser.close();
console.log('');
if (fail === 0) console.log('e2e-tampilan: SEMUA LULUS');
else console.log(`e2e-tampilan: ${fail} GAGAL`);
process.exit(fail === 0 ? 0 : 1);
