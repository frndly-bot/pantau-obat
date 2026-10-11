package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// File ini mengurus token perangkat untuk notifikasi push.
//
// Token bersifat UNIK di seluruh tabel, bukan per user. Alasannya: satu
// perangkat hanya boleh terhubung ke satu akun. Kalau user berganti akun di
// perangkat yang sama, token lama harus PINDAH kepemilikan, bukan dibuat
// baris baru — kalau tidak, pengingat obat user sebelumnya akan terus
// dikirim ke perangkat itu. Untuk data kesehatan, itu kebocoran.

const deviceTokenColumns = `
    id, user_id, platform, token, app_version, last_seen_at, revoked_at, created_at`

// DeviceToken adalah satu perangkat penerima push.
type DeviceToken struct {
	ID         string
	UserID     string
	Platform   domain.DevicePlatform
	Token      string
	AppVersion *string
	LastSeenAt time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

// RegisterDeviceToken mendaftarkan token perangkat.
//
// Idempoten per token: memanggil ulang dengan token yang sama hanya
// memutakhirkan last_seen_at dan platform. Ini yang membuat aplikasi boleh
// memanggil endpoint ini setiap kali dibuka, tanpa khawatir menumpuk baris.
//
// Kalau token itu sudah dimiliki user LAIN, kepemilikannya dipindahkan.
// Skenario nyatanya: dua orang berbagi satu ponsel, atau user berganti akun.
// Tanpa pemindahan, notifikasi akun lama akan terus sampai ke perangkat itu.
func (db *DB) RegisterDeviceToken(ctx context.Context, q Querier, userID string, in DeviceTokenInput) (*DeviceToken, error) {
	var t DeviceToken
	err := q.QueryRow(ctx, `
        INSERT INTO device_tokens (user_id, platform, token, app_version)
        VALUES ($1, $2, $3, $4)
        ON CONFLICT (token) DO UPDATE
           SET user_id      = EXCLUDED.user_id,
               platform     = EXCLUDED.platform,
               app_version  = EXCLUDED.app_version,
               last_seen_at = now(),
               revoked_at   = NULL
        RETURNING `+deviceTokenColumns,
		userID, in.Platform, in.Token, in.AppVersion,
	).Scan(&t.ID, &t.UserID, &t.Platform, &t.Token, &t.AppVersion,
		&t.LastSeenAt, &t.RevokedAt, &t.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("mendaftarkan perangkat: %w", err)
	}
	return &t, nil
}

// DeviceTokenInput adalah data pendaftaran perangkat.
type DeviceTokenInput struct {
	Platform   domain.DevicePlatform
	Token      string
	AppVersion *string
}

// RevokeDeviceToken mencabut satu token perangkat milik user.
//
// Dibatasi ownerID supaya satu user tidak bisa mencabut perangkat user lain
// hanya dengan menebak tokennya.
func (db *DB) RevokeDeviceToken(ctx context.Context, q Querier, userID, token string) error {
	tag, err := q.Exec(ctx, `
        UPDATE device_tokens SET revoked_at = now()
         WHERE token = $1 AND user_id = $2 AND revoked_at IS NULL`, token, userID)
	if err != nil {
		return fmt.Errorf("mencabut perangkat: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// ListDeviceTokens mengambil daftar perangkat aktif milik user.
func (db *DB) ListDeviceTokens(ctx context.Context, q Querier, userID string) ([]DeviceToken, error) {
	rows, err := q.Query(ctx, `
        SELECT `+deviceTokenColumns+`
          FROM device_tokens
         WHERE user_id = $1 AND revoked_at IS NULL
         ORDER BY last_seen_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("mendaftar perangkat: %w", err)
	}
	defer rows.Close()

	var out []DeviceToken
	for rows.Next() {
		var t DeviceToken
		if err := rows.Scan(&t.ID, &t.UserID, &t.Platform, &t.Token,
			&t.AppVersion, &t.LastSeenAt, &t.RevokedAt, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("membaca perangkat: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// PurgeRevokedTokens menghapus fisik perangkat yang sudah dicabut lama.
//
// Aman dihapus karena token yang sudah dicabut tidak mungkin dipakai lagi;
// menyimpannya hanya menambah ukuran tabel. Dijalankan worker retensi,
// sesuai tabel retensi di data-model-v2 bagian 11 (90 hari).
func (db *DB) PurgeRevokedTokens(ctx context.Context, olderThan time.Duration) (int64, error) {
	tag, err := db.pool.Exec(ctx, `
        DELETE FROM device_tokens
         WHERE revoked_at IS NOT NULL
           AND revoked_at < now() - $1::interval`,
		fmt.Sprintf("%d seconds", int64(olderThan.Seconds())))
	if err != nil {
		return 0, fmt.Errorf("menyapu perangkat dicabut: %w", err)
	}
	return tag.RowsAffected(), nil
}

// deviceTokenByToken mengambil satu perangkat berdasarkan token.
func (db *DB) deviceTokenByToken(ctx context.Context, q Querier, token string) (*DeviceToken, error) {
	var t DeviceToken
	err := q.QueryRow(ctx, `
        SELECT `+deviceTokenColumns+` FROM device_tokens WHERE token = $1`, token,
	).Scan(&t.ID, &t.UserID, &t.Platform, &t.Token, &t.AppVersion,
		&t.LastSeenAt, &t.RevokedAt, &t.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &t, nil
}
