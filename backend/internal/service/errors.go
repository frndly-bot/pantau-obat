package service

import (
	"errors"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	"github.com/frndly-bot/pantau-obat/backend/internal/repo/postgres"
	"github.com/jackc/pgx/v5"
)

// isNotFound melaporkan apakah error berarti "data tidak ada".
//
// Dipakai untuk membedakan dua hal yang sering tercampur: "bagian opsional
// dari agregat memang tidak ada" (mis. obat tanpa baris stok, karena stok
// hanya dibuat bila user mengisinya) versus "query-nya benar-benar gagal".
// Yang pertama bukan error; yang kedua harus dilaporkan.
//
// Ketiga bentuk diperiksa karena sentinel domain dan error pgx bisa muncul
// tergantung jalur mana yang menemukannya lebih dulu.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, domain.ErrNotFound) || errors.Is(err, pgx.ErrNoRows)
}

// isConflict melaporkan apakah error berarti "bentrok dengan data yang ada".
func isConflict(err error) bool {
	return errors.Is(err, domain.ErrDuplicateMedication) ||
		errors.Is(err, domain.ErrDuplicateTimes) ||
		errors.Is(err, domain.ErrLogOccurrenceTaken) ||
		errors.Is(err, domain.ErrInvitePending) ||
		errors.Is(err, domain.ErrStockInsufficient) ||
		errors.Is(err, domain.ErrCatalogImmutable)
}

// isForbidden melaporkan apakah error berarti "tidak berhak".
func isForbidden(err error) bool {
	return errors.Is(err, domain.ErrForbidden) ||
		errors.Is(err, domain.ErrAdminScopeDenied) ||
		errors.Is(err, domain.ErrCareNotAccepted)
}

// pastikan postgres tetap diimpor bila suatu saat dipakai di file ini.
var _ = postgres.IsUniqueViolation
