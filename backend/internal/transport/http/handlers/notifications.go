package handlers

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	"github.com/frndly-bot/pantau-obat/backend/internal/service"
	pohttp "github.com/frndly-bot/pantau-obat/backend/internal/transport/http"
)

// Notifications adalah handler untuk preferensi dan perangkat notifikasi.
type Notifications struct {
	Svc    *service.Notifications
	Logger *slog.Logger
}

// NewNotifications membuat handler notifikasi.
func NewNotifications(svc *service.Notifications, logger *slog.Logger) *Notifications {
	return &Notifications{Svc: svc, Logger: logger}
}

// Routes mendaftarkan endpoint notifikasi.
func (h *Notifications) Routes(r chi.Router) {
	r.Get("/notification-prefs", h.GetPrefs)
	r.Patch("/notification-prefs", h.UpdatePrefs)

	r.Post("/device-tokens", h.RegisterDevice)
	r.Get("/device-tokens", h.ListDevices)
	r.Delete("/device-tokens/{token}", h.UnregisterDevice)

	r.Get("/notifications", h.Inbox)
}

// updatePrefsRequest adalah body PATCH /notification-prefs.
type updatePrefsRequest struct {
	Channel        string  `json:"channel"`
	Enabled        *bool   `json:"enabled"`
	QuietStart     *string `json:"quiet_start"`
	QuietEnd       *string `json:"quiet_end"`
	AdvanceMinutes *int    `json:"advance_minutes"`
}

type registerDeviceRequest struct {
	Platform   string  `json:"platform"`
	Token      string  `json:"token"`
	AppVersion *string `json:"app_version"`
}

// GetPrefs menangani GET /api/v1/notification-prefs.
func (h *Notifications) GetPrefs(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	prefs, err := h.Svc.AllPrefs(r.Context(), id.UserID)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	data := make([]map[string]any, 0, len(prefs))
	for _, p := range prefs {
		data = append(data, prefsResponse(p))
	}
	pohttp.WriteList(w, r, data, pohttp.Page{
		NextCursor: nil,
		HasMore:    false,
		Limit:      len(data),
	})
}

// UpdatePrefs menangani PATCH /api/v1/notification-prefs.
func (h *Notifications) UpdatePrefs(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	var req updatePrefsRequest
	if err := decodeAuthBody(w, r, &req); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	channel := domain.Channel(req.Channel)
	if req.Channel == "" {
		return
	}
	if !channel.Valid() {
		writeMedicationValidation(w, r, errInvalidField("channel", "kanal tidak dikenal"))
		return
	}

	// Mulai dari nilai yang SUDAH ADA, bukan dari nol. PATCH yang hanya
	// ingin mengubah jam sunyi tidak boleh mematikan kanalnya, dan
	// sebaliknya. Nilai yang tidak dikirim dipertahankan.
	current, err := h.Svc.PrefsFor(r.Context(), id.UserID, channel)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	// VALIDASI DILAKUKAN PADA YANG DIKIRIM KLIEN, sebelum digabung dengan
	// nilai lama.
	//
	// Urutannya penting dan sempat salah: sebelumnya nilai lama diisi lebih
	// dulu, sehingga permintaan yang hanya memuat quiet_start menjadi
	// "lengkap dua sisi" begitu digabung — validasi lolos, dan quiet_end
	// diam-diam dipakai dari nilai lama. User mengira sudah mengubah jendela
	// sunyi, padahal hanya setengahnya.
	reqStartSet := req.QuietStart != nil && strings.TrimSpace(*req.QuietStart) != ""
	reqEndSet := req.QuietEnd != nil && strings.TrimSpace(*req.QuietEnd) != ""
	if reqStartSet != reqEndSet {
		writeMedicationValidation(w, r, errInvalidField("quiet_start",
			"jam sunyi harus diisi keduanya atau dikosongkan keduanya"))
		return
	}

	in := service.UpdatePrefsInput{
		Channel:        channel,
		Enabled:        current.Enabled,
		AdvanceMinutes: current.AdvanceMinutes,
	}
	if current.QuietStart != "" {
		in.QuietStart = &current.QuietStart
	}
	if current.QuietEnd != "" {
		in.QuietEnd = &current.QuietEnd
	}

	if req.Enabled != nil {
		in.Enabled = *req.Enabled
	}
	if req.AdvanceMinutes != nil {
		in.AdvanceMinutes = *req.AdvanceMinutes
	}
	if req.QuietStart != nil {
		in.QuietStart = req.QuietStart
	}
	if req.QuietEnd != nil {
		in.QuietEnd = req.QuietEnd
	}
	// Jam sunyi diisi kosong berarti diminta dihapus. Keduanya kosong
	// sekaligus, karena satu sisi saja sudah ditolak di atas.
	if (req.QuietStart != nil && *req.QuietStart == "") &&
		(req.QuietEnd != nil && *req.QuietEnd == "") {
		in.ClearQuiet = true
	}

	updated, err := h.Svc.UpdatePrefs(r.Context(), id.UserID, in, metaOf(r))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, prefsResponse(*updated))
}

// RegisterDevice menangani POST /api/v1/device-tokens.
func (h *Notifications) RegisterDevice(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	var req registerDeviceRequest
	if err := decodeAuthBody(w, r, &req); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	t, err := h.Svc.RegisterDevice(r.Context(), id.UserID, service.RegisterDeviceInput{
		Platform:   domain.DevicePlatform(req.Platform),
		Token:      req.Token,
		AppVersion: req.AppVersion,
	}, metaOf(r))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusCreated, map[string]any{
		"id":           t.ID,
		"platform":     string(t.Platform),
		"last_seen_at": t.LastSeenAt.UTC().Format(time.RFC3339),
		"created_at":   t.CreatedAt.UTC().Format(time.RFC3339),
	})
}

// ListDevices menangani GET /api/v1/device-tokens.
//
// Token TIDAK dikembalikan. Ia kredensial pengiriman: siapa pun yang
// memegangnya bisa mengirim notifikasi ke perangkat itu. Yang dibutuhkan
// klien hanya tahu perangkat mana saja yang terdaftar, dan itu cukup
// dijawab dengan id, platform, dan waktu terakhir terlihat.
func (h *Notifications) ListDevices(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	devices, err := h.Svc.ListDevices(r.Context(), id.UserID)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	data := make([]map[string]any, 0, len(devices))
	for _, d := range devices {
		data = append(data, map[string]any{
			"id":           d.ID,
			"platform":     string(d.Platform),
			"app_version":  d.AppVersion,
			"last_seen_at": d.LastSeenAt.UTC().Format(time.RFC3339),
			"created_at":   d.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	pohttp.WriteList(w, r, data, pohttp.Page{
		NextCursor: nil,
		HasMore:    false,
		Limit:      len(data),
	})
}

// UnregisterDevice menangani DELETE /api/v1/device-tokens/{token}.
func (h *Notifications) UnregisterDevice(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	// Token perangkat adalah JSON dengan karakter ( ) { } / + = %, sehingga
	// DI-ENCODE klien saat ditaruh di path URL. chi.URLParam mengembalikan
	// bentuk mentah dari URL, jadi harus di-decode dulu sebelum dicocokkan
	// dengan yang tersimpan. Tanpa ini, pencabutan selalu gagal 404.
	rawToken := chi.URLParam(r, "token")
	token, err := url.PathUnescape(rawToken)
	if err != nil {
		// Bukan escape valid: pakai apa adanya, supaya token yang memang
		// memuat '%' secara harfiah tetap bisa dicabut.
		token = rawToken
	}

	if err := h.Svc.UnregisterDevice(r.Context(), id.UserID,
		token, metaOf(r)); err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, map[string]any{"unregistered": true})
}

// Inbox menangani GET /api/v1/notifications.
func (h *Notifications) Inbox(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	limit := parsePositiveInt(r.URL.Query().Get("limit"), 30)

	items, err := h.Svc.Inbox(r.Context(), id.UserID, limit)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	data := make([]map[string]any, 0, len(items))
	for _, it := range items {
		entry := map[string]any{
			"id":         it.ID,
			"kind":       it.Kind,
			"status":     it.Status,
			"channel":    it.Channel,
			"title":      it.Title,
			"created_at": it.CreatedAt.UTC().Format(time.RFC3339),
		}
		if it.SentAt != nil {
			entry["sent_at"] = it.SentAt.UTC().Format(time.RFC3339)
		} else {
			entry["sent_at"] = nil
		}
		data = append(data, entry)
	}

	pohttp.WriteList(w, r, data, pohttp.Page{
		NextCursor: nil,
		HasMore:    len(items) == limit,
		Limit:      limit,
	})
}

// prefsResponse menyusun balasan preferensi satu kanal.
func prefsResponse(p domain.ChannelPrefs) map[string]any {
	return map[string]any{
		"channel":         string(p.Channel),
		"enabled":         p.Enabled,
		"quiet_start":     emptyToNil(p.QuietStart),
		"quiet_end":       emptyToNil(p.QuietEnd),
		"advance_minutes": p.AdvanceMinutes,
		"throttled":       p.Throttled,
	}
}

// emptyToNil mengubah string kosong menjadi null di JSON.
//
// Perbedaannya penting bagi klien: "" berarti user mengosongkan jam sunyi,
// sedangkan null berarti tidak ada jam sunyi. Mengirim "" akan membuat UI
// menampilkan kotak jam yang kosong, bukan tidak ada jam sunyi.
func emptyToNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// writeErr memetakan error service ke balasan HTTP.
func (h *Notifications) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	if resp, ok := validationResponse(err); ok {
		pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeValidationFailed,
			resp.message, resp.details)
		return
	}
	if !isKnownDomainError(err) {
		h.Logger.ErrorContext(r.Context(), "kesalahan tak terduga pada notifikasi",
			slog.String("error", err.Error()))
	}
	pohttp.WriteDomainError(w, r, err)
}
