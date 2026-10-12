package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	pohttp "github.com/frndly-bot/pantau-obat/backend/internal/transport/http"
)

// TestIsKnownDomainError_MencakupSemuaSentinel menjaga dua daftar tetap sinkron.
//
// Latar belakang: `isKnownDomainError` menentukan apakah sebuah error dicatat
// sebagai ERROR di log. Kalau daftarnya tertinggal dari `MapDomainError`,
// konflik yang sepenuhnya normal — mis. mencatat dosis yang sudah tercatat
// (409 LOG_OCCURRENCE_TAKEN) — akan tercatat sebagai ERROR. Log jadi penuh
// kebisingan, dan masalah sungguhan tenggelam di dalamnya.
//
// Uji ini menyusuri SELURUH sentinel domain, memetakannya lewat
// `MapDomainError`, lalu memastikan setiap error yang punya status HTTP
// eksplisit (bukan 500) juga dikenali `isKnownDomainError`. Sentinel baru
// yang lupa didaftarkan akan langsung menggagalkan uji ini.
func TestIsKnownDomainError_MencakupSemuaSentinel(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrNotFound", domain.ErrNotFound},
		{"ErrForbidden", domain.ErrForbidden},
		{"ErrUnauthenticated", domain.ErrUnauthenticated},
		{"ErrInvalidCredentials", domain.ErrInvalidCredentials},
		{"ErrTokenRevoked", domain.ErrTokenRevoked},
		{"ErrAdminScopeDenied", domain.ErrAdminScopeDenied},
		{"ErrCareNotAccepted", domain.ErrCareNotAccepted},
		{"ErrValidation", domain.ErrValidation},
		{"ErrDuplicateMedication", domain.ErrDuplicateMedication},
		{"ErrLogOccurrenceTaken", domain.ErrLogOccurrenceTaken},
		{"ErrDuplicateTimes", domain.ErrDuplicateTimes},
		{"ErrInvitePending", domain.ErrInvitePending},
		{"ErrStockInsufficient", domain.ErrStockInsufficient},
		{"ErrCatalogImmutable", domain.ErrCatalogImmutable},
		{"ErrCatalogDuplicateName", domain.ErrCatalogDuplicateName},
		{"ErrRateLimited", domain.ErrRateLimited},
	}

	for _, s := range sentinels {
		t.Run(s.name, func(t *testing.T) {
			status, code, _ := pohttp.MapDomainError(s.err)

			// Error yang dipetakan ke 500 memang TIDAK dikenal — itu benar,
			// karena 500 berarti ada yang tidak terduga.
			if status == http.StatusInternalServerError {
				t.Fatalf("%s dipetakan ke 500; seharusnya punya status eksplisit", s.name)
			}

			if !isKnownDomainError(s.err) {
				t.Errorf(
					"%s punya pemetaan HTTP (%d %s) tetapi TIDAK dikenali "+
						"isKnownDomainError — akibatnya konflik normal ini akan "+
						"tercatat sebagai ERROR di log",
					s.name, status, code,
				)
			}
		})
	}
}

// TestIsKnownDomainError_Terbungkus memastikan pembungkusan tidak lolos.
//
// Handler sering menerima error yang sudah dibungkus, mis.
// `fmt.Errorf("%w: ...", domain.ErrValidation)`. Kalau pemeriksaan tidak
// memakai errors.Is, error terbungkus akan dianggap tidak dikenal dan dicatat
// sebagai ERROR padahal sepenuhnya normal.
func TestIsKnownDomainError_Terbungkus(t *testing.T) {
	wrapped := []struct {
		name string
		err  error
	}{
		{"validation dibungkus", fmt.Errorf("%w: password lama salah", domain.ErrValidation)},
		{"occurrence dibungkus", fmt.Errorf("%w: 07:00 pada 2026-10-12", domain.ErrLogOccurrenceTaken)},
		{"notfound dibungkus", fmt.Errorf("%w: obat", domain.ErrNotFound)},
		{"forbidden dibungkus", fmt.Errorf("%w: hanya manager", domain.ErrForbidden)},
	}

	for _, w := range wrapped {
		t.Run(w.name, func(t *testing.T) {
			if !isKnownDomainError(w.err) {
				t.Errorf("error terbungkus %q tidak dikenali; errors.Is hilang?", w.err)
			}
		})
	}
}

// TestIsKnownDomainError_ErrorAsing memastikan error yang benar-benar tidak
// terduga TIDAK dianggap dikenal — supaya ia tetap tercatat sebagai ERROR.
func TestIsKnownDomainError_ErrorAsing(t *testing.T) {
	if isKnownDomainError(errors.New("koneksi database terputus")) {
		t.Error("error asing dianggap dikenal; ia harus tetap tercatat sebagai ERROR")
	}
	if isKnownDomainError(nil) {
		t.Error("nil dianggap dikenal; seharusnya tidak")
	}
}
