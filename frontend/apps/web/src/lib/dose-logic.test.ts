/**
 * Uji unit logika dosis bersama.
 *
 * Yang paling penting diuji di sini adalah contoh nyata dari review UX:
 * tiga dosis sehari, pagi sudah diminum, siang dan malam belum waktunya.
 * Server menghitung 1/(3−2) = 100%, sedangkan versi lama Hari Ini
 * menghitung 1/3 = 33%. Fungsi bersama harus menghasilkan angka SERVER.
 */
import { describe, expect, it } from 'vitest';
import {
	computeRatio,
	countActionable,
	countDone,
	formatRatio,
	rankSlot,
	summarizeDay
} from './dose-logic';

describe('computeRatio — mengikuti aturan server', () => {
	it('hanya menghitung `taken`, bukan `late`', () => {
		// `late` adalah keterlambatan, bukan kepatuhan. Server tidak
		// memasukkannya ke pembilang; klien harus sama.
		const slots = [
			{ status: 'taken' },
			{ status: 'late' },
			{ status: 'skipped' }
		];
		expect(computeRatio(slots)).toBeCloseTo(1 / 3);
	});

	it('mengecualikan dosis yang masih menunggu dari penyebut', () => {
		// Contoh dari review UX: pagi sudah diminum, siang & malam belum
		// waktunya. Penyebut = 3 − 2 = 1, jadi 100% — bukan 33%.
		const slots = [{ status: 'taken' }, { status: 'pending' }, { status: 'pending' }];
		expect(computeRatio(slots)).toBe(1);
		expect(formatRatio(computeRatio(slots))).toBe('100%');
	});

	it('mengembalikan null bila belum ada yang bisa dinilai', () => {
		// Ini yang paling mudah salah: `null` bukan 0%. Menampilkannya
		// sebagai 0% membuat "belum ada data" terbaca sebagai "gagal semua".
		const slots = [{ status: 'pending' }, { status: 'due' }];
		expect(computeRatio(slots)).toBeNull();
		expect(formatRatio(null)).toBe('Belum ada data');
	});

	it('mengembalikan null untuk daftar kosong', () => {
		expect(computeRatio([])).toBeNull();
	});

	it('membatasi hasil maksimum 100%', () => {
		// Catatan `taken` bisa lebih banyak dari slot terjadwal (mis. PRN),
		// dan itu tidak boleh menghasilkan lebih dari 100%.
		const slots = [{ status: 'taken' }, { status: 'taken' }, { status: 'taken' }, { status: 'skipped' }];
		const r = computeRatio(slots);
		expect(r).toBeLessThanOrEqual(1);
		expect(r).toBeCloseTo(0.75);
	});

	it('tidak pernah melebihi 100% meski pembilang lebih besar dari penyebut', () => {
		// Keadaan ini bisa terjadi: PRN dicatat berkali-kali tanpa slot
		// terjadwal, sedangkan penyebut hanya menghitung slot yang pasti.
		// Tanpa pembatasan, hasilnya akan tampil sebagai 300%.
		const slots = [{ status: 'taken' }, { status: 'taken' }, { status: 'taken' }];
		// Penyebut 3, pembilang 3 → tepat 100%, bukan lebih.
		expect(computeRatio(slots)).toBe(1);
	});

	it('menghitung 0% bila semua dosis yang pasti justru dilewati', () => {
		// 0% yang SEBENARNYA berbeda dari "belum ada data".
		const slots = [{ status: 'skipped' }, { status: 'skipped' }];
		expect(computeRatio(slots)).toBe(0);
		expect(formatRatio(computeRatio(slots))).toBe('0%');
	});

	it('membedakan "belum ada data" dari "0%"', () => {
		// Dua keadaan ini tampak sama bila keduanya dicetak 0%.
		const belumAda = formatRatio(computeRatio([{ status: 'pending' }]));
		const nolPersen = formatRatio(computeRatio([{ status: 'skipped' }]));
		expect(belumAda).not.toBe(nolPersen);
		expect(belumAda).toBe('Belum ada data');
		expect(nolPersen).toBe('0%');
	});
});

describe('penghitung', () => {
	it('countDone mencakup taken, late, dan skipped', () => {
		expect(countDone([{ status: 'taken' }, { status: 'late' }, { status: 'skipped' }])).toBe(3);
	});

	it('countDone tidak menghitung pending, due, maupun snoozed', () => {
		expect(
			countDone([{ status: 'pending' }, { status: 'due' }, { status: 'snoozed' }])
		).toBe(0);
	});

	it('countActionable menghitung due dan pending', () => {
		expect(
			countActionable([{ status: 'due' }, { status: 'pending' }, { status: 'taken' }])
		).toBe(2);
	});
});

describe('summarizeDay', () => {
	it('menyebut jumlah menunggu secara terpisah', () => {
		// Tanpa ini, "3 dari 7" terlihat seperti kegagalan padahal 4 sisanya
		// belum waktunya.
		const s = summarizeDay([
			{ status: 'taken' },
			{ status: 'late' },
			{ status: 'skipped' },
			{ status: 'pending' },
			{ status: 'due' }
		]);
		expect(s.done).toBe(3);
		expect(s.total).toBe(5);
		expect(s.waiting).toBe(2);
		expect(s.text).toBe('3 dari 5 tercatat · 2 menunggu');
	});

	it('tidak menyebut "menunggu" bila tidak ada yang menunggu', () => {
		const s = summarizeDay([{ status: 'taken' }, { status: 'skipped' }]);
		expect(s.text).toBe('2 dari 2 tercatat');
	});
});

describe('rankSlot — urutan tampilan', () => {
	it('menaruh yang perlu ditindak lebih dulu', () => {
		expect(rankSlot('due')).toBeLessThan(rankSlot('pending'));
		expect(rankSlot('pending')).toBeLessThan(rankSlot('snoozed'));
		expect(rankSlot('snoozed')).toBeLessThan(rankSlot('taken'));
	});

	it('menaruh status yang sudah selesai paling akhir', () => {
		expect(rankSlot('taken')).toBe(rankSlot('late'));
		expect(rankSlot('taken')).toBe(rankSlot('skipped'));
	});
});
