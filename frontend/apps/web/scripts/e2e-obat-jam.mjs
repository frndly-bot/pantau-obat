/**
 * Uji regresi: kelola jam minum & riwayat dosis di Detail Obat.
 *
 * MASALAH YANG DIKUNCI DI SINI
 *
 * Halaman Detail Obat menampilkan jam minum sebagai CHIP yang tidak bisa
 * diklik, dan sama sekali tidak menampilkan catatan dosis obat itu — hanya
 * transaksi stok. Padahal `addTime`/`removeTime` sudah tersedia, dan
 * pertanyaan yang paling sering muncul saat membuka satu obat adalah "sudah
 * berapa kali saya minum obat ini?".
 *
 * Yang diuji: jam benar-benar tersimpan/dihapus di server, dan angka riwayat
 * memakai aturan yang SAMA dengan server (pembilang hanya `taken`).
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

const email = `jam-${Date.now()}@example.com`;
const tok = (
	await (
		await fetch(`${API}/auth/register`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json', Origin: ORIGIN },
			body: JSON.stringify({ email, password, full_name: 'Uji Jam', timezone: 'Asia/Jakarta' })
		})
	).json()
).data.access_token;
const H = { Authorization: `Bearer ${tok}`, 'Content-Type': 'application/json', Origin: ORIGIN };

const today = new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Jakarta' }).format(new Date());

const med = (
	await (
		await fetch(`${API}/medications`, {
			method: 'POST',
			headers: H,
			body: JSON.stringify({
				name: 'Obat Jam Uji',
				dose_value: 1,
				dose_unit_code: 'tablet',
				route_code: 'O',
				dosage_form_code: 'LOCAL-TABS',
				meal_relation: 'after_meal',
				start_date: today,
				start_time: '05:00',
				frequency: {
					pattern: 'daily',
					times_per_day: 1,
					tolerance_minutes: 60,
					start_time: '05:00',
					schedule_timezone: 'Asia/Jakarta'
				},
				times: [{ time_local: '05:00', label: 'morning' }],
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

// Catat satu dosis supaya riwayat tidak kosong.
const sched = (await (await fetch(`${API}/schedule`, { headers: H })).json()).data;
const slot = sched.find((s) => s.medication_id === med.id);
if (slot) {
	await fetch(`${API}/medications/${med.id}/logs`, {
		method: 'POST',
		headers: H,
		body: JSON.stringify({
			schedule_id: slot.schedule_id,
			slot_date: today,
			scheduled_hour: slot.scheduled_hour,
			scheduled_minute: slot.scheduled_minute,
			scheduled_tz: 'Asia/Jakarta',
			status: 'taken'
		})
	});
}

/** Membaca jam minum dari server. */
async function jamDiServer() {
	const r = await fetch(`${API}/medications/${med.id}`, { headers: H });
	return ((await r.json()).data?.times ?? []).map((t) => t.time_local).sort();
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

await page.goto(`${BASE}/obat/${med.id}`, { waitUntil: 'networkidle' });
await page.waitForTimeout(2000);

const sebelum = await jamDiServer();
check('jam awal ada di server', sebelum.length === 1, sebelum.join(', '));

// ── Tambah jam ──
check('tombol Tambah jam ada', (await page.getByRole('button', { name: 'Tambah jam', exact: true }).count()) === 1);

await page.getByRole('button', { name: 'Tambah jam', exact: true }).click();
await page.waitForTimeout(700);
check('dialog Tambah jam terbuka', (await page.locator('[role="dialog"]').count()) === 1);
check('dialog menjelaskan batasnya', ((await page.textContent('[role="dialog"]')) ?? '').includes('dosis ke depan'));

await page.locator('#nt-time').fill('20:00');
await page.getByRole('button', { name: 'Tambah', exact: true }).click();
await page.waitForTimeout(2500);

const sesudah = await jamDiServer();
check('jam baru tersimpan di server', sesudah.includes('20:00'), sesudah.join(', '));
check('jam lama tetap ada', sesudah.includes('05:00'), sesudah.join(', '));

// ── Hapus jam ──
const tombolHapus = page.locator('button[aria-label^="Hapus jam"]');
check('tombol hapus jam ada', (await tombolHapus.count()) >= 1, `${await tombolHapus.count()} tombol`);

// Hapus jam 20:00 (yang baru ditambahkan).
const labelHapus = page.locator('button[aria-label="Hapus jam 20:00"]');
if ((await labelHapus.count()) > 0) {
	await labelHapus.click();
	await page.waitForTimeout(2500);
	const setelahHapus = await jamDiServer();
	check('jam terhapus dari server', !setelahHapus.includes('20:00'), setelahHapus.join(', '));
} else {
	check('tombol hapus jam 20:00 ditemukan', false, 'tidak ada');
}

// ── Riwayat dosis obat ini ──
const teks = (await page.textContent('main')) ?? '';
check('seksi riwayat dosis tampil', teks.includes('Riwayat dosis obat ini'));
check('tombol "Semua riwayat" ada', (await page.getByRole('button', { name: 'Semua riwayat', exact: true }).count()) === 1);

const rLogs = await fetch(`${API}/medications/${med.id}/logs`, { headers: H });
const logsServer = (await rLogs.json()).data ?? [];
check('server punya catatan dosis', logsServer.length >= 1, `${logsServer.length} catatan`);

// Angka "tepat waktu" harus memakai aturan server: pembilang hanya `taken`.
const taken = logsServer.filter((l) => l.status === 'taken').length;
const resolved = logsServer.filter((l) => ['taken', 'late', 'skipped'].includes(l.status)).length;
const rasioHarapan = resolved === 0 ? null : Math.min(1, taken / resolved);
const teksRasio = rasioHarapan === null ? 'Belum ada data' : `${Math.round(rasioHarapan * 100)}%`;
check('angka tepat waktu tampil', teks.includes(teksRasio), `mengharapkan ${teksRasio}`);

check('tidak ada error JS', errs.length === 0, errs.slice(0, 2).join(' | '));

await browser.close();
console.log('');
if (fail === 0) console.log('e2e-obat-jam: SEMUA LULUS');
else console.log(`e2e-obat-jam: ${fail} GAGAL`);
process.exit(fail === 0 ? 0 : 1);
