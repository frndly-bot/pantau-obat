/**
 * Uji unit mesin i18n.
 *
 * MASALAH YANG DIKUNCI DI SINI
 *
 * `createTranslator` mengembalikan NAMA KUNCI bila kunci tidak ditemukan.
 * Perilaku itu disengaja (kunci yang belum dipetakan langsung terlihat saat
 * menambah bahasa), tapi ia juga menyamarkan dua bug:
 *
 *   1. Kamus berisi kunci bersarang (`settings.appearance.theme`), sedangkan
 *      `lookup` hanya menelusuri SATU tingkat (`domain.rest`). Akibatnya
 *      halaman menampilkan `settings.appearance.theme` alih-alih "Tema" —
 *      terlihat seperti teks yang belum diterjemahkan, padahal terjemahannya
 *      ADA di kamus.
 *   2. Kamus diisi lewat `await` sehingga render pertama masih kosong dan
 *      menampilkan kunci mentah sebelum berganti beberapa milidetik kemudian.
 *
 * Uji ini memakai kamus sungguhan (bukan tiruan), supaya kunci yang diuji
 * memang yang ada di produksi.
 */
import { describe, expect, it } from 'vitest';
import { createTranslator, translate, type Dictionary } from '@pantau-obat/i18n';
import id from '@pantau-obat/i18n/locales/id.json';
import en from '@pantau-obat/i18n/locales/en.json';

const dict = id as unknown as Dictionary;
const dictEn = en as unknown as Dictionary;

describe('i18n — penelusuran kunci', () => {
	it('menemukan kunci satu tingkat', () => {
		expect(translate(dict, 'common.cancel')).toBe('Batal');
	});

	it('menemukan kunci BERSARANG (domain.grup.kunci)', () => {
		// Inilah bug yang pernah lolos: kunci ini ada di kamus, tapi lookup
		// lama hanya menelusuri satu tingkat sehingga mengembalikan nama
		// kuncinya.
		const hasil = translate(dict, 'settings.appearance.theme');
		expect(hasil).toBe('Tema');
		expect(hasil).not.toBe('settings.appearance.theme');
	});

	it('semua kunci bersarang di kamus benar-benar bisa ditemukan', () => {
		// Memindai seluruh kamus: setiap nilai yang berupa objek harus bisa
		// dijangkau lewat kunci bertitik. Ini menangkap kunci bersarang baru
		// yang ditambahkan tanpa memperbarui penelusurannya.
		const gagal: string[] = [];
		for (const [domain, isi] of Object.entries(dict)) {
			for (const [kunci, nilai] of Object.entries(isi)) {
				if (typeof nilai !== 'object') continue;
				for (const [sub] of Object.entries(nilai as Record<string, string>)) {
					const jalur = `${domain}.${kunci}.${sub}`;
					const hasil = translate(dict, jalur);
					if (hasil === jalur) gagal.push(jalur);
				}
			}
		}
		expect(gagal).toEqual([]);
	});

	it('mengembalikan nama kunci bila kunci memang tidak ada', () => {
		// Perilaku ini disengaja: kunci yang belum dipetakan harus terlihat.
		expect(translate(dict, 'tidak.ada')).toBe('tidak.ada');
	});

	it('memakai kamus cadangan bila kunci tidak ada di locale aktif', () => {
		// `en` dan `id` sama-sama lengkap, jadi kunci apa pun yang ada di id
		// harus ada juga di en. Yang diuji: mekanisme cadangan bekerja saat
		// locale aktif kehilangan kunci.
		const kosong: Dictionary = {};
		expect(translate(kosong, 'common.cancel', undefined, dict)).toBe('Batal');
	});
});

describe('i18n — cakupan terjemahan', () => {
	/** Mengumpulkan seluruh jalur kunci bertitik dari sebuah kamus. */
	function jalurSemua(obj: unknown, awalan = ''): string[] {
		if (typeof obj !== 'object' || obj === null) return [];
		const out: string[] = [];
		for (const [k, v] of Object.entries(obj)) {
			const jalur = awalan ? `${awalan}.${k}` : k;
			if (typeof v === 'string') out.push(jalur);
			else out.push(...jalurSemua(v, jalur));
		}
		return out;
	}

	it('kunci `en` dan `id` sama banyaknya', () => {
		const idJalur = jalurSemua(dict).sort();
		const enJalur = jalurSemua(dictEn).sort();
		expect(enJalur.length).toBe(idJalur.length);
		// Kunci yang ada di id tapi belum diterjemahkan di en akan muncul
		// sebagai selisih — itu tanda terjemahan yang belum lengkap.
		expect(enJalur).toEqual(idJalur);
	});

	it('tidak ada nilai terjemahan yang kosong', () => {
		const kosong = jalurSemua(dict).filter((j) => translate(dict, j).trim() === '');
		expect(kosong).toEqual([]);
	});

	it('kunci yang dipakai halaman Pengaturan benar-benar ada', () => {
		// Kunci ini dipakai langsung oleh markup, jadi salah ketik akan
		// tampil sebagai teks kunci di layar.
		const dipakai = [
			'settings.appearance.theme',
			'settings.appearance.theme_hint',
			'settings.appearance.light',
			'settings.appearance.dark',
			'settings.appearance.language',
			'settings.appearance.language_hint'
		];
		for (const k of dipakai) {
			expect(translate(dict, k), `kunci ${k}`).not.toBe(k);
		}
	});
});
