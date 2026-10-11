package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	"github.com/frndly-bot/pantau-obat/backend/internal/service"
	pohttp "github.com/frndly-bot/pantau-obat/backend/internal/transport/http"
)

// Users adalah handler untuk /users/*.
type Users struct {
	Svc    *service.Users
	Logger *slog.Logger
}

// NewUsers membuat handler profil.
func NewUsers(svc *service.Users, logger *slog.Logger) *Users {
	return &Users{Svc: svc, Logger: logger}
}

// Routes mendaftarkan endpoint profil.
//
// Semua rute di sini WAJIB sudah login; router memasang RequireAuth pada
// kelompok ini. Tidak ada endpoint yang bisa mengakses profil user lain:
// id user selalu diambil dari token, bukan dari path maupun query.
func (h *Users) Routes(r chi.Router) {
	r.Get("/users/me", h.Get)
	r.Patch("/users/me", h.Update)
	r.Delete("/users/me", h.Delete)
	r.Get("/users/me/audit-log", h.AuditLog)
}

// updateProfileRequest adalah body PATCH /users/me.
//
// Memakai pointer supaya bisa membedakan "field tidak dikirim" dari
// "field dikirim bernilai kosong". Tanpa itu, PATCH yang hanya ingin
// mengubah nama akan ikut mengosongkan telepon.
type updateProfileRequest struct {
	FullName *string `json:"full_name"`
	Phone    *string `json:"phone"`
	Timezone *string `json:"timezone"`
}

// Get menangani GET /api/v1/users/me.
func (h *Users) Get(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		pohttp.WriteError(w, r, http.StatusUnauthorized, pohttp.CodeUnauthenticated,
			"sesi tidak valid, silakan masuk lagi", nil)
		return
	}

	user, err := h.Svc.Profile(r.Context(), id.UserID)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	pohttp.WriteJSON(w, r, http.StatusOK, profileResponse(user))
}

// Update menangani PATCH /api/v1/users/me.
func (h *Users) Update(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		pohttp.WriteError(w, r, http.StatusUnauthorized, pohttp.CodeUnauthenticated,
			"sesi tidak valid, silakan masuk lagi", nil)
		return
	}

	var req updateProfileRequest
	if err := decodeAuthBody(w, r, &req); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	user, err := h.Svc.UpdateProfile(r.Context(), id.UserID, service.ProfileUpdate{
		FullName: req.FullName,
		Phone:    req.Phone,
		Timezone: req.Timezone,
	}, metaOf(r))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	pohttp.WriteJSON(w, r, http.StatusOK, profileResponse(user))
}

// Delete menangani DELETE /api/v1/users/me.
//
// Menghapus akun berarti menganonimkan identitas DAN mencabut seluruh akses,
// termasuk access token yang sedang dipakai request ini. Karena itu balasan
// sukses dikirim sebelum pencabutan berlaku penuh; setelah ini token yang
// sama tidak akan diterima lagi.
func (h *Users) Delete(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		pohttp.WriteError(w, r, http.StatusUnauthorized, pohttp.CodeUnauthenticated,
			"sesi tidak valid, silakan masuk lagi", nil)
		return
	}

	if err := h.Svc.DeleteProfile(r.Context(), id.UserID, metaOf(r)); err != nil {
		h.writeErr(w, r, err)
		return
	}

	// Bersihkan cookie refresh di browser supaya klien tidak mencoba
	// memperbarui sesi dengan token yang sudah mati.
	http.SetCookie(w, &http.Cookie{
		Name:     "po_refresh",
		Value:    "",
		Path:     "/api/v1/auth",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	pohttp.WriteJSON(w, r, http.StatusOK, map[string]any{
		"deleted":          true,
		"anonymized":       true,
		"access_revoked":   true,
		"logs_preserved":   true,
		"purge_after_days": 30,
	})
}

// AuditLog menangani GET /api/v1/users/me/audit-log.
func (h *Users) AuditLog(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		pohttp.WriteError(w, r, http.StatusUnauthorized, pohttp.CodeUnauthenticated,
			"sesi tidak valid, silakan masuk lagi", nil)
		return
	}

	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		for _, c := range raw {
			if c < '0' || c > '9' {
				pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeValidationFailed,
					"limit harus berupa angka", []pohttp.FieldIssue{{Field: "limit", Issue: "invalid"}})
				return
			}
		}
		for _, c := range raw {
			limit = limit*10 + int(c-'0')
			if limit > 1000 {
				break
			}
		}
	}

	items, err := h.Svc.AuditLog(r.Context(), id.UserID, limit)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	data := make([]map[string]any, 0, len(items))
	for _, it := range items {
		entry := map[string]any{
			"id":            it.ID,
			"actor_id":      it.ActorID,
			"actor_role":    it.ActorRole,
			"action":        it.Action,
			"resource_type": it.ResourceType,
			"resource_id":   it.ResourceID,
			"created_at":    it.CreatedAt.UTC().Format(time.RFC3339),
		}
		// IP hanya ditampilkan bila ada. Alamat kosong lebih baik tidak
		// dikirim daripada dikirim sebagai string kosong.
		if it.IP != "" {
			entry["ip"] = it.IP
		}
		data = append(data, entry)
	}

	pohttp.WriteList(w, r, data, pohttp.Page{
		NextCursor: nil,
		HasMore:    false,
		Limit:      len(data),
	})
}

// writeErr memetakan error service ke balasan HTTP.
func (h *Users) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	if !isKnownDomainError(err) {
		h.Logger.ErrorContext(r.Context(), "kesalahan tak terduga pada profil",
			slog.String("error", err.Error()))
	}
	// Tipe error validasi spesifik punya penanganan sendiri supaya field
	// yang bermasalah bisa disebutkan.
	if resp, ok := validationResponse(err); ok {
		pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeValidationFailed,
			resp.message, resp.details)
		return
	}
	pohttp.WriteDomainError(w, r, err)
}

// profileResponse menyusun representasi profil untuk JSON.
//
// Field null tetap dikirim bila kolomnya ada di skema, sesuai konvensi
// api-v2 bagian 1 butir 8.
func profileResponse(u *domain.User) map[string]any {
	var scope any
	if u.AdminScope != nil {
		list := make([]string, len(u.AdminScope))
		for i, s := range u.AdminScope {
			list[i] = string(s)
		}
		scope = list
	}

	var lastLogin any
	if u.LastLoginAt != nil {
		lastLogin = u.LastLoginAt.UTC().Format(time.RFC3339)
	}

	return map[string]any{
		"id":            u.ID,
		"email":         u.Email,
		"full_name":     u.FullName,
		"phone":         u.Phone,
		"timezone":      u.Timezone,
		"is_active":     u.IsActive,
		"role":          string(u.Role),
		"admin_scope":   scope,
		"last_login_at": lastLogin,
		"created_at":    u.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":    u.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
