package http

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// uuidPathParams adalah nama parameter path yang selalu berupa UUID v7.
//
// Hanya nama-nama ini yang diperiksa. Parameter lain (mis. `token` pada
// /device-tokens/{token}, atau `id` numerik pada job notifikasi) sengaja
// tidak masuk daftar.
var uuidPathParams = map[string]bool{
	"id":      true,
	"tid":     true,
	"logId":   true,
	"ownerId": true,
}

// numericPathSegments adalah potongan pola rute di mana parameter `id`
// berupa bilangan bulat, bukan UUID.
//
// Job notifikasi memakai BIGINT IDENTITY, bukan uuidv7(). Polanya diperiksa
// dari chi RoutePattern supaya aturan ini hanya berlaku di sana, tanpa
// menebak-nebak dari nilai.
var numericPathSegments = []string{"/jobs/"}

// ValidatePathParams menolak parameter path yang jelas tidak sah SEBELUM
// nilainya sampai ke database.
//
// Kenapa perlu: id yang bukan UUID diteruskan apa adanya ke query, dan
// PostgreSQL menolaknya dengan SQLSTATE 22P02. Tanpa penjagaan ini, klien
// menerima 500 INTERNAL — status yang menyalahkan server untuk kesalahan
// format yang dilakukan klien, dan menyembunyikan penyebabnya di log.
// Terbukti terjadi: GET /api/v1/medications/not-a-uuid dan
// GET /api/v1/catalog/not-a-uuid sama-sama membalas 500.
//
// Middleware ini dipasang per kelompok rute setelah route cocok, sehingga
// URLParams sudah terisi.
func ValidatePathParams() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rc := chi.RouteContext(r.Context())
			if rc == nil {
				next.ServeHTTP(w, r)
				return
			}

			for i, name := range rc.URLParams.Keys {
				value := rc.URLParams.Values[i]
				if value == "" {
					continue
				}

				if isNumericParam(rc.RoutePattern(), name) {
					if !isAllDigits(value) {
						rejectParam(w, r, name, "harus berupa angka")
						return
					}
					continue
				}

				if !uuidPathParams[name] {
					continue
				}
				if _, err := uuid.Parse(value); err != nil {
					rejectParam(w, r, name, "harus berupa UUID")
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// isNumericParam melaporkan apakah parameter bernama name pada pola rute itu
// berupa angka, bukan UUID.
func isNumericParam(pattern, name string) bool {
	if name != "id" {
		return false
	}
	for _, seg := range numericPathSegments {
		if strings.Contains(pattern, seg) {
			return true
		}
	}
	return false
}

// rejectParam membalas 400 dengan field dan alasan yang jelas.
func rejectParam(w http.ResponseWriter, r *http.Request, field, reason string) {
	WriteError(w, r, http.StatusBadRequest, CodeValidationFailed,
		"parameter "+field+" "+reason,
		[]FieldIssue{{Field: field, Issue: "invalid"}})
}

// isAllDigits melaporkan apakah s hanya berisi angka 0-9 dan tidak kosong.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
