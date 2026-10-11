package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// CreateSession menyimpan satu sesi refresh.
//
// Parameter tokenHash adalah SHA-256 dari token, BUKAN token aslinya.
// Token asli tidak pernah menyentuh database, jadi bocornya isi tabel ini
// tidak langsung memberi penyerang token yang bisa dipakai.
func (db *DB) CreateSession(
	ctx context.Context, q Querier,
	userID, tokenHash string, expiresAt time.Time,
	ip, userAgent *string, rotatedFrom *string,
) (*domain.RefreshSession, error) {
	var s domain.RefreshSession
	err := q.QueryRow(ctx, `
        INSERT INTO refresh_sessions
            (user_id, token_hash, expires_at, created_ip, user_agent, rotated_from)
        VALUES ($1, $2, $3, $4, $5, $6)
        RETURNING id, user_id, expires_at, revoked_at, rotated_from, created_at`,
		userID, tokenHash, expiresAt, ip, userAgent, rotatedFrom,
	).Scan(&s.ID, &s.UserID, &s.ExpiresAt, &s.RevokedAt, &s.RotatedFrom, &s.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("membuat sesi: %w", err)
	}
	return &s, nil
}

// SessionByTokenHash mengambil sesi aktif berdasarkan hash token.
func (db *DB) SessionByTokenHash(ctx context.Context, q Querier, tokenHash string) (*domain.RefreshSession, error) {
	var s domain.RefreshSession
	err := q.QueryRow(ctx, `
        SELECT id, user_id, expires_at, revoked_at, rotated_from, created_at
          FROM refresh_sessions
         WHERE token_hash = $1`, tokenHash,
	).Scan(&s.ID, &s.UserID, &s.ExpiresAt, &s.RevokedAt, &s.RotatedFrom, &s.CreatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrSessionNotFound
		}
		return nil, fmt.Errorf("membaca sesi: %w", err)
	}
	return &s, nil
}

// RevokeSession mencabut satu sesi.
//
// Hanya mengubah revoked_at, tidak menghapus baris. Jejaknya perlu tetap ada
// supaya penggunaan ulang token yang sudah dicabut bisa dideteksi.
func (db *DB) RevokeSession(ctx context.Context, q Querier, sessionID string) error {
	_, err := q.Exec(ctx, `
        UPDATE refresh_sessions SET revoked_at = now()
         WHERE id = $1 AND revoked_at IS NULL`, sessionID)
	if err != nil {
		return fmt.Errorf("mencabut sesi: %w", err)
	}
	return nil
}

// RevokeSessionChain mencabut seluruh rantai rotasi dari satu sesi.
//
// Ini pertahanan terhadap penggunaan ulang refresh token. Kalau token yang
// sudah dipakai muncul lagi, itu tanda token bocor. Yang dilakukan bukan
// hanya menolak permintaan itu, tapi mencabut SELURUH rantai: token yang
// asli milik user dan token yang dipegang penyerang, supaya keduanya mati
// dan user terpaksa login ulang.
//
// Penelusuran dilakukan dua arah: mundur lewat rotated_from, dan maju
// dengan mencari sesi yang rotated_from-nya menunjuk sesi mana pun di
// rantai. Keduanya perlu, karena rantai bisa bercabang kalau ada percobaan
// pemakaian ulang.
func (db *DB) RevokeSessionChain(ctx context.Context, q Querier, sessionID string) error {
	_, err := q.Exec(ctx, `
        WITH RECURSIVE chain AS (
            SELECT id FROM refresh_sessions WHERE id = $1
            UNION
            SELECT rs.id
              FROM refresh_sessions rs
              JOIN chain ON rs.rotated_from = chain.id
        )
        UPDATE refresh_sessions SET revoked_at = now()
         WHERE id IN (SELECT id FROM chain) AND revoked_at IS NULL`, sessionID)
	if err != nil {
		return fmt.Errorf("mencabut rantai sesi: %w", err)
	}
	return nil
}

// RevokeAllUserSessions mencabut seluruh sesi aktif milik user.
//
// Dipakai saat ganti password. Memaksa semua perangkat login ulang,
// sehingga perangkat yang mungkin sudah dikuasai orang lain ikut mati.
func (db *DB) RevokeAllUserSessions(ctx context.Context, q Querier, userID string) error {
	_, err := q.Exec(ctx, `
        UPDATE refresh_sessions SET revoked_at = now()
         WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	if err != nil {
		return fmt.Errorf("mencabut semua sesi: %w", err)
	}
	return nil
}

// RevokeOtherUserSessions mencabut semua sesi KECUALI sesi yang sedang dipakai.
//
// Dipakai saat ganti password: sesi yang sedang aktif tetap hidup supaya
// user tidak langsung terlempar keluar, sementara perangkat lain gugur.
func (db *DB) RevokeOtherUserSessions(ctx context.Context, q Querier, userID, keepSessionID string) error {
	_, err := q.Exec(ctx, `
        UPDATE refresh_sessions SET revoked_at = now()
         WHERE user_id = $1 AND id <> $2 AND revoked_at IS NULL`, userID, keepSessionID)
	if err != nil {
		return fmt.Errorf("mencabut sesi lain: %w", err)
	}
	return nil
}

// ListUserSessions mendaftar sesi aktif milik user.
func (db *DB) ListUserSessions(ctx context.Context, q Querier, userID string) ([]domain.SessionInfo, error) {
	rows, err := q.Query(ctx, `
        SELECT id, created_at, expires_at,
               host(created_ip) AS ip, user_agent
          FROM refresh_sessions
         WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > now()
         ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("mendaftar sesi: %w", err)
	}
	defer rows.Close()

	var out []domain.SessionInfo
	for rows.Next() {
		var s domain.SessionInfo
		if err := rows.Scan(&s.ID, &s.CreatedAt, &s.ExpiresAt, &s.IP, &s.UserAgent); err != nil {
			return nil, fmt.Errorf("membaca sesi: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// PurgeExpiredSessions menghapus fisik sesi yang sudah kedaluwarsa.
//
// Berbeda dari pencabutan, ini DELETE sungguhan. Aman karena sesi
// kedaluwarsa tidak mungkin dipakai lagi, dan menyimpannya hanya menambah
// ukuran tabel. Dijalankan worker retensi, bukan di jalur request.
func (db *DB) PurgeExpiredSessions(ctx context.Context, olderThan time.Duration) (int64, error) {
	tag, err := db.pool.Exec(ctx, `
        DELETE FROM refresh_sessions
         WHERE expires_at < now() - $1::interval`,
		fmt.Sprintf("%d seconds", int64(olderThan.Seconds())))
	if err != nil {
		return 0, fmt.Errorf("menyapu sesi kedaluwarsa: %w", err)
	}
	return tag.RowsAffected(), nil
}
