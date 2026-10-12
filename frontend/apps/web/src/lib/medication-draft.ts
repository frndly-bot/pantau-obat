/**
 * Logika wizard tambah obat — murni, tanpa DOM.
 *
 * Dipisah dari komponen supaya bisa diuji sendiri dan supaya aturannya jelas
 * terbaca. Aturan di sini MENIRU validasi backend
 * (`service/medications.go` bagian validateTimes dan `domain.FrequencyParams.Validate`),
 * dengan tujuan yang berbeda: menangkap kesalahan SEBELUM dikirim, supaya
 * pengguna tidak menunggu balasan 400 untuk hal yang bisa diketahui di layar.
 *
 * Backend tetap sumber kebenaran. Kalau keduanya berbeda, backend yang menang.
 */
import type { CycleAnchor, FrequencyPattern, TimeLabel } from '@pantau-obat/api-client';

/** Bitmask hari (Minggu=64, Senin=1, … Sabtu=32), sesuai `EveryDay = 127`. */
export const EVERY_DAY = 127;

export const WEEKDAYS: ReadonlyArray<{ bit: number; key: string }> = [
	{ bit: 1, key: 'mon' },
	{ bit: 2, key: 'tue' },
	{ bit: 4, key: 'wed' },
	{ bit: 8, key: 'thu' },
	{ bit: 16, key: 'fri' },
	{ bit: 32, key: 'sat' },
	{ bit: 64, key: 'sun' }
];

export const TIME_LABELS: readonly TimeLabel[] = ['morning', 'afternoon', 'evening', 'dawn', 'other'];

/** Satu jam minum dalam draf. */
export interface DraftTime {
	time_local: string;
	label: TimeLabel;
}

/** Draf obat — bentuk yang diisi pengguna sebelum dikirim. */
export interface MedicationDraft {
	// Langkah 1: sumber
	catalog_id: string | null;

	// Langkah 2: detail
	name: string;
	dose_value: number;
	dose_unit_code: string;
	strength_note: string;
	route_code: string;
	dosage_form_code: string;
	meal_relation: string;
	must_finish: boolean;
	start_date: string;
	end_date: string;
	notes: string;
	notes_reminder: string;

	// Langkah 3: pola
	pattern: FrequencyPattern;
	times_per_day: number;
	interval_hours: number;
	days_of_week: number;
	cycle_intake_days: number;
	cycle_pause_days: number;
	cycle_anchor: CycleAnchor;
	cycle_anchor_dom: number;
	prn_max_per_day: number;
	tolerance_minutes: number;

	// Langkah 4: jam
	times: DraftTime[];

	// Langkah 5: stok
	track_stock: boolean;
	stock_unit_code: string;
	initial_qty: number;
	qty_per_dose: number;
	refill_threshold_qty: number | null;
	refill_reminder: boolean;
}

/** Draf awal. Nilai bawaannya masuk akal untuk kasus paling umum. */
export function blankDraft(today = '', timezone = ''): MedicationDraft {
	return {
		catalog_id: null,
		name: '',
		dose_value: 1,
		dose_unit_code: 'tablet',
		strength_note: '',
		route_code: 'O',
		dosage_form_code: 'LOCAL-TAB',
		meal_relation: 'after_meal',
		must_finish: false,
		start_date: today,
		end_date: '',
		notes: '',
		notes_reminder: '',
		pattern: 'daily',
		times_per_day: 2,
		interval_hours: 8,
		days_of_week: EVERY_DAY,
		cycle_intake_days: 21,
		cycle_pause_days: 7,
		cycle_anchor: 'from_start',
		cycle_anchor_dom: 1,
		prn_max_per_day: 4,
		tolerance_minutes: 60,
		times: [
			{ time_local: '07:00', label: 'morning' },
			{ time_local: '19:00', label: 'evening' }
		],
		track_stock: true,
		stock_unit_code: 'tablet',
		initial_qty: 30,
		qty_per_dose: 1,
		refill_threshold_qty: 5,
		refill_reminder: true
	};
	// `timezone` dipakai backend dari profil; disimpan di sini agar pemanggil
	// sadar nilai itu tidak diisi pengguna.
	void timezone;
}

/** Apakah pola ini memakai jam terjadwal. PRN tidak. */
export function patternHasTimes(pattern: FrequencyPattern): boolean {
	return pattern !== 'prn';
}

/**
 * Menyesuaikan daftar jam agar cocok dengan pola.
 *
 * Dipanggil saat pengguna berganti pola, supaya draf tidak tertinggal dalam
 * keadaan yang pasti ditolak backend:
 * - `daily` dengan times_per_day=3 butuh TEPAT 3 jam.
 * - `prn` tidak boleh punya jam sama sekali.
 * - pola lain butuh minimal satu jam.
 */
export function syncTimes(draft: MedicationDraft): void {
	if (draft.pattern === 'prn') {
		draft.times = [];
		return;
	}

	if (draft.pattern === 'daily') {
		const target = draft.times_per_day;
		while (draft.times.length < target) {
			draft.times.push({ time_local: suggestTime(draft.times.length, target), label: 'other' });
		}
		if (draft.times.length > target) draft.times = draft.times.slice(0, target);
		return;
	}

	// interval / weekly / monthly_cycle: minimal satu jam.
	if (draft.times.length === 0) {
		draft.times.push({ time_local: '07:00', label: 'morning' });
	}
}

/** Usulan jam ke-`index` dari `total` dosis sehari, tersebar merata. */
export function suggestTime(index: number, total: number): string {
	if (total <= 0) return '07:00';
	// Sebar dari 07:00, maksimal sampai 22:00.
	const startHour = 7;
	const span = Math.max(1, total - 1);
	const step = span > 0 ? Math.floor((22 - startHour) / span) : 0;
	const hour = Math.min(23, startHour + index * Math.max(1, step));
	return `${String(hour).padStart(2, '0')}:00`;
}

/** Menerjemahkan hari terpilih menjadi bitmask. */
export function daysToBitmask(days: readonly string[]): number {
	let mask = 0;
	for (const key of days) {
		const found = WEEKDAYS.find((d) => d.key === key);
		if (found) mask |= found.bit;
	}
	return mask;
}

/** Menguraikan bitmask menjadi daftar kunci hari. */
export function bitmaskToDays(mask: number): string[] {
	return WEEKDAYS.filter((d) => (mask & d.bit) !== 0).map((d) => d.key);
}

/** Hasil pemeriksaan draf. */
export interface ValidationResult {
	/** Pesan per langkah (0-4). Kosong berarti langkah itu valid. */
	byStep: string[][];
	/** Semua pesan, sudah diratakan. */
	all: string[];
}

/**
 * Memeriksa draf mengikuti aturan backend.
 *
 * Mengembalikan pesan per langkah supaya UI bisa menandai langkah mana yang
 * bermasalah, bukan hanya menampilkan satu daftar panjang di akhir.
 */
export function validateDraft(draft: MedicationDraft): ValidationResult {
	const steps: string[][] = [[], [], [], [], []];

	// Langkah 2: detail obat.
	if (!draft.name.trim()) steps[1].push('Nama obat belum diisi.');
	if (!(draft.dose_value > 0)) steps[1].push('Jumlah per dosis harus lebih dari nol.');
	if (!draft.dose_unit_code.trim()) steps[1].push('Satuan dosis belum dipilih.');
	if (!draft.route_code.trim()) steps[1].push('Rute belum dipilih.');
	if (!draft.start_date) steps[1].push('Tanggal mulai terapi belum diisi.');
	if (draft.end_date && draft.end_date < draft.start_date) {
		steps[1].push('Tanggal selesai tidak boleh sebelum tanggal mulai.');
	}

	// Langkah 3: pola.
	switch (draft.pattern) {
		case 'daily':
			if (draft.times_per_day < 1 || draft.times_per_day > 24) {
				steps[2].push('Berapa kali sehari harus antara 1 dan 24.');
			}
			break;
		case 'interval':
			if (draft.interval_hours < 1 || draft.interval_hours > 168) {
				steps[2].push('Interval harus antara 1 dan 168 jam.');
			}
			break;
		case 'weekly':
			if (draft.days_of_week < 1 || draft.days_of_week > 127) {
				steps[2].push('Pilih minimal satu hari.');
			}
			break;
		case 'monthly_cycle':
			if (draft.cycle_intake_days < 1 || draft.cycle_intake_days > 365) {
				steps[2].push('Hari minum siklus harus antara 1 dan 365.');
			}
			if (draft.cycle_pause_days < 0 || draft.cycle_pause_days > 365) {
				steps[2].push('Hari jeda harus antara 0 dan 365.');
			}
			if (draft.cycle_anchor === 'dom' && (draft.cycle_anchor_dom < 1 || draft.cycle_anchor_dom > 31)) {
				steps[2].push('Tanggal titik mulai harus antara 1 dan 31.');
			}
			break;
		case 'prn':
			if (draft.prn_max_per_day < 1 || draft.prn_max_per_day > 24) {
				steps[2].push('Batas peringatan per hari harus antara 1 dan 24.');
			}
			break;
	}

	// Langkah 4: jam.
	if (patternHasTimes(draft.pattern)) {
		if (draft.times.length === 0) {
			steps[3].push('Minimal satu jam minum harus diisi.');
		}
		const seen = new Set<string>();
		for (const t of draft.times) {
			if (!/^\d{2}:\d{2}$/.test(t.time_local)) {
				steps[3].push(`Jam "${t.time_local}" tidak berbentuk HH:MM.`);
				continue;
			}
			if (seen.has(t.time_local)) {
				steps[3].push(`Jam ${t.time_local} dipakai lebih dari sekali.`);
			}
			seen.add(t.time_local);
		}
		if (draft.pattern === 'daily' && draft.times.length !== draft.times_per_day) {
			steps[3].push(
				`Pola harian dengan ${draft.times_per_day} kali sehari butuh tepat ${draft.times_per_day} jam, tetapi ada ${draft.times.length}.`
			);
		}
	} else if (draft.times.length > 0) {
		steps[3].push('Obat PRN tidak memakai jam minum terjadwal.');
	}

	// Langkah 5: stok.
	if (draft.track_stock) {
		if (!draft.stock_unit_code.trim()) steps[4].push('Satuan stok belum dipilih.');
		if (draft.initial_qty < 0) steps[4].push('Stok awal tidak boleh negatif.');
		if (draft.qty_per_dose <= 0) steps[4].push('Pemakaian per dosis harus lebih dari nol.');
		if (draft.refill_threshold_qty !== null && draft.refill_threshold_qty < 0) {
			steps[4].push('Ambang restock tidak boleh negatif.');
		}
	}

	const all = steps.flat();
	return { byStep: steps, all };
}

/** Bentuk request yang dikirim ke `POST /medications`. */
export interface CreateMedicationPayload {
	name: string;
	dose_value: number;
	dose_unit_code: string;
	route_code: string;
	dosage_form_code: string | null;
	strength_note: string | null;
	meal_relation: string;
	must_finish: boolean;
	start_date: string;
	end_date: string | null;
	notes: string | null;
	notes_reminder: string | null;
	catalog_id: string | null;
	as_needed: boolean;
	frequency: Record<string, unknown>;
	times: Array<{ time_local: string; label: string }>;
	stock?: Record<string, unknown>;
}

/**
 * Menyusun payload dari draf.
 *
 * Hanya memuat field yang bermakna untuk pola terpilih — mengirim field pola
 * lain akan ditolak atau, lebih buruk, diam-diam diabaikan dan membuat
 * perilaku sebenarnya berbeda dari yang dilihat pengguna.
 */
export function toPayload(draft: MedicationDraft, timezone: string): CreateMedicationPayload {
	const frequency: Record<string, unknown> = {
		pattern: draft.pattern,
		tolerance_minutes: draft.tolerance_minutes,
		schedule_timezone: timezone
	};

	switch (draft.pattern) {
		case 'daily':
			frequency.times_per_day = draft.times_per_day;
			break;
		case 'interval':
			frequency.interval_hours = draft.interval_hours;
			break;
		case 'weekly':
			frequency.days_of_week = draft.days_of_week;
			break;
		case 'monthly_cycle':
			frequency.cycle_intake_days = draft.cycle_intake_days;
			frequency.cycle_pause_days = draft.cycle_pause_days;
			frequency.cycle_anchor = draft.cycle_anchor;
			if (draft.cycle_anchor === 'dom') frequency.cycle_anchor_dom = draft.cycle_anchor_dom;
			break;
		case 'prn':
			frequency.prn_max_per_day = draft.prn_max_per_day;
			break;
	}

	const payload: CreateMedicationPayload = {
		name: draft.name.trim(),
		dose_value: draft.dose_value,
		dose_unit_code: draft.dose_unit_code,
		route_code: draft.route_code,
		dosage_form_code: draft.dosage_form_code || null,
		strength_note: draft.strength_note.trim() || null,
		meal_relation: draft.meal_relation,
		must_finish: draft.must_finish,
		start_date: draft.start_date,
		end_date: draft.end_date || null,
		notes: draft.notes.trim() || null,
		notes_reminder: draft.notes_reminder.trim() || null,
		catalog_id: draft.catalog_id,
		as_needed: draft.pattern === 'prn',
		frequency,
		times: []
	};

	// PRN tidak punya jam. Mengirimnya akan ditolak backend.
	if (patternHasTimes(draft.pattern)) {
		payload.times = draft.times.map((t) => ({ time_local: t.time_local, label: t.label }));
	}

	if (draft.track_stock) {
		payload.stock = {
			unit_code: draft.stock_unit_code,
			initial_qty: draft.initial_qty,
			qty_per_dose: draft.qty_per_dose,
			refill_threshold_qty: draft.refill_threshold_qty,
			refill_reminder: draft.refill_reminder
		};
	}

	return payload;
}
