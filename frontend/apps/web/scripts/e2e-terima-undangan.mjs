/**
 * Uji alur terima undangan lewat tautan email (halaman /terima-undangan).
 *
 * Yang diuji adalah hal-hal yang mudah salah dan tidak terlihat dari kode:
 *
 * 1. Tautan undangan yang dibangun backend benar-benar membuka halaman
 *    (sebelum ini 404 — alur caregiver buntu).
 * 2. Pengunjung yang BELUM masuk diarahkan ke login dan, setelah masuk,
 *    kembali ke halaman undangan — bukan terdampar di beranda.
 * 3. Email yang cocok bisa menerima, dan setelah itu relasi benar-benar
 *    `accepted` di backend (bukan hanya tampilan yang berubah).
 * 4. Email yang TIDAK cocok tidak bisa menerima (backend menolak 403), dan
 *    halaman menjelaskannya alih-alih menampilkan tombol yang pasti gagal.
 * 5. Menolak undangan juga bekerja.
 *
 * Dijalankan terhadap backend dan frontend yang benar-benar berjalan.
 */
import { chromium } from 'playwright';

const API = 'http://127.0.0.1:8081/api/v1';
const BASE = 'http://127.0.0.1:5173';
const ORIGIN = 'http://localhost:5173';
const password = 'SandiUji123!';

let fail = 0;
const check = (l, ok, d = '') => {
	console.log(`${ok ? 'OK  ' : 'GAGAL'} ${l}${d ? ' — ' + d : ''}`);
	if (!ok) fail++;
};

const stamp = Date.now();
const ownerEmail = `own-inv-${stamp}@example.com`;
const goodEmail = `cg-good-${stamp}@example.com`;
const wrongEmail = `cg-wrong-${stamp}@example.com`;

const reg = async (e, n) => {
	const r = await fetch(`${API}/auth/register`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json', Origin: ORIGIN },
		body: JSON.stringify({ email: e, password, full_name: n, timezone: 'Asia/Jakarta' })
	});
	return (await r.json()).data.access_token;
};

const ownerTok = await reg(ownerEmail, 'Pemilik Undangan');
await reg(goodEmail, 'Pendamping Benar');
await reg(wrongEmail, 'Pendamping Salah');

/** Membuat undangan dan mengembalikan URL lengkapnya. */
async function invite(email, relation = 'child', role = 'helper') {
	const r = await fetch(`${API}/care-links`, {
		method: 'POST',
		headers: {
			Authorization: `Bearer ${ownerTok}`,
			'Content-Type': 'application/json',
			Origin: ORIGIN
		},
		body: JSON.stringify({ invite_email: email, relation, role })
	});
	const body = await r.json();
	return { status: r.status, url: body.data?.invite_url ?? null };
}

/** Membaca status relasi dari sisi pemilik. */
async function ownerLinks() {
	const r = await fetch(`${API}/care-links?as=owner`, {
		headers: { Authorization: `Bearer ${ownerTok}`, Origin: ORIGIN }
	});
	return (await r.json()).data ?? [];
}

const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH });

/**
 * Memasukkan kredensial pada halaman login yang sedang tampil.
 *
 * Menunggu hidrasi selesai LEBIH DULU, sebelum mengisi apa pun. Halaman
 * login di-prerender: form sudah tampil sebelum JavaScript siap, dan input
 * sengaja dikunci sampai hidrasi selesai supaya isian yang diketik lebih
 * awal tidak dihapus oleh hidrasi.
 */
async function loginOn(page, email) {
	await page.waitForSelector('#email', { timeout: 10000 });
	await waitForHydration(page);
	await page.fill('#email', email);
	await page.fill('#password', password);
	await page.click('button[type="submit"]');
}

/**
 * Menunggu sampai form siap dipakai.
 *
 * Tanda yang dipakai adalah input email tidak lagi disabled — atribut yang
 * memang dipasang aplikasi untuk menandai hidrasi selesai.
 *
 * CATATAN: di halaman DAFTAR, tombol submit sengaja tetap terkunci sampai
 * checkbox persetujuan medis dicentang. Jadi di sana tanda "siap" adalah
 * input yang tidak disabled, BUKAN tombol yang aktif — lihat
 * `setujuiDanDaftar`.
 */
async function waitForHydration(page, timeout = 15000) {
	await page.waitForFunction(
		() => {
			const input = document.querySelector('#email');
			return !!input && !input.disabled;
		},
		null,
		{ timeout }
	);
}

/**
 * Mencentang persetujuan medis di halaman daftar, lalu mengirim form.
 *
 * Halaman daftar mengunci tombolnya sampai pengguna menyatakan mengerti
 * bahwa aplikasi ini alat bantu, bukan pengganti nasihat dokter. Uji yang
 * menekan tombol tanpa mencentang akan menunggu selamanya.
 */
async function setujuiDanDaftar(page) {
	const kotak = page.locator('input[type="checkbox"]').first();
	if ((await kotak.count()) > 0) await kotak.check();
	await page.click('button[type="submit"]');
}

/** Menunggu sampai PATHNAME cocok. */
async function waitForPath(page, re, timeout = 15000) {
	await page.waitForFunction(
		(pattern) => new RegExp(pattern).test(window.location.pathname),
		re.source,
		{ timeout }
	);
}

// ============================================================
// 1. Tautan undangan membuka halaman (dulu 404)
// ============================================================
const inv1 = await invite(goodEmail);
check('undangan dibuat', inv1.status === 201 || inv1.status === 200, `status ${inv1.status}`);
check('balasan memuat invite_url', !!inv1.url);
check(
	'invite_url menunjuk ke /terima-undangan',
	(inv1.url ?? '').includes('/terima-undangan?token='),
	inv1.url ?? '(kosong)'
);

// ============================================================
// 2. Belum masuk → diminta masuk → kembali ke undangan
// ============================================================
const ctxGuest = await browser.newContext({ viewport: { width: 390, height: 844 } });
const guest = await ctxGuest.newPage();
const guestErrs = [];
guest.on('pageerror', (e) => guestErrs.push(e.message));

await guest.goto(inv1.url, { waitUntil: 'networkidle' });
const guestText = await guest.locator('body').innerText();
check('halaman undangan terbuka (bukan 404)', !guestText.includes('404'));
check(
	'pengunjung belum masuk diminta masuk dulu',
	/undangan pemantauan|masuk untuk menerima/i.test(guestText),
	guestText.slice(0, 60).replace(/\n/g, ' ')
);

// Klik "Masuk untuk menerima" → harus ke /login dengan next
await guest.click('text=Masuk untuk menerima');
await waitForPath(guest, /^\/login$/, 10000);
const loginUrl = new URL(guest.url());
check('diarahkan ke /login', loginUrl.pathname === '/login');
check(
	'login membawa ?next= ke halaman undangan',
	(loginUrl.searchParams.get('next') ?? '').includes('/terima-undangan'),
	loginUrl.searchParams.get('next') ?? '(kosong)'
);

// Masuk dengan email yang BENAR → harus kembali ke undangan, bukan beranda
await loginOn(guest, goodEmail);
await waitForPath(guest, /^\/terima-undangan$/);
check('setelah masuk kembali ke halaman undangan (bukan beranda)', /terima-undangan/.test(guest.url()), guest.url());

const confirmText = await guest.locator('body').innerText();
check(
	'email yang cocok melihat tombol Terima',
	/terima undangan/i.test(confirmText),
	confirmText.slice(0, 60).replace(/\n/g, ' ')
);

// Terima
await guest.click('button:has-text("Terima undangan")');
await guest.waitForSelector('text=Undangan diterima', { timeout: 15000 });
check('penerimaan berhasil di UI', true);

// Verifikasi NYATA di backend, bukan hanya tampilan
const afterAccept = await ownerLinks();
const accepted = afterAccept.find((l) => l.invite_email === goodEmail);
check(
	'status di backend benar-benar accepted',
	accepted?.status === 'accepted',
	accepted?.status ?? '(tidak ditemukan)'
);
check(
	'caregiver_id terisi setelah diterima',
	!!accepted?.caregiver_id,
	accepted?.caregiver_id ?? '(kosong)'
);
check('tidak ada error JS di alur terima', guestErrs.length === 0, guestErrs.join('; '));

// ============================================================
// 3. Email BERBEDA tidak bisa menerima
// ============================================================
const inv2 = await invite(goodEmail);
check('undangan kedua dibuat', !!inv2.url);

const ctxWrong = await browser.newContext({ viewport: { width: 390, height: 844 } });
const wrong = await ctxWrong.newPage();
const wrongErrs = [];
wrong.on('pageerror', (e) => wrongErrs.push(e.message));

// Masuk dulu sebagai akun yang salah, lalu buka tautan undangan
await wrong.goto(`${BASE}/login`);
await loginOn(wrong, wrongEmail);
await waitForPath(wrong, /^\/hari-ini$/);

await wrong.goto(inv2.url, { waitUntil: 'networkidle' });
await wrong.waitForSelector('button:has-text("Terima undangan")', { timeout: 10000 });
await wrong.click('button:has-text("Terima undangan")');

// Backend menolak 403 → halaman harus menjelaskan, bukan diam
await wrong.waitForSelector('text=Undangan untuk akun lain', { timeout: 15000 });
const wrongText = await wrong.locator('body').innerText();
check('email berbeda dijelaskan sebagai akun lain', /undangan untuk akun lain/i.test(wrongText));
check('ada jalan keluar (masuk akun lain)', /masuk dengan akun lain/i.test(wrongText));

// Backend TIDAK boleh mengubah status
const stillPending = (await ownerLinks()).find(
	(l) => l.invite_email === goodEmail && l.status === 'pending'
);
check('undangan tetap pending setelah percobaan email salah', !!stillPending);
check('tidak ada error JS di alur email salah', wrongErrs.length === 0, wrongErrs.join('; '));

// ============================================================
// 4. Menolak undangan
// ============================================================
// Undangan untuk email yang sama tidak boleh menumpuk: index
// uq_care_pending_invite menolak undangan baru selama yang lama masih
// `pending` (backend membalas 409 INVITE_PENDING). Karena undangan kedua di
// blok sebelumnya sengaja dibiarkan pending, uji penolakan memakai akun lain.
const rejectEmail = `cg-reject-${stamp}@example.com`;
await reg(rejectEmail, 'Pendamping Penolak');
const inv3 = await invite(rejectEmail, 'family', 'viewer');
const ctxReject = await browser.newContext({ viewport: { width: 390, height: 844 } });
const rejecter = await ctxReject.newPage();

await rejecter.goto(`${BASE}/login`);
await loginOn(rejecter, rejectEmail);
await waitForPath(rejecter, /^\/hari-ini$/);

await rejecter.goto(inv3.url, { waitUntil: 'networkidle' });
await rejecter.waitForSelector('button:has-text("Tolak")', { timeout: 10000 });
await rejecter.click('button:has-text("Tolak")');
await rejecter.waitForSelector('text=Undangan ditolak', { timeout: 15000 });
check('penolakan berhasil di UI', true);

const rejected = (await ownerLinks()).find((l) => l.invite_email === rejectEmail && l.status === 'rejected');
check('status di backend menjadi rejected', !!rejected, rejected?.status ?? '(tidak ada)');

// ============================================================
// 5. Token tidak valid / sudah dipakai
// ============================================================
const ctxBad = await browser.newContext({ viewport: { width: 390, height: 844 } });
const bad = await ctxBad.newPage();
await bad.goto(`${BASE}/login`);
await loginOn(bad, goodEmail);
await waitForPath(bad, /^\/hari-ini$/);

await bad.goto(`${BASE}/terima-undangan?token=token-palsu-tidak-ada`, { waitUntil: 'networkidle' });
await bad.waitForSelector('button:has-text("Terima undangan")', { timeout: 10000 });
await bad.click('button:has-text("Terima undangan")');
await bad.waitForSelector('text=Undangan tidak dapat dibuka', { timeout: 15000 });
const badText = await bad.locator('body').innerText();
check('token palsu dijelaskan dengan menolong', /tidak ditemukan/i.test(badText), badText.slice(0, 80).replace(/\n/g, ' '));

// Halaman tanpa token sama sekali
await bad.goto(`${BASE}/terima-undangan`, { waitUntil: 'networkidle' });
const noTokenText = await bad.locator('body').innerText();
check('tanpa token dijelaskan sebagai tautan tidak lengkap', /tidak lengkap/i.test(noTokenText));

// ============================================================
// 6. REGRESI: isian sebelum hidrasi tidak boleh hilang
// ============================================================
// Hidrasi Svelte menimpa nilai DOM dengan state awal. Sebelum diperbaiki,
// pengguna yang mengetik selagi halaman masih dimuat melihat isiannya
// HILANG tanpa pesan apa pun — dan tidak ada permintaan jaringan sama
// sekali. Di ponsel lambat, itu jalur yang paling sering terjadi.
{
	const ctxH = await browser.newContext({ viewport: { width: 390, height: 844 } });
	const ph = await ctxH.newPage();
	const reqs = [];
	ph.on('request', (r) => {
		if (r.url().includes('/auth/login')) reqs.push(r.url());
	});

	await ph.goto(`${BASE}/login`);
	await ph.waitForSelector('#email', { timeout: 10000 });

	// Isi SEGERA, tanpa menunggu hidrasi — persis seperti pengguna yang cepat.
	await ph.fill('#email', goodEmail).catch(() => {});
	await ph.fill('#password', password).catch(() => {});

	await waitForHydration(ph);

	// Setelah hidrasi selesai, isian harus MASIH ADA (input dikunci, jadi
	// hidrasi tidak menghapusnya) — atau form memang belum bisa diisi.
	const nilai = await ph.evaluate(() => ({
		email: document.querySelector('#email')?.value ?? '',
		passLen: (document.querySelector('#password')?.value ?? '').length
	}));
	check(
		'isian tidak hilang diam-diam setelah hidrasi',
		nilai.email === goodEmail && nilai.passLen > 0,
		`email="${nilai.email}" passLen=${nilai.passLen}`
	);

	await ph.click('button[type="submit"]');
	await waitForPath(ph, /^\/hari-ini$/, 20000);
	check('login tetap berhasil setelah mengisi sebelum hidrasi', reqs.length > 0);
	await ctxH.close();
}

await browser.close();

console.log('');
if (fail === 0) console.log('e2e-terima-undangan: SEMUA LULUS');
else console.log(`e2e-terima-undangan: ${fail} GAGAL`);
process.exit(fail === 0 ? 0 : 1);
