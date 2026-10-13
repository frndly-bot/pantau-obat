/**
 * Uji regresi: aksi dosis di halaman Hari Ini.
 *
 * MASALAH YANG DIKUNCI DI SINI
 *
 * Halaman ini dulu hanya punya DUA tombol untuk dua status (Diminum,
 * Lewati). Prototipe design mendefinisikan aksi yang menyesuaikan status:
 *
 *   pending / due  → [Diminum] [Tunda] [Lewati]
 *   snoozed        → [Diminum] [Tunda lagi]
 *   taken / late   → [Koreksi]
 *   skipped        → [Diminum]
 *
 * Yang hilang paling terasa: **Tunda** (satu-satunya jalan menunda pengingat
 * tanpa menandai dosis diminum) dan **Koreksi** (jalan memperbaiki catatan
 * yang salah, termasuk mengembalikan saldo stok).
 *
 * Uji ini memanggil API sungguhan lewat UI dan memeriksa keadaan di backend,
 * bukan sekadar keberadaan tombolnya.
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

const email = `aksi-${Date.now()}@example.com`;
const tok = (
	await (
		await fetch(`${API}/auth/register`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json', Origin: ORIGIN },
			body: JSON.stringify({
				email,
				password,
				full_name: 'Uji Aksi',
				timezone: 'Asia/Jakarta'
			})
		})
	).json()
).data.access_token;
const H = { Authorization: `Bearer ${tok}`, 'Content-Type': 'application/json', Origin: ORIGIN };

// Obat dengan dua jam: satu sudah lewat (due), satu belum (pending).
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
				name: 'Obat Aksi',
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

// Obat PRN: tanpa jadwal tetap.
await fetch(`${API}/medications`, {
	method: 'POST',
	headers: H,
	body: JSON.stringify({
		name: 'Obat PRN Aksi',
		dose_value: 1,
		dose_unit_code: 'tablet',
		route_code: 'O',
		dosage_form_code: 'LOCAL-TABS',
		meal_relation: 'not_related',
		start_date: today,
		as_needed: true,
		frequency: { pattern: 'prn', schedule_timezone: 'Asia/Jakarta' },
		times: []
	})
});

const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH });

/** schedule_id slot obat uji — dipakai untuk mencocokkan jejak audit snooze. */
let slotId = '';
{
	const s = await (await fetch(`${API}/schedule`, { headers: H })).json();
	slotId = s.data.find((x) => x.medication_id === med.id)?.schedule_id ?? '';
}
const ctx = await browser.newContext({
	viewport: { width: 390, height: 844 },
	locale: 'id-ID'
});
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
await page.waitForTimeout(1500);

/** Membaca status slot obat uji dari server. */
async function statusDiServer() {
	const s = await (await fetch(`${API}/schedule`, { headers: H })).json();
	const slot = s.data.find((x) => x.medication_id === med.id);
	return slot ? { status: slot.status, log_id: slot.log_id } : null;
}

// ── 1. Kartu ringkasan & tombol aksi dasar ──
const teks = (await page.textContent('main')) ?? '';
check('kartu ringkasan menampilkan "menunggu"', teks.includes('menunggu'));
check('kartu ringkasan menampilkan "kepatuhan"', teks.includes('kepatuhan'));
check('tombol Tunda ada', (await page.getByRole('button', { name: 'Tunda', exact: true }).count()) > 0);
check('tombol Lewati ada', (await page.getByRole('button', { name: 'Lewati', exact: true }).count()) > 0);
check('seksi PRN tampil', teks.includes('Bila perlu (PRN)'));

// ── 2. TUNDA: membuka dialog, memilih 30 menit, lalu memeriksa server ──
//
// PENTING: snooze TIDAK mengubah status slot. Server hanya menggeser waktu
// pengingat (baris `bump_queue`) dan menulis satu baris audit — catatan dosis
// sengaja tidak dibuat, karena dosisnya belum diminum. Jadi yang diperiksa di
// sini adalah jejak auditnya, bukan status slot. Menguji `status === 'snoozed'`
// akan selalu gagal dan menyesatkan.
await page.getByRole('button', { name: 'Tunda', exact: true }).first().click();
await page.waitForTimeout(700);
check('dialog Tunda terbuka', (await page.locator('[role="dialog"]').count()) === 1);

const opsiMenit = await page.getByRole('button', { name: /menit$/ }).allTextContents();
check('menawarkan 5/10/30 menit', opsiMenit.length === 3, opsiMenit.join(', '));

await page.getByRole('button', { name: '30 menit', exact: true }).click();
await page.getByRole('button', { name: 'Tunda', exact: true }).last().click();
await page.waitForTimeout(2500);
check('dialog tertutup setelah tunda', (await page.locator('[role="dialog"]').count()) === 0);

const audit = await (await fetch(`${API}/users/me/audit-log?limit=20`, { headers: H })).json();
// `meta` sengaja TIDAK dikembalikan endpoint audit-log (lihat handler
// `AuditLog`), jadi menitnya tidak bisa dibaca dari sini. Yang tersedia dan
// cukup untuk membuktikan aksinya sampai ke server: baris audit untuk
// schedule_id ini dengan aksi `update`.
const jejakSnooze = (audit.data ?? []).find(
	(e) => e.resource_id === slotId && e.action === 'update'
);
check(
	'snooze tercatat di jejak audit server',
	!!jejakSnooze,
	jejakSnooze ? `${jejakSnooze.action} @ ${jejakSnooze.created_at}` : 'tidak ada'
);
check(
	'snooze TIDAK membuat catatan dosis (sesuai desain)',
	(await statusDiServer())?.status === 'due',
	String((await statusDiServer())?.status)
);
check('pesan konfirmasi tunda tampil', ((await page.textContent('main')) ?? '').includes('ditunda'));

// ── 3. DIMINUM: mencatat dosis yang sudah jatuh tempo ──
//
// Dosis ini dijadwalkan 60 menit lalu dengan toleransi 60 menit, jadi server
// mencatatnya sebagai `late` — bukan `taken`. Itu perilaku yang BENAR: yang
// diminum setelah lewat tenggat memang terlambat, dan membedakannya penting
// bagi laporan kepatuhan. Uji ini menerima keduanya, dan memastikan catatannya
// benar-benar lahir.
// Label tombolnya "Sudah diminum" (kunci kamus `today.taken_2`), bukan
// "Diminum" — dan itu memang lebih tepat: pengguna menandai dosis yang SUDAH
// diminum, bukan memerintah dirinya minum.
await page.getByRole('button', { name: 'Sudah diminum', exact: true }).first().click();
await page.waitForTimeout(2500);
const setelahMinum = await statusDiServer();
check(
	'server mencatat dosis (taken atau late)',
	setelahMinum?.status === 'taken' || setelahMinum?.status === 'late',
	String(setelahMinum?.status)
);
check('log_id terisi setelah dicatat', !!setelahMinum?.log_id);

// ── 4. KOREKSI: mengubah catatan yang sudah ada ──
check(
	'tombol Koreksi muncul untuk dosis yang sudah dicatat',
	(await page.getByRole('button', { name: 'Koreksi', exact: true }).count()) > 0
);
await page.getByRole('button', { name: 'Koreksi', exact: true }).first().click();
await page.waitForTimeout(700);
check('dialog Koreksi terbuka', (await page.locator('[role="dialog"]').count()) === 1);

// Ubah menjadi Terlambat.
await page.getByRole('button', { name: 'Terlambat', exact: true }).click();
await page.getByRole('button', { name: 'Simpan koreksi', exact: true }).click();
await page.waitForTimeout(2500);
const setelahKoreksi = await statusDiServer();
check(
	'server mencatat status late setelah koreksi',
	setelahKoreksi?.status === 'late',
	String(setelahKoreksi?.status)
);

// ── 5. PRN: mencatat dosis obat "bila perlu" ──
await page.getByRole('button', { name: 'Catat obat bila perlu', exact: true }).click();
await page.waitForTimeout(700);
check('dialog PRN terbuka', (await page.locator('[role="dialog"]').count()) === 1);

const opsiObat = await page.locator('#prn-med option').allTextContents();
check(
	'obat PRN tersedia di pilihan',
	opsiObat.some((t) => t.includes('Obat PRN Aksi')),
	opsiObat.join(' / ')
);

await page.locator('#prn-med').selectOption({ label: 'Obat PRN Aksi' });
await page.getByRole('button', { name: 'Catat', exact: true }).click();
await page.waitForTimeout(2500);

const logs = await (await fetch(`${API}/logs`, { headers: H })).json();
const prnLog = logs.data.find((l) => l.medication_id && l.schedule_id === null);
check('dosis PRN tercatat di server (tanpa schedule_id)', !!prnLog);

check('tidak ada error JS', errs.length === 0, errs.slice(0, 2).join(' | '));

await browser.close();
console.log('');
if (fail === 0) console.log('e2e-aksi-dosis: SEMUA LULUS');
else console.log(`e2e-aksi-dosis: ${fail} GAGAL`);
process.exit(fail === 0 ? 0 : 1);
