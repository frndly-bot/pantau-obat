import { describe, expect, it } from 'vitest';
import {
	blankDraft,
	bitmaskToDays,
	daysToBitmask,
	EVERY_DAY,
	patternHasTimes,
	suggestTime,
	syncTimes,
	toPayload,
	validateDraft,
	type MedicationDraft
} from './medication-draft';

function draft(overrides: Partial<MedicationDraft> = {}): MedicationDraft {
	return { ...blankDraft('2026-10-12', 'Asia/Jakarta'), ...overrides };
}

describe('blankDraft', () => {
	it('menghasilkan draf yang valid apa adanya', () => {
		const result = validateDraft(draft({ name: 'Amlodipine' }));
		expect(result.all).toEqual([]);
	});

	it('memakai pola harian dua kali sehari sebagai bawaan', () => {
		const d = blankDraft('2026-10-12', 'Asia/Jakarta');
		expect(d.pattern).toBe('daily');
		expect(d.times_per_day).toBe(2);
		expect(d.times).toHaveLength(2);
	});
});

describe('patternHasTimes', () => {
	it('PRN tidak punya jam, pola lain punya', () => {
		expect(patternHasTimes('prn')).toBe(false);
		expect(patternHasTimes('daily')).toBe(true);
		expect(patternHasTimes('interval')).toBe(true);
		expect(patternHasTimes('weekly')).toBe(true);
		expect(patternHasTimes('monthly_cycle')).toBe(true);
	});
});

describe('syncTimes', () => {
	it('menambah jam sampai jumlahnya sama dengan times_per_day', () => {
		const d = draft({ pattern: 'daily', times_per_day: 4, times: [{ time_local: '07:00', label: 'morning' }] });
		syncTimes(d);
		expect(d.times).toHaveLength(4);
	});

	it('memotong jam bila melebihi times_per_day', () => {
		const d = draft({
			pattern: 'daily',
			times_per_day: 1,
			times: [
				{ time_local: '07:00', label: 'morning' },
				{ time_local: '19:00', label: 'evening' }
			]
		});
		syncTimes(d);
		expect(d.times).toHaveLength(1);
	});

	it('menghapus semua jam untuk PRN — backend menolak PRN berjam', () => {
		const d = draft({ pattern: 'prn', times: [{ time_local: '07:00', label: 'morning' }] });
		syncTimes(d);
		expect(d.times).toEqual([]);
	});

	it('memberi minimal satu jam untuk pola selain daily', () => {
		const d = draft({ pattern: 'weekly', times: [] });
		syncTimes(d);
		expect(d.times).toHaveLength(1);
	});

	it('menghasilkan jam unik saat menambah banyak jam', () => {
		const d = draft({ pattern: 'daily', times_per_day: 6, times: [] });
		syncTimes(d);
		const jam = d.times.map((t) => t.time_local);
		expect(new Set(jam).size).toBe(jam.length);
	});
});

describe('suggestTime', () => {
	it('mulai dari 07:00', () => {
		expect(suggestTime(0, 3)).toBe('07:00');
	});

	it('tidak melewati 23:00 walau jumlahnya banyak', () => {
		for (let i = 0; i < 30; i++) {
			const jam = suggestTime(i, 30);
			expect(jam >= '07:00' && jam <= '23:00').toBe(true);
		}
	});
});

describe('hari (bitmask)', () => {
	it('Senin dan Kamis', () => {
		// Senin=1, Kamis=8
		expect(daysToBitmask(['mon', 'thu'])).toBe(9);
	});

	it('semua hari = 127', () => {
		expect(daysToBitmask(['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'])).toBe(EVERY_DAY);
	});

	it('bolak-balik tetap sama', () => {
		expect(bitmaskToDays(9).sort()).toEqual(['mon', 'thu'].sort());
		expect(bitmaskToDays(EVERY_DAY)).toHaveLength(7);
	});

	it('Minggu = 64, sesuai konvensi backend', () => {
		expect(daysToBitmask(['sun'])).toBe(64);
	});
});

describe('validateDraft', () => {
	it('menolak nama kosong', () => {
		const r = validateDraft(draft({ name: '   ' }));
		expect(r.byStep[1].join(' ')).toContain('Nama obat');
	});

	it('menolak daily yang jumlah jamnya tidak cocok', () => {
		const d = draft({ pattern: 'daily', times_per_day: 3, times: [{ time_local: '07:00', label: 'morning' }] });
		const r = validateDraft(d);
		expect(r.byStep[3].join(' ')).toContain('tepat 3 jam');
	});

	it('menolak PRN yang membawa jam', () => {
		const d = draft({ pattern: 'prn', times: [{ time_local: '07:00', label: 'morning' }] });
		const r = validateDraft(d);
		expect(r.byStep[3].join(' ')).toContain('PRN tidak memakai jam');
	});

	it('menolak jam duplikat', () => {
		const d = draft({
			pattern: 'daily',
			times_per_day: 2,
			times: [
				{ time_local: '07:00', label: 'morning' },
				{ time_local: '07:00', label: 'evening' }
			]
		});
		const r = validateDraft(d);
		expect(r.byStep[3].join(' ')).toContain('dipakai lebih dari sekali');
	});

	it('menolak tanggal selesai sebelum tanggal mulai', () => {
		const r = validateDraft(draft({ name: 'X', start_date: '2026-10-12', end_date: '2026-10-01' }));
		expect(r.byStep[1].join(' ')).toContain('tidak boleh sebelum');
	});

	it('menolak weekly tanpa hari terpilih', () => {
		const r = validateDraft(draft({ name: 'X', pattern: 'weekly', days_of_week: 0 }));
		expect(r.byStep[2].join(' ')).toContain('minimal satu hari');
	});

	it('menolak interval di luar 1..168', () => {
		expect(validateDraft(draft({ name: 'X', pattern: 'interval', interval_hours: 0 })).byStep[2]).not.toEqual([]);
		expect(validateDraft(draft({ name: 'X', pattern: 'interval', interval_hours: 200 })).byStep[2]).not.toEqual([]);
	});

	it('menerima siklus bulanan yang lengkap', () => {
		const r = validateDraft(
			draft({
				name: 'X',
				pattern: 'monthly_cycle',
				cycle_intake_days: 21,
				cycle_pause_days: 7,
				cycle_anchor: 'from_start',
				times: [{ time_local: '20:00', label: 'evening' }]
			})
		);
		expect(r.all).toEqual([]);
	});

	it('menolak anchor dom tanpa tanggal yang sah', () => {
		const r = validateDraft(
			draft({ name: 'X', pattern: 'monthly_cycle', cycle_anchor: 'dom', cycle_anchor_dom: 40 })
		);
		expect(r.byStep[2].join(' ')).toContain('1 dan 31');
	});
});

describe('toPayload', () => {
	it('harian: mengirim times_per_day dan jam', () => {
		const p = toPayload(draft({ name: 'Amlodipine' }), 'Asia/Jakarta');
		expect(p.frequency.pattern).toBe('daily');
		expect(p.frequency.times_per_day).toBe(2);
		expect(p.frequency.schedule_timezone).toBe('Asia/Jakarta');
		expect(p.times).toHaveLength(2);
		expect(p.as_needed).toBe(false);
	});

	it('PRN: as_needed true dan TANPA jam', () => {
		const p = toPayload(draft({ name: 'Paracetamol', pattern: 'prn', times: [] }), 'Asia/Jakarta');
		expect(p.as_needed).toBe(true);
		expect(p.times).toEqual([]);
		expect(p.frequency.pattern).toBe('prn');
	});

	it('interval: hanya mengirim field pola itu', () => {
		const p = toPayload(draft({ name: 'X', pattern: 'interval', interval_hours: 8 }), 'Asia/Jakarta');
		expect(p.frequency.interval_hours).toBe(8);
		expect(p.frequency).not.toHaveProperty('times_per_day');
		expect(p.frequency).not.toHaveProperty('days_of_week');
	});

	it('weekly: mengirim bitmask', () => {
		const p = toPayload(draft({ name: 'X', pattern: 'weekly', days_of_week: 9 }), 'Asia/Jakarta');
		expect(p.frequency.days_of_week).toBe(9);
	});

	it('monthly_cycle: mengirim anchor, dom hanya bila perlu', () => {
		const a = toPayload(
			draft({ name: 'X', pattern: 'monthly_cycle', cycle_anchor: 'from_start' }),
			'Asia/Jakarta'
		);
		expect(a.frequency.cycle_anchor).toBe('from_start');
		expect(a.frequency).not.toHaveProperty('cycle_anchor_dom');

		const b = toPayload(
			draft({ name: 'X', pattern: 'monthly_cycle', cycle_anchor: 'dom', cycle_anchor_dom: 15 }),
			'Asia/Jakarta'
		);
		expect(b.frequency.cycle_anchor_dom).toBe(15);
	});

	it('tanpa stok: field stock tidak dikirim', () => {
		const p = toPayload(draft({ name: 'X', track_stock: false }), 'Asia/Jakarta');
		expect(p).not.toHaveProperty('stock');
	});

	it('dengan stok: field stock dikirim lengkap', () => {
		const p = toPayload(draft({ name: 'X' }), 'Asia/Jakarta');
		expect(p.stock).toMatchObject({ unit_code: 'tablet', initial_qty: 30, qty_per_dose: 1 });
	});

	it('teks kosong menjadi null, bukan string kosong', () => {
		const p = toPayload(draft({ name: 'X', notes: '   ', end_date: '' }), 'Asia/Jakarta');
		expect(p.notes).toBeNull();
		expect(p.end_date).toBeNull();
	});
});
