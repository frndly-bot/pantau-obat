package http

import (
	"net/http"
)

// RequireAuth memastikan request sudah punya identitas. Dipasang pada semua
// rute yang butuh login. Middleware Auth yang mengisi identitas; middleware
// ini hanya memeriksanya.
func RequireAuth() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if IdentityFrom(r.Context()) == nil {
				WriteError(w, r, http.StatusUnauthorized, CodeUnauthenticated,
					"sesi tidak valid, silakan masuk lagi", nil)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAdmin membatasi rute pada admin dengan scope tertentu.
//
// Dipasang pada grup /admin/*. Aturan:
//   - bukan admin            -> 403 FORBIDDEN
//   - admin, scope NULL      -> lolos (akses penuh)
//   - admin, scope memuat x  -> lolos hanya untuk x
//   - admin, scope tanpa x   -> 403 ADMIN_SCOPE_DENIED
//
// ADMIN_SCOPE_DENIED dibedakan dari FORBIDDEN karena artinya berbeda bagi
// klien: FORBIDDEN berarti "kamu bukan admin", ADMIN_SCOPE_DENIED berarti
// "kamu admin, tapi bukan bidang ini". UI menampilkan pesan yang berbeda.
//
// scope kosong ("") berarti cukup admin, tanpa syarat scope. Hanya dipakai
// untuk rute yang memang tidak terikat satu bidang.
func RequireAdmin(scope string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := IdentityFrom(r.Context())
			if id == nil {
				WriteError(w, r, http.StatusUnauthorized, CodeUnauthenticated,
					"sesi tidak valid, silakan masuk lagi", nil)
				return
			}
			if !id.IsAdmin() {
				WriteError(w, r, http.StatusForbidden, CodeForbidden,
					"tidak berhak mengakses data ini", nil)
				return
			}
			if scope == "" || id.HasScope(scope) {
				next.ServeHTTP(w, r)
				return
			}
			// Sertakan scope yang dibutuhkan dan yang dimiliki, supaya
			// operator bisa langsung tahu apa yang kurang.
			WriteError(w, r, http.StatusForbidden, CodeAdminScopeDenied,
				"peran admin Anda tidak mencakup bidang ini", map[string]any{
					"required_scope": scope,
					"your_scope":     id.AdminScope,
				})
		})
	}
}
