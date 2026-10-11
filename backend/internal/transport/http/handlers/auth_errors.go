package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	pohttp "github.com/frndly-bot/pantau-obat/backend/internal/transport/http"
)

// ErrForbiddenField dipakai saat klien mengirim field yang sengaja tidak
// diterima API.
var ErrForbiddenField = errors.New("field tidak diizinkan")

// forbiddenFieldError menjelaskan field mana yang ditolak dan kenapa.
type forbiddenFieldError struct {
	Field  string
	Reason string
}

func (e *forbiddenFieldError) Error() string { return e.Reason }

// fieldRule menyatakan satu field yang tidak boleh dikirim klien, beserta
// alasan yang bisa ditampilkan.
type fieldRule struct {
	Name   string
	Reason string
}

// accountForbiddenFields adalah field AKUN yang ditolak secara eksplisit.
//
// Kenapa eksplisit, bukan mengandalkan DisallowUnknownFields:
//
// Menolak semua field asing memang mencegah `role` admin diterima, tapi
// efeknya adalah efek samping, bukan keputusan. Kalau nanti seseorang
// menambahkan field Role ke struct request supaya "lengkap",
// perlindungannya hilang tanpa ada yang sadar.
//
// PENTING: daftar ini BERLAKU PER-ENDPOINT, bukan global.
//
// Alasannya nyata dan sempat jadi bug: `role` berarti DUA hal berbeda di
// aplikasi ini —
//
//   - users.role (user/admin)            -> HARUS ditolak dari API
//   - care_links.role (viewer/helper)    -> HARUS boleh; itu inti fitur
//     caregiver, tempat pasien menentukan tingkat izin orang yang
//     memantau dirinya
//
// Daftar global menolak keduanya, sehingga pasien TIDAK BISA lagi memilih
// apakah caregivernya boleh mencatat dosis atas namanya. Bug itu hanya
// ketahuan karena alur undangan benar-benar dijalankan.
//
// Jadi: pakai accountForbiddenFields untuk endpoint yang menyentuh data
// akun, dan nil untuk endpoint yang sah menerima `role` dalam arti lain.
//
// Field asing LAIN yang tidak berbahaya tetap diabaikan, supaya klien versi
// lebih baru bisa mengirim field tambahan tanpa langsung ditolak.
var accountForbiddenFields = []fieldRule{
	{
		Name: "role",
		Reason: "peran akun tidak bisa diatur lewat API. Peran admin hanya " +
			"ditetapkan lewat operasi database oleh operator.",
	},
	{
		Name: "admin_scope",
		Reason: "cakupan admin tidak bisa diatur lewat API. " +
			"Hanya operator yang menetapkannya lewat database.",
	},
	{
		Name:   "is_active",
		Reason: "status aktif akun hanya bisa diubah lewat endpoint admin.",
	},
	{
		Name:   "id",
		Reason: "id akun dibuat server dan tidak boleh ditentukan klien.",
	},
	{
		Name:   "password_hash",
		Reason: "hash password dikelola server dan tidak pernah diterima klien.",
	},
}

// decodeAuthBody membaca body JSON untuk endpoint autentikasi.
//
// Dua lapis pemeriksaan:
//  1. Field terlarang diperiksa LEBIH DULU, dari peta mentah. Urutan ini
//     penting supaya `{"role":"admin"}` dibalas dengan pesan yang
//     menjelaskan, bukan pesan validasi lain yang kebetulan muncul duluan.
//  2. Setelah itu baru di-decode ke struct, dan field asing yang bukan
//     termasuk daftar terlarang diabaikan.
func decodeAuthBody(w http.ResponseWriter, r *http.Request, dst any) error {
	return decodeBodyWithRules(w, r, dst, accountForbiddenFields)
}

// decodeBodyWithRules membaca body dengan daftar field terlarang yang
// diberikan. rules == nil berarti tidak ada field yang ditolak secara
// khusus; field asing tetap diabaikan.
//
// Dipakai endpoint yang sah menerima `role` dalam arti lain, mis. undangan
// caregiver, sehingga pemeriksaan akun tidak boleh berlaku di sana.
func decodeBodyWithRules(w http.ResponseWriter, r *http.Request, dst any, rules []fieldRule) error {
	raw, err := readLimitedBody(w, r)
	if err != nil {
		return err
	}

	// Lapis 1: periksa field terlarang pada bentuk mentah.
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return &jsonSyntaxError{err: err}
	}
	for _, rule := range rules {
		if _, present := probe[rule.Name]; present {
			return &forbiddenFieldError{Field: rule.Name, Reason: rule.Reason}
		}
	}

	// Lapis 2: decode normal. Field asing lain diabaikan dengan sengaja,
	// jadi DisallowUnknownFields TIDAK dipakai di sini.
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	if err := dec.Decode(dst); err != nil {
		return &jsonSyntaxError{err: err}
	}
	if dec.More() {
		return &jsonSyntaxError{err: errors.New("body memuat lebih dari satu objek JSON")}
	}
	return nil
}

// jsonSyntaxError menandai body yang bukan JSON yang bisa dibaca.
type jsonSyntaxError struct{ err error }

func (e *jsonSyntaxError) Error() string {
	return fmt.Sprintf("body bukan JSON yang valid: %v", e.err)
}

// writeAuthDecodeError membalas kegagalan pembacaan body.
func writeAuthDecodeError(w http.ResponseWriter, r *http.Request, err error) {
	var forbidden *forbiddenFieldError
	if errors.As(err, &forbidden) {
		pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeValidationFailed,
			forbidden.Reason,
			map[string]any{
				"field":  forbidden.Field,
				"issue":  "not_allowed",
				"detail": forbidden.Reason,
			})
		return
	}
	pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeInvalidJSON,
		"body bukan JSON yang valid", nil)
}

// hasField memeriksa apakah JSON mentah memuat field tertentu. Dipakai
// untuk membedakan "field tidak dikirim" dari "field dikirim bernilai kosong".
func hasField(raw json.RawMessage, name string) bool {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	_, ok := probe[name]
	return ok
}

// pastikan domain dipakai, supaya impornya tidak dihapus saat refactor.
var _ = domain.ErrValidation
