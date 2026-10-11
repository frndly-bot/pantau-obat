package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// File ini menampung query untuk webhook masuk: melacak delivery dari
// provider_id, dan membaca integrasi Telegram yang sudah terhubung.

// DeliveryByProvider mengembalikan delivery berdasar provider_id penyedia
// email.
//
// provider_id unik secara bisnis (api-v2 bagian 12: replay ditolak), tapi
// TIDAK punya UNIQUE constraint di skema — kolomnya nullable dan bisa kosong
// untuk delivery yang belum pernah terkirim. Idempotensi karena itu ditegakkan
// di sini: ambil satu, dan biarkan pemanggil memutuskan apa artinya.
//
// ErrNotFound berarti provider_id belum pernah tercatat.
func (db *DB) DeliveryByProvider(ctx context.Context, q Querier, providerID string) (*Delivery, error) {
	row := q.QueryRow(ctx, `
        SELECT channel, status, attempts, error_code, error_msg, sent_at
          FROM notification_deliveries
         WHERE provider_id = $1
         ORDER BY id DESC
         LIMIT 1`, providerID)

	var d Delivery
	err := row.Scan(&d.Channel, &d.Status, &d.Attempts, &d.ErrorCode, &d.ErrorMsg, &d.SentAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("mencari delivery dari provider_id: %w", err)
	}
	return &d, nil
}

// IntegrationByExternalID mengambil integrasi aktif milik satu akun eksternal
// (mis. chat ID Telegram).
//
// Dipakai webhook Telegram untuk memetakan update dari chat ke user aplikasi.
// revoked_at ikut diperiksa karena UNIQUE (provider, external_id) membuat
// baris lama tidak dihapus saat relasi dicabut — ia hanya ditandai.
func (db *DB) IntegrationByExternalID(ctx context.Context, q Querier, provider, externalID string) (userID string, err error) {
	err = q.QueryRow(ctx, `
        SELECT user_id
          FROM user_integrations
         WHERE provider = $1 AND external_id = $2 AND revoked_at IS NULL`,
		provider, externalID).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", domain.ErrNotFound
		}
		return "", fmt.Errorf("mencari integrasi %s: %w", provider, err)
	}
	return userID, nil
}
