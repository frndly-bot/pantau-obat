package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	"github.com/frndly-bot/pantau-obat/backend/internal/repo/postgres"
)

// RevokeAccess mematikan SEMUA akses seorang user, seketika.
//
// Dua hal dilakukan bersama, dan keduanya wajib:
//
//  1. Garis potong token dipasang. Tanpa ini, access token yang sudah
//     beredar masih lolos verifikasi JWT sampai 24 jam.
//  2. Seluruh sesi refresh dicabut. Tanpa ini, refresh token yang masih
//     hidup bisa menukar access token BARU, dan token baru itu diterbitkan
//     setelah garis potong, sehingga justru lolos.
//
// Dijalankan dalam satu transaksi supaya tidak ada jendela di mana hanya
// salah satu yang berlaku. Jendela seperti itu justru yang paling berbahaya:
// akses tampak sudah dicabut, padahal masih bisa diperbarui.
//
// alasan disimpan untuk audit. revokedBy nil berarti user memicunya sendiri.
func (s *Auth) RevokeAccess(ctx context.Context, userID, reason string, revokedBy *string) error {
	return s.db.WithTx(ctx, func(tx pgx.Tx) error {
		revokedAt, err := s.db.RevokeAccessAndSessions(ctx, tx, userID, reason, revokedBy)
		if err != nil {
			return err
		}

		actorRole := string(domain.ActorUser)
		if revokedBy != nil {
			actorRole = string(domain.ActorAdmin)
		}

		return s.auditOrFail(ctx, tx, postgres.AuditEntry{
			ActorID:      revokedBy,
			OwnerID:      &userID,
			ActorRole:    actorRole,
			Action:       postgres.ActionRevoke,
			ResourceType: postgres.ResourceAuth,
			ResourceID:   &userID,
			AfterState: map[string]any{
				"revoked_at": revokedAt.UTC().Format(time.RFC3339Nano),
				"reason":     reason,
			},
			Meta: map[string]any{
				"all_access_revoked": true,
				"sessions_revoked":   true,
			},
		})
	})
}

// RevokedSince mengembalikan waktu pencabutan terakhir untuk user, atau
// waktu nol bila belum pernah dicabut.
func (s *Auth) RevokedSince(ctx context.Context, userID string) (time.Time, error) {
	rev, err := s.db.RevocationByUser(ctx, s.db.Pool(), userID)
	if err != nil {
		return time.Time{}, err
	}
	if rev == nil {
		return time.Time{}, nil
	}
	return rev.RevokedAt, nil
}

// IsTokenRevoked memeriksa apakah token yang diterbitkan pada issuedAt
// sudah tidak berlaku lagi untuk user ini.
func (s *Auth) IsTokenRevoked(ctx context.Context, userID string, issuedAt time.Time) (bool, error) {
	rev, err := s.db.RevocationByUser(ctx, s.db.Pool(), userID)
	if err != nil {
		return false, err
	}
	return rev.IsRevoked(issuedAt), nil
}

// revokeForUser menjalankan pencabutan di dalam transaksi yang sudah ada.
// Dipakai oleh alur yang mencabut sebagai bagian dari perubahan lain
// (ganti password, nonaktifkan akun).
func (s *Auth) revokeForUserTx(ctx context.Context, tx pgx.Tx, userID, reason string, by *string) error {
	_, err := s.db.RevokeAccessAndSessions(ctx, tx, userID, reason, by)
	if err != nil {
		s.logger.ErrorContext(ctx, "gagal mencabut akses",
			slog.String("user_id", userID),
			slog.String("reason", reason),
			slog.String("error", err.Error()))
		return err
	}
	return nil
}
