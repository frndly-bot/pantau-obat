// Package http berisi lapisan transport: router, middleware, dan handler.
//
// ATURAN IMPORT: paket ini boleh mengimpor `service` dan `domain`,
// tapi tidak pernah `repo` langsung. Semua akses data lewat service.
package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// Envelope adalah bentuk balasan tunggal untuk SELURUH endpoint.
//
// Ini menggantikan bentuk ganda di dokumen lama ({ok,data,meta} vs
// {data,page}). Satu bentuk berarti klien hanya perlu satu jalur baca.
type Envelope struct {
	Data  any          `json:"data,omitempty"`
	Page  *Page        `json:"page,omitempty"`
	Error *ErrorDetail `json:"error,omitempty"`
}

// Page memuat informasi pagination.
type Page struct {
	NextCursor *string `json:"next_cursor"`
	HasMore    bool    `json:"has_more"`
	Limit      int     `json:"limit"`

	// Total adalah jumlah SELURUH baris yang cocok, dan hanya dikirim bila
	// memang dihitung.
	//
	// Endpoint user memakai cursor dan tidak punya konsep total: cursor
	// menelusuri tanpa menghitung, dan COUNT terpisah pada tabel log yang
	// besar justru mahal. Field ini nil di sana, sehingga tidak dikirim.
	//
	// Endpoint admin MEMBUTUHKAN total, karena operator menavigasi halaman
	// dan perlu tahu seberapa banyak yang ada. Di sana nilainya diisi.
	Total *int `json:"total,omitempty"`
}

// ErrorDetail adalah badan error. `code` untuk mesin, `message` aman
// ditampilkan ke user, `details` opsional untuk kesalahan validasi.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// Kode error API. Nilai ini STABIL; klien boleh bergantung padanya.
// Daftar lengkap dan kapan munculnya ada di docs/design-v2/api-v2.md bagian 1.
const (
	CodeValidationFailed   = "VALIDATION_FAILED"
	CodeInvalidJSON        = "INVALID_JSON"
	CodeUnauthenticated    = "UNAUTHENTICATED"
	CodeInvalidCredentials = "INVALID_CREDENTIALS"
	CodeTokenRevoked       = "TOKEN_REVOKED"
	CodeForbidden          = "FORBIDDEN"
	CodeAdminScopeDenied   = "ADMIN_SCOPE_DENIED"
	CodeCareNotAccepted    = "CARE_NOT_ACCEPTED"
	CodeNotFound           = "NOT_FOUND"
	CodeDuplicateMed       = "DUPLICATE_MEDICATION"
	CodeLogOccurrenceTaken = "LOG_OCCURRENCE_TAKEN"
	CodeDuplicateTimes     = "DUPLICATE_TIMES"
	CodeInvitePending      = "INVITE_PENDING"
	CodeStockInsufficient  = "STOCK_INSUFFICIENT"
	CodeCatalogImmutable   = "CATALOG_IMMUTABLE"
	CodeCatalogDuplicate   = "CATALOG_DUPLICATE_NAME"
	CodeRateLimited        = "RATE_LIMITED"
	// CodeSnoozeLimit dipisah dari RATE_LIMITED: yang satu soal kuota
	// permintaan, yang lain soal aturan penundaan dosis. Klien menampilkan
	// pesan berbeda untuk keduanya.
	CodeSnoozeLimit = "SNOOZE_LIMIT"
	// CodeNotAvailable: fitur opsional yang belum dikonfigurasi operator
	// (mis. penyimpanan foto). Berbeda dari INTERNAL karena tidak ada yang
	// rusak — hanya belum dinyalakan — dan berbeda dari VALIDATION_FAILED
	// karena pengguna tidak bisa memperbaikinya sendiri.
	CodeNotAvailable = "NOT_AVAILABLE"
	CodeInternal     = "INTERNAL"
)

// FieldIssue menjelaskan satu masalah pada satu field.
type FieldIssue struct {
	Field string `json:"field"`
	Issue string `json:"issue"`
}

// WriteJSON menulis balasan sukses berisi data.
func WriteJSON(w http.ResponseWriter, r *http.Request, status int, data any) {
	writeEnvelope(w, r, status, Envelope{Data: data})
}

// WriteList menulis balasan sukses berisi daftar beserta informasi halaman.
func WriteList(w http.ResponseWriter, r *http.Request, data any, page Page) {
	writeEnvelope(w, r, statusOK(r), Envelope{Data: data, Page: &page})
}

// WriteError menulis balasan error memakai kode dan pesan eksplisit.
func WriteError(w http.ResponseWriter, r *http.Request, status int, code, message string, details any) {
	writeEnvelope(w, r, status, Envelope{Error: &ErrorDetail{
		Code: code, Message: message, Details: details,
	}})
}

// WriteDomainError menerjemahkan sentinel error dari domain menjadi
// status HTTP + kode error. Satu tempat pemetaan, supaya handler tidak
// perlu menulis switch yang sama berulang kali.
//
// Error yang tidak dikenal diperlakukan sebagai INTERNAL dan TIDAK
// membocorkan pesan aslinya ke klien; detailnya masuk log.
func WriteDomainError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, msg := MapDomainError(err)
	writeEnvelope(w, r, status, Envelope{Error: &ErrorDetail{Code: code, Message: msg}})
}

// MapDomainError memetakan sentinel domain ke pasangan status dan kode.
//
// Untuk ErrValidation DAN ErrForbidden, pesan spesifik dari error DITERUSKAN
// bila ada. Alasannya: keduanya dipakai dengan pembungkusan seperti
// `fmt.Errorf("%w: password lama salah", domain.ErrValidation)`, dan pesan
// setelah titik dua itulah yang berguna bagi user. Untuk ErrForbidden,
// pesan itu menjelaskan PERAN yang dibutuhkan ("mengelola obat pasien hanya
// untuk peran manager") — menggantinya dengan kalimat generik membuat user
// tidak tahu harus apa.
//
// Ini AMAN dilakukan untuk keduanya karena pesannya selalu ditulis sendiri
// oleh kode aplikasi. Error lain tetap memakai pesan tetap, supaya detail
// internal (SQL, nama tabel, path berkas) tidak pernah bocor ke klien.
func MapDomainError(err error) (status int, code, message string) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, CodeNotFound, "data tidak ditemukan"
	case errors.Is(err, domain.ErrUnauthenticated):
		return http.StatusUnauthorized, CodeUnauthenticated, "sesi tidak valid, silakan masuk lagi"
	case errors.Is(err, domain.ErrInvalidCredentials):
		return http.StatusUnauthorized, CodeInvalidCredentials, "email atau password salah"
	case errors.Is(err, domain.ErrTokenRevoked):
		return http.StatusUnauthorized, CodeTokenRevoked, "sesi sudah dicabut, silakan masuk lagi"
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden, CodeForbidden, forbiddenMessage(err)
	case errors.Is(err, domain.ErrAdminScopeDenied):
		return http.StatusForbidden, CodeAdminScopeDenied, "peran admin Anda tidak mencakup bidang ini"
	case errors.Is(err, domain.ErrCareNotAccepted):
		return http.StatusForbidden, CodeCareNotAccepted, "relasi caregiver belum aktif"
	case errors.Is(err, domain.ErrValidation):
		return http.StatusBadRequest, CodeValidationFailed, validationMessage(err)
	case errors.Is(err, domain.ErrDuplicateMedication):
		return http.StatusConflict, CodeDuplicateMed, "obat dengan nama dan dosis itu sudah ada"
	case errors.Is(err, domain.ErrLogOccurrenceTaken):
		return http.StatusConflict, CodeLogOccurrenceTaken, "dosis pada jam itu sudah tercatat"
	case errors.Is(err, domain.ErrDuplicateTimes):
		return http.StatusConflict, CodeDuplicateTimes, "jam untuk pola hari itu sudah ada"
	case errors.Is(err, domain.ErrInvitePending):
		return http.StatusConflict, CodeInvitePending, "undangan untuk email itu masih menunggu"
	case errors.Is(err, domain.ErrStockInsufficient):
		return http.StatusConflict, CodeStockInsufficient, "stok tidak cukup untuk mencatat dosis ini"
	case errors.Is(err, domain.ErrCatalogImmutable):
		return http.StatusConflict, CodeCatalogImmutable, "preset katalog hanya bisa diubah admin"
	case errors.Is(err, domain.ErrCatalogDuplicateName):
		return http.StatusConflict, CodeCatalogDuplicate, "nama katalog itu sudah dipakai"
	case errors.Is(err, domain.ErrRateLimited):
		return http.StatusTooManyRequests, CodeRateLimited, "terlalu banyak permintaan"
	case errors.Is(err, domain.ErrSnoozeLimit):
		// 429, bukan 400: permintaannya sah, hanya ditolak karena batas
		// percobaan. Klien memakai status ini untuk menjelaskan bahwa
		// dosisnya kini dianggap terlewat — bukan menyuruh memperbaiki isian.
		return http.StatusTooManyRequests, CodeSnoozeLimit,
			"dosis ini sudah ditunda tiga kali; sekarang dianggap terlewat"
	default:
		return http.StatusInternalServerError, CodeInternal, "terjadi kesalahan pada server"
	}
}

// forbiddenMessage mengambil pesan spesifik dari error Forbidden.
//
// Bentuk yang diharapkan: "tidak berhak mengakses objek ini: <penjelasan>".
// Yang diambil adalah bagian setelah titik dua pertama. Bila tidak ada
// penjelasan, dipakai pesan umum.
func forbiddenMessage(err error) string {
	const fallback = "tidak berhak mengakses data ini"
	msg := err.Error()
	if i := strings.Index(msg, ": "); i >= 0 && i+2 < len(msg) {
		if specific := strings.TrimSpace(msg[i+2:]); specific != "" {
			return specific
		}
	}
	return fallback
}

// validationMessage mengambil pesan spesifik dari error validasi.
//
// Bentuk yang diharapkan: "<pesan umum>: <pesan spesifik>". Yang diambil
// adalah bagian setelah titik dua TERAKHIR, karena pembungkusan bisa
// bertingkat. Bila tidak ada titik dua, pesan aslinya dipakai apa adanya,
// dan bila kosong, dipakai pesan umum.
func validationMessage(err error) string {
	const fallback = "input tidak valid"
	if err == nil {
		return fallback
	}
	msg := err.Error()
	if i := strings.LastIndex(msg, ": "); i >= 0 && i+2 < len(msg) {
		specific := strings.TrimSpace(msg[i+2:])
		if specific != "" {
			return specific
		}
	}
	return fallback
}

// WriteInternal mencatat error asli ke log lalu membalas pesan generik.
// Pesan asli TIDAK dikirim ke klien supaya tidak membocorkan detail internal.
func WriteInternal(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	if logger != nil {
		logger.ErrorContext(r.Context(), "kesalahan internal",
			slog.String("error", err.Error()),
			slog.String("path", r.URL.Path),
		)
	}
	writeEnvelope(w, r, http.StatusInternalServerError, Envelope{Error: &ErrorDetail{
		Code: CodeInternal, Message: "terjadi kesalahan pada server",
	}})
}

// DecodeJSON membaca body JSON dengan tiga penjagaan penting:
//  1. body dibatasi ukurannya supaya tidak bisa dijadikan vektor memori,
//  2. field yang tidak dikenal DITOLAK, supaya salah tulis tidak diam-diam diabaikan,
//  3. body hanya boleh berisi satu objek JSON, bukan dua yang ditempel.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	const maxBody = 1 << 20 // 1 MiB
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return err
	}
	// Pastikan tidak ada isi tambahan setelah objek pertama.
	if dec.More() {
		return errors.New("body hanya boleh berisi satu objek JSON")
	}
	return nil
}

func writeEnvelope(w http.ResponseWriter, r *http.Request, status int, env Envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if id := RequestIDFrom(r.Context()); id != "" {
		w.Header().Set("X-Request-Id", id)
	}
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(env); err != nil {
		// Header sudah terkirim; tidak ada yang bisa dilakukan selain mencatat.
		slog.Default().Error("gagal menulis balasan JSON", "error", err)
	}
}

// statusOK mengembalikan 200. Disediakan sebagai fungsi supaya pemanggil
// tidak menulis angka telanjang saat butuh status dinamis.
func statusOK(*http.Request) int { return http.StatusOK }
