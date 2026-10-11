package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// File ini mengurus jejak pengiriman notifikasi.
//
// Pemisahan dengan bump_queue disengaja dan penting:
//
//   - bump_queue menjawab "KAPAN sesuatu harus terjadi". Umurnya pendek,
//     dibersihkan setelah 7 hari.
//   - notification_jobs mencatat "apa yang SUDAH dikirim dan hasilnya".
//     Umurnya lebih panjang (30 hari), karena dipakai menjawab pertanyaan
//     "kenapa saya tidak dapat notifikasi".
//
// Menggabungkan keduanya akan membuat snooze mengotori riwayat pengiriman:
// snooze mengubah WAKTU, bukan isi pesan, dan itu tidak boleh terlihat
// sebagai pengiriman baru.

// JobInput adalah data satu job pengiriman baru.
type JobInput struct {
	Kind     domain.JobKind
	OwnerID  string
	DedupKey string
	Payload  map[string]any
	RunAt    time.Time
}

// CreateNotificationJob menyimpan job pengiriman.
//
// dedup_key bersifat UNIQUE, sehingga job yang sama tidak dibuat dua kali.
// Kalau sudah ada, fungsi ini mengembalikan id yang SUDAH ADA, bukan error:
// pemanggil biasanya hanya ingin tahu job mana yang mewakili pengiriman ini.
func (db *DB) CreateNotificationJob(ctx context.Context, q Querier, in JobInput) (int64, error) {
	payload, err := json.Marshal(in.Payload)
	if err != nil {
		return 0, fmt.Errorf("menyusun payload job: %w", err)
	}
	if len(payload) == 0 || string(payload) == "null" {
		payload = []byte("{}")
	}

	var id int64
	err = q.QueryRow(ctx, `
        INSERT INTO notification_jobs (kind, owner_id, dedup_key, payload, run_at)
        VALUES ($1,$2,$3,$4,$5)
        ON CONFLICT (dedup_key) DO UPDATE
           SET updated_at = now()
        RETURNING id`,
		in.Kind, in.OwnerID, in.DedupKey, payload, in.RunAt,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("membuat job notifikasi: %w", err)
	}
	return id, nil
}

// MarkJobDone menandai job selesai dikirim.
func (db *DB) MarkJobDone(ctx context.Context, q Querier, jobID int64) error {
	tag, err := q.Exec(ctx, `
        UPDATE notification_jobs
           SET status = 'done', claimed_at = NULL
         WHERE id = $1 AND status <> 'done'`, jobID)
	if err != nil {
		return fmt.Errorf("menandai job selesai: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Bisa berarti job sudah selesai sebelumnya. Itu bukan kegagalan:
		// pengiriman tetap dianggap berhasil.
		return nil
	}
	return nil
}

// MarkJobFailed menandai job gagal beserta alasannya.
//
// attempts dinaikkan; kalau sudah mencapai max_attempts, statusnya jadi
// failed permanen supaya tidak dicoba selamanya.
func (db *DB) MarkJobFailed(ctx context.Context, q Querier, jobID int64, reason string) error {
	tag, err := q.Exec(ctx, `
        UPDATE notification_jobs
           SET attempts   = attempts + 1,
               last_error = $2,
               status     = CASE WHEN attempts + 1 >= max_attempts
                                 THEN 'failed' ELSE 'pending' END,
               claimed_at = NULL
         WHERE id = $1`, jobID, reason)
	if err != nil {
		return fmt.Errorf("menandai job gagal: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// JobByID mengambil satu job.
func (db *DB) JobByID(ctx context.Context, q Querier, jobID int64) (*NotificationJob, error) {
	var j NotificationJob
	err := q.QueryRow(ctx, `
        SELECT id, kind, status, dedup_key, owner_id, payload, run_at,
               attempts, max_attempts, last_error, claimed_at, created_at
          FROM notification_jobs WHERE id = $1`, jobID,
	).Scan(&j.ID, &j.Kind, &j.Status, &j.DedupKey, &j.OwnerID, &j.Payload,
		&j.RunAt, &j.Attempts, &j.MaxAttempts, &j.LastError, &j.ClaimedAt,
		&j.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("membaca job notifikasi: %w", err)
	}
	return &j, nil
}

// NotificationJob adalah satu baris notification_jobs.
type NotificationJob struct {
	ID          int64
	Kind        string
	Status      string
	DedupKey    string
	OwnerID     string
	Payload     []byte
	RunAt       time.Time
	Attempts    int
	MaxAttempts int
	LastError   *string
	ClaimedAt   *time.Time
	CreatedAt   time.Time
}

// DeliveryInput adalah hasil pengiriman pada satu kanal.
type DeliveryInput struct {
	JobID      int64
	Channel    domain.Channel
	Status     domain.DeliveryStatus
	ProviderID *string
	ErrorCode  *string
	ErrorMsg   *string
}

// RecordDelivery mencatat hasil pengiriman satu kanal.
//
// UNIQUE (job_id, channel) membuat satu job hanya punya satu baris per kanal.
// Karena itu dipakai UPSERT: percobaan ulang memperbarui baris yang sama,
// bukan menumpuk baris baru. Kalau menumpuk, riwayat "berapa kali percobaan"
// tidak bisa dibaca dari jumlah baris.
func (db *DB) RecordDelivery(ctx context.Context, q Querier, in DeliveryInput) error {
	_, err := q.Exec(ctx, `
        INSERT INTO notification_deliveries (
            job_id, channel, status, provider_id, error_code, error_msg,
            attempts, sent_at
        ) VALUES ($1,$2,$3,$4,$5,$6,1,
                  CASE WHEN $3 = 'sent' THEN now() ELSE NULL END)
        ON CONFLICT (job_id, channel) DO UPDATE
           SET status      = EXCLUDED.status,
               provider_id = COALESCE(EXCLUDED.provider_id, notification_deliveries.provider_id),
               error_code  = EXCLUDED.error_code,
               error_msg   = EXCLUDED.error_msg,
               attempts    = notification_deliveries.attempts + 1,
               sent_at     = CASE WHEN EXCLUDED.status = 'sent'
                                  THEN now()
                                  ELSE notification_deliveries.sent_at END`,
		in.JobID, in.Channel, in.Status, in.ProviderID, in.ErrorCode, in.ErrorMsg)
	if err != nil {
		return fmt.Errorf("mencatat hasil pengiriman: %w", err)
	}
	return nil
}

// JobWithDeliveries adalah job beserta hasil pengiriman tiap kanal.
type JobWithDeliveries struct {
	NotificationJob
	Deliveries []Delivery
}

// Delivery adalah satu baris notification_deliveries.
type Delivery struct {
	Channel   string
	Status    string
	Attempts  int
	ErrorCode *string
	ErrorMsg  *string
	SentAt    *time.Time
}

// ListJobsFilter adalah syarat pencarian daftar job.
type ListJobsFilter struct {
	Status string
	Kind   string
	Limit  int
}

// ListJobs mengambil daftar job untuk halaman admin.
//
// TIDAK memuat payload: payload berisi detail pesan yang bisa memuat nama
// obat, dan halaman admin tidak boleh menampilkan isi obat pasien. Yang
// dibutuhkan operator hanya status dan jumlah percobaan.
func (db *DB) ListJobs(ctx context.Context, q Querier, f ListJobsFilter) ([]NotificationJob, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 50
	}

	where := []string{"1 = 1"}
	args := []any{}

	if f.Status != "" {
		args = append(args, f.Status)
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}
	if f.Kind != "" {
		args = append(args, f.Kind)
		where = append(where, fmt.Sprintf("kind = $%d", len(args)))
	}
	args = append(args, f.Limit)

	rows, err := q.Query(ctx, `
        SELECT id, kind, status, dedup_key, owner_id, '{}'::jsonb, run_at,
               attempts, max_attempts, last_error, claimed_at, created_at
          FROM notification_jobs
         WHERE `+joinAnd(where)+`
         ORDER BY created_at DESC
         LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("mendaftar job notifikasi: %w", err)
	}
	defer rows.Close()

	var out []NotificationJob
	for rows.Next() {
		var j NotificationJob
		if err := rows.Scan(&j.ID, &j.Kind, &j.Status, &j.DedupKey, &j.OwnerID,
			&j.Payload, &j.RunAt, &j.Attempts, &j.MaxAttempts, &j.LastError,
			&j.ClaimedAt, &j.CreatedAt); err != nil {
			return nil, fmt.Errorf("membaca job notifikasi: %w", err)
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// RetryJob mengembalikan job gagal ke keadaan pending supaya dicoba lagi.
//
// attempts di-reset ke nol, karena operator yang memutuskan untuk mencoba
// ulang — bukan sistem yang mengulang otomatis. Kalau tidak di-reset, job
// yang sudah mencapai max_attempts akan langsung gagal lagi.
func (db *DB) RetryJob(ctx context.Context, q Querier, jobID int64) error {
	tag, err := q.Exec(ctx, `
        UPDATE notification_jobs
           SET status = 'pending', attempts = 0, last_error = NULL,
               claimed_at = NULL
         WHERE id = $1 AND status = 'failed'`, jobID)
	if err != nil {
		return fmt.Errorf("mengulang job notifikasi: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Job tidak ditemukan, atau statusnya bukan failed. Keduanya berarti
		// job ini tidak bisa diulang sekarang.
		return domain.ErrNotFound
	}
	return nil
}

// CancelJob membatalkan job yang belum selesai.
func (db *DB) CancelJob(ctx context.Context, q Querier, jobID int64) error {
	tag, err := q.Exec(ctx, `
        UPDATE notification_jobs
           SET status = 'cancelled', claimed_at = NULL
         WHERE id = $1 AND status IN ('pending','claimed')`, jobID)
	if err != nil {
		return fmt.Errorf("membatalkan job notifikasi: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// JobStats adalah agregat job untuk halaman admin.
//
// TIDAK memuat identitas pasien maupun nama obat. Operator hanya perlu tahu
// berapa yang berhasil dan berapa yang gagal.
type JobStats struct {
	Pending   int
	Claimed   int
	Done      int
	Failed    int
	Cancelled int
}

// JobStats menghitung job per status.
func (db *DB) JobStats(ctx context.Context, q Querier) (JobStats, error) {
	var s JobStats
	rows, err := q.Query(ctx, `
        SELECT status, count(*)::int FROM notification_jobs GROUP BY status`)
	if err != nil {
		return s, fmt.Errorf("menghitung statistik job: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return s, fmt.Errorf("menghitung statistik job: %w", err)
		}
		switch status {
		case "pending":
			s.Pending = n
		case "claimed":
			s.Claimed = n
		case "done":
			s.Done = n
		case "failed":
			s.Failed = n
		case "cancelled":
			s.Cancelled = n
		}
	}
	return s, rows.Err()
}

// joinAnd menyatukan syarat WHERE dengan AND.
func joinAnd(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " AND "
		}
		out += p
	}
	return out
}
