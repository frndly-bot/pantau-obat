// Package worker memuat loop latar: scheduler, dispatcher, pass harian.
//
// ATURAN IMPORT: worker boleh mengimpor `service`, `repo`, dan `domain`.
// TIDAK BOLEH mengimpor `transport`.
//
// Alasannya bukan kerapian: kalau worker mengimpor transport, memindahkannya
// jadi binary kedua nanti akan menyeret seluruh lapisan HTTP ikut berpindah.
// Dengan batas ini, memisahkan worker hanya perlu file main.go baru.
package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	"github.com/frndly-bot/pantau-obat/backend/internal/repo/postgres"
)

// BumpItem adalah satu baris antrean pengingat.
//
// bump_queue menjawab pertanyaan "KAPAN sesuatu harus terjadi", berbeda dari
// notification_jobs yang mencatat "apa yang sudah dikirim". Pemisahan itu
// disengaja: snooze mengubah waktu, bukan isi pesan, sehingga riwayat
// pengiriman tidak ikut kotor.
type BumpItem struct {
	ID           int64
	Kind         domain.BumpKind
	OwnerID      string
	MedicationID *string
	ScheduleID   *string
	ScheduledAt  time.Time
	SlotDate     time.Time
	DedupKey     string
	Attempts     int
	SkipTag      *string
}

// EnqueueBump menambahkan satu item ke antrean, atau mengabaikannya bila
// sudah ada.
//
// Idempoten lewat uq_bump_dedup: kunci yang sama tidak akan membuat baris
// kedua selama baris pertama belum selesai. Ini yang membuat scheduler boleh
// berjalan setiap menit tanpa takut menggandakan pengingat.
func (db *DBQueue) EnqueueBump(ctx context.Context, q Querier, in EnqueueInput) (bool, error) {
	tag, err := q.Exec(ctx, `
        INSERT INTO bump_queue (
            kind, owner_id, medication_id, schedule_id, scheduled_at,
            slot_date, dedup_key, skip_tag
        ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
        ON CONFLICT (dedup_key) WHERE done_at IS NULL DO NOTHING`,
		in.Kind, in.OwnerID, in.MedicationID, in.ScheduleID,
		in.ScheduledAt, in.SlotDate, in.DedupKey, in.SkipTag)
	if err != nil {
		return false, fmt.Errorf("menambah antrean pengingat: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// EnqueueInput adalah data satu item antrean.
type EnqueueInput struct {
	Kind         domain.BumpKind
	OwnerID      string
	MedicationID *string
	ScheduleID   *string
	ScheduledAt  time.Time
	SlotDate     time.Time
	DedupKey     string

	// SkipTag menandai alasan sebuah item tidak dikirim tepat waktu, mis.
	// "snooze" atau "quiet". Disimpan supaya riwayatnya bisa ditelusuri:
	// item yang tertunda berkali-kali karena jam sunyi perlu bisa dikenali,
	// bukan hanya terlihat sebagai baris berulang tanpa sebab.
	SkipTag *string
}

// ClaimDueBumps mengambil item antrean yang sudah waktunya, dengan kunci.
//
// Memakai FOR UPDATE SKIP LOCKED, dan itu yang membuat beberapa proses bisa
// mengambil pekerjaan yang sama tanpa saling menunggu: baris yang sedang
// diklaim proses lain akan dilewati, bukan ditunggu. Untuk satu proses,
// perilakunya sama saja dengan SELECT biasa.
//
// Yang di-claim adalah item yang scheduled_at-nya sudah lewat DAN belum
// selesai. Batas jumlah menjaga satu siklus tidak mengambil terlalu banyak
// sekaligus, sehingga item yang baru masuk tidak kelaparan.
func (db *DBQueue) ClaimDueBumps(ctx context.Context, q Querier, now time.Time, limit int) ([]BumpItem, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := q.Query(ctx, `
        WITH claimed AS (
            SELECT id FROM bump_queue
             WHERE done_at IS NULL
               AND scheduled_at <= $1
             ORDER BY scheduled_at
             LIMIT $2
               FOR UPDATE SKIP LOCKED
        )
        UPDATE bump_queue b
           SET claimed_at = now(),
               attempts   = b.attempts + 1
          FROM claimed c
         WHERE b.id = c.id
        RETURNING b.id, b.kind, b.owner_id, b.medication_id, b.schedule_id,
                  b.scheduled_at, b.slot_date, b.dedup_key, b.attempts, b.skip_tag`,
		now, limit)
	if err != nil {
		return nil, fmt.Errorf("mengambil antrean yang jatuh tempo: %w", err)
	}
	defer rows.Close()

	var out []BumpItem
	for rows.Next() {
		var it BumpItem
		if err := rows.Scan(&it.ID, &it.Kind, &it.OwnerID, &it.MedicationID,
			&it.ScheduleID, &it.ScheduledAt, &it.SlotDate, &it.DedupKey,
			&it.Attempts, &it.SkipTag); err != nil {
			return nil, fmt.Errorf("membaca antrean: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// MarkBumpDone menandai item antrean selesai.
//
// Tidak menghapus baris, hanya mengisi done_at. Alasannya: baris selesai
// dipakai untuk menyelidiki "kenapa saya tidak dapat notifikasi", dan
// pembersihannya dilakukan worker retensi setelah 7 hari, bukan di sini.
func (db *DBQueue) MarkBumpDone(ctx context.Context, q Querier, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := q.Exec(ctx, `
        UPDATE bump_queue SET done_at = now()
         WHERE id = ANY($1) AND done_at IS NULL`, ids)
	if err != nil {
		return fmt.Errorf("menandai antrean selesai: %w", err)
	}
	return nil
}

// ReleaseStaleBumps melepas item yang sudah diklaim tapi tidak selesai.
//
// Ini penting untuk pemulihan: kalau proses mati di tengah pengiriman, item
// itu tertinggal dalam keadaan claimed_at terisi tapi done_at kosong, dan
// tanpa dilepas ia tidak akan pernah diproses lagi — pengingatnya hilang
// tanpa jejak.
//
// attempts sudah dinaikkan saat claim, sehingga item yang berulang gagal
// tidak berputar selamanya: maxAttempts membatasinya.
func (db *DBQueue) ReleaseStaleBumps(ctx context.Context, q Querier, olderThan time.Time, maxAttempts int) (int64, error) {
	tag, err := q.Exec(ctx, `
        UPDATE bump_queue
           SET claimed_at = NULL
         WHERE done_at IS NULL
           AND claimed_at IS NOT NULL
           AND claimed_at < $1
           AND attempts < $2`, olderThan, maxAttempts)
	if err != nil {
		return 0, fmt.Errorf("melepas antrean basi: %w", err)
	}
	return tag.RowsAffected(), nil
}

// CountPendingBumps menghitung antrean yang belum selesai. Dipakai health
// check internal dan halaman admin untuk melihat tumpukan.
func (db *DBQueue) CountPendingBumps(ctx context.Context, q Querier) (int, error) {
	var n int
	err := q.QueryRow(ctx, `
        SELECT count(*) FROM bump_queue WHERE done_at IS NULL`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("menghitung antrean: %w", err)
	}
	return n, nil
}

// PurgeDoneBumps menghapus baris antrean yang sudah selesai dan lebih tua
// dari cutoff.
//
// bump_queue menjawab "KAPAN sesuatu harus terjadi" — umurnya pendek. Riwayat
// yang lebih lama tidak punya pembaca: hasil pengiriman tercatat terpisah di
// notification_jobs (30 hari). Baris selesai yang dibiarkan hanya
// menggelembungkan tabel dan memperlambat index-nya.
//
// HANYA baris dengan done_at yang dihapus. Baris yang belum selesai adalah
// pekerjaan yang belum dilakukan — menghapusnya berarti menghilangkan
// pengingat user.
func (db *DBQueue) PurgeDoneBumps(ctx context.Context, q Querier, olderThan time.Duration) (int64, error) {
	tag, err := q.Exec(ctx, `
        DELETE FROM bump_queue
         WHERE done_at IS NOT NULL
           AND done_at < now() - $1::interval`,
		fmt.Sprintf("%d seconds", int64(olderThan.Seconds())))
	if err != nil {
		return 0, fmt.Errorf("membersihkan antrean usang: %w", err)
	}
	return tag.RowsAffected(), nil
}

// Querier adalah irisan yang dipenuhi pool maupun transaksi.
type Querier = postgres.Querier

// DBQueue membungkus pool untuk operasi antrean.
//
// Dipisah dari worker loop supaya logika antrean bisa diuji tanpa menjalankan
// loop-nya, dan supaya loop tidak perlu tahu detail SQL.
type DBQueue struct {
	DB *postgres.DB
}

// NewDBQueue membuat pembungkus antrean.
func NewDBQueue(db *postgres.DB) *DBQueue {
	return &DBQueue{DB: db}
}

// WithTx meneruskan ke transaksi pool.
func (q *DBQueue) WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return q.DB.WithTx(ctx, fn)
}

// txOrPool mengembalikan pool sebagai Querier.
func (q *DBQueue) txOrPool() Querier { return q.DB.Pool() }

// ErrWorkerStopped menandai loop yang berhenti karena context dibatalkan.
var ErrWorkerStopped = errors.New("worker dihentikan")
