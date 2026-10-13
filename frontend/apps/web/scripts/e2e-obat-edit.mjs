/**
 * Uji regresi: ubah data obat dan hentikan/arsipkan.
 *
 * MASALAH YANG DIKUNCI DI SINI
 *
 * Sebelum ini, halaman Detail Obat TIDAK punya cara apa pun untuk mengubah
 * obat setelah dibuat: wizard hanya bisa MENAMBAH. Artinya salah ketik nama
 * obat, dosis keliru, atau catatan yang ingin diperbarui tidak bisa
 * diperbaiki sama sekali dari aplikasi.
 *
 * Dua tindakan yang diuji, dan keduanya berbeda akibat:
 *
 *   nonaktifkan (`is_active = false`) — obat berhenti muncul di jadwal,
 *   datanya utuh, bisa diaktifkan lagi.
 *
 *   arsipkan (`DELETE`) — soft delete. Riwayat dosis TETAP ADA. Server
 *   membalas `logs_preserved: true` untuk menegaskan itu, dan uji ini
 *   memeriksa janji tersebut benar-benar ditepati.
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

const email = `edit-${Date.now()}@example.com`;
const tok = (
	await (
		await fetch(`${API}/auth/register`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json', Origin: ORIGIN },
			body: JSON.stringify({ email, password, full_name: 'Uji Edit', timezone: 'Asia/Jakarta' })
		})
	).json()
).data.access_token;
const H = { Authorization: `Bearer ${tok}`, 'Content-Type': 'application/json', Origin: ORIGIN };

const nowWib = new Intl.DateTimeFormat('en-GB', {
	hour: '2-digit',
	minute: '2-digit',
	hour12: false,
	timeZone: 'Asia/Jakarta'
}).format(new Date());
const [nh, nm] = nowWib.split(':').map(Number);
const past = `${String(Math.max(0, Math.floor((nh * 60 + nm - 60) / 60))).padStart(2, '0')}:00`;
const today = new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Jakarta' }).format(new Date());

const med = (
	await (
		await fetch(`${API}/medications`, {
			method: 'POST',
			headers: H,
			body: JSON.stringify({
				name: 'Nama Salah Ketik',
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

// Catat satu dosis supaya ada riwayat yang harus dipertahankan saat arsip.
const sched = (await (await fetch(`${API}/schedule`, { headers: H })).json()).data;
const slot = sched.find((s) => s.medication_id === med.id);
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

/** Membaca keadaan obat dari server. */
async function medDiServer() {
	const r = await fetch(`${API}/medications/${med.id}`, { headers: H });
	if (r.status !== 200) return { status: r.status };
	return (await r.json()).data;
}

await page.goto(`${BASE}/obat/${med.id}`, { waitUntil: 'networkidle' });
await page.waitForTimeout(1500);

// ── 1. Tombol aksi tersedia ──
check('tombol Ubah ada', (await page.getByRole('button', { name: 'Ubah', exact: true }).count()) === 1);
check(
	'tombol Hentikan/arsipkan ada',
	(await page.getByRole('button', { name: /Hentikan/i }).count()) === 1
);

// ── 2. UBAH: memperbaiki nama yang salah ketik ──
await page.getByRole('button', { name: 'Ubah', exact: true }).click();
await page.waitForTimeout(700);
check('dialog Ubah terbuka', (await page.locator('[role="dialog"]').count()) === 1);

const isianNama = page.locator('#ed-name');
check('nama obat terisi di dialog', (await isianNama.inputValue()) === 'Nama Salah Ketik');

await isianNama.fill('Nama Sudah Benar');
await page.locator('#ed-notes').fill('catatan baru');
await page.getByRole('button', { name: 'Simpan', exact: true }).click();
await page.waitForTimeout(2500);

const setelahEdit = await medDiServer();
check('nama obat berubah di server', setelahEdit.name === 'Nama Sudah Benar', String(setelahEdit.name));
check('catatan pribadi tersimpan', setelahEdit.notes === 'catatan baru', String(setelahEdit.notes));
check('halaman menampilkan nama baru', ((await page.textContent('main')) ?? '').includes('Nama Sudah Benar'));

// Jadwal TIDAK boleh ikut berubah — dialog edit memang tidak menyentuhnya.
check(
	'jadwal tidak ikut berubah (sesuai batas yang dijelaskan)',
	setelahEdit.frequency?.pattern === 'daily',
	String(setelahEdit.frequency?.pattern)
);

// ── 3. HENTIKAN SEMENTARA ──
await page.getByRole('button', { name: /Hentikan/i }).click();
await page.waitForTimeout(700);
check('dialog Hentikan terbuka', (await page.locator('[role="dialog"]').count()) === 1);

// Pencocokan HARUS spesifik ke tombol di dalam dialog.
// Label pembuka dialog juga memuat kata "Hentikan" (`medDetail.stop_archive`
// = "Hentikan / arsipkan"), sehingga pola /Hentikan/i cocok dengan DUA
// tombol sekaligus dan kliknya menjadi ambigu.
const dialogStop = page.locator('[role="dialog"]');
await dialogStop.getByRole('button', { name: /Hentikan sementara/i }).click();
await page.waitForTimeout(2500);

const setelahHenti = await medDiServer();
check('obat menjadi tidak aktif di server', setelahHenti.is_active === false, String(setelahHenti.is_active));
check(
	'obat hilang dari jadwal hari ini',
	!((await (await fetch(`${API}/schedule`, { headers: H })).json()).data ?? []).some(
		(s) => s.medication_id === med.id
	)
);
check(
	'tombol berubah menjadi Aktifkan kembali',
	(await page.getByRole('button', { name: 'Aktifkan kembali', exact: true }).count()) === 1
);

// ── 4. AKTIFKAN KEMBALI ──
await page.getByRole('button', { name: 'Aktifkan kembali', exact: true }).click();
await page.waitForTimeout(2500);
check('obat aktif kembali di server', (await medDiServer()).is_active === true);

// ── 5. ARSIPKAN — riwayat harus TETAP ADA ──
const logsSebelum = (await (await fetch(`${API}/logs?limit=50`, { headers: H })).json()).data.length;

await page.getByRole('button', { name: /Hentikan/i }).click();
await page.waitForTimeout(700);
await dialogStop.getByRole('button', { name: /Arsipkan/i }).click();
await page.waitForTimeout(3000);

check('dialihkan ke daftar obat setelah arsip', page.url().endsWith('/obat'), page.url());

const logsSesudah = (await (await fetch(`${API}/logs?limit=50`, { headers: H })).json()).data.length;
check(
	'RIWAYAT TIDAK HILANG setelah diarsipkan',
	logsSesudah >= logsSebelum,
	`sebelum ${logsSebelum}, sesudah ${logsSesudah}`
);
check(
	'obat tidak lagi muncul di daftar aktif',
	!((await (await fetch(`${API}/medications?limit=100`, { headers: H })).json()).data ?? []).some(
		(m) => m.id === med.id
	)
);

check('tidak ada error JS', errs.length === 0, errs.slice(0, 2).join(' | '));

await browser.close();
console.log('');
if (fail === 0) console.log('e2e-obat-edit: SEMUA LULUS');
else console.log(`e2e-obat-edit: ${fail} GAGAL`);
process.exit(fail === 0 ? 0 : 1);
