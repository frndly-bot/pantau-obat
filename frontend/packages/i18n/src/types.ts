/**
 * Kamus i18n Pantau Obat.
 *
 * Sumber asli: `design/assets/i18n.js` (prototipe) — dikonversi apa adanya ke
 * JSON agar bisa dipakai alat terjemahan apa pun dan tidak bisa menjalankan
 * kode saat dimuat. Struktur: `{ <domain>: { <key>: <teks> } }`.
 *
 * Kunci stabil & deskriptif (`today.title`, `meds.wizard.add_title`) — teks
 * Indonesia TIDAK dipakai sebagai kunci, jadi menambah bahasa baru cukup
 * menambahkan satu berkas JSON.
 */
import id from '../locales/id.json';

export type Locale = 'id' | 'en';

/**
 * Kamus bertipe: `{ [domain]: { [key]: string } }`.
 *
 * Nilai boleh bersarang satu tingkat lagi (`{ domain: { grup: { key: string } } }`),
 * karena kamus memang memakainya — mis. `settings.appearance.theme`. Tanpa
 * kedalaman tambahan ini, `lookup` tidak akan pernah bisa menemukan kunci
 * ber-titik-tiga dan selalu mengembalikan nama kuncinya.
 */
export type DictionaryValue = string | Record<string, string>;
export type Dictionary = Record<string, Record<string, DictionaryValue>>;

/** Kode locale bawaan bila preferensi pengguna tidak diketahui. */
export const DEFAULT_LOCALE: Locale = 'id';

/** Locale yang selalu dipakai sebagai cadangan terakhir. */
export const FALLBACK_LOCALE: Locale = 'id';

export const LOCALES: readonly Locale[] = ['id', 'en'] as const;

/** Nama bahasa dalam bahasa aslinya (endonim) — untuk pemilih bahasa. */
export const LOCALE_LABELS: Record<Locale, string> = {
	id: 'Bahasa Indonesia',
	en: 'English',
};

/**
 * Semua kunci yang tersedia, sebagai gabungan `domain.key`.
 * Dipakai untuk type-safety: salah ketik ketahuan saat `tsc`, bukan diam-diam
 * tampil nama kunci saat runtime (perilaku prototipe).
 */
export type TranslationKey = {
	[D in keyof typeof id]: `${D & string}.${keyof (typeof id)[D] & string}`;
}[keyof typeof id];

export function isLocale(value: unknown): value is Locale {
	return typeof value === 'string' && (LOCALES as readonly string[]).includes(value);
}
