/**
 * Uji regresi: sisi "Saya memantau" di halaman Caregiver.
 *
 * MASALAH YANG DIKUNCI DI SINI
 *
 * Halaman Caregiver dulu hanya menampilkan DAFTAR pasien tanpa satu pun yang
 * bisa diklik. Akibatnya seseorang bisa menerima undangan jadi pendamping,
 * lalu tidak bisa melakukan pekerjaannya sama sekali.
 *
 * Yang diuji BUKAN keberadaan tombolnya, melainkan bahwa pembatasan peran
 * benar-benar ditegakkan SERVER — bukan sekadar disembunyikan di UI:
 *
 *   viewer   → boleh melihat, TIDAK boleh mencatat dosis
 *   helper   → boleh mencatat, TIDAK boleh mengubah obat
 *   manager  → boleh keduanya
 *
 * Uji ini memanggil API langsung untuk peran yang tidak boleh, supaya
 * tombol yang disembunyikan tidak menjadi satu-satunya perlindungan.
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

const stamp = Date.now();
const pasienEmail = `pasien-${stamp}@example.com`;
const viewerEmail = `viewer-${stamp}@example.com`;
const helperEmail = `helper-${stamp}@example.com`;
const managerEmail = `manager-${stamp}@example.com`;

async function daftar(email, nama) {
	const r = await fetch(`${API}/auth/register`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json', Origin: ORIGIN },
		body: JSON.stringify({ email, password, full_name: nama, timezone: 'Asia/Jakarta' })
	});
	const j = await r.json();
	if (!j.data?.access_token) throw new Error(`daftar ${email} gagal: ${JSON.stringify(j).slice(0, 200)}`);
	return j.data.access_token;
}

const tokPasien = await daftar(pasienEmail, 'Pasien Uji');
await daftar(viewerEmail, 'Viewer Uji');
await daftar(helperEmail, 'Helper Uji');
await daftar(managerEmail, 'Manager Uji');

const H = (t) => ({ Authorization: `Bearer ${t}`, 'Content-Type': 'application/json', Origin: ORIGIN });

// Pasien menyiapkan satu obat dengan dosis yang sudah jatuh tempo.
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
			headers: H(tokPasien),
			body: JSON.stringify({
				name: 'Obat Pasien Uji',
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

// Pasien mengundang ketiga peran.
async function undang(email, role) {
	const r = await fetch(`${API}/care-links`, {
		method: 'POST',
		headers: H(tokPasien),
		body: JSON.stringify({ invite_email: email, relation: 'family', role })
	});
	const j = await r.json();
	return j.data?.invite_url ?? null;
}

/** Menerima undangan memakai token dari tautan. */
async function terima(url, tok) {
	const token = new URL(url).searchParams.get('token');
	const r = await fetch(`${API}/care-links/accept`, {
		method: 'POST',
		headers: H(tok),
		body: JSON.stringify({ token })
	});
	return r.status;
}

const urlViewer = await undang(viewerEmail, 'viewer');
const urlHelper = await undang(helperEmail, 'helper');
const urlManager = await undang(managerEmail, 'manager');

const tokViewer = (await (await fetch(`${API}/auth/login`, {
	method: 'POST',
	headers: { 'Content-Type': 'application/json', Origin: ORIGIN },
	body: JSON.stringify({ email: viewerEmail, password })
})).json()).data.access_token;
const tokHelper = (await (await fetch(`${API}/auth/login`, {
	method: 'POST',
	headers: { 'Content-Type': 'application/json', Origin: ORIGIN },
	body: JSON.stringify({ email: helperEmail, password })
})).json()).data.access_token;
const tokManager = (await (await fetch(`${API}/auth/login`, {
	method: 'POST',
	headers: { 'Content-Type': 'application/json', Origin: ORIGIN },
	body: JSON.stringify({ email: managerEmail, password })
})).json()).data.access_token;

check('undangan viewer diterima', (await terima(urlViewer, tokViewer)) === 200);
check('undangan helper diterima', (await terima(urlHelper, tokHelper)) === 200);
check('undangan manager diterima', (await terima(urlManager, tokManager)) === 200);

// ── 1. Endpoint jadwal pasien: yang membuka jalan bagi semuanya ──
const rJadwal = await fetch(`${API}/care/${med.owner_id}/schedule`, { headers: H(tokHelper) });
const jadwal = await rJadwal.json();
check('caregiver bisa membaca jadwal pasien', rJadwal.status === 200, `status ${rJadwal.status}`);

const slot = (jadwal.data ?? []).find((s) => s.medication_id === med.id);
check('jadwal memuat dosis pasien', !!slot, slot ? `status ${slot.status}` : 'tidak ditemukan');
check('slot membawa schedule_id (syarat mencatat)', !!slot?.schedule_id);

// ── 2. Pembatasan peran DITEGAKKAN SERVER, bukan hanya disembunyikan UI ──
const bodyCatat = JSON.stringify({
	schedule_id: slot?.schedule_id,
	slot_date: today,
	scheduled_hour: slot?.scheduled_hour,
	scheduled_minute: slot?.scheduled_minute,
	scheduled_tz: 'Asia/Jakarta',
	status: 'taken'
});

const rViewer = await fetch(`${API}/care/${med.owner_id}/logs`, {
	method: 'POST',
	headers: H(tokViewer),
	body: bodyCatat
});
check('VIEWER DITOLAK mencatat dosis (403)', rViewer.status === 403, `status ${rViewer.status}`);

// Helper mencatat dosis yang SAMA dengan yang ditolak viewer. Urutannya
// penting: viewer dicoba lebih dulu, dan karena ditolak, slotnya masih
// kosong saat helper mencoba.
const rHelper = await fetch(`${API}/care/${med.owner_id}/logs`, {
	method: 'POST',
	headers: H(tokHelper),
	body: bodyCatat
});
check('HELPER BOLEH mencatat dosis (201)', rHelper.status === 201, `status ${rHelper.status}`);

const rHelperObat = await fetch(`${API}/care/${med.owner_id}/medications`, {
	method: 'POST',
	headers: H(tokHelper),
	body: JSON.stringify({
		name: 'Obat Dari Helper',
		dose_value: 1,
		dose_unit_code: 'tablet',
		route_code: 'O',
		dosage_form_code: 'LOCAL-TABS',
		meal_relation: 'after_meal',
		start_date: today,
		start_time: '08:00',
		frequency: {
			pattern: 'daily',
			times_per_day: 1,
			tolerance_minutes: 60,
			start_time: '08:00',
			schedule_timezone: 'Asia/Jakarta'
		},
		times: [{ time_local: '08:00', label: 'morning' }]
	})
});
check('HELPER DITOLAK mengubah obat (403)', rHelperObat.status === 403, `status ${rHelperObat.status}`);

const rManagerObat = await fetch(`${API}/care/${med.owner_id}/medications`, {
	method: 'POST',
	headers: H(tokManager),
	body: JSON.stringify({
		name: 'Obat Dari Manager',
		dose_value: 1,
		dose_unit_code: 'tablet',
		// route_code wajib, dan `times` harus cocok dengan times_per_day —
		// server menolak kombinasi yang tidak konsisten, dan itu benar.
		route_code: 'O',
		dosage_form_code: 'LOCAL-TABS',
		meal_relation: 'after_meal',
		start_date: today,
		start_time: '08:00',
		frequency: {
			pattern: 'daily',
			times_per_day: 1,
			tolerance_minutes: 60,
			start_time: '08:00',
			schedule_timezone: 'Asia/Jakarta'
		},
		times: [{ time_local: '08:00', label: 'morning' }]
	})
});
check('MANAGER BOLEH mengubah obat (201)', rManagerObat.status === 201, `status ${rManagerObat.status}`);

// ── 3. Blok `permissions` dihitung dari peran, bukan dipatok ──
async function izin(tok) {
	const r = await fetch(`${API}/care/${med.owner_id}/overview`, { headers: H(tok) });
	return (await r.json()).data?.permissions;
}

const izinViewer = await izin(tokViewer);
check('viewer: can_read true, can_write_log false, can_manage_meds false',
	izinViewer?.can_read === true && izinViewer?.can_write_log === false && izinViewer?.can_manage_meds === false,
	JSON.stringify(izinViewer));

const izinHelper = await izin(tokHelper);
check('helper: can_write_log true, can_manage_meds false',
	izinHelper?.can_write_log === true && izinHelper?.can_manage_meds === false,
	JSON.stringify(izinHelper));

const izinManager = await izin(tokManager);
check('manager: can_manage_meds TRUE (sebelumnya dipatok false)',
	izinManager?.can_manage_meds === true,
	JSON.stringify(izinManager));

// ── 4. Privasi: field privat pasien TIDAK boleh terkirim ──
const rMeds = await fetch(`${API}/care/${med.owner_id}/medications`, { headers: H(tokViewer) });
const teksMeds = await rMeds.text();
check('prescription_photo_url TIDAK dikirim ke caregiver', !teksMeds.includes('prescription_photo_url'));
check('notes pribadi pasien TIDAK dikirim ke caregiver', !teksMeds.includes('"notes"'));

// ── 5. UI: kartu pasien bisa diklik ──
const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH });
const ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, locale: 'id-ID' });
const page = await ctx.newPage();
const errs = [];
page.on('pageerror', (e) => errs.push(e.message));

await page.goto(`${BASE}/login`, { waitUntil: 'networkidle' });
await page.waitForFunction(() => {
	const i = document.querySelector('#email');
	return !!i && !i.disabled;
}, null, { timeout: 20000 });
await page.fill('#email', helperEmail);
await page.fill('#password', password);
await page.click('button[type="submit"]');
await page.waitForURL(/hari-ini/, { timeout: 30000 });

await page.goto(`${BASE}/caregiver`, { waitUntil: 'networkidle' });
await page.waitForTimeout(1800);
check('kartu pasien bisa diklik', (await page.locator('a[href^="/caregiver/"]').count()) > 0);

await page.locator('a[href^="/caregiver/"]').first().click();
await page.waitForURL(/caregiver\/[0-9a-f-]+/, { timeout: 20000 }).catch(() => {});
await page.waitForTimeout(2000);

check('halaman pasien terbuka', /caregiver\/[0-9a-f-]+/.test(page.url()), page.url());
const teks = (await page.textContent('main')) ?? '';
check('nama pasien tampil', teks.includes('Pasien Uji'), teks.slice(0, 80));
check('kepatuhan pasien tampil', teks.includes('kepatuhan') || teks.includes('tercatat'));
check('obat pasien tampil', teks.includes('Obat Pasien Uji'));
check('helper MELIHAT tombol catat dosis', teks.includes('Catat dosis atas nama pasien'));

check('tidak ada error JS', errs.length === 0, errs.slice(0, 2).join(' | '));

// ── 6. Viewer TIDAK melihat tombol catat ──
const ctx2 = await browser.newContext({ viewport: { width: 390, height: 844 }, locale: 'id-ID' });
const page2 = await ctx2.newPage();
await page2.goto(`${BASE}/login`, { waitUntil: 'networkidle' });
await page2.waitForFunction(() => {
	const i = document.querySelector('#email');
	return !!i && !i.disabled;
}, null, { timeout: 20000 });
await page2.fill('#email', viewerEmail);
await page2.fill('#password', password);
await page2.click('button[type="submit"]');
await page2.waitForURL(/hari-ini/, { timeout: 30000 });
await page2.goto(`${BASE}/caregiver/${med.owner_id}`, { waitUntil: 'networkidle' });
await page2.waitForTimeout(2000);
const teksViewer = (await page2.textContent('main')) ?? '';
check('viewer TIDAK melihat tombol catat dosis', !teksViewer.includes('Catat dosis atas nama pasien'));
check('viewer diberi tahu batasnya', teksViewer.includes('hanya bisa melihat'));

await browser.close();
console.log('');
if (fail === 0) console.log('e2e-caregiver-pasien: SEMUA LULUS');
else console.log(`e2e-caregiver-pasien: ${fail} GAGAL`);
process.exit(fail === 0 ? 0 : 1);
