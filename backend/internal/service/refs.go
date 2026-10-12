package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/repo/postgres"
)

// Refs menyediakan tabel referensi untuk formulir obat.
type Refs struct {
	db     *postgres.DB
	logger *slog.Logger
}

// NewRefs membuat layanan referensi.
func NewRefs(db *postgres.DB, logger *slog.Logger) *Refs {
	return &Refs{db: db, logger: logger}
}

// All mengambil seluruh tabel referensi.
func (s *Refs) All(ctx context.Context) (*postgres.Refs, error) {
	var out *postgres.Refs
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		refs, err := s.db.AllRefs(ctx, tx)
		if err != nil {
			return err
		}
		out = refs
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("mengambil tabel referensi: %w", err)
	}
	return out, nil
}
