package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	"github.com/frndly-bot/pantau-obat/backend/internal/repo/postgres"
	"github.com/frndly-bot/pantau-obat/backend/internal/service"
	pohttp "github.com/frndly-bot/pantau-obat/backend/internal/transport/http"
)

// Admin adalah handler untuk seluruh rute /admin/*.
//
// Setiap sub-kelompok memasang RequireAdmin(scope) SENDIRI, bukan satu
// pemeriksaan di depan. Alasannya: cakupan yang berbeda untuk bidang yang
// berbeda. Admin dengan cakupan `catalog` tidak boleh menyentuh
// /admin/users, dan itu lebih jelas ditegakkan per sub-kelompok.
type Admin struct {
	Svc    *service.Admin
	Logger *slog.Logger
}

// NewAdmin membuat handler admin.
func NewAdmin(svc *service.Admin, logger *slog.Logger) *Admin {
	return &Admin{Svc: svc, Logger: logger}
}

// Routes mendaftarkan seluruh endpoint admin.
//
// Pemanggil sudah memasang RequireAuth. Di sini setiap sub-kelompok menambah
// RequireAdmin(scope)-nya sendiri.
func (h *Admin) Routes(r chi.Router) {
	r.Route("/admin", func(r chi.Router) {
		// Cakupan: users
		r.Group(func(r chi.Router) {
			r.Use(pohttp.RequireAdmin(string(domain.ScopeUsers)))
			r.Get("/users", h.ListUsers)
			r.Get("/users/{id}", h.GetUser)
			r.Post("/users/{id}/disable", h.DisableUser)
			r.Post("/users/{id}/enable", h.EnableUser)
			r.Post("/users/{id}/erase", h.EraseUser)
			r.Post("/users/{id}/grant-admin", h.GrantAdmin)
			r.Post("/users/{id}/revoke-admin", h.RevokeAdmin)
		})

		// Cakupan: catalog
		r.Group(func(r chi.Router) {
			r.Use(pohttp.RequireAdmin(string(domain.ScopeCatalog)))
			r.Get("/catalog", h.ListCatalog)
			r.Post("/catalog", h.CreatePreset)
			r.Post("/catalog/{id}/publish", h.PublishCatalog)
			r.Post("/catalog/{id}/unpublish", h.UnpublishCatalog)
			r.Post("/catalog/{id}/archive", h.ArchiveCatalog)
		})

		// Cakupan: notifications
		r.Group(func(r chi.Router) {
			r.Use(pohttp.RequireAdmin(string(domain.ScopeNotifications)))
			r.Get("/notifications/jobs", h.ListJobs)
			r.Post("/notifications/jobs/{id}/retry", h.RetryJob)
			r.Post("/notifications/jobs/{id}/cancel", h.CancelJob)
		})

		// Cakupan: reports
		r.Group(func(r chi.Router) {
			r.Use(pohttp.RequireAdmin(string(domain.ScopeReports)))
			r.Get("/stats", h.Stats)
		})
	})
}

// -----------------------------------------------------------------------------
// Bentuk kabel
// -----------------------------------------------------------------------------

type eraseUserRequest struct {
	Reason string `json:"reason"`
}

type adminGrantRequest struct {
	Scope  *[]string `json:"admin_scope"`
	Reason string    `json:"reason"`
}

type reasonRequest struct {
	Reason string `json:"reason"`
}

type createPresetRequest struct {
	DisplayName    string  `json:"display_name"`
	NamaDagang     *string `json:"nama_dagang"`
	GenerikNama    *string `json:"generik_nama"`
	DosageFormCode *string `json:"dosage_form_code"`
	RouteCode      *string `json:"route_code"`
	UcumCode       *string `json:"ucum_code"`
	UomName        *string `json:"uom_name"`
	DoseValueText  *string `json:"dose_value_text"`
}

// -----------------------------------------------------------------------------
// Akun
// -----------------------------------------------------------------------------

// ListUsers menangani GET /api/v1/admin/users.
func (h *Admin) ListUsers(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	limit := parsePositiveInt(q.Get("limit"), 50)
	offset := parsePositiveInt(q.Get("offset"), 0)
	if offset > 0 {
		offset-- // parsePositiveInt mengembalikan default 0 untuk "0"
	}

	users, total, err := h.Svc.ListUsers(r.Context(), actor, service.ListUsersInput{
		Query:  q.Get("q"),
		Role:   q.Get("role"),
		Status: q.Get("status"),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	data := make([]map[string]any, 0, len(users))
	for i := range users {
		data = append(data, adminUserResponse(&users[i]))
	}

	// Memakai WriteList, BUKAN WriteJSON.
	//
	// WriteJSON membungkus isinya menjadi {"data": <isi>}. Kalau di dalamnya
	// sudah ada kunci "data", hasilnya bersarang dua lapis: data.data, dan
	// "page" ikut terkubur di dalam. Itu bug yang benar-benar terjadi dan
	// membuat klien tidak bisa membaca daftarnya.
	//
	// WriteList mengisi "data" dan "page" di tingkat atas, sesuai envelope
	// yang sama dengan endpoint lain.
	pohttp.WriteList(w, r, data, pohttp.Page{
		NextCursor: nil,
		HasMore:    offset+len(data) < total,
		Limit:      limit,

		// Total diisi di sini karena halaman admin memang membutuhkannya
		// untuk menavigasi. Endpoint user tidak mengisinya, karena cursor
		// menelusuri tanpa menghitung dan COUNT terpisah pada tabel log
		// yang besar justru mahal.
		Total: &total,
	})
}

// GetUser menangani GET /api/v1/admin/users/{id}.
func (h *Admin) GetUser(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}

	user, err := h.Svc.GetUser(r.Context(), actor, chi.URLParam(r, "id"))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	pohttp.WriteJSON(w, r, http.StatusOK, adminUserResponse(user))
}

// DisableUser menangani POST /api/v1/admin/users/{id}/disable.
func (h *Admin) DisableUser(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}

	if err := h.Svc.SetUserActive(r.Context(), actor, chi.URLParam(r, "id"), false, metaOf(r)); err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, map[string]any{
		"is_active":      false,
		"access_revoked": true,
		"note":           "seluruh sesi dan token aktif ikut dicabut",
	})
}

// EnableUser menangani POST /api/v1/admin/users/{id}/enable.
func (h *Admin) EnableUser(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}

	if err := h.Svc.SetUserActive(r.Context(), actor, chi.URLParam(r, "id"), true, metaOf(r)); err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, map[string]any{
		"is_active": true,
		"note":      "user perlu masuk ulang untuk mendapat token baru",
	})
}

// EraseUser menangani POST /api/v1/admin/users/{id}/erase.
func (h *Admin) EraseUser(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}

	var req eraseUserRequest
	if err := decodeAuthBody(w, r, &req); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	if err := h.Svc.EraseUser(r.Context(), actor, chi.URLParam(r, "id"), req.Reason, metaOf(r)); err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, map[string]any{
		"erased":           true,
		"anonymized":       true,
		"access_revoked":   true,
		"logs_preserved":   true,
		"purge_after_days": 30,
	})
}

// GrantAdmin menangani POST /api/v1/admin/users/{id}/grant-admin.
func (h *Admin) GrantAdmin(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}

	var req adminGrantRequest
	// decodeBodyWithRules dengan daftar KOSONG, bukan decodeAuthBody.
	//
	// Ini endpoint SATU-SATUNYA yang sah menerima `admin_scope`: tugasnya
	// memang menetapkan cakupan admin, dan sudah dijaga
	// RequireAdmin(users). Memakai daftar terlarang akun di sini membuat
	// endpoint-nya menolak field yang justru jadi intinya — bug yang
	// benar-benar terjadi dan membuat pemberian peran admin mustahil
	// lewat API.
	//
	// Bahaya yang dicegah daftar akun tetap terjaga di tempat lain: user
	// tidak bisa menaikkan cakupannya sendiri lewat /auth/register maupun
	// /users/me, karena keduanya memakai decodeAuthBody.
	if err := decodeBodyWithRules(w, r, &req, nil); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	// admin_scope null berarti akses penuh, dan itu harus dibedakan dari
	// field yang tidak dikirim sama sekali.
	var scope []domain.AdminScope
	if req.Scope != nil {
		for _, s := range *req.Scope {
			scope = append(scope, domain.AdminScope(s))
		}
		// Slice kosong dari klien berarti "tanpa cakupan apa pun", bukan
		// "akses penuh". Dibedakan supaya tidak ada admin tanpa kemampuan
		// yang dikira punya akses penuh.
		if scope == nil {
			scope = []domain.AdminScope{}
		}
	}

	if err := h.Svc.GrantAdmin(r.Context(), actor, service.GrantAdminInput{
		UserID: chi.URLParam(r, "id"),
		Scope:  scope,
		Reason: req.Reason,
	}, metaOf(r)); err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, map[string]any{
		"role":             string(domain.RoleAdmin),
		"admin_scope":      scopeStringsAny(scope),
		"relogin_required": true,
	})
}

// RevokeAdmin menangani POST /api/v1/admin/users/{id}/revoke-admin.
func (h *Admin) RevokeAdmin(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}

	var req reasonRequest
	if err := decodeAuthBody(w, r, &req); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	if err := h.Svc.RevokeAdmin(r.Context(), actor, chi.URLParam(r, "id"), req.Reason, metaOf(r)); err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, map[string]any{
		"role":             string(domain.RoleUser),
		"admin_scope":      nil,
		"relogin_required": true,
	})
}

// -----------------------------------------------------------------------------
// Katalog
// -----------------------------------------------------------------------------

// ListCatalog menangani GET /api/v1/admin/catalog.
func (h *Admin) ListCatalog(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	entries, err := h.Svc.ListCatalog(r.Context(), actor, service.AdminCatalogInput{
		CurationStatus: q.Get("status"),
		Query:          q.Get("q"),
		Limit:          parsePositiveInt(q.Get("limit"), 50),
	})
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	data := make([]map[string]any, 0, len(entries))
	for i := range entries {
		data = append(data, adminCatalogResponse(&entries[i]))
	}

	pohttp.WriteList(w, r, data, pohttp.Page{
		NextCursor: nil,
		HasMore:    false,
		Limit:      len(data),
	})
}

// CreatePreset menangani POST /api/v1/admin/catalog.
func (h *Admin) CreatePreset(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}

	var req createPresetRequest
	if err := decodeAuthBody(w, r, &req); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	entry, err := h.Svc.CreatePreset(r.Context(), actor, service.CreatePresetInput{
		DisplayName:    req.DisplayName,
		NamaDagang:     req.NamaDagang,
		GenerikNama:    req.GenerikNama,
		DosageFormCode: req.DosageFormCode,
		RouteCode:      req.RouteCode,
		UcumCode:       req.UcumCode,
		UomName:        req.UomName,
		DoseValueText:  req.DoseValueText,
	}, metaOf(r))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusCreated, adminCatalogResponse(entry))
}

// PublishCatalog menangani POST /api/v1/admin/catalog/{id}/publish.
func (h *Admin) PublishCatalog(w http.ResponseWriter, r *http.Request) {
	h.setStatus(w, r, domain.CurationPublished)
}

// UnpublishCatalog menangani POST /api/v1/admin/catalog/{id}/unpublish.
func (h *Admin) UnpublishCatalog(w http.ResponseWriter, r *http.Request) {
	h.setStatus(w, r, domain.CurationDraft)
}

// ArchiveCatalog menangani POST /api/v1/admin/catalog/{id}/archive.
func (h *Admin) ArchiveCatalog(w http.ResponseWriter, r *http.Request) {
	h.setStatus(w, r, domain.CurationArchived)
}

// setStatus menangani perubahan status kurasi.
//
// Ketiganya berbagi jalur yang sama; yang membedakan hanya status tujuannya.
// Sub-jalur terpisah dipakai, bukan satu endpoint dengan field status di
// body, karena itu membuat niatnya jelas di log dan di audit.
func (h *Admin) setStatus(w http.ResponseWriter, r *http.Request, status domain.CurationStatus) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}

	var req reasonRequest
	if err := decodeAuthBody(w, r, &req); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	entry, err := h.Svc.SetCurationStatus(r.Context(), actor,
		chi.URLParam(r, "id"), status, req.Reason, metaOf(r))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, adminCatalogResponse(entry))
}

// -----------------------------------------------------------------------------
// Notifikasi
// -----------------------------------------------------------------------------

// ListJobs menangani GET /api/v1/admin/notifications/jobs.
func (h *Admin) ListJobs(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	jobs, err := h.Svc.ListJobs(r.Context(), actor,
		q.Get("status"), q.Get("kind"), parsePositiveInt(q.Get("limit"), 50))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	data := make([]map[string]any, 0, len(jobs))
	for _, j := range jobs {
		// payload TIDAK dikirim. Isinya bisa memuat detail pesan, dan
		// halaman admin tidak perlu menampilkan isi notifikasi pasien.
		entry := map[string]any{
			"id":           j.ID,
			"kind":         j.Kind,
			"status":       j.Status,
			"attempts":     j.Attempts,
			"max_attempts": j.MaxAttempts,
			"last_error":   j.LastError,
			"run_at":       j.RunAt.UTC().Format(time.RFC3339),
			"created_at":   j.CreatedAt.UTC().Format(time.RFC3339),
		}
		data = append(data, entry)
	}

	pohttp.WriteList(w, r, data, pohttp.Page{
		NextCursor: nil,
		HasMore:    false,
		Limit:      len(data),
	})
}

// RetryJob menangani POST /api/v1/admin/notifications/jobs/{id}/retry.
func (h *Admin) RetryJob(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}

	jobID, ok := h.parseJobID(w, r)
	if !ok {
		return
	}

	if err := h.Svc.RetryJob(r.Context(), actor, jobID, metaOf(r)); err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, map[string]any{
		"retried": true,
		"status":  string(domain.JobPending),
	})
}

// CancelJob menangani POST /api/v1/admin/notifications/jobs/{id}/cancel.
func (h *Admin) CancelJob(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}

	jobID, ok := h.parseJobID(w, r)
	if !ok {
		return
	}

	if err := h.Svc.CancelJob(r.Context(), actor, jobID, metaOf(r)); err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, map[string]any{
		"cancelled": true,
		"status":    string(domain.JobCancelled),
	})
}

// -----------------------------------------------------------------------------
// Agregat
// -----------------------------------------------------------------------------

// Stats menangani GET /api/v1/admin/stats.
func (h *Admin) Stats(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}

	s, err := h.Svc.Stats(r.Context(), actor)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	// Seluruh angka di sini AGGREGAT. Tidak ada nama obat, tidak ada email
	// pasien, tidak ada identitas siapa pun. Itu yang membuat halaman ini
	// aman dilihat operator tanpa melanggar privasi pengguna.
	pohttp.WriteJSON(w, r, http.StatusOK, map[string]any{
		"users_active":        s.UsersActive,
		"users_disabled":      s.UsersDisabled,
		"users_admin":         s.UsersAdmin,
		"medications_active":  s.MedicationsActive,
		"catalog_presets":     s.CatalogPresets,
		"catalog_drafts":      s.CatalogDraft,
		"jobs_failed_24h":     s.JobsFailed24h,
		"deliveries_sent_24h": s.DeliveriesSent24h,
		"logs_today":          s.LogsToday,
	})
}

// -----------------------------------------------------------------------------
// Helper
// -----------------------------------------------------------------------------

// actor menyusun identitas admin dari context.
func (h *Admin) actor(w http.ResponseWriter, r *http.Request) (service.AdminActor, bool) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return service.AdminActor{}, false
	}
	return service.AdminActor{
		UserID: id.UserID,
		Scope:  id.AdminScope,
	}, true
}

// parseJobID mengurai id job dari path.
func (h *Admin) parseJobID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := chi.URLParam(r, "id")
	if raw == "" {
		writeMedicationValidation(w, r, errInvalidField("id", "id job wajib diisi"))
		return 0, false
	}
	var n int64
	for _, c := range raw {
		if c < '0' || c > '9' {
			writeMedicationValidation(w, r, errInvalidField("id", "id job harus berupa angka"))
			return 0, false
		}
		n = n*10 + int64(c-'0')
		if n > 1<<62 {
			writeMedicationValidation(w, r, errInvalidField("id", "id job di luar rentang"))
			return 0, false
		}
	}
	return n, true
}

// adminUserResponse menyusun balasan akun untuk admin.
//
// password_hash TIDAK pernah ikut. Hash hanya dibaca jalur login, dan tidak
// punya alasan untuk muncul di halaman admin.
func adminUserResponse(u *postgres.AdminUser) map[string]any {
	var scope any
	if u.AdminScope != nil {
		list := make([]string, len(u.AdminScope))
		for i, s := range u.AdminScope {
			list[i] = string(s)
		}
		scope = list
	}

	out := map[string]any{
		"id":          u.ID,
		"email":       u.Email,
		"full_name":   u.FullName,
		"phone":       u.Phone,
		"timezone":    u.Timezone,
		"is_active":   u.IsActive,
		"role":        string(u.Role),
		"admin_scope": scope,
		"created_at":  u.CreatedAt.UTC().Format(time.RFC3339),

		// Jumlah obat, BUKAN daftarnya. Admin perlu tahu akun ini aktif
		// dipakai, tanpa perlu tahu obat apa yang diminum seseorang.
		"medication_count": u.MedicationCount,
	}
	if u.LastLoginAt != nil {
		out["last_login_at"] = u.LastLoginAt.UTC().Format(time.RFC3339)
	} else {
		out["last_login_at"] = nil
	}
	return out
}

// adminCatalogResponse menyusun balasan katalog untuk admin.
func adminCatalogResponse(e *postgres.CatalogEntry) map[string]any {
	return map[string]any{
		"id":               e.ID,
		"display_name":     e.DisplayName,
		"nama_dagang":      e.NamaDagang,
		"generik_nama":     e.GenerikNama,
		"is_preset":        e.IsPreset,
		"curation_status":  string(e.CurationStatus),
		"source":           string(e.Source),
		"dosage_form_code": e.DosageFormCode,
		"dosage_form_name": e.DosageFormName,
		"route_code":       e.RouteCode,
		"route_name":       e.RouteName,
		"dose_value_text":  e.DoseValueText,
		"ucum_code":        e.UcumCode,
		"uom_name":         e.UomName,
		"kfa_code":         e.KFACode,
		"bpom_nie":         e.BpomNIE,
		"curated_by":       e.CuratedBy,
		"curated_at":       formatTimePtr(e.CuratedAt),
		"created_at":       e.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":       e.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// scopeStringsAny mengubah cakupan menjadi bentuk JSON.
func scopeStringsAny(scopes []domain.AdminScope) any {
	if scopes == nil {
		return nil
	}
	out := make([]string, len(scopes))
	for i, s := range scopes {
		out[i] = string(s)
	}
	return out
}

// writeErr memetakan error service ke balasan HTTP.
func (h *Admin) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	if resp, ok := validationResponse(err); ok {
		pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeValidationFailed,
			resp.message, resp.details)
		return
	}
	if !isKnownDomainError(err) {
		h.Logger.ErrorContext(r.Context(), "kesalahan tak terduga pada admin",
			slog.String("error", err.Error()))
	}
	pohttp.WriteDomainError(w, r, err)
}
