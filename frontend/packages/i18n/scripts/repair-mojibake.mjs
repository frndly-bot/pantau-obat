#!/usr/bin/env node
/**
 * Memperbaiki mojibake pada kamus i18n.
 *
 * MASALAH: `design/assets/i18n.js` sudah salah-encode sejak awal. Karakter
 * UTF-8 asli diperlakukan seolah-olah satu byte = satu karakter, lalu
 * di-encode UTF-8 lagi. Contoh:
 *   "6–20 digit"  →  "6â€"20 digit"
 *   "akun…"       →  "akunâ€¦"
 *
 * Kuncinya: konversi byte→karakter memakai **Windows-1252**, bukan latin-1.
 * Bedanya penting — pada cp1252, byte 0x80–0x9F dipetakan ke karakter khusus
 * (0x80→€, 0x93→", 0x85→…, 0x94→", …), bukan ke karakter kontrol.
 *
 * PEMULIHAN: ubah tiap karakter kembali menjadi byte lewat tabel cp1252,
 * lalu decode byte itu sebagai UTF-8. Hasilnya dipakai hanya bila bersih
 * (tanpa U+FFFD), sehingga teks yang memang bukan mojibake tidak dirusak.
 */
import { readFileSync, writeFileSync } from 'node:fs';

/** Peta karakter cp1252 (0x80–0x9F) → byte aslinya. */
const CP1252_TO_BYTE = new Map([
	[0x20ac, 0x80], [0x201a, 0x82], [0x0192, 0x83], [0x201e, 0x84],
	[0x2026, 0x85], [0x2020, 0x86], [0x2021, 0x87], [0x02c6, 0x88],
	[0x2030, 0x89], [0x0160, 0x8a], [0x2039, 0x8b], [0x0152, 0x8c],
	[0x017d, 0x8e], [0x2018, 0x91], [0x2019, 0x92], [0x201c, 0x93],
	[0x201d, 0x94], [0x2022, 0x95], [0x2013, 0x96], [0x2014, 0x97],
	[0x02dc, 0x98], [0x2122, 0x99], [0x0161, 0x9a], [0x203a, 0x9b],
	[0x0153, 0x9c], [0x017e, 0x9e], [0x0178, 0x9f],
]);

/** Karakter yang bisa berasal dari satu byte mojibake. */
function toByte(codePoint) {
	if (codePoint >= 0x00 && codePoint <= 0xff) return codePoint;
	return CP1252_TO_BYTE.get(codePoint) ?? null;
}

/**
 * Pola karakter yang bisa muncul dari byte mojibake cp1252:
 * latin-1 (U+0080–U+00FF) + karakter khusus cp1252 (€ U+20AC, … U+2026, dst).
 * Rentang tidak bisa dipakai karena karakter khusus itu tersebar di beberapa
 * blok Unicode (U+0192, U+20AC, U+2122, …), jadi didaftar eksplisit.
 */
const MOJIBAKE_CHARS =
	'\\u0080-\\u00ff\\u0152\\u0153\\u0160\\u0161\\u0178\\u017d\\u017e' +
	'\\u0192\\u02c6\\u02dc\\u2013\\u2014\\u2018\\u2019\\u201a\\u201c\\u201d' +
	'\\u201e\\u2020\\u2021\\u2022\\u2026\\u2030\\u2039\\u203a\\u20ac\\u2122';

const MOJIBAKE_RE = new RegExp(`[${MOJIBAKE_CHARS}]+`, 'g');
const MOJIBAKE_TEST = new RegExp(`[${MOJIBAKE_CHARS}]`);

/**
 * Menemukan run karakter yang menyerupai mojibake dan mencoba memulihkannya.
 */
function repair(text) {
	if (typeof text !== 'string' || !MOJIBAKE_TEST.test(text)) return text;

	return text.replace(MOJIBAKE_RE, (run) => {
		const bytes = [];
		for (const ch of run) {
			const byte = toByte(ch.codePointAt(0));
			if (byte === null) return run; // ada karakter di luar dugaan
			bytes.push(byte);
		}
		const fixed = Buffer.from(bytes).toString('utf8');
		if (fixed.includes('\uFFFD')) return run;
		return fixed;
	});
}

let total = 0;
for (const loc of ['id', 'en']) {
	const path = new URL(`../locales/${loc}.json`, import.meta.url);
	const data = JSON.parse(readFileSync(path, 'utf8'));
	let count = 0;
	for (const domain of Object.keys(data)) {
		const bucket = data[domain];
		if (!bucket || typeof bucket !== 'object') continue;
		for (const key of Object.keys(bucket)) {
			const before = bucket[key];
			const after = repair(before);
			if (after !== before) {
				bucket[key] = after;
				count++;
			}
		}
	}
	writeFileSync(path, JSON.stringify(data, null, 2) + '\n');
	console.log(`${loc}: ${count} entri diperbaiki`);
	total += count;
}
console.log(`total: ${total}`);
