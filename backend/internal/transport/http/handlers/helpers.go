package handlers

import (
	"errors"
	"net/http"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	"github.com/frndly-bot/pantau-obat/backend/internal/service"
	pohttp "github.com/frndly-bot/pantau-obat/backend/internal/transport/http"
)

// metaOf menyusun metadata request untuk audit.
//
// Dikumpulkan di satu fungsi supaya setiap handler mencatat hal yang sama.
// Kalau tiap handler menyusunnya sendiri, cepat atau lambat ada yang lupa
// mengisi IP atau user agent, dan jejak auditnya jadi tidak lengkap justru
// di tempat yang paling membutuhkannya.
func metaOf(r *http.Request) service.Meta {
	return service.Meta{
		IP:        pohttp.ClientIP(r),
		UserAgent: service.NormalizeUserAgent(r.UserAgent()),
	}
}

// validationResult adalah hasil pemetaan error validasi bertipe.
type validationResult struct {
	message string
	details any
}

// validationResponse memetakan error validasi bertipe ke pesan dan detail
// yang menyebutkan field mana yang bermasalah.
//
// Mengembalikan ok=false bila error bukan salah satu tipe validasi yang
// dikenal, sehingga pemanggil bisa melanjutkan ke pemetaan umum.
//
// Urutan pemeriksaan mengikuti kemungkinan kemunculannya, bukan abjad:
// field yang paling sering bermasalah diperiksa lebih dulu.
func validationResponse(err error) (validationResult, bool) {
	var emailErr *domain.EmailError
	if errors.As(err, &emailErr) {
		return validationResult{
			message: emailErr.Reason,
			details: []pohttp.FieldIssue{{Field: "email", Issue: "invalid"}},
		}, true
	}

	var nameErr *domain.NameError
	if errors.As(err, &nameErr) {
		return validationResult{
			message: nameErr.Reason,
			details: []pohttp.FieldIssue{{Field: "full_name", Issue: "invalid"}},
		}, true
	}

	var phoneErr *domain.PhoneError
	if errors.As(err, &phoneErr) {
		return validationResult{
			message: phoneErr.Reason,
			details: []pohttp.FieldIssue{{Field: "phone", Issue: "invalid"}},
		}, true
	}

	var tzErr *domain.TimezoneError
	if errors.As(err, &tzErr) {
		return validationResult{
			message: tzErr.Reason,
			details: []pohttp.FieldIssue{{Field: "timezone", Issue: "invalid"}},
		}, true
	}

	var weakErr *domain.WeakPasswordError
	if errors.As(err, &weakErr) {
		return validationResult{
			message: weakErr.Reason,
			details: []pohttp.FieldIssue{{Field: "password", Issue: "weak"}},
		}, true
	}

	return validationResult{}, false
}

// parsePositiveInt mengurai bilangan bulat positif dari string query.
//
// Nilai yang tidak valid atau di luar batas dikembalikan sebagai default,
// bukan sebagai error. Alasannya: parameter seperti `limit` hanya
// memengaruhi jumlah baris, bukan keamanan atau kebenaran data, dan menolak
// seluruh request karena pagination yang aneh justru mengganggu. Batas
// maksimum tetap ditegakkan di sini supaya klien tidak bisa meminta seluruh
// tabel sekaligus.
func parsePositiveInt(raw string, def int) int {
	if raw == "" {
		return def
	}
	n := 0
	for _, c := range raw {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
		if n > 1_000_000 {
			return def
		}
	}
	if n <= 0 {
		return def
	}
	const maxLimit = 100
	if n > maxLimit {
		return maxLimit
	}
	return n
}
