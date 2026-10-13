/**
 * Uji tata letak di lebar layar yang berbeda (plan Fase 4 item 22).
 *
 * Suite lain memakai satu ukuran (390px). Yang diuji di sini adalah hal yang
 * hanya muncul pada lebar ekstrem:
 *
 *  - 360px — ponsel Android kelas menengah, target utama aplikasi. Di bawah
 *    ini tata letak mulai rusak (tombol berdesakan, teks terpotong).
 *  - 1440px — desktop. Aplikasi ini mobile-first, jadi yang perlu dipastikan
 *    adalah isinya tidak melar mengikuti lebar layar tanpa batas.
 *
 * Yang diperiksa bukan tampilan bagus atau tidak, melainkan hal yang bisa
 * diukur dan selalu salah bila terjadi: overflow horizontal (halaman bisa
 * digeser ke samping), elemen yang lebih lebar dari layar, dan target sentuh
 * yang terlalu kecil untuk jari (< 44px, standar aksesibilitas).
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

// Akun uji dengan satu obat, supaya halaman tidak kosong saat diperiksa.
const email = `layout-${Date.now()}@example.com`;
const reg = await fetch(`${API}/auth/register`, {
	method: 'POST',
	headers: { 'Content-Type': 'application/json', Origin: ORIGIN },
	body: JSON.stringify({ email, password, full_name: 'Uji Tata Letak', timezone: 'Asia/Jakarta' })
});
const token = (await reg.json()).data.access_token;

await fetch(`${API}/medications`, {
	method: 'POST',
	headers: {
		Authorization: `Bearer ${token}`,
		'Content-Type': 'application/json',
		Origin: ORIGIN
	},
	body: JSON.stringify({
		name: 'Paracetamol',
		dosage: '500 mg',
		dosage_form: 'tablet',
		route: 'oral',
		frequency_pattern: 'daily',
		times: ['08:00', '20:00'],
		start_date: new Date().toISOString().slice(0, 10),
		stock_count: 30,
		stock_unit: 'tablet'
	})
});

const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH });

/** Halaman ber-guard yang harus diperiksa. */
const HALAMAN = ['/hari-ini', '/obat', '/riwayat', '/caregiver', '/pengaturan'];

/**
 * Memeriksa satu halaman pada satu lebar layar.
 *
 * `document.documentElement.scrollWidth > clientWidth` adalah tanda pasti
 * ada overflow horizontal. Diukur pada elemen akar, bukan `body`, karena
 * `body` bisa saja tidak melaporkan lebar sebenarnya.
 */
async function periksa(page, jalur, lebar) {
	await page.goto(`${BASE}${jalur}`, { waitUntil: 'networkidle' });
	// Tunggu konten tergambar; halaman ber-guard butuh pemulihan sesi dulu.
	await page.waitForTimeout(1200);

	const hasil = await page.evaluate(() => {
		const de = document.documentElement;
		const overflow = de.scrollWidth - de.clientWidth;

		// Elemen yang lebih lebar dari viewport (penyebab overflow).
		const lebar = window.innerWidth;
		const melar = [];
		for (const el of document.querySelectorAll('body *')) {
			const r = el.getBoundingClientRect();
			if (r.width > lebar + 1 && r.height > 0) {
				const tag = el.tagName.toLowerCase();
				const cls = (el.className?.toString?.() ?? '').slice(0, 40);
				melar.push(`${tag}.${cls} (${Math.round(r.width)}px)`);
			}
		}

		// Target sentuh terlalu kecil (hanya elemen yang bisa ditekan).
		//
		// Tautan DI DALAM KALIMAT dikecualikan, sesuai WCAG 2.5.8 yang
		// menyatakan "inline" sebagai pengecualian: ukurannya dibatasi tinggi
		// baris teks di sekitarnya, dan memaksanya 44px akan merusak paragraf.
		// Yang benar-benar diuji adalah kontrol yang berdiri sendiri —
		// tombol, input, dan tautan bergaya blok.
		const kecil = [];
		for (const el of document.querySelectorAll('button, a[href], input, select')) {
			const r = el.getBoundingClientRect();
			if (r.width === 0 || r.height === 0) continue;
			const gaya = getComputedStyle(el);
			const inlineDalamTeks = gaya.display === 'inline' && el.tagName === 'A';
			if (inlineDalamTeks) continue;
			if (r.height < 44) {
				const label = (el.textContent ?? '').trim().slice(0, 24) || el.tagName.toLowerCase();
				// Sertakan URL: kegagalan di sini sering berarti halaman
				// ber-guard sedang dialihkan ke /login, sehingga yang diukur
				// sebenarnya halaman lain. Tanpa URL, gejalanya menyesatkan.
				kecil.push(`${label} (${Math.round(r.height)}px, ${gaya.display}, di ${location.pathname})`);
			}
		}

		return {
			overflow,
			melar: melar.slice(0, 3),
			kecil: kecil.slice(0, 4),
			jumlahKecil: kecil.length,
			url: location.pathname
		};
	});

	return hasil;
}

// 768 dan 1024 = tablet. Ini yang paling penting untuk uji target sentuh:
// aturan 44px dulu hanya berlaku di dalam `@media (max-width: 720px)`,
// sehingga di rentang inilah target sentuh justru mengecil — tepat pada
// perangkat yang paling mungkin dipegang satu tangan sambil beraktivitas.
for (const lebar of [360, 768, 1024, 1440]) {
	// `locale: 'id-ID'`: uji ini mencari teks Indonesia, sedangkan bawaan
	// Playwright adalah `en-US` sehingga halaman tampil berbahasa Inggris.
	const ctx = await browser.newContext({ viewport: { width: lebar, height: 800 }, locale: 'id-ID' });
	const page = await ctx.newPage();

	// Masuk sekali per konteks; sesi dipulihkan dari cookie refresh.
	await page.goto(`${BASE}/login`);
	await page.waitForFunction(
		() => {
			const i = document.querySelector('#email');
			const b = document.querySelector('button[type="submit"]');
			return !!i && !i.disabled && !!b && !b.disabled;
		},
		null,
		{ timeout: 20000 }
	);
	await page.fill('#email', email);
	await page.fill('#password', password);
	await page.click('button[type="submit"]');
	// Batas waktu lebih longgar dari suite lain: uji ini dijalankan terakhir
	// dalam rangkaian, saat server sudah melayani belasan suite sebelumnya.
	// Halaman Hari Ini juga memuat dua permintaan (jadwal + daftar obat),
	// sehingga lebih lambat daripada halaman lain. Batas 20 detik terbukti
	// kurang pada beban itu, dan kegagalannya bukan bug aplikasi.
	await page.waitForURL(/hari-ini/, { timeout: 45000 });

	console.log(`\n──────── ${lebar}px ────────`);
	for (const jalur of HALAMAN) {
		const h = await periksa(page, jalur, lebar);
		check(`${jalur} tidak bisa digeser ke samping`, h.overflow <= 1, `overflow ${h.overflow}px`);
		if (h.melar.length) {
			check(`${jalur} tidak ada elemen lebih lebar dari layar`, false, h.melar.join(', '));
		}
		if (lebar <= 1024) {
			// Target sentuh 44px diperiksa sampai lebar tablet, BUKAN hanya
			// di 360px. Aturan ini dulu berada di dalam `@media (max-width:
			// 720px)`, sehingga bug-nya justru muncul di rentang yang tadinya
			// dilewati uji ini: tablet. Di desktop dengan kursor ukurannya
			// tidak relevan, jadi 1440 dikecualikan.
			check(
				`${jalur} target sentuh ≥44px`,
				h.jumlahKecil === 0,
				h.jumlahKecil ? `${h.jumlahKecil} terlalu kecil: ${h.kecil.join(', ')}` : ''
			);
		}
	}

	await ctx.close();
}

// Halaman publik juga diperiksa — landing adalah yang paling mungkin melar.
console.log(`\n──────── halaman publik (360px) ────────`);
{
	const ctx = await browser.newContext({ viewport: { width: 360, height: 800 }, locale: 'id-ID' });
	const page = await ctx.newPage();
	for (const jalur of ['/', '/login', '/register']) {
		await page.goto(`${BASE}${jalur}`, { waitUntil: 'networkidle' });
		const h = await page.evaluate(() => {
			const de = document.documentElement;
			return de.scrollWidth - de.clientWidth;
		});
		check(`${jalur} tidak bisa digeser ke samping`, h <= 1, `overflow ${h}px`);
	}
	await ctx.close();
}

await browser.close();

console.log('');
if (fail === 0) console.log('e2e-layout: SEMUA LULUS');
else console.log(`e2e-layout: ${fail} GAGAL`);
process.exit(fail === 0 ? 0 : 1);
