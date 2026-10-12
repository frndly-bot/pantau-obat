package postgres

import (
	"context"
	"fmt"
	"time"
)

// File ini menampung query ekspor riwayat pemakaian (TRD H2).
//
// Ekspor DIBACA LANGSUNG dengan cursor, bukan dimuat seluruhnya ke memori:
// riwayat seumur hidup bisa jutaan baris, dan endpoint ini mengembalikan CSV
// yang di-stream ke klien. `ExportLogs` menerima callback per baris — pemanggil
// menulis ke ResponseWriter sambil iterasi.

// ExportLogRow adalah satu baris riwayat siap CSV.
//
// Nama obat DISERTAKAN: tujuan ekspor adalah dibawa ke dokter, dan "obat apa"
// adalah inti laporannya. Yang tidak disertakan: prescription_photo_url
// (path internal), dan recorded_by ditampilkan sebagai peran, bukan UUID.
type ExportLogRow struct {
	SlotDate       time.Time
	ScheduledHour  *int16
	ScheduledMin   *int16
	MedicationName string
	DoseValue      float64
	DoseUnit       string
	Status         string
	TakenAt        *time.Time
	Source         string
	Note           *string
}

// ExportLogs membaca seluruh riwayat pemakaian dalam rentang tanggal,
// memanggil fn untuk setiap baris.
//
// HANYA milik owner yang keluar — parameter ownerID wajib dan dipasang di
// WHERE, bukan diharapkan dari filter klien. Ekspor atas nama orang lain
// lewat jalur ini tidak mungkin.
//
// Tanpa LIMIT: pemanggil menentukan rentang, dan rentang tanpa batas adalah
// keputusan pemanggil. Status tanpa soft-delete filter otomatis: baris
// ter-koreksi memang harus tidak ikut.
func (db *DB) ExportLogs(ctx context.Context, ownerID string, from, to time.Time, fn func(ExportLogRow) error) error {
	rows, err := db.pool.Query(ctx, `
        SELECT l.slot_date, l.scheduled_hour, l.scheduled_minute,
               m.name, m.dose_value, m.dose_unit_code,
               l.status, l.taken_at, l.source, l.note
          FROM medication_logs l
          JOIN medications m ON m.id = l.medication_id
         WHERE l.owner_id = $1
           AND l.deleted_at IS NULL
           AND l.slot_date >= $2
           AND l.slot_date <= $3
         ORDER BY l.slot_date ASC, l.scheduled_hour ASC NULLS LAST`,
		ownerID, from, to)
	if err != nil {
		return fmt.Errorf("membaca riwayat ekspor: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var r ExportLogRow
		if err := rows.Scan(&r.SlotDate, &r.ScheduledHour, &r.ScheduledMin,
			&r.MedicationName, &r.DoseValue, &r.DoseUnit,
			&r.Status, &r.TakenAt, &r.Source, &r.Note); err != nil {
			return fmt.Errorf("membaca baris ekspor: %w", err)
		}
		if err := fn(r); err != nil {
			return err
		}
	}
	return rows.Err()
}
