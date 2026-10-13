/**
 * Uji kontras WCAG 2.1 AA untuk token warna.
 *
 * KENAPA UJI INI ADA
 *
 * Dua pasangan warna dari prototipe design gagal ambang AA dan sempat ikut
 * tersalin ke `packages/ui/src/lib/styles/theme.css`:
 *
 *   teks putih di --primary (mode terang)     3.65:1  (butuh 4.5)
 *   teks putih di --destructive (mode gelap)  2.77:1  (butuh 4.5)
 *
 * Yang pertama muncul di setiap tombol konfirmasi; yang kedua tepat di
 * tombol hapus akun. Keduanya tidak terlihat salah di layar — hanya
 * ketahuan dari hitungan. Karena itu angkanya dihitung ulang di sini
 * SETIAP kali token berubah, bukan diperiksa sekali saat ditulis.
 *
 * CARA KERJA
 *
 * 1. Baca nilai token langsung dari theme.css (bukan disalin ke berkas ini),
 *    supaya uji tidak bisa "lulus" karena nilainya sudah usang.
 * 2. Konversi OKLCH -> sRGB dengan rumus resmi CSS Color 4.
 * 3. Hitung rasio kontras WCAG 2.1.
 *
 * Ambang: 4.5:1 teks normal, 3:1 elemen antarmuka & teks besar.
 */
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const cssPath = join(here, '../../../packages/ui/src/lib/styles/theme.css');
const css = readFileSync(cssPath, 'utf8');

/**
 * Mengambil blok token untuk satu tema.
 *
 * Blok terang adalah `:root { … }` pertama, blok gelap adalah yang memuat
 * `data-theme="dark"`. Dipotong berdasarkan penanda komentar agar tidak
 * bergantung pada urutan baris.
 */
function block(theme) {
	if (theme === 'light') {
		const start = css.indexOf(':root {');
		return css.slice(start, css.indexOf('/* ---------------- GELAP'));
	}
	const start = css.indexOf(':root[data-theme="dark"]');
	return css.slice(start, css.indexOf('/* ---------------- Turunan'));
}

/** Membaca nilai satu token dari blok tema. */
function token(theme, name) {
	const m = block(theme).match(new RegExp(`--${name}:\\s*oklch\\(([^)]+)\\)`));
	if (!m) throw new Error(`token --${name} tidak ditemukan di tema ${theme}`);
	const parts = m[1].trim().split(/[\s/]+/).map(Number);
	return { L: parts[0], C: parts[1] ?? 0, H: parts[2] ?? 0 };
}

/** OKLCH -> sRGB linear -> sRGB (CSS Color 4). */
function toSrgb({ L, C, H }) {
	const h = (H * Math.PI) / 180;
	const a = C * Math.cos(h);
	const b = C * Math.sin(h);

	const l_ = L + 0.3963377774 * a + 0.2158037573 * b;
	const m_ = L - 0.1055613458 * a - 0.0638541728 * b;
	const s_ = L - 0.0894841775 * a - 1.291485548 * b;

	const l = l_ ** 3;
	const m = m_ ** 3;
	const s = s_ ** 3;

	const enc = (x) => {
		const v = Math.max(0, Math.min(1, x));
		return v <= 0.0031308 ? 12.92 * v : 1.055 * v ** (1 / 2.4) - 0.055;
	};

	return [
		enc(4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s),
		enc(-1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s),
		enc(-0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s)
	];
}

/** Luminans relatif WCAG. */
function luminance(rgb) {
	const [r, g, b] = rgb.map((x) => (x <= 0.04045 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4));
	return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

/** Rasio kontras WCAG 2.1 dari dua warna sRGB. */
function contrastRgb(fgRgb, bgRgb) {
	const a = luminance(fgRgb);
	const b = luminance(bgRgb);
	const [hi, lo] = a > b ? [a, b] : [b, a];
	return (hi + 0.05) / (lo + 0.05);
}

/** Rasio kontras WCAG 2.1 dari dua token OKLCH. */
function contrast(fg, bg) {
	return contrastRgb(toSrgb(fg), toSrgb(bg));
}

// ── Pasangan yang diuji ──
// `min` 4.5 untuk teks normal; 3.0 untuk elemen antarmuka dan teks besar.
const PAIRS = [
	['light', 'primary-foreground', 'primary', 4.5, 'teks tombol utama'],
	['light', 'destructive-foreground', 'destructive', 4.5, 'teks tombol hapus'],
	['light', 'foreground', 'background', 4.5, 'naskah utama'],
	['light', 'muted-foreground', 'background', 4.5, 'teks sekunder'],
	['light', 'muted-foreground', 'muted', 4.5, 'label di dalam kepingan latar'],
	['light', 'primary', 'background', 4.5, 'tautan & aksen sebagai teks'],
	['light', 'card-foreground', 'card', 4.5, 'naskah di dalam kartu'],

	['dark', 'primary-foreground', 'primary', 4.5, 'teks tombol utama'],
	['dark', 'destructive-foreground', 'destructive', 4.5, 'teks tombol hapus'],
	['dark', 'foreground', 'background', 4.5, 'naskah utama'],
	['dark', 'muted-foreground', 'background', 4.5, 'teks sekunder'],
	['dark', 'muted-foreground', 'muted', 4.5, 'label di dalam kepingan latar'],
	['dark', 'card-foreground', 'card', 4.5, 'naskah di dalam kartu'],
	// --err dipakai sebagai TEKS badge (`text-err`), jadi ia harus lulus
	// sebagai teks di atas latar gelap. Ini alasan mode gelap memakai teks
	// gelap di tombol destructive, bukan menggelapkan --destructive.
	['dark', 'err', 'background', 4.5, 'badge status "terlewat"'],
	['light', 'err', 'background', 4.5, 'badge status "terlewat"'],
	['light', 'ok', 'background', 4.5, 'badge status "sesuai"'],
	['dark', 'ok', 'background', 4.5, 'badge status "sesuai"'],
	// Elemen antarmuka (ambang 3:1).
	//
	// CATATAN PENTING soal --border di mode terang.
	//
	// WCAG 2.1 §1.4.11 mensyaratkan 3:1 HANYA untuk elemen yang diperlukan
	// untuk memahami antarmuka — misalnya garis batas kolom input, yang
	// tanpanya pengguna tidak tahu di mana harus mengetik. Garis batas
	// dekoratif (tepi kartu, pemisah antar-bagian) TIDAK termasuk, dan
	// menuntutnya 3:1 akan memaksa garis gelap yang membuat tampilan
	// bergaris-garis. Karena itu:
	//
	//   --input   diuji 3:1  → ini garis kolom form, fungsional
	//   --border TIDAK diuji → dekoratif, sengaja halus
	//
	// Keduanya sempat bernilai sama di prototipe (0.922); sekarang --input
	// digelapkan agar memenuhi ambang, --border dibiarkan apa adanya.
	['light', 'ring', 'background', 3.0, 'cincin fokus'],
	['dark', 'ring', 'background', 3.0, 'cincin fokus'],
	['light', 'input', 'background', 3.0, 'garis kolom form'],
	['dark', 'input', 'background', 3.0, 'garis kolom form']
];

let fail = 0;
let lastTheme = '';
for (const [theme, fg, bg, min, label] of PAIRS) {
	if (theme !== lastTheme) {
		console.log(`\n── ${theme.toUpperCase()} ──`);
		lastTheme = theme;
	}
	const r = contrast(token(theme, fg), token(theme, bg));
	const ok = r >= min;
	if (!ok) fail++;
	console.log(
		`${ok ? 'OK  ' : 'GAGAL'} ${r.toFixed(2).padStart(5)}:1  (butuh ${min})  --${fg} di --${bg}  · ${label}`
	);
}

/**
 * BADGE — diuji terpisah karena latarnya BUKAN halaman.
 *
 * Badge memakai pola `bg-warna/15 text-warna`. Artinya latar yang benar-benar
 * dilihat mata adalah campuran 15% warna itu sendiri ke atas halaman. Menguji
 * `text-ok` terhadap putih akan memberi angka yang terlalu optimistis dan
 * MELOLOSKAN warna yang sebenarnya gagal.
 *
 * Inilah yang terjadi pada prototipe: keenam warna badge di mode terang
 * berkisar 1.91:1 – 4.17:1 terhadap latar badge-nya sendiri. Semuanya gagal.
 */
function badgeBackground(theme, name) {
	const page = toSrgb(token(theme, 'background'));
	const color = toSrgb(token(theme, name));
	return color.map((c, i) => c * 0.15 + page[i] * 0.85);
}

const BADGE_COLORS = ['ok', 'warn', 'err', 'info', 'na', 'private'];
for (const theme of ['light', 'dark']) {
	console.log(`\n── BADGE · ${theme.toUpperCase()} (latar = warna/15 di atas halaman) ──`);
	for (const name of BADGE_COLORS) {
		const r = contrastRgb(toSrgb(token(theme, name)), badgeBackground(theme, name));
		const ok = r >= 4.5;
		if (!ok) fail++;
		console.log(
			`${ok ? 'OK  ' : 'GAGAL'} ${r.toFixed(2).padStart(5)}:1  (butuh 4.5)  badge --${name}`
		);
	}
}

console.log('');
if (fail === 0) {
	console.log('check-contrast: SEMUA LULUS (WCAG 2.1 AA)');
} else {
	console.log(`check-contrast: ${fail} GAGAL — perbaiki token sebelum melanjutkan`);
}
process.exit(fail === 0 ? 0 : 1);
