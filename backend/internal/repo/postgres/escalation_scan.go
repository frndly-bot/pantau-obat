package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// OwnersWithMissedDoses mengembalikan pasien yang punya catatan dosis
// terlewat dalam 30 hari terakhir.
//
// Sengaja mengembalikan daftar yang LEBAR, bukan langsung yang streak-nya
// sudah melewati batas: perhitungan streak butuh zona waktu masing-masing
// pasien, dan itu tidak bisa dikerjakan di dalam satu query yang sama.
// Menyaring di sini dulu membuat jumlah pasien yang dihitung kecil — hanya
// yang memang punya catatan terlewat — bukan seluruh pengguna aplikasi.
func (db *DB) OwnersWithMissedDoses(ctx context.Context, q Querier, now time.Time) ([]string, error) {
	rows, err := q.Query(ctx, `
        SELECT DISTINCT e.owner_id
          FROM medication_dose_events e
          JOIN medications m ON m.id = e.medication_id
         WHERE e.kind = 'missed'
           AND m.deleted_at IS NULL
           AND e.event_date >= $1::date - INTERVAL '30 days'`,
		now)
	if err != nil {
		return nil, fmt.Errorf("mencari pasien dengan dosis terlewat: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("membaca pasien: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// TimezoneOf mengembalikan zona waktu satu pengguna.
//
// Mengembalikan zona bawaan bila pengguna tidak ditemukan, supaya pemanggil
// tidak perlu menangani error yang tidak mengubah perilaku: zona bawaan
// memang nilai yang benar untuk pengguna yang belum mengaturnya.
func (db *DB) TimezoneOf(ctx context.Context, q Querier, userID string) string {
	var tz string
	err := q.QueryRow(ctx, `SELECT timezone FROM users WHERE id = $1`, userID).Scan(&tz)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.DefaultTimezone
		}
		return domain.DefaultTimezone
	}
	if tz == "" {
		return domain.DefaultTimezone
	}
	return tz
}
