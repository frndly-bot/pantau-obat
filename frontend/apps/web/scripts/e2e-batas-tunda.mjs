/**
 * Uji batas penundaan (TRD D2) dari SISI PENGGUNA.
 *
 * Uji API sudah membuktikan permintaan ke-4 ditolak. Yang belum terbukti:
 * apakah PENGGUNA melihat alasannya. Kalau layarnya hanya menampilkan
 * "Gagal menyimpan catatan", pengguna akan menekan tombolnya berulang kali
 * tanpa pernah tahu bahwa dosisnya kini dianggap terlewat — dan aturan yang
 * ditegakkan tetapi tidak dijelaskan sama saja dengan kerusakan.
 */
import { chromium } from 'playwright';

const BASE = process.env.BASE_URL ?? 'http://127.0.0.1:5173';
const email = process.env.UJI_EMAIL ?? `tunda-${Date.now()}@example.com`;
const sandi = 'SandiUji123!';

const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH });
const ctx = await browser.newContext({ locale: 'id-ID' });
const page = await ctx.newPage();

let lulus = 0;
let gagal = 0;
function cek(nama, syarat, info = '') {
	if (syarat) {
		lulus++;
		console.log(`  ✓ ${nama}`);
	} else {
		gagal++;
		console.log(`  ✗ ${nama}${info ? ` — ${info}` : ''}`);
	}
}

const hariIni = new Intl.DateTimeFormat('sv-SE', { timeZone: 'Asia/Jakarta' }).format(new Date());

/** Menyiapkan akun + satu obat dengan jam yang sudah lewat, lewat API. */
async function siapkan() {
	const reg = await fetch(`${BASE}/api/v1/auth/register`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({
			email,
			password: sandi,
			full_name: 'Uji Tunda',
			timezone: 'Asia/Jakarta'
		})
	});
	if (!reg.ok) throw new Error(`registrasi gagal: ${reg.status}`);
	const tok = (await reg.json()).data.access_token;

	// Jam 00:05 supaya dosisnya pasti sudah lewat hari ini, apa pun jam ujinya.
	const buat = await fetch(`${BASE}/api/v1/medications`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${tok}` },
		body: JSON.stringify({
			name: 'Obat Uji Tunda',
			dose_value: 1,
			dose_unit_code: 'tablet',
			route_code: 'O',
			dosage_form_code: 'LOCAL-TABS',
			meal_relation: 'after_meal',
			start_date: hariIni,
			start_time: '00:05',
			frequency: {
				pattern: 'daily',
				times_per_day: 1,
				tolerance_minutes: 60,
				start_time: '00:05',
				schedule_timezone: 'Asia/Jakarta'
			},
			times: [{ time_local: '00:05', label: 'morning' }]
		})
	});
	if (!buat.ok) throw new Error(`buat obat gagal: ${buat.status} ${await buat.text()}`);
	return tok;
}

try {
	await siapkan();
	console.log(`\n=== Akun ${email} siap dengan satu obat ===`);

	// Masuk lewat UI supaya halaman Hari Ini memakai sesi yang sama.
	await page.goto(`${BASE}/login`, { waitUntil: 'networkidle' });
	await page.locator('#email').fill(email);
	await page.locator('#password').fill(sandi);
	await page.getByRole('button', { name: /^masuk$/i }).click();
	await page.waitForURL('**/hari-ini', { timeout: 20000 });
	// Daftar dosis dimuat lewat API setelah halaman terbuka; tanpa jeda ini
	// tombolnya belum ada dan uji akan gagal karena alasan yang salah.
	await page.getByRole('button', { name: /^tunda$/i }).first().waitFor({ timeout: 20000 });
	cek('berhasil masuk dan dosis tampil di halaman Hari Ini', true);

	/** Menekan tombol Tunda, memilih 5 menit, lalu konfirmasi. */
	async function tunda() {
		const tombol = page.getByRole('button', { name: /^tunda$/i }).first();
		if ((await tombol.count()) === 0) return null;
		await tombol.click();
		await page.waitForTimeout(700);

		// Dialog: pilih durasi, lalu tombol footer juga berlabel "Tunda".
		const lima = page.getByRole('button', { name: /^5 menit$/i });
		if ((await lima.count()) > 0) await lima.first().click();
		// Tombol konfirmasi adalah yang TERAKHIR di dialog.
		await page.getByRole('button', { name: /^tunda$/i }).last().click();
		await page.waitForTimeout(2200);
		return (await page.locator('body').innerText()).replace(/\s+/g, ' ');
	}

	console.log('\n=== Naskah dialog menyebut aturannya LEBIH DULU ===');
	await page.getByRole('button', { name: /^tunda$/i }).first().click();
	await page.waitForTimeout(700);
	const teksDialog = (await page.locator('body').innerText()).replace(/\s+/g, ' ');
	cek(
		'dialog menyebut batas tiga kali sebelum pengguna menekan',
		/tiga kali/i.test(teksDialog),
		teksDialog.slice(0, 200)
	);
	// Tutup dialog tanpa menyimpan.
	await page.keyboard.press('Escape');
	await page.waitForTimeout(500);

	console.log('\n=== Tunda berulang sampai batas tercapai ===');
	let pesanTerakhir = '';
	for (let i = 1; i <= 4; i++) {
		const t = await tunda();
		if (t === null) {
			cek(`tombol tunda tersedia pada percobaan ke-${i}`, false, 'tombol hilang');
			break;
		}
		pesanTerakhir = t;
		console.log(`  · percobaan ke-${i} selesai`);
	}

	console.log('\n=== Yang dilihat pengguna saat batas tercapai ===');
	// Inti uji ini: pesannya harus menjelaskan AKIBATNYA, bukan sekadar gagal.
	cek(
		'pesan menyebut dosis dianggap terlewat',
		/terlewat/i.test(pesanTerakhir),
		pesanTerakhir.slice(0, 200)
	);
	cek(
		'bukan pesan generik yang tidak menjelaskan apa pun',
		!/^Gagal menyimpan catatan\.?$/i.test(pesanTerakhir.trim()),
		pesanTerakhir.slice(0, 120)
	);
} catch (e) {
	gagal++;
	console.log(`\n  ✗ uji terhenti: ${e.message}`);
} finally {
	await browser.close();
}

console.log(`\n${'='.repeat(50)}`);
console.log(`LULUS: ${lulus}   GAGAL: ${gagal}`);
process.exit(gagal === 0 ? 0 : 1);
