/**
 * Uji aplikasi lewat Cloudflare Tunnel (akses dari internet sungguhan).
 *
 * Yang diuji di sini adalah hal yang TIDAK bisa dibuktikan dari localhost:
 * apakah cookie refresh (`po_refresh`, SameSite=Strict, Secure) benar-benar
 * tersimpan dan bertahan saat aplikasi diakses lewat HTTPS publik.
 *
 * Kalau halaman dan API tidak berada di satu origin — atau atribut cookie
 * tidak cocok dengan HTTPS — gejalanya khas: login tampak berhasil, lalu
 * halaman memantul ke /login begitu dimuat ulang. Uji ini memeriksa tepat
 * hal itu, dan memeriksa cookie-nya langsung, bukan hanya tampilan.
 */
import { chromium } from 'playwright';

const BASE = process.env.TUNNEL_URL;
if (!BASE) {
	console.error('TUNNEL_URL harus diisi, mis. https://xxx.trycloudflare.com');
	process.exit(2);
}

const password = 'SandiUji123!';
const email = `tunnel-e2e-${Date.now()}@example.com`;

let fail = 0;
const check = (l, ok, d = '') => {
	console.log(`${ok ? 'OK  ' : 'GAGAL'} ${l}${d ? ' — ' + d : ''}`);
	if (!ok) fail++;
};

const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH });
const ctx = await browser.newContext({ viewport: { width: 390, height: 844 } });
const page = await ctx.newPage();

const errs = [];
page.on('pageerror', (e) => errs.push(e.message));

// ── Daftar lewat tunnel ──
console.log('── Daftar akun lewat internet ──');
await page.goto(`${BASE}/register`, { waitUntil: 'networkidle' });
// Tanda "siap" adalah INPUT yang tidak disabled, bukan tombol yang aktif:
// tombol daftar sengaja tetap terkunci sampai persetujuan medis dicentang.
await page.waitForFunction(
	() => {
		const i = document.querySelector('#email');
		return !!i && !i.disabled;
	},
	null,
	{ timeout: 25000 }
);
await page.fill('#full_name', 'Uji Tunnel');
await page.fill('#email', email);
await page.fill('#password', password);
// Persetujuan medis wajib dicentang sebelum tombol daftar aktif.
await page.locator('input[type="checkbox"]').first().check();
await page.click('button[type="submit"]');
await page.waitForURL(/hari-ini/, { timeout: 30000 });
check('pendaftaran berhasil lewat tunnel', true);

// ── Cookie refresh benar-benar terpasang? ──
console.log('\n── Cookie refresh ──');
const cookies = await ctx.cookies();
const refresh = cookies.find((c) => c.name === 'po_refresh');
check('cookie po_refresh terpasang', !!refresh);
if (refresh) {
	check('cookie HttpOnly (tidak terbaca JS)', refresh.httpOnly);
	check('cookie Secure (hanya lewat HTTPS)', refresh.secure);
	check('cookie SameSite=Strict', /strict/i.test(refresh.sameSite), refresh.sameSite);
	check(
		'cookie terbatas pada path auth',
		refresh.path.startsWith('/api/v1/auth'),
		refresh.path
	);
}

// ── Sesi bertahan setelah muat ulang (uji paling penting) ──
console.log('\n── Muat ulang: sesi harus bertahan ──');
await page.reload({ waitUntil: 'networkidle' });
await page.waitForTimeout(2500);
check('tetap di halaman ber-guard setelah refresh', /hari-ini/.test(page.url()), page.url());

// ── Muat ulang kedua kali, kali ini langsung ──
await page.goto(`${BASE}/obat`, { waitUntil: 'networkidle' });
await page.waitForTimeout(2000);
check('halaman lain terbuka tanpa diminta masuk lagi', /obat/.test(page.url()), page.url());

// ── Data bertahan: buat obat, muat ulang, masih ada ──
console.log('\n── Data bertahan antar muat ulang ──');
const namaObat = `TunnelObat${Date.now() % 100000}`;
const token = await page.evaluate(async () => {
	// Ambil access token lewat pemulihan sesi yang sama dengan aplikasi:
	// cookie refresh dikirim otomatis, balasannya berisi token baru.
	const r = await fetch('/api/v1/auth/refresh', { method: 'POST' });
	const j = await r.json();
	return j?.data?.access_token ?? null;
});
check('refresh token bekerja dari dalam halaman', !!token);

if (token) {
	const dibuat = await page.evaluate(
		async ([t, nama]) => {
			const r = await fetch('/api/v1/medications', {
				method: 'POST',
				headers: { Authorization: `Bearer ${t}`, 'Content-Type': 'application/json' },
				// Struktur mengikuti kontrak backend: `frequency` bersarang,
				// dan kode referensi diambil dari tabel ref_* (route 'O' =
				// oral, form 'LOCAL-TAB' = tablet). Mengarang kode sendiri
				// menghasilkan 500 dari pelanggaran foreign key — sudah
				// dibuktikan, dan itu bukan bug aplikasi melainkan kontrak
				// yang tidak diikuti.
				body: JSON.stringify({
					name: nama,
					dose_value: 500,
					dose_unit_code: 'mg',
					route_code: 'O',
					dosage_form_code: 'LOCAL-TAB',
					start_date: new Date().toISOString().slice(0, 10),
					frequency: { pattern: 'daily', times_per_day: 1 },
					times: [{ time_local: '08:00' }],
					stock: { unit_code: 'tablet', initial_qty: 10, qty_per_dose: 1 }
				})
			});
			return r.status;
		},
		[token, namaObat]
	);
	check('obat dibuat lewat tunnel', dibuat === 201, `status ${dibuat}`);

	await page.goto(`${BASE}/obat`, { waitUntil: 'networkidle' });
	await page.waitForTimeout(2000);
	const isi = await page.locator('body').innerText();
	check('obat tampil di daftar setelah muat ulang', isi.includes(namaObat), namaObat);
}

// ── Halaman publik tetap bisa diakses tanpa sesi ──
console.log('\n── Halaman publik ──');
const ctxAnon = await browser.newContext({ viewport: { width: 390, height: 844 } });
const anon = await ctxAnon.newPage();
for (const [jalur, harapan] of [
	['/', /pantau obat/i],
	['/login', /masuk/i],
	['/register', /daftar|buat akun/i]
]) {
	await anon.goto(`${BASE}${jalur}`, { waitUntil: 'networkidle' });
	const t = await anon.locator('body').innerText();
	check(`${jalur} tampil tanpa sesi`, harapan.test(t));
}
await ctxAnon.close();

// ── Halaman ber-guard menolak tamu ──
const ctxTamu = await browser.newContext({ viewport: { width: 390, height: 844 } });
const tamu = await ctxTamu.newPage();
await tamu.goto(`${BASE}/hari-ini`, { waitUntil: 'networkidle' });
await tamu.waitForTimeout(2500);
check('tamu diarahkan ke login', /login/.test(tamu.url()), tamu.url());
await ctxTamu.close();

check('tidak ada error JS', errs.length === 0, errs.slice(0, 2).join(' | '));

await browser.close();
console.log('');
if (fail === 0) console.log('e2e-tunnel: SEMUA LULUS');
else console.log(`e2e-tunnel: ${fail} GAGAL`);
process.exit(fail === 0 ? 0 : 1);
