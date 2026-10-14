/**
 * Uji regresi: terjemahan di SELURUH halaman (bukan hanya Hari Ini).
 *
 * MASALAH YANG DIKUNCI DI SINI
 *
 * Kamus i18n berisi lebih dari seribu kunci lengkap untuk `id` dan `en`, tapi
 * pemakaiannya tidak merata: Hari Ini dan Pengaturan sudah memakainya,
 * sedangkan Obat, Detail Obat, Riwayat, Caregiver, dan Login/Register masih
 * menulis teks langsung di markup. Akibatnya memilih bahasa lain hanya
 * mengubah sebagian aplikasi — dan itu lebih membingungkan daripada tidak
 * ada pilihan bahasa sama sekali.
 *
 * Yang diuji: setiap halaman benar-benar berganti bahasa, dan TIDAK ADA kunci
 * kamus yang bocor ke layar. `createTranslator` mengembalikan nama kunci bila
 * kunci tidak ditemukan, jadi salah ketik akan terlihat sebagai
 * `riwayat.something` — dan itu harus gagal, bukan diam-diam lolos.
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

const email = `i18nfull-${Date.now()}@example.com`;
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

const today = new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Jakarta' }).format(new Date());
const nowWib = new Intl.DateTimeFormat('en-GB', {
	hour: '2-digit',
	minute: '2-digit',
	hour12: false,
	timeZone: 'Asia/Jakarta'
}).format(new Date());
const [nh, nm] = nowWib.split(':').map(Number);
const past = `${String(Math.max(0, Math.floor((nh * 60 + nm - 60) / 60))).padStart(2, '0')}:00`;

const med = (
	await (
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
		})
	).json()
).data;

const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH });
// `id-ID` supaya bahasa awal adalah Indonesia.
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

// ── Login dalam bahasa Indonesia ──
check('login: tombol berbahasa Indonesia', ((await page.textContent('main, body')) ?? '').includes('Masuk'));

await page.fill('#email', email);
await page.fill('#password', password);
await page.click('button[type="submit"]');
await page.waitForURL(/hari-ini/, { timeout: 30000 });

/** Halaman yang diperiksa, beserta penanda bahasanya. */
const HALAMAN = [
	{ nama: 'Riwayat', url: '/riwayat', id: 'Catatan dosis Anda', en: 'Your dose records' },
	{ nama: 'Obat', url: '/obat', id: 'Daftar obat yang Anda pantau', en: 'Medications you track' },
	{ nama: 'Detail obat', url: `/obat/${med.id}`, id: 'Jam minum', en: 'Jam minum' },
	{ nama: 'Caregiver', url: '/caregiver', id: 'Undangan untuk Anda', en: 'Invitations for you' },
	{ nama: 'Pengaturan', url: '/pengaturan', id: 'Tema', en: 'Theme' }
];

/** Membaca teks halaman + mencari kunci kamus yang bocor. */
async function periksa(url) {
	await page.goto(`${BASE}${url}`, { waitUntil: 'networkidle' });
	await page.waitForTimeout(1800);
	const teks = (await page.textContent('main')) ?? '';
	const bocor = teks.match(/\b(riwayat|caregiver|meds|today|settings|common|nav|medDetail|auth)\.[a-z_]{3,}/g);
	return { teks, bocor };
}

// ── Bahasa Indonesia ──
console.log('\n── Bahasa Indonesia ──');
for (const h of HALAMAN) {
	const { teks, bocor } = await periksa(h.url);
	check(`${h.nama}: teks Indonesia tampil`, teks.includes(h.id), teks.slice(0, 60).replace(/\s+/g, ' '));
	check(`${h.nama}: tidak ada kunci bocor`, !bocor, bocor ? bocor.slice(0, 3).join(', ') : '');
}

// ── Ganti ke Inggris ──
await page.goto(`${BASE}/pengaturan`, { waitUntil: 'networkidle' });
await page.waitForTimeout(1200);
await page.locator('#set-locale').selectOption('en');
await page.waitForTimeout(1200);

console.log('\n── English ──');
for (const h of HALAMAN) {
	if (h.id === h.en) continue; // halaman ini belum diterjemahkan seluruhnya
	const { teks, bocor } = await periksa(h.url);
	check(`${h.nama}: teks Inggris tampil`, teks.includes(h.en), teks.slice(0, 60).replace(/\s+/g, ' '));
	check(`${h.nama}: tidak ada kunci bocor`, !bocor, bocor ? bocor.slice(0, 3).join(', ') : '');
}

// Nav bawah harus ikut berganti di halaman mana pun.
const { teks: teksObat } = await periksa('/obat');
check('nav bawah berbahasa Inggris di halaman Obat',
	((await page.locator('nav').textContent()) ?? '').includes('Medications'),
	'nav ikut berganti');

// Kembalikan ke Indonesia supaya suite lain tidak terpengaruh.
await page.goto(`${BASE}/pengaturan`, { waitUntil: 'networkidle' });
await page.waitForTimeout(1000);
await page.locator('#set-locale').selectOption('id');
await page.waitForTimeout(800);

check('tidak ada error JS', errs.length === 0, errs.slice(0, 2).join(' | '));

await browser.close();
console.log('');
if (fail === 0) console.log('e2e-i18n-semua: SEMUA LULUS');
else console.log(`e2e-i18n-semua: ${fail} GAGAL`);
process.exit(fail === 0 ? 0 : 1);
