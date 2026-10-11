package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// File ini mengurus pencatatan dosis.
//
// Inti keamanannya ada di database, bukan di sini:
//
//   - uq_log_occurrence (partial unique index) menjamin satu dosis terjadwal
//     hanya punya SATU log hidup, walau request dikirim berulang.
//   - uq_stock_txn_log menjamin satu log hanya memotong stok SEKALI.
//
// Keduanya partial: baris yang sudah soft-delete tidak dihitung, sehingga
// koreksi (soft-delete lama + insert baru) tetap boleh dilakukan.

const logColumns = `
    id, medication_id, schedule_id, owner_id, recorded_by, slot_date,
    scheduled_at, scheduled_hour, scheduled_minute, scheduled_tz,
    status, taken_at, note, source, created_at, updated_at`

// scanLog membaca satu baris medication_logs.
func scanLog(row pgx.Row) (*domain.MedicationLog, error) {
	var l domain.MedicationLog
	err := row.Scan(
		&l.ID, &l.MedicationID, &l.ScheduleID, &l.OwnerID, &l.RecordedBy,
		&l.SlotDate, &l.ScheduledAt, &l.ScheduledHour, &l.ScheduledMinute,
		&l.ScheduledTZ, &l.Status, &l.TakenAt, &l.Note, &l.Source,
		&l.CreatedAt, &l.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("membaca catatan dosis: %w", err)
	}
	return &l, nil
}

// CreateLogInput adalah data satu catatan dosis baru.
type CreateLogInput struct {
	MedicationID string
	ScheduleID   *string
	OwnerID      string
	RecordedBy   string

	SlotDate        time.Time
	ScheduledAt     *time.Time
	ScheduledHour   *int
	ScheduledMinute *int
	ScheduledTZ     *string

	Status  domain.LogStatus
	TakenAt *time.Time
	Note    *string
	Source  domain.LogSource
}

// CreateLog menyimpan catatan dosis.
//
// Tidak melakukan pemeriksaan anti-ganda sendiri. Pemeriksaan dilakukan
// database lewat uq_log_occurrence, dan itu disengaja: pemeriksaan di
// aplikasi selalu punya jendela balapan antara "cek dulu" dan "tulis",
// sedangkan unique index tidak.
func (db *DB) CreateLog(ctx context.Context, q Querier, in CreateLogInput) (*domain.MedicationLog, error) {
	if in.Source == "" {
		in.Source = domain.SourceSelf
	}

	row := q.QueryRow(ctx, `
        INSERT INTO medication_logs (
            medication_id, schedule_id, owner_id, recorded_by, slot_date,
            scheduled_at, scheduled_hour, scheduled_minute, scheduled_tz,
            status, taken_at, note, source
        ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
        RETURNING `+logColumns,
		in.MedicationID, in.ScheduleID, in.OwnerID, in.RecordedBy, in.SlotDate,
		in.ScheduledAt, in.ScheduledHour, in.ScheduledMinute, in.ScheduledTZ,
		in.Status, in.TakenAt, in.Note, in.Source,
	)

	l, err := scanLog(row)
	if err != nil {
		// Duplikat dosis terjadwal. Nama index dipakai untuk membedakannya
		// dari pelanggaran unique lain yang mungkin ada di tabel ini.
		if IsUniqueViolation(err) && strings.Contains(ConstraintName(err), "uq_log_occurrence") {
			return nil, domain.ErrLogOccurrenceTaken
		}
		return nil, err
	}
	return l, nil
}

// LogByID mengambil satu catatan dosis milik user.
func (db *DB) LogByID(ctx context.Context, q Querier, id, ownerID string) (*domain.MedicationLog, error) {
	row := q.QueryRow(ctx, `
        SELECT `+logColumns+`
          FROM medication_logs
         WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL`, id, ownerID)
	return scanLog(row)
}

// LogByOccurrence mencari catatan hidup untuk satu dosis terjadwal.
//
// Dipakai untuk memeriksa apakah dosis ini sudah tercatat, dan siapa yang
// mencatatnya. Dosis PRN tidak punya kunci, jadi fungsi ini tidak berlaku
// untuknya.
func (db *DB) LogByOccurrence(ctx context.Context, q Querier, medicationID, scheduleID string, slotDate time.Time, hour, minute int) (*domain.MedicationLog, error) {
	row := q.QueryRow(ctx, `
        SELECT `+logColumns+`
          FROM medication_logs
         WHERE medication_id = $1 AND schedule_id = $2 AND slot_date = $3
           AND scheduled_hour = $4 AND scheduled_minute = $5
           AND deleted_at IS NULL`,
		medicationID, scheduleID, slotDate, hour, minute)
	return scanLog(row)
}

// ListLogsFilter adalah syarat pencarian catatan dosis.
type ListLogsFilter struct {
	OwnerID      string
	MedicationID string
	From         *time.Time
	To           *time.Time
	Status       string
	Limit        int
}

// ListLogs mengambil daftar catatan dosis.
func (db *DB) ListLogs(ctx context.Context, q Querier, f ListLogsFilter) ([]*domain.MedicationLog, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}

	where := []string{"owner_id = $1", "deleted_at IS NULL"}
	args := []any{f.OwnerID}

	if f.MedicationID != "" {
		args = append(args, f.MedicationID)
		where = append(where, fmt.Sprintf("medication_id = $%d", len(args)))
	}
	if f.From != nil {
		args = append(args, *f.From)
		where = append(where, fmt.Sprintf("slot_date >= $%d", len(args)))
	}
	if f.To != nil {
		args = append(args, *f.To)
		where = append(where, fmt.Sprintf("slot_date <= $%d", len(args)))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}

	args = append(args, f.Limit)

	rows, err := q.Query(ctx, `
        SELECT `+logColumns+`
          FROM medication_logs
         WHERE `+strings.Join(where, " AND ")+`
         ORDER BY slot_date DESC, scheduled_hour DESC NULLS LAST, created_at DESC
         LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("mendaftar catatan dosis: %w", err)
	}
	defer rows.Close()

	var out []*domain.MedicationLog
	for rows.Next() {
		l, err := scanLog(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// SoftDeleteLog menandai satu catatan dosis terhapus.
//
// Dipakai untuk KOREKSI: baris lama di-soft-delete, baris baru dibuat, dan
// keduanya dalam satu transaksi. Soft-delete, bukan DELETE fisik, karena
// uq_log_occurrence bersifat partial — kalau barisnya benar-benar dihapus,
// tidak ada jejak bahwa dosis itu pernah dicatat dengan status berbeda.
func (db *DB) SoftDeleteLog(ctx context.Context, q Querier, id, ownerID string) error {
	tag, err := q.Exec(ctx, `
        UPDATE medication_logs SET deleted_at = now()
         WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL`, id, ownerID)
	if err != nil {
		return fmt.Errorf("menghapus catatan dosis: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// CountLogsByStatus menghitung catatan dosis per status pada rentang tanggal.
func (db *DB) CountLogsByStatus(ctx context.Context, q Querier, ownerID string, from, to time.Time) (map[string]int, error) {
	rows, err := q.Query(ctx, `
        SELECT status, count(*)
          FROM medication_logs
         WHERE owner_id = $1 AND slot_date >= $2 AND slot_date <= $3
           AND deleted_at IS NULL
         GROUP BY status`, ownerID, from, to)
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

// StockForConsume mengambil saldo stok dengan kunci, untuk dipotong.
//
// FOR UPDATE dipakai supaya dua request yang mencatat dosis pada saat
// bersamaan tidak membaca saldo yang sama. Tanpa kunci ini, keduanya bisa
// melihat saldo 10 dan masing-masing memotong 1, sehingga hasilnya 9
// padahal seharusnya 8.
func (db *DB) StockForConsume(ctx context.Context, q Querier, medicationID string) (*domain.MedicationStock, error) {
	row := q.QueryRow(ctx, `
        SELECT `+stockColumns+`
          FROM medication_stock
         WHERE medication_id = $1 AND deleted_at IS NULL
           FOR UPDATE`, medicationID)
	return scanStock(row)
}

// ClearReminderQueue membatalkan pengingat yang masih menggantung untuk satu
// dosis, setelah dosis itu dicatat.
//
// Tanpa ini, user yang mencatat dosisnya lebih awal tetap akan menerima
// pengingat pada jam yang dijadwalkan, dan itu mengganggu: notifikasi untuk
// sesuatu yang sudah dikerjakan.
func (db *DB) ClearReminderQueue(ctx context.Context, q Querier, medicationID, scheduleID string, slotDate time.Time, hour, minute int) error {
	key := domain.OccurrenceKey(scheduleID, slotDate, hour, minute)
	_, err := q.Exec(ctx, `
        UPDATE bump_queue SET done_at = now()
         WHERE dedup_key = $1 AND done_at IS NULL`, key)
	if err != nil {
		return fmt.Errorf("membatalkan pengingat: %w", err)
	}
	return nil
}
