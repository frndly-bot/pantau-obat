/**
 * Uji regresi: terjemahan halaman (i18n dipakai di seluruh aplikasi).
 *
 * MASALAH YANG DIKUNCI DI SINI
 *
 * Kamus i18n berisi lebih dari seribu kunci lengkap untuk `id` dan `en`,
 * tetapi NOL pemakaian di seluruh aplikasi: semua teks ditulis langsung di
 * markup. Akibatnya memilih bahasa lain tidak mengubah apa pun kecuali
 * halaman Pengaturan.
 *
 * Yang diuji bukan "apakah ada pemanggilan i18n", melainkan apakah TEKS DI
 * LAYAR benar-benar berganti saat bahasa diganti — termasuk nav bawah, yang
 * muncul di semua halaman sehingga paling mudah terlupakan.
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

const email = `i18nui-${Date.now()}@example.com`;
const tok = (
	await (
		await fetch(`${API}/auth/register`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json', Origin: ORIGIN },
			body: JSON.stringify({ email, password, full_name: 'Uji Bahasa', timezone: 'Asia/Jakarta' })
		})
	).json()
).data.access_token;
const H = { Authorization: `Bearer ${tok}`, 'Content-Type': 'application/json', Origin: ORIGIN };

// Obat dengan satu dosis yang sudah jatuh tempo, supaya tombol aksinya muncul.
const nowWib = new Intl.DateTimeFormat('en-GB', {
	hour: '2-digit',
	minute: '2-digit',
	hour12: false,
	timeZone: 'Asia/Jakarta'
}).format(new Date());
const [nh, nm] = nowWib.split(':').map(Number);
const past = `${String(Math.max(0, Math.floor((nh * 60 + nm - 60) / 60))).padStart(2, '0')}:00`;
const today = new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Jakarta' }).format(new Date());

await fetch(`${API}/medications`, {
	method: 'POST',
	headers: H,
	body: JSON.stringify({
		name: 'Obat Bahasa',
		dose_value: 1,
		dose_unit_code: 'tablet',
		route_code: 'O',
		dosage_form_code: 'LOCAL-TABS',
		meal_relation: 'after_meal',
		start_date: today,
		start_time: past,
		frequency: {
			pattern: 'daily',
			times_per_day: 1,
			tolerance_minutes: 60,
			start_time: past,
			schedule_timezone: 'Asia/Jakarta'
		},
		times: [{ time_local: past, label: 'morning' }],
		stock: {
			unit_code: 'tablet',
			initial_qty: 30,
			qty_per_dose: 1,
			refill_threshold_qty: 5,
			refill_reminder: true
		}
	})
});

const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH });
// Locale peramban `id-ID` supaya bahasa awal adalah Indonesia — aplikasi
// menghormati preferensi peramban, dan bawaan Playwright adalah `en-US`.
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
await page.waitForTimeout(1800);

const navText = async () => (await page.locator('nav').textContent()) ?? '';
const mainText = async () => (await page.textContent('main')) ?? '';

// ── Bahasa Indonesia ──
const navId = await navText();
check('nav berbahasa Indonesia', navId.includes('Hari Ini') && navId.includes('Riwayat'), navId.trim().slice(0, 40));

const mainId = await mainText();
check('tombol aksi berbahasa Indonesia', mainId.includes('Tunda') && mainId.includes('Lewati'));
check('kartu ringkasan berbahasa Indonesia', mainId.includes('kepatuhan') && mainId.includes('menunggu'));

// ── Ganti ke Inggris lewat halaman Pengaturan ──
await page.goto(`${BASE}/pengaturan`, { waitUntil: 'networkidle' });
await page.waitForTimeout(1200);
await page.locator('#set-locale').selectOption('en');
await page.waitForTimeout(1000);

// ── Kembali ke Hari Ini: seluruh teks harus berbahasa Inggris ──
await page.goto(`${BASE}/hari-ini`, { waitUntil: 'networkidle' });
await page.waitForTimeout(1800);

const navEn = await navText();
check(
	'nav ikut berganti bahasa (Today / History)',
	navEn.includes('Today') && navEn.includes('History'),
	navEn.trim().slice(0, 50)
);
check('nav TIDAK lagi berbahasa Indonesia', !navEn.includes('Hari Ini'));

const mainEn = await mainText();
check('tombol aksi berganti bahasa (Snooze / Skip)', mainEn.includes('Snooze') && mainEn.includes('Skip'));
check('kartu ringkasan berganti bahasa (adherence)', mainEn.includes('adherence'));

// Tidak boleh ada kunci kamus yang bocor ke layar. `createTranslator`
// mengembalikan nama kunci bila tidak ditemukan, jadi kunci yang salah
// ketik akan terlihat sebagai `today.something` di halaman.
const bocor = (await mainText()).match(/\b(today|history|meds|settings|common|nav)\.[a-z_]+/g);
check('tidak ada kunci kamus yang bocor ke layar', !bocor, bocor ? bocor.slice(0, 3).join(', ') : '');

// Kembalikan ke Indonesia supaya suite lain tidak terpengaruh.
await page.goto(`${BASE}/pengaturan`, { waitUntil: 'networkidle' });
await page.waitForTimeout(1000);
await page.locator('#set-locale').selectOption('id');
await page.waitForTimeout(800);

check('tidak ada error JS', errs.length === 0, errs.slice(0, 2).join(' | '));

await browser.close();
console.log('');
if (fail === 0) console.log('e2e-i18n-halaman: SEMUA LULUS');
else console.log(`e2e-i18n-halaman: ${fail} GAGAL`);
process.exit(fail === 0 ? 0 : 1);
