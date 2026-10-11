package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// RevocationByUser membaca garis potong pencabutan milik user.
//
// Mengembalikan nil, nil bila user belum pernah dicabut. Itu bukan error:
// kondisi normal untuk mayoritas user.
//
// Pembacaan ini terjadi di setiap request terautentikasi, jadi jalurnya
// harus murah. Primary key-nya user_id, sehingga ini satu lookup index.
// Belum ada cache; kalau nanti terbukti jadi titik panas, hasilnya bisa
// di-cache pendek tanpa mengubah bentuk API di sini.
func (db *DB) RevocationByUser(ctx context.Context, q Querier, userID string) (*domain.Revocation, error) {
	var r domain.Revocation
	err := q.QueryRow(ctx, `
        SELECT user_id, revoked_at, reason
          FROM user_token_revocations
         WHERE user_id = $1`, userID,
	).Scan(&r.UserID, &r.RevokedAt, &r.Reason)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("membaca pencabutan token: %w", err)
	}
	return &r, nil
}

// SetRevocation memasang atau memperbarui garis potong pencabutan.
//
// Memakai UPSERT, bukan INSERT biasa: satu user hanya punya SATU garis
// potong. Pencabutan berikutnya cukup memindahkan garis itu ke depan.
// Membiarkan banyak baris akan membuat pemeriksaan harus memilih yang mana,
// dan itu justru menambah peluang salah.
//
// revoked_at memakai now() dari database, bukan waktu aplikasi, supaya
// garis potong memakai jam yang sama dengan `iat` pada token yang
// diverifikasi di proses ini juga.
func (db *DB) SetRevocation(ctx context.Context, q Querier, userID, reason string, revokedBy *string) error {
	_, err := q.Exec(ctx, `
        INSERT INTO user_token_revocations (user_id, revoked_at, reason, revoked_by)
        VALUES ($1, now(), $2, $3)
        ON CONFLICT (user_id) DO UPDATE
           SET revoked_at = now(),
               reason     = EXCLUDED.reason,
               revoked_by = EXCLUDED.revoked_by`,
		userID, reason, revokedBy)
	if err != nil {
		return fmt.Errorf("memasang pencabutan token: %w", err)
	}
	return nil
}

// ClearRevocation menghapus garis potong, sehingga token baru berlaku normal.
//
// Dipakai saat admin mengaktifkan kembali akun yang sebelumnya dinonaktifkan.
// Tanpa ini, garis potong lama akan terus menolak token walau akun sudah
// aktif lagi.
//
// Perhatikan: ini TIDAK menghidupkan kembali token lama yang sudah ditolak.
// Token lama tetap tidak berlaku karena `iat`-nya tetap sebelum garis
// potong lama; hanya saja garis potongnya kini hilang, dan yang menentukan
// adalah masa berlaku masing-masing token.
func (db *DB) ClearRevocation(ctx context.Context, q Querier, userID string) error {
	_, err := q.Exec(ctx, `DELETE FROM user_token_revocations WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("menghapus pencabutan token: %w", err)
	}
	return nil
}

// RevokeAccessAndSessions mencabut SEMUA akses seorang user dalam satu
// transaksi: garis potong token dipasang, dan seluruh sesi refresh dicabut.
//
// Keduanya harus bersama-sama. Kalau hanya sesi yang dicabut, access token
// yang sudah beredar masih hidup sampai 24 jam. Kalau hanya garis potong
// yang dipasang, refresh token masih bisa menukar token baru, dan token
// baru itu diterbitkan SETELAH garis potong, sehingga lolos.
//
// revokedAt dikembalikan supaya pemanggil bisa memakainya di audit.
func (db *DB) RevokeAccessAndSessions(
	ctx context.Context, q Querier, userID, reason string, revokedBy *string,
) (time.Time, error) {
	if err := db.SetRevocation(ctx, q, userID, reason, revokedBy); err != nil {
		return time.Time{}, err
	}
	if err := db.RevokeAllUserSessions(ctx, q, userID); err != nil {
		return time.Time{}, err
	}

	var revokedAt time.Time
	if err := q.QueryRow(ctx,
		`SELECT revoked_at FROM user_token_revocations WHERE user_id = $1`, userID,
	).Scan(&revokedAt); err != nil {
		return time.Time{}, fmt.Errorf("membaca waktu pencabutan: %w", err)
	}
	return revokedAt, nil
}

// PurgeOldRevocations membuang garis potong yang sudah tidak relevan.
//
// Sebuah garis potong hanya berguna selama masih ada token yang mungkin
// diterbitkan sebelum garis itu dan belum kedaluwarsa. Setelah lewat umur
// access token terpanjang, tidak ada lagi token seperti itu, jadi barisnya
// aman dihapus. Dijalankan worker retensi.
func (db *DB) PurgeOldRevocations(ctx context.Context, olderThan time.Duration) (int64, error) {
	tag, err := db.pool.Exec(ctx, `
        DELETE FROM user_token_revocations
         WHERE revoked_at < now() - $1::interval`,
		fmt.Sprintf("%d seconds", int64(olderThan.Seconds())))
	if err != nil {
		return 0, fmt.Errorf("menyapu pencabutan lama: %w", err)
	}
	return tag.RowsAffected(), nil
}
