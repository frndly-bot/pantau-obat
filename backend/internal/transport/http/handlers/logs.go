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

// Logs adalah handler untuk /logs/* dan /medications/{id}/logs.
type Logs struct {
	Svc    *service.Logs
	Users  *service.Users
	Logger *slog.Logger
}

// NewLogs membuat handler pencatatan dosis.
//
// Users dipakai untuk membaca zona waktu pemilik, karena "hari ini"
// bergantung padanya.
func NewLogs(svc *service.Logs, users *service.Users, logger *slog.Logger) *Logs {
	return &Logs{Svc: svc, Users: users, Logger: logger}
}

// Routes mendaftarkan endpoint pencatatan dosis.
func (h *Logs) Routes(r chi.Router) {
	r.Get("/medications/{id}/logs", h.ListByMedication)
	r.Post("/medications/{id}/logs", h.Record)
	r.Post("/medications/{id}/logs/prn", h.RecordPRN)

	// from-reminder: mencatat dosis langsung dari notifikasi. Bentuknya sama
	// dengan Record, tapi medication_id datang dari BODY, bukan path.
	//
	// Kenapa endpoint terpisah: notifikasi hanya membawa schedule_id dan
	// tanggal, dan klien yang mengklik notifikasi belum tentu punya
	// medication_id di tangan. Menaruhnya di body membuat satu request cukup,
	// tanpa perlu mencari obat dulu. Sesuai api-v2 bagian 7.
	r.Post("/logs/from-reminder", h.RecordFromReminder)

	r.Get("/logs", h.List)
	r.Get("/logs/today", h.Today)
	r.Get("/logs/export", h.Export)
	r.Patch("/logs/{logId}", h.Correct)
	r.Post("/logs/{logId}/snooze", h.Snooze)
}

// -----------------------------------------------------------------------------
// Bentuk kabel
// -----------------------------------------------------------------------------

type recordLogRequest struct {
	ScheduleID      string  `json:"schedule_id"`
	SlotDate        string  `json:"slot_date"`
	ScheduledHour   *int    `json:"scheduled_hour"`
	ScheduledMinute *int    `json:"scheduled_minute"`
	ScheduledTZ     string  `json:"scheduled_tz"`
	Status          string  `json:"status"`
	TakenAt         *string `json:"taken_at"`
	Note            *string `json:"note"`
}

type prnLogRequest struct {
	SlotDate string  `json:"slot_date"`
	Status   string  `json:"status"`
	TakenAt  *string `json:"taken_at"`
	Note     *string `json:"note"`
}

// reminderLogRequest adalah body POST /logs/from-reminder.
//
// Berbeda dari recordLogRequest hanya pada satu hal: medication_id ikut di
// body. Sisanya sengaja sama persis supaya klien notifikasi bisa memakai
// bentuk payload yang sama dengan pencatatan biasa.
type reminderLogRequest struct {
	MedicationID    string  `json:"medication_id"`
	ScheduleID      string  `json:"schedule_id"`
	SlotDate        string  `json:"slot_date"`
	ScheduledHour   *int    `json:"scheduled_hour"`
	ScheduledMinute *int    `json:"scheduled_minute"`
	ScheduledTZ     string  `json:"scheduled_tz"`
	Status          string  `json:"status"`
	TakenAt         *string `json:"taken_at"`
	Note            *string `json:"note"`
}

type correctLogRequest struct {
	Status  string  `json:"status"`
	TakenAt *string `json:"taken_at"`
	Note    *string `json:"note"`
}

type snoozeRequest struct {
	Minutes int `json:"minutes"`
}

// Record menangani POST /api/v1/medications/{id}/logs.
func (h *Logs) Record(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	var req recordLogRequest
	if err := decodeAuthBody(w, r, &req); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	in, err := toRecordInput(req)
	if err != nil {
		writeMedicationValidation(w, r, err)
		return
	}

	res, err := h.Svc.Record(r.Context(), id.UserID, chi.URLParam(r, "id"), in, metaOf(r))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusCreated, recordResultResponse(res))
}

// RecordFromReminder menangani POST /api/v1/logs/from-reminder.
//
// Sama seperti Record, tapi medication_id diambil dari BODY. Dipakai saat
// user menekan "sudah diminum" pada notifikasi: notifikasi hanya membawa
// schedule_id dan tanggal, dan memaksa klien mencari medication_id lebih
// dulu akan menambah satu request yang tidak perlu.
func (h *Logs) RecordFromReminder(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	var req reminderLogRequest
	if err := decodeAuthBody(w, r, &req); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	if req.MedicationID == "" {
		writeMedicationValidation(w, r,
			errInvalidField("medication_id", "medication_id wajib diisi"))
		return
	}

	in, err := toRecordInput(recordLogRequest{
		ScheduleID:      req.ScheduleID,
		SlotDate:        req.SlotDate,
		ScheduledHour:   req.ScheduledHour,
		ScheduledMinute: req.ScheduledMinute,
		ScheduledTZ:     req.ScheduledTZ,
		Status:          req.Status,
		TakenAt:         req.TakenAt,
		Note:            req.Note,
	})
	if err != nil {
		writeMedicationValidation(w, r, err)
		return
	}

	res, err := h.Svc.Record(r.Context(), id.UserID, req.MedicationID, in, metaOf(r))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusCreated, recordResultResponse(res))
}

// RecordPRN menangani POST /api/v1/medications/{id}/logs/prn.
func (h *Logs) RecordPRN(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	var req prnLogRequest
	if err := decodeAuthBody(w, r, &req); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	in := service.PRNInput{
		Status: domain.LogStatus(req.Status),
		Note:   req.Note,
	}
	if req.Status != "" && !in.Status.Valid() {
		writeMedicationValidation(w, r, errInvalidField("status", "status tidak dikenal"))
		return
	}
	if req.SlotDate != "" {
		d, err := time.Parse(time.DateOnly, req.SlotDate)
		if err != nil {
			writeMedicationValidation(w, r, errInvalidField("slot_date", "tanggal harus berbentuk YYYY-MM-DD"))
			return
		}
		in.SlotDate = d
	} else {
		in.SlotDate = h.todayFor(r, id.UserID)
	}
	if req.TakenAt != nil && *req.TakenAt != "" {
		t, err := time.Parse(time.RFC3339, *req.TakenAt)
		if err != nil {
			writeMedicationValidation(w, r, errInvalidField("taken_at", "waktu harus berformat RFC3339"))
			return
		}
		in.TakenAt = &t
	}

	res, err := h.Svc.RecordPRN(r.Context(), id.UserID, chi.URLParam(r, "id"), in, metaOf(r))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusCreated, recordResultResponse(res))
}

// ListByMedication menangani GET /api/v1/medications/{id}/logs.
func (h *Logs) ListByMedication(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	f, err := h.parseListFilter(r)
	if err != nil {
		writeMedicationValidation(w, r, err)
		return
	}
	f.MedicationID = chi.URLParam(r, "id")

	logs, err := h.Svc.List(r.Context(), id.UserID, f)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	h.writeLogs(w, r, logs, f.Limit)
}

// List menangani GET /api/v1/logs.
func (h *Logs) List(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	f, err := h.parseListFilter(r)
	if err != nil {
		writeMedicationValidation(w, r, err)
		return
	}
	f.MedicationID = r.URL.Query().Get("medication_id")

	logs, err := h.Svc.List(r.Context(), id.UserID, f)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	h.writeLogs(w, r, logs, f.Limit)
}

// Today menangani GET /api/v1/logs/today.
func (h *Logs) Today(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	logs, err := h.Svc.Today(r.Context(), id.UserID, h.timezoneFor(r, id.UserID))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	h.writeLogs(w, r, logs, 200)
}

// Correct menangani PATCH /api/v1/logs/{logId}.
func (h *Logs) Correct(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	var req correctLogRequest
	if err := decodeAuthBody(w, r, &req); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	in := service.CorrectInput{
		Status: domain.LogStatus(req.Status),
		Note:   req.Note,
	}
	if req.TakenAt != nil && *req.TakenAt != "" {
		t, err := time.Parse(time.RFC3339, *req.TakenAt)
		if err != nil {
			writeMedicationValidation(w, r, errInvalidField("taken_at", "waktu harus berformat RFC3339"))
			return
		}
		in.TakenAt = &t
	}

	res, err := h.Svc.Correct(r.Context(), id.UserID, chi.URLParam(r, "logId"), in, metaOf(r))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, recordResultResponse(res))
}

// Snooze menangani POST /api/v1/logs/{logId}/snooze.
func (h *Logs) Snooze(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	var req snoozeRequest
	if err := decodeAuthBody(w, r, &req); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}
	if req.Minutes == 0 {
		req.Minutes = 10
	}

	// logId di sini dipakai sebagai schedule_id, karena snooze menggeser
	// pengingat milik JAM jadwal, bukan milik catatan dosis. Catatan dosis
	// belum ada saat snooze dilakukan.
	//
	// medication_id datang dari query dan WAJIB ada: tanpa itu, pencarian
	// obat memakai id kosong, PostgreSQL menolaknya sebagai UUID tidak sah,
	// dan klien menerima 500 — status yang menyalahkan server untuk
	// parameter yang kurang. Diperiksa di sini supaya balasannya 400.
	medicationID := r.URL.Query().Get("medication_id")
	if medicationID == "" {
		writeMedicationValidation(w, r,
			errInvalidField("medication_id", "medication_id wajib ada di query"))
		return
	}

	err := h.Svc.Snooze(r.Context(), id.UserID,
		medicationID, chi.URLParam(r, "logId"), req.Minutes, metaOf(r))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, map[string]any{
		"snoozed": true,
		"minutes": req.Minutes,
	})
}

// -----------------------------------------------------------------------------
// Helper
// -----------------------------------------------------------------------------

// parseListFilter mengurai filter dari query string.
func (h *Logs) parseListFilter(r *http.Request) (service.LogListFilter, error) {
	q := r.URL.Query()
	f := service.LogListFilter{
		Status: q.Get("status"),
		Source: q.Get("source"),
		Limit:  parsePositiveInt(q.Get("limit"), 50),
	}

	if f.Status != "" {
		s := domain.LogStatus(f.Status)
		if !s.Valid() {
			return f, errInvalidField("status", "status tidak dikenal")
		}
	}

	if f.Source != "" {
		s := domain.LogSource(f.Source)
		if !s.Valid() {
			return f, errInvalidField("source", "sumber tidak dikenal")
		}
	}

	if raw := q.Get("from"); raw != "" {
		d, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			return f, errInvalidField("from", "tanggal harus berbentuk YYYY-MM-DD")
		}
		f.From = &d
	}
	if raw := q.Get("to"); raw != "" {
		d, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			return f, errInvalidField("to", "tanggal harus berbentuk YYYY-MM-DD")
		}
		f.To = &d
	}
	if f.From != nil && f.To != nil && f.To.Before(*f.From) {
		return f, errInvalidField("to", "tanggal akhir tidak boleh sebelum tanggal awal")
	}

	return f, nil
}

// timezoneFor membaca zona waktu user.
//
// Gagal membaca zona tidak menggagalkan request: dipakai nilai bawaan.
// Menolak permintaan hanya karena zona waktu sulit dibaca akan menghentikan
// user dari mencatat dosisnya, dan itu jauh lebih merugikan.
func (h *Logs) timezoneFor(r *http.Request, userID string) string {
	u, err := h.Users.Profile(r.Context(), userID)
	if err != nil || u.Timezone == "" {
		return domain.DefaultTimezone
	}
	return u.Timezone
}

// todayFor mengembalikan tanggal hari ini di zona waktu user.
func (h *Logs) todayFor(r *http.Request, userID string) time.Time {
	tz := h.timezoneFor(r, userID)
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.FixedZone("WIB", 7*3600)
	}
	now := time.Now().In(loc)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
}

// writeLogs menulis balasan daftar catatan dosis.
func (h *Logs) writeLogs(w http.ResponseWriter, r *http.Request, logs []*domain.MedicationLog, limit int) {
	data := make([]map[string]any, 0, len(logs))
	for _, l := range logs {
		data = append(data, logResponse(l))
	}
	pohttp.WriteList(w, r, data, pohttp.Page{
		NextCursor: nil,
		HasMore:    len(logs) == limit,
		Limit:      limit,
	})
}

// toRecordInput memetakan bentuk kabel ke input service.
func toRecordInput(req recordLogRequest) (service.RecordInput, error) {
	in := service.RecordInput{
		ScheduleID:  req.ScheduleID,
		ScheduledTZ: req.ScheduledTZ,
		Status:      domain.LogStatus(req.Status),
		Note:        req.Note,
	}

	if req.Status != "" && !in.Status.Valid() {
		return in, errInvalidField("status", "status tidak dikenal")
	}
	if req.SlotDate == "" {
		return in, errInvalidField("slot_date", "tanggal dosis wajib diisi")
	}
	d, err := time.Parse(time.DateOnly, req.SlotDate)
	if err != nil {
		return in, errInvalidField("slot_date", "tanggal harus berbentuk YYYY-MM-DD")
	}
	in.SlotDate = d

	if req.ScheduledHour != nil {
		in.ScheduledHour = *req.ScheduledHour
	}
	if req.ScheduledMinute != nil {
		in.ScheduledMinute = *req.ScheduledMinute
	}

	if req.TakenAt != nil && *req.TakenAt != "" {
		t, err := time.Parse(time.RFC3339, *req.TakenAt)
		if err != nil {
			return in, errInvalidField("taken_at", "waktu harus berformat RFC3339")
		}
		in.TakenAt = &t
	}

	return in, nil
}

// logResponse menyusun balasan satu catatan dosis.
func logResponse(l *domain.MedicationLog) map[string]any {
	out := map[string]any{
		"id":            l.ID,
		"medication_id": l.MedicationID,
		"schedule_id":   l.ScheduleID,
		"owner_id":      l.OwnerID,
		"recorded_by":   l.RecordedBy,
		"slot_date":     l.SlotDate.Format(time.DateOnly),
		"status":        string(l.Status),
		"source":        string(l.Source),
		"note":          l.Note,
		"created_at":    l.CreatedAt.UTC().Format(time.RFC3339),
		"taken_at":      formatTimePtr(l.TakenAt),
	}

	if l.ScheduledHour != nil && l.ScheduledMinute != nil {
		out["scheduled_hour"] = *l.ScheduledHour
		out["scheduled_minute"] = *l.ScheduledMinute
		out["scheduled_time"] = domain.FormatClock(*l.ScheduledHour, *l.ScheduledMinute)
	} else {
		out["scheduled_hour"] = nil
		out["scheduled_minute"] = nil
		out["scheduled_time"] = nil
	}
	out["scheduled_tz"] = l.ScheduledTZ

	// Kunci dosis hanya bermakna untuk dosis terjadwal. Dikirim supaya klien
	// bisa mencocokkan catatan dengan pengingat yang diterimanya, tanpa perlu
	// menyusun ulang formatnya sendiri.
	if key := l.OccurrenceKey(); key != "" {
		out["occurrence_key"] = key
	} else {
		out["occurrence_key"] = nil
	}

	return out
}

// recordResultResponse menyusun balasan hasil pencatatan, termasuk perubahan
// saldo. Perubahan stok ikut dilaporkan supaya klien bisa langsung
// memperbarui tampilan tanpa GET ulang.
func recordResultResponse(res *service.RecordResult) map[string]any {
	out := map[string]any{"log": logResponse(res.Log)}
	if res.Stock != nil {
		out["stock"] = map[string]any{
			"consumed":      res.Stock.Consumed,
			"qty_delta":     res.Stock.QtyDelta,
			"balance_after": res.Stock.BalanceAfter,
			"unit_code":     res.Stock.UnitCode,
			"needs_refill":  res.Stock.NeedsRefill,
		}
	} else {
		out["stock"] = nil
	}
	return out
}

// writeErr memetakan error service ke balasan HTTP.
func (h *Logs) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	if resp, ok := validationResponse(err); ok {
		pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeValidationFailed,
			resp.message, resp.details)
		return
	}
	if !isKnownDomainError(err) {
		h.Logger.ErrorContext(r.Context(), "kesalahan tak terduga pada catatan dosis",
			slog.String("error", err.Error()))
	}
	pohttp.WriteDomainError(w, r, err)
}
