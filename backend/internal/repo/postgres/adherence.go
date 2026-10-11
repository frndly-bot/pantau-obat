package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// File ini menyediakan data mentah untuk perhitungan kepatuhan.
//
// Pembagiannya disengaja:
//
//   - Repo menyediakan FAKTA: obat apa saja yang punya jadwal, berapa log
//     yang tercatat, berapa yang terlewat.
//   - Service menghitung PENYEBUT dari aturan, memakai mesin aturan domain.
//
// Penyebut tidak dihitung di sini karena bergantung pada pola frekuensi,
// siklus, dan zona waktu — semuanya logika Go yang sudah punya test sendiri.
// Menuliskannya ulang dalam SQL berarti ada dua implementasi yang bisa
// berbeda, dan yang salah akan sulit dilacak.

// MedicationWithSchedule adalah satu obat lengkap dengan aturan dan jamnya,
// dalam satu baris per obat.
type MedicationWithSchedule struct {
	domain.Medication
	Frequency *domain.MedicationFrequency
	Times     []domain.MedicationTime
}

// ListMedicationsWithSchedule mengambil semua obat aktif milik user beserta
// aturan frekuensi dan jam aktifnya.
//
// Mengembalikan satu entri per obat, bukan satu entri per (obat, jam) seperti
// ScanDueCandidates. Perbedaan bentuk ini disengaja: perhitungan kepatuhan
// butuh seluruh daftar jam sekaligus untuk menilai sebaran mingguan, dan
// bentuk baris-per-jam akan memaksa pengelompokan ulang di atasnya.
func (db *DB) ListMedicationsWithSchedule(ctx context.Context, q Querier, ownerID string) ([]MedicationWithSchedule, error) {
	rows, err := q.Query(ctx, `
        SELECT `+medicationColumns+`
          FROM medications
         WHERE owner_id = $1 AND deleted_at IS NULL
         ORDER BY created_at DESC`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("mendaftar obat beserta jadwal: %w", err)
	}
	defer rows.Close()

	var meds []MedicationWithSchedule
	index := map[string]int{}

	for rows.Next() {
		m, err := scanMedication(rows)
		if err != nil {
			return nil, err
		}
		index[m.ID] = len(meds)
		meds = append(meds, MedicationWithSchedule{Medication: *m})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(meds) == 0 {
		return nil, nil
	}

	// Aturan frekuensi diambil sekaligus untuk semua obat, bukan satu query
	// per obat. Dengan 20 obat, perbedaannya 2 query versus 21 query.
	freqs, err := db.fetchFrequencies(ctx, q, ownerID)
	if err != nil {
		return nil, err
	}
	for _, f := range freqs {
		if i, ok := index[f.MedicationID]; ok {
			meds[i].Frequency = f
		}
	}

	times, err := db.fetchTimes(ctx, q, ownerID)
	if err != nil {
		return nil, err
	}
	for _, t := range times {
		if i, ok := index[t.MedicationID]; ok {
			meds[i].Times = append(meds[i].Times, t)
		}
	}

	return meds, nil
}

// fetchFrequencies mengambil seluruh aturan frekuensi milik user.
func (db *DB) fetchFrequencies(ctx context.Context, q Querier, ownerID string) ([]*domain.MedicationFrequency, error) {
	rows, err := q.Query(ctx, `
        SELECT `+frequencyColumns+`
          FROM medication_frequency
         WHERE owner_id = $1 AND deleted_at IS NULL`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("membaca aturan frekuensi: %w", err)
	}
	defer rows.Close()

	var out []*domain.MedicationFrequency
	for rows.Next() {
		f, err := scanFrequency(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// fetchTimes mengambil seluruh jam minum aktif milik user.
func (db *DB) fetchTimes(ctx context.Context, q Querier, ownerID string) ([]domain.MedicationTime, error) {
	rows, err := q.Query(ctx, `
        SELECT `+timeColumns+`
          FROM medication_times
         WHERE owner_id = $1 AND deleted_at IS NULL
         ORDER BY sort_order, time_local`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("membaca jam minum: %w", err)
	}
	defer rows.Close()

	var out []domain.MedicationTime
	for rows.Next() {
		t, err := scanMedicationTime(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// CountLogsForRange menghitung catatan dosis per status pada rentang tanggal.
//
// Mengembalikan peta status -> jumlah. Status yang tidak muncul berarti nol,
// dan pemanggil tidak perlu mengisi default.
func (db *DB) CountLogsForRange(ctx context.Context, q Querier, medicationID string, from, to time.Time) (map[string]int, error) {
	rows, err := q.Query(ctx, `
        SELECT status, count(*)::int
          FROM medication_logs
         WHERE medication_id = $1
           AND slot_date >= $2 AND slot_date <= $3
           AND deleted_at IS NULL
         GROUP BY status`, medicationID, from, to)
	if err != nil {
		return nil, fmt.Errorf("menghitung catatan dosis: %w", err)
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, fmt.Errorf("menghitung catatan dosis: %w", err)
		}
		out[status] = n
	}
	return out, rows.Err()
}

// CountMissedForRange menghitung dosis terlewat pada rentang tanggal.
//
// Dibaca dari medication_dose_events, BUKAN dari medication_logs. Dosis yang
// terlewat tidak punya baris log — sistem yang mencatatnya di dose_events
// dengan kind='missed'. Mencarinya di logs akan selalu menghasilkan nol, dan
// itu membuat angka kepatuhan terlalu tinggi.
func (db *DB) CountMissedForRange(ctx context.Context, q Querier, medicationID string, from, to time.Time) (int, error) {
	var n int
	err := q.QueryRow(ctx, `
        SELECT count(*)::int
          FROM medication_dose_events
         WHERE medication_id = $1
           AND event_date >= $2 AND event_date <= $3
           AND kind = 'missed'`, medicationID, from, to).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("menghitung dosis terlewat: %w", err)
	}
	return n, nil
}

// AdherenceDailyRow adalah satu baris rekap harian dari view.
type AdherenceDailyRow struct {
	SlotDate     time.Time
	LoggedDoses  int
	TakenDoses   int
	SkippedDoses int
	MissedDoses  int
	KnownDoses   int
}

// AdherenceDailyRows mengambil rekap harian dari view v_adherence_daily.
//
// Memakai view, bukan query langsung, supaya definisi "apa itu dosis yang
// diketahui" hanya ada di satu tempat. Kalau view berubah, laporan ikut
// berubah tanpa ada dua versi logika yang bisa berbeda.
func (db *DB) AdherenceDailyRows(ctx context.Context, q Querier, ownerID string, from, to time.Time) ([]AdherenceDailyRow, error) {
	rows, err := q.Query(ctx, `
        SELECT slot_date, logged_doses, taken_doses, skipped_doses,
               missed_doses, known_doses
          FROM v_adherence_daily
         WHERE owner_id = $1 AND slot_date >= $2 AND slot_date <= $3
         ORDER BY slot_date DESC`, ownerID, from, to)
	if err != nil {
		return nil, fmt.Errorf("membaca rekap kepatuhan harian: %w", err)
	}
	defer rows.Close()

	var out []AdherenceDailyRow
	for rows.Next() {
		var r AdherenceDailyRow
		if err := rows.Scan(&r.SlotDate, &r.LoggedDoses, &r.TakenDoses,
			&r.SkippedDoses, &r.MissedDoses, &r.KnownDoses); err != nil {
			return nil, fmt.Errorf("membaca rekap kepatuhan harian: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
