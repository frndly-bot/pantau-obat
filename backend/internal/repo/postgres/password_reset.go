package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// ResetToken adalah satu token pemulihan sandi yang tersimpan.
//
// Yang disimpan adalah HASH token, bukan tokennya. Token mentah hanya ada di
// tautan yang dikirim lewat email; bila database bocor, isinya tidak cukup
// untuk dipakai masuk.
type ResetToken struct {
	UserID    string
	ExpiresAt time.Time
}

// CreatePasswordResetToken menyimpan token pemulihan baru.
//
// Token lama milik user yang sama ditandai terpakai lebih dulu. Alasannya:
// pengguna yang meminta ulang karena email pertama tidak datang akan
// mengeklik tautan TERBARU — dan kalau yang lama masih berlaku, dua tautan
// hidup untuk satu akun sekaligus. Itu memperbesar permukaan serangan tanpa
// manfaat apa pun.
func (db *DB) CreatePasswordResetToken(
	ctx context.Context, q Querier, userID, tokenHash string, expiresAt time.Time,
) error {
	if _, err := q.Exec(ctx, `
        UPDATE password_reset_tokens
           SET used_at = now()
         WHERE user_id = $1 AND used_at IS NULL`, userID); err != nil {
		return fmt.Errorf("membatalkan token pemulihan lama: %w", err)
	}

	_, err := q.Exec(ctx, `
        INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
        VALUES ($1, $2, $3)`, userID, tokenHash, expiresAt)
	if err != nil {
		return fmt.Errorf("menyimpan token pemulihan: %w", err)
	}
	return nil
}

// ConsumePasswordResetToken memakai token pemulihan SEKALI.
//
// Mengembalikan userID bila token sah, belum kedaluwarsa, dan belum dipakai.
// Menandai token terpakai dalam transaksi yang SAMA dengan pembaruan sandi —
// sehingga token tidak bisa hangus tanpa sandi benar-benar berubah, dan
// sebaliknya tidak bisa dipakai dua kali bila penulisan sandi gagal.
func (db *DB) ConsumePasswordResetToken(
	ctx context.Context, q Querier, tokenHash string,
) (string, error) {
	var userID string
	err := q.QueryRow(ctx, `
        UPDATE password_reset_tokens
           SET used_at = now()
         WHERE token_hash = $1
           AND used_at IS NULL
           AND expires_at > now()
        RETURNING user_id`, tokenHash).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Tidak dibedakan antara "tidak ada", "sudah dipakai", dan
			// "kedaluwarsa". Membedakannya hanya membantu penyerang menebak
			// token mana yang pernah ada.
			return "", domain.ErrNotFound
		}
		return "", fmt.Errorf("memakai token pemulihan: %w", err)
	}
	return userID, nil
}

// LastResetRequestAt melaporkan kapan permintaan pemulihan terakhir dibuat
// untuk satu user. Dipakai untuk menahan permintaan beruntun.
func (db *DB) LastResetRequestAt(ctx context.Context, q Querier, userID string) (time.Time, error) {
	var t time.Time
	err := q.QueryRow(ctx, `
        SELECT created_at FROM password_reset_tokens
         WHERE user_id = $1
         ORDER BY created_at DESC
         LIMIT 1`, userID).Scan(&t)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("membaca permintaan pemulihan terakhir: %w", err)
	}
	return t, nil
}
