/**
 * Uji regresi: bagan kepatuhan harian & kalender di halaman Riwayat.
 *
 * MASALAH YANG DIKUNCI DI SINI
 *
 * Riwayat dulu hanya menampilkan daftar catatan. Dua hal yang penting tidak
 * ada, dan keduanya punya akar yang sama:
 *
 *   1. **Bagan kepatuhan harian** — tidak ada gambaran sekilas.
 *   2. **Kalender** — hari yang bolong tidak terlihat SAMA SEKALI.
 *
 * Akarnya: dosis terlewat TIDAK PERNAH menjadi baris `medication_logs`. Ia
 * hidup di tabel terpisah (`medication_dose_events`), dan hanya rekap
 * `GET /adherence/daily` yang membawanya. Jadi membangun bagan dari daftar
 * catatan akan selalu gagal menggambarkan hari yang bolong.
 *
 * Uji ini memakai data yang MEMANG punya dosis terlewat, lalu memastikan
 * angkanya benar-benar muncul — bukan hanya bahwa elemennya ada.
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

const email = `bagan-${Date.now()}@example.com`;
const tok = (
	await (
		await fetch(`${API}/auth/register`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json', Origin: ORIGIN },
			body: JSON.stringify({ email, password, full_name: 'Uji Bagan', timezone: 'Asia/Jakarta' })
		})
	).json()
).data.access_token;
const H = { Authorization: `Bearer ${tok}`, 'Content-Type': 'application/json', Origin: ORIGIN };

const today = new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Jakarta' }).format(new Date());

// Dua jam: satu pagi (sudah lewat → akan terlewat), satu malam (belum).
// Dosis pagi sengaja TIDAK dicatat supaya muncul sebagai terlewat.
await fetch(`${API}/medications`, {
	method: 'POST',
	headers: H,
	body: JSON.stringify({
		name: 'Obat Bagan',
		dose_value: 1,
		dose_unit_code: 'tablet',
		route_code: 'O',
		dosage_form_code: 'LOCAL-TABS',
		meal_relation: 'after_meal',
		start_date: today,
		start_time: '05:00',
		frequency: {
			pattern: 'daily',
			times_per_day: 2,
			tolerance_minutes: 60,
			start_time: '05:00',
			schedule_timezone: 'Asia/Jakarta'
		},
		times: [
			{ time_local: '05:00', label: 'morning' },
			{ time_local: '23:30', label: 'evening' }
		],
		stock: {
			unit_code: 'tablet',
			initial_qty: 60,
			qty_per_dose: 1,
			refill_threshold_qty: 5,
			refill_reminder: true
		}
	})
});

// Endpoint rekap harus benar-benar mengembalikan baris untuk hari ini.
const rHarian = await fetch(`${API}/adherence/daily?from=${today}&to=${today}`, { headers: H });
const jHarian = await rHarian.json();
check('endpoint /adherence/daily mengembalikan data', rHarian.status === 200 && (jHarian.data ?? []).length > 0,
	`${(jHarian.data ?? []).length} baris`);

const hariIni = (jHarian.data ?? [])[0];
check('rekap memuat jumlah terjadwal', (hariIni?.scheduled_doses ?? 0) >= 2, `terjadwal ${hariIni?.scheduled_doses}`);
check(
	'rekap memakai field yang BENAR (scheduled_doses, bukan total)',
	'scheduled_doses' in (hariIni ?? {}) && !('total' in (hariIni ?? {})),
	Object.keys(hariIni ?? {}).join(', ')
);

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

await page.goto(`${BASE}/riwayat`, { waitUntil: 'networkidle' });
await page.waitForTimeout(2500);

const teks = (await page.textContent('main')) ?? '';

// ── Bagan ──
check('bagan kepatuhan harian tampil', teks.includes('Kepatuhan harian'));
check('bagan menjelaskan warna batang', teks.includes('Batang kuning menandai'));
check('bagan menampilkan jumlah hari', /\d+ hari/.test(teks));
check('bagan menampilkan total terjadwal', teks.includes('Terjadwal'));
check('bagan menampilkan total terlewat', teks.includes('Terlewat'));
check('bagan menampilkan "hari tanpa data"', teks.includes('Hari tanpa data'));

// ── Kalender ──
check('kalender tampil', teks.includes('Kalender'));
check('kalender punya keterangan warna',
	teks.includes('Ada dosis tercatat') && teks.includes('Ada yang terlewat') && teks.includes('Belum ada data'));

const kotakKalender = await page.locator('main .grid.grid-cols-7 > div').count();
check('kalender menggambar hari', kotakKalender >= 1, `${kotakKalender} kotak`);

// ── Bagan TIDAK boleh memakai rumus sendiri ──
// Angka "Terjadwal" harus sama dengan yang dikirim server, bukan hasil
// hitungan ulang dari daftar catatan (yang tidak memuat dosis terlewat).
const angkaTerjadwal = teks.match(/Terjadwal\s*(\d+)/)?.[1];
check(
	'total terjadwal di bagan sama dengan server',
	Number(angkaTerjadwal) === hariIni?.scheduled_doses,
	`UI ${angkaTerjadwal}, server ${hariIni?.scheduled_doses}`
);

check('tidak ada error JS', errs.length === 0, errs.slice(0, 2).join(' | '));

await browser.close();
console.log('');
if (fail === 0) console.log('e2e-riwayat-bagan: SEMUA LULUS');
else console.log(`e2e-riwayat-bagan: ${fail} GAGAL`);
process.exit(fail === 0 ? 0 : 1);
