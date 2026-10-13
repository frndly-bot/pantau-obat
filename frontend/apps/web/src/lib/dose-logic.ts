/**
 * Logika bersama halaman berdaftar dosis.
 *
 * KENAPA BERKAS INI ADA
 *
 * Review UX pada prototipe design menemukan satu masalah yang muncul lagi
 * di setiap halaman: **rasio kepatuhan dihitung empat cara berbeda**. Riwayat
 * memakai `(taken + late) / total`, Hari Ini memakai `done / seluruh slot`
 * (termasuk yang belum jatuh tempo), Caregiver memakai `done / doses_total`,
 * dan Detail Obat memakai `nTaken / logs.length`.
 *
 * Akibatnya nyata: dengan tiga dosis sehari di mana pagi sudah diminum dan
 * siang serta malam belum waktunya, server menghitung 1/(3−2) = **100%**
 * sementara Hari Ini menghitung 1/3 = **33%**. Dua layar, satu hari, selisih
 * 67 poin. Pengguna yang melihat keduanya tidak punya cara tahu mana yang
 * benar.
 *
 * Server sudah punya definisi tunggal (`ComputeRatio` di `adherence.go`):
 *   pembilang : hanya `taken` — `late` TIDAK dihitung
 *   penyebut  : dosis yang keadaannya sudah pasti (terjadwal − menunggu)
 *   hasil     : `null` bila penyebut ≤ 0, BUKAN 0%
 *
 * Fungsi di bawah meniru aturan itu supaya klien dan server tidak pernah
 * berbeda. Yang paling mudah salah adalah `null`: menampilkannya sebagai 0%
 * membuat "belum ada data" terbaca sebagai "gagal semua" — pada aplikasi
 * kesehatan, itu tuduhan yang tidak berdasar.
 */

/** Status slot yang keadaannya sudah pasti (tidak lagi menunggu). */
const RESOLVED = ['taken', 'late', 'skipped'] as const;

/** Status slot yang sudah ditindak, termasuk yang tidak dihitung rasio. */
const DONE = ['taken', 'late', 'skipped'] as const;

export interface RatioInput {
	status: string;
}

/**
 * Menghitung rasio kepatuhan mengikuti aturan server.
 *
 * @returns angka 0..1, atau `null` bila belum ada dosis yang keadaannya pasti.
 *          `null` BUKAN kegagalan — ia berarti "belum ada data".
 */
export function computeRatio(slots: readonly RatioInput[]): number | null {
	if (slots.length === 0) return null;

	const taken = slots.filter((s) => s.status === 'taken').length;
	const resolved = slots.filter((s) => (RESOLVED as readonly string[]).includes(s.status)).length;

	// Penyebut nol atau negatif → belum ada yang bisa dinilai.
	if (resolved <= 0) return null;

	const ratio = taken / resolved;
	// Dibatasi maksimum 1: catatan `taken` yang lebih banyak dari slot
	// terjadwal (mis. PRN) tidak boleh menghasilkan lebih dari 100%.
	return Math.min(1, ratio);
}

/** Berapa dosis yang sudah ditindak (termasuk yang dilewati). */
export function countDone(slots: readonly RatioInput[]): number {
	return slots.filter((s) => (DONE as readonly string[]).includes(s.status)).length;
}

/** Berapa dosis yang masih menunggu ditindak. */
export function countActionable(slots: readonly RatioInput[]): number {
	return slots.filter((s) => s.status === 'due' || s.status === 'pending').length;
}

/**
 * Format rasio untuk ditampilkan.
 *
 * Membedakan "belum ada data" dari "0%". Ini inti temuan review: `null`
 * yang ditampilkan sebagai 0% membuat pengguna mengira semua dosisnya
 * terlewat, padahal sistemnya hanya belum punya cukup informasi.
 */
export function formatRatio(ratio: number | null): string {
	if (ratio === null) return 'Belum ada data';
	return `${Math.round(ratio * 100)}%`;
}

/** Urutan tampilan dosis: yang perlu ditindak lebih dulu. */
export function rankSlot(status: string): number {
	if (status === 'due') return 0;
	if (status === 'pending') return 1;
	if (status === 'snoozed') return 2;
	return 3;
}

/**
 * Menyusun ringkasan "sudah / total" untuk satu hari.
 *
 * Menyertakan jumlah yang menunggu secara terpisah, karena tanpa itu
 * "3 dari 7" terlihat seperti kegagalan padahal 4 sisanya belum waktunya.
 */
export function summarizeDay(slots: readonly RatioInput[]): {
	done: number;
	total: number;
	waiting: number;
	ratio: number | null;
	text: string;
} {
	const total = slots.length;
	const done = countDone(slots);
	const waiting = countActionable(slots);
	const ratio = computeRatio(slots);

	return {
		done,
		total,
		waiting,
		ratio,
		text: `${done} dari ${total} tercatat${waiting > 0 ? ` · ${waiting} menunggu` : ''}`
	};
}
