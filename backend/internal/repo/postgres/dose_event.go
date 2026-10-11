package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// DoseEventInput adalah satu penyimpangan jadwal.
//
// event_key memakai occurrence key yang sama dengan medication_logs dan
// bump_queue: "<schedule_id>|<YYYY-MM-DD>|<HH>:<MM>". Karena kolomnya
// PRIMARY KEY, satu dosis hanya bisa punya satu catatan penyimpangan, bahkan
// bila perubahannya dilakukan berkali-kali.
type DoseEventInput struct {
	EventKey        string
	OwnerID         string
	MedicationID    string
	ScheduleID      *string
	EventDate       time.Time
	ScheduledHour   int
	ScheduledMinute int
	ScheduledTZ     string
	Kind            domain.DoseEventKind
	Reason          string
}

// InsertDoseEvent mencatat satu penyimpangan jadwal.
//
// Memakai ON CONFLICT DO NOTHING karena penyimpangan yang sama bisa
// terdeteksi lebih dari sekali (misalnya pass harian berjalan dua kali
// setelah proses di-restart). Menimpa yang lama dengan yang baru justru
// menghilangkan jejak kejadian pertama.
func (db *DB) InsertDoseEvent(ctx context.Context, q Querier, in DoseEventInput) error {
	var reason *string
	if in.Reason != "" {
		reason = &in.Reason
	}

	_, err := q.Exec(ctx, `
        INSERT INTO medication_dose_events (
            event_key, owner_id, medication_id, schedule_id, event_date,
            scheduled_hour, scheduled_minute, scheduled_tz, kind, reason
        ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
        ON CONFLICT (event_key) DO NOTHING`,
		in.EventKey, in.OwnerID, in.MedicationID, in.ScheduleID, in.EventDate,
		in.ScheduledHour, in.ScheduledMinute, in.ScheduledTZ, in.Kind, reason,
	)
	if err != nil {
		return fmt.Errorf("mencatat penyimpangan jadwal: %w", err)
	}
	return nil
}

// DoseEvent adalah satu baris penyimpangan yang sudah tercatat.
type DoseEvent struct {
	EventKey        string
	MedicationID    string
	ScheduleID      *string
	EventDate       time.Time
	ScheduledHour   int
	ScheduledMinute int
	Kind            string
	Reason          *string
	CreatedAt       time.Time
}

// ListDoseEvents mengambil penyimpangan pada rentang tanggal.
func (db *DB) ListDoseEvents(ctx context.Context, q Querier, ownerID string, from, to time.Time, kinds ...domain.DoseEventKind) ([]DoseEvent, error) {
	rows, err := q.Query(ctx, `
        SELECT event_key, medication_id, schedule_id, event_date,
               scheduled_hour, scheduled_minute, kind, reason, created_at
          FROM medication_dose_events
         WHERE owner_id = $1
           AND event_date >= $2 AND event_date <= $3
         ORDER BY event_date DESC, scheduled_hour DESC`,
		ownerID, from, to)
	if err != nil {
		return nil, fmt.Errorf("membaca penyimpangan jadwal: %w", err)
	}
	defer rows.Close()

	var out []DoseEvent
	for rows.Next() {
		var e DoseEvent
		if err := rows.Scan(&e.EventKey, &e.MedicationID, &e.ScheduleID,
			&e.EventDate, &e.ScheduledHour, &e.ScheduledMinute, &e.Kind,
			&e.Reason, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("membaca penyimpangan jadwal: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// MissedCountForDate menghitung berapa dosis terlewat pada satu tanggal.
func (db *DB) MissedCountForDate(ctx context.Context, q Querier, ownerID string, date time.Time) (int, error) {
	var n int
	err := q.QueryRow(ctx, `
        SELECT count(*) FROM medication_dose_events
         WHERE owner_id = $1 AND event_date = $2 AND kind = 'missed'`,
		ownerID, date).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("menghitung dosis terlewat: %w", err)
	}
	return n, nil
}
