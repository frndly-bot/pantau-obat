package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/auth"
	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	"github.com/frndly-bot/pantau-obat/backend/internal/service"
	pohttp "github.com/frndly-bot/pantau-obat/backend/internal/transport/http"
)

// Auth adalah handler untuk endpoint /auth/*.
type Auth struct {
	Svc          *service.Auth
	Logger       *slog.Logger
	IsProduction bool
}

// NewAuth membuat handler autentikasi.
func NewAuth(svc *service.Auth, logger *slog.Logger, isProduction bool) *Auth {
	return &Auth{Svc: svc, Logger: logger, IsProduction: isProduction}
}

// Routes mendaftarkan seluruh endpoint autentikasi.
//
// Perhatikan: /register, /login, dan /refresh TIDAK memakai RequireAuth,
// karena justru merekalah yang menghasilkan token. Ketiganya dilindungi
// rate limit di layer router.
func (h *Auth) Routes(r chi.Router) {
	r.Post("/auth/register", h.Register)
	r.Post("/auth/login", h.Login)
	r.Post("/auth/refresh", h.Refresh)
	r.Post("/auth/logout", h.Logout)
	r.Get("/auth/me", h.Me)
	r.Post("/auth/change-password", h.ChangePassword)
}

// registerRequest adalah body POST /auth/register.
//
// Didefinisikan di transport, bukan domain, karena ini bentuk kabel (wire
// format). domain.RegisterInput adalah bentuk internal; pemisahan ini
// membuat perubahan JSON tidak otomatis mengubah tipe domain, dan
// sebaliknya.
type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
	Timezone string `json:"timezone"`
	Phone    string `json:"phone"`
}

// Register menangani POST /api/v1/auth/register.
func (h *Auth) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeAuthBody(w, r, &req); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	// Perhatikan: TIDAK ADA field role yang dibaca di sini. Peran selalu
	// 'user' di database. Tidak ada jalur bagi klien untuk mengubahnya.
	res, err := h.Svc.Register(r.Context(), domain.RegisterInput{
		Email:    req.Email,
		Password: req.Password,
		FullName: req.FullName,
		Timezone: req.Timezone,
		Phone:    req.Phone,
	}, h.meta(r))

	if err != nil {
		h.writeAuthError(w, r, err)
		return
	}

	h.setRefreshCookie(w, res.RefreshToken)
	pohttp.WriteJSON(w, r, http.StatusCreated, authResponse(res, false))
}

// loginRequest adalah body POST /auth/login.
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login menangani POST /api/v1/auth/login.
func (h *Auth) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeAuthBody(w, r, &req); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	res, err := h.Svc.Login(r.Context(), domain.LoginInput{
		Email:    req.Email,
		Password: req.Password,
	}, h.meta(r))
	if err != nil {
		h.writeAuthError(w, r, err)
		return
	}

	h.setRefreshCookie(w, res.RefreshToken)
	pohttp.WriteJSON(w, r, http.StatusOK, authResponse(res, true))
}

// Refresh menangani POST /api/v1/auth/refresh.
//
// Refresh token dibaca dari cookie, BUKAN dari body. Alasannya: cookie
// HttpOnly tidak bisa dibaca JavaScript, sehingga XSS tidak bisa mencuri
// refresh token. Kalau token dikirim lewat body, ia harus disimpan di
// tempat yang bisa dibaca JavaScript, dan perlindungan itu hilang.
func (h *Auth) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil || cookie.Value == "" {
		pohttp.WriteError(w, r, http.StatusUnauthorized, pohttp.CodeUnauthenticated,
			"sesi tidak valid, silakan masuk lagi", nil)
		return
	}

	res, err := h.Svc.Refresh(r.Context(), cookie.Value, h.meta(r))
	if err != nil {
		// Token yang ditolak harus langsung dibuang dari browser, supaya
		// klien tidak mencoba lagi dengan token yang sudah mati.
		h.clearRefreshCookie(w)
		h.writeAuthError(w, r, err)
		return
	}

	h.setRefreshCookie(w, res.RefreshToken)
	pohttp.WriteJSON(w, r, http.StatusOK, authResponse(res, false))
}

// logoutRequest adalah body POST /auth/logout.
type logoutRequest struct {
	AllDevices bool `json:"all_devices"`
}

// Logout menangani POST /api/v1/auth/logout.
//
// Mencabut token di SERVER, bukan hanya menghapus cookie di browser.
// Menghapus cookie saja tidak menghentikan token yang sudah tersalin.
func (h *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		pohttp.WriteError(w, r, http.StatusUnauthorized, pohttp.CodeUnauthenticated,
			"sesi tidak valid, silakan masuk lagi", nil)
		return
	}

	var req logoutRequest
	// Body boleh kosong: logout tanpa body berarti hanya sesi ini.
	if r.ContentLength > 0 {
		if err := decodeAuthBody(w, r, &req); err != nil {
			writeAuthDecodeError(w, r, err)
			return
		}
	}

	if err := h.Svc.Logout(r.Context(), id.UserID, id.SessionID, req.AllDevices, h.meta(r)); err != nil {
		h.writeAuthError(w, r, err)
		return
	}

	h.clearRefreshCookie(w)
	pohttp.WriteJSON(w, r, http.StatusOK, map[string]any{"logged_out": true})
}

// Me menangani GET /api/v1/auth/me.
func (h *Auth) Me(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		pohttp.WriteError(w, r, http.StatusUnauthorized, pohttp.CodeUnauthenticated,
			"sesi tidak valid, silakan masuk lagi", nil)
		return
	}

	user, err := h.Svc.Me(r.Context(), id.UserID)
	if err != nil {
		h.writeAuthError(w, r, err)
		return
	}
	pohttp.WriteJSON(w, r, http.StatusOK, userResponse(user))
}

// changePasswordRequest adalah body POST /auth/change-password.
type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// ChangePassword menangani POST /api/v1/auth/change-password.
func (h *Auth) ChangePassword(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		pohttp.WriteError(w, r, http.StatusUnauthorized, pohttp.CodeUnauthenticated,
			"sesi tidak valid, silakan masuk lagi", nil)
		return
	}

	var req changePasswordRequest
	if err := decodeAuthBody(w, r, &req); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	err := h.Svc.ChangePassword(r.Context(), id.UserID, id.SessionID, domain.ChangePasswordInput{
		CurrentPassword: req.CurrentPassword,
		NewPassword:     req.NewPassword,
	}, h.meta(r))
	if err != nil {
		h.writeAuthError(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, map[string]any{
		"changed":                true,
		"other_sessions_revoked": true,
	})
}

// -----------------------------------------------------------------------------
// Helper
// -----------------------------------------------------------------------------

// meta menyusun metadata request untuk audit.
//
// User-Agent dipotong sebelum disimpan; tanpa batas, satu klien bisa
// menyuntikkan string sepanjang apa pun ke kolom audit.
func (h *Auth) meta(r *http.Request) service.Meta {
	return service.Meta{
		IP:        pohttp.ClientIP(r),
		UserAgent: service.NormalizeUserAgent(r.UserAgent()),
	}
}

// setRefreshCookie memasang cookie refresh.
//
// Atributnya penting dan masing-masing punya alasan:
//   - HttpOnly : JavaScript tidak bisa membacanya, sehingga XSS tidak bisa
//     mencuri refresh token.
//   - Secure   : hanya terkirim lewat HTTPS. Dimatikan di development,
//     karena localhost bukan HTTPS dan cookie Secure akan diabaikan.
//   - SameSite=Strict : tidak terkirim pada permintaan lintas situs,
//     sehingga serangan CSRF tidak bisa memakai cookie ini.
//   - Path     : dibatasi ke /api/v1/auth, supaya permintaan ke endpoint
//     lain tidak ikut membawa refresh token.
func (h *Auth) setRefreshCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    token,
		Path:     auth.SessionCookiePath,
		MaxAge:   int((30 * 24 * time.Hour).Seconds()),
		HttpOnly: true,
		Secure:   h.IsProduction,
		SameSite: http.SameSiteStrictMode,
	})
}

// clearRefreshCookie menghapus cookie refresh di browser.
func (h *Auth) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    "",
		Path:     auth.SessionCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.IsProduction,
		SameSite: http.SameSiteStrictMode,
	})
}

// writeAuthError memetakan error service ke balasan HTTP.
//
// Validasi dan password lemah mendapat penanganan khusus karena punya tipe
// error sendiri, supaya field yang bermasalah bisa disebutkan.
func (h *Auth) writeAuthError(w http.ResponseWriter, r *http.Request, err error) {
	var weak *domain.WeakPasswordError
	if errors.As(err, &weak) {
		pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeValidationFailed,
			weak.Reason, []pohttp.FieldIssue{{Field: "password", Issue: "weak"}})
		return
	}
	var emailErr *domain.EmailError
	if errors.As(err, &emailErr) {
		pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeValidationFailed,
			emailErr.Reason, []pohttp.FieldIssue{{Field: "email", Issue: "invalid"}})
		return
	}
	var nameErr *domain.NameError
	if errors.As(err, &nameErr) {
		pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeValidationFailed,
			nameErr.Reason, []pohttp.FieldIssue{{Field: "full_name", Issue: "invalid"}})
		return
	}
	var tzErr *domain.TimezoneError
	if errors.As(err, &tzErr) {
		pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeValidationFailed,
			tzErr.Reason, []pohttp.FieldIssue{{Field: "timezone", Issue: "invalid"}})
		return
	}
	var phoneErr *domain.PhoneError
	if errors.As(err, &phoneErr) {
		pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeValidationFailed,
			phoneErr.Reason, []pohttp.FieldIssue{{Field: "phone", Issue: "invalid"}})
		return
	}

	if errors.Is(err, domain.ErrAccountInactive) {
		pohttp.WriteError(w, r, http.StatusForbidden, pohttp.CodeForbidden,
			"akun ini dinonaktifkan", nil)
		return
	}

	// Error yang tidak dikenali dicatat lengkap, tapi klien hanya menerima
	// pesan generik. Pesan internal tidak boleh bocor ke luar.
	if !isKnownDomainError(err) {
		h.Logger.ErrorContext(r.Context(), "kesalahan tak terduga pada autentikasi",
			slog.String("error", err.Error()))
	}

	pohttp.WriteDomainError(w, r, err)
}

// isKnownDomainError melaporkan apakah error punya pemetaan HTTP khusus.
func isKnownDomainError(err error) bool {
	for _, sentinel := range []error{
		domain.ErrNotFound, domain.ErrUnauthenticated, domain.ErrInvalidCredentials,
		domain.ErrTokenRevoked, domain.ErrForbidden, domain.ErrAdminScopeDenied,
		domain.ErrCareNotAccepted, domain.ErrValidation, domain.ErrRateLimited,
	} {
		if errors.Is(err, sentinel) {
			return true
		}
	}
	return false
}

// authResponse menyusun bentuk balasan login/registrasi.
//
// register=false menyembunyikan refresh token dari body, karena sudah
// dikirim lewat cookie HttpOnly. Mengirimnya juga ke body akan membuatnya
// bisa dibaca JavaScript, dan itu membatalkan alasan memakai cookie.
func authResponse(res *domain.AuthResult, includeUser bool) map[string]any {
	out := map[string]any{
		"access_token": res.AccessToken,
		"token_type":   "Bearer",
		"expires_in":   res.ExpiresIn,
	}
	if includeUser {
		out["user"] = userResponse(res.User)
	} else {
		out["user"] = userResponse(res.User)
	}
	return out
}

// userResponse menyusun representasi user untuk JSON.
//
// AdminScope selalu dikirim, termasuk null. Field null tetap dikirim bila
// kolomnya ada di skema, sesuai konvensi api-v2 bagian 1 butir 8.
func userResponse(u *domain.User) map[string]any {
	var scope any
	if u.AdminScope != nil {
		list := make([]string, len(u.AdminScope))
		for i, s := range u.AdminScope {
			list[i] = string(s)
		}
		scope = list
	} else {
		scope = nil
	}

	var grantedBy any
	if u.AdminGrantedBy != nil {
		grantedBy = *u.AdminGrantedBy
	}

	return map[string]any{
		"id":               u.ID,
		"email":            u.Email,
		"full_name":        u.FullName,
		"phone":            u.Phone,
		"timezone":         u.Timezone,
		"is_active":        u.IsActive,
		"role":             string(u.Role),
		"admin_scope":      scope,
		"admin_granted_by": grantedBy,
		"last_login_at":    u.LastLoginAt,
		"created_at":       u.CreatedAt.UTC().Format(time.RFC3339),
	}
}
