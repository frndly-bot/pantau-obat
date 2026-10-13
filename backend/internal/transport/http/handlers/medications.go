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

// Medications adalah handler untuk /medications/*.
type Medications struct {
	Svc    *service.Medications
	Logger *slog.Logger
}

// NewMedications membuat handler obat.
func NewMedications(svc *service.Medications, logger *slog.Logger) *Medications {
	return &Medications{Svc: svc, Logger: logger}
}

// Routes mendaftarkan endpoint obat beserta jam minumnya.
//
// Semua rute memakai ownerID dari token, bukan dari path. Tidak ada endpoint
// yang bisa menyentuh obat milik user lain; akses caregiver punya jalurnya
// sendiri di /care/{ownerId}/*.
func (h *Medications) Routes(r chi.Router) {
	r.Get("/medications", h.List)
	r.Post("/medications", h.Create)
	r.Get("/medications/{id}", h.Get)
	r.Patch("/medications/{id}", h.Update)
	r.Delete("/medications/{id}", h.Delete)

	// Jam minum sebagai sub-jalur obat, karena jam tidak punya makna
	// tanpa obatnya.
	r.Get("/medications/{id}/times", h.ListTimes)
	r.Post("/medications/{id}/times", h.CreateTime)
	r.Patch("/medications/{id}/times/{tid}", h.UpdateTime)
	r.Delete("/medications/{id}/times/{tid}", h.DeleteTime)
}

// -----------------------------------------------------------------------------
// Bentuk kabel (wire format)
// -----------------------------------------------------------------------------

type frequencyRequest struct {
	Pattern          string `json:"pattern"`
	TimesPerDay      *int   `json:"times_per_day"`
	IntervalHours    *int   `json:"interval_hours"`
	DaysOfWeek       *int   `json:"days_of_week"`
	IntervalMinHours *int   `json:"interval_min_hours"`

	CycleIntakeDays *int    `json:"cycle_intake_days"`
	CyclePauseDays  *int    `json:"cycle_pause_days"`
	CycleAnchor     *string `json:"cycle_anchor"`
	CycleAnchorDOM  *int    `json:"cycle_anchor_dom"`

	PRNMaxPerDay *int `json:"prn_max_per_day"`

	ToleranceMinutes int    `json:"tolerance_minutes"`
	StartTime        string `json:"start_time"`
	ScheduleTimezone string `json:"schedule_timezone"`
}

type timeRequest struct {
	TimeLocal    string  `json:"time_local"`
	Label        *string `json:"label"`
	Weekdays     *int    `json:"weekdays"`
	DoseOverride *string `json:"dose_override"`
}

type stockRequest struct {
	UnitCode           string   `json:"unit_code"`
	InitialQty         float64  `json:"initial_qty"`
	QtyPerDose         float64  `json:"qty_per_dose"`
	RefillThresholdQty *float64 `json:"refill_threshold_qty"`
	RefillReminder     bool     `json:"refill_reminder"`
}

type createMedicationRequest struct {
	Name           string  `json:"name"`
	DoseValue      float64 `json:"dose_value"`
	DoseUnitCode   string  `json:"dose_unit_code"`
	StrengthNote   *string `json:"strength_note"`
	RouteCode      string  `json:"route_code"`
	DosageFormCode *string `json:"dosage_form_code"`
	MealRelation   string  `json:"meal_relation"`
	CatalogID      *string `json:"catalog_id"`

	AsNeeded   bool `json:"as_needed"`
	MustFinish bool `json:"must_finish"`

	StartDate string  `json:"start_date"`
	StartTime string  `json:"start_time"`
	EndDate   *string `json:"end_date"`

	Notes                *string `json:"notes"`
	NotesReminder        *string `json:"notes_reminder"`
	PrescriptionPhotoURL *string `json:"prescription_photo_url"`
	ColorTag             *string `json:"color_tag"`

	Frequency frequencyRequest `json:"frequency"`
	Times     []timeRequest    `json:"times"`
	Stock     *stockRequest    `json:"stock"`
}

type updateMedicationRequest struct {
	Name           *string  `json:"name"`
	DoseValue      *float64 `json:"dose_value"`
	DoseUnitCode   *string  `json:"dose_unit_code"`
	StrengthNote   *string  `json:"strength_note"`
	RouteCode      *string  `json:"route_code"`
	DosageFormCode *string  `json:"dosage_form_code"`
	MealRelation   *string  `json:"meal_relation"`

	AsNeeded   *bool `json:"as_needed"`
	MustFinish *bool `json:"must_finish"`

	StartDate *string `json:"start_date"`
	StartTime *string `json:"start_time"`
	EndDate   *string `json:"end_date"`

	Notes         *string `json:"notes"`
	NotesReminder *string `json:"notes_reminder"`
	ColorTag      *string `json:"color_tag"`
	IsActive      *bool   `json:"is_active"`
}

// -----------------------------------------------------------------------------
// Handler
// -----------------------------------------------------------------------------

// Create menangani POST /api/v1/medications.
func (h *Medications) Create(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	var req createMedicationRequest
	// Endpoint obat memakai aturan field yang NETRAL (nil), bukan aturan akun.
	//
	// `decodeAuthBody` membawa daftar field terlarang milik AKUN — termasuk
	// `is_active`, yang di sana berarti "status aktif akun" dan hanya boleh
	// diubah operator lewat database. Pada obat, `is_active` justru field
	// yang SAH: itulah cara pengguna menghentikan obatnya sendiri.
	//
	// Memakai decoder akun di sini membuat "Hentikan sementara" selalu gagal
	// dengan pesan yang membingungkan ("status aktif akun hanya bisa diubah
	// lewat endpoint admin") padahal yang diubah adalah obat.
	if err := decodeBodyWithRules(w, r, &req, nil); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	in, err := toCreateInput(req)
	if err != nil {
		writeMedicationValidation(w, r, err)
		return
	}

	med, err := h.Svc.Create(r.Context(), id.UserID, in, metaOf(r))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusCreated, medicationResponse(med, true))
}

// List menangani GET /api/v1/medications.
func (h *Medications) List(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	q := r.URL.Query()
	limit := parsePositiveInt(q.Get("limit"), 20)

	meds, err := h.Svc.List(r.Context(), id.UserID, service.ListFilter{
		Status: q.Get("status"),
		Query:  q.Get("q"),
		Limit:  limit,
	})
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	data := make([]map[string]any, 0, len(meds))
	for _, m := range meds {
		// Daftar tidak membawa jam dan stok lengkap; itu hanya di detail.
		// Alasannya: satu halaman daftar 20 obat akan menembak 20 query
		// tambahan kalau setiap baris dilengkapi.
		data = append(data, medicationResponse(m, false))
	}

	pohttp.WriteList(w, r, data, pohttp.Page{
		NextCursor: nil,
		HasMore:    len(meds) == limit,
		Limit:      limit,
	})
}

// Get menangani GET /api/v1/medications/{id}.
func (h *Medications) Get(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	med, err := h.Svc.Get(r.Context(), id.UserID, chi.URLParam(r, "id"))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, medicationResponse(med, true))
}

// Update menangani PATCH /api/v1/medications/{id}.
func (h *Medications) Update(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	var req updateMedicationRequest
	if err := decodeBodyWithRules(w, r, &req, nil); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	in, err := toUpdateInput(req)
	if err != nil {
		writeMedicationValidation(w, r, err)
		return
	}

	med, err := h.Svc.Update(r.Context(), id.UserID, chi.URLParam(r, "id"), in, metaOf(r))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, medicationResponse(med, true))
}

// Delete menangani DELETE /api/v1/medications/{id}.
func (h *Medications) Delete(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	if err := h.Svc.Delete(r.Context(), id.UserID, chi.URLParam(r, "id"), metaOf(r)); err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, map[string]any{
		"deleted":           true,
		"logs_preserved":    true,
		"reminders_stopped": true,
	})
}

// ListTimes menangani GET /api/v1/medications/{id}/times.
func (h *Medications) ListTimes(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	// Obat diperiksa dulu supaya user lain tidak bisa melihat jam obat
	// milik orang lain lewat id yang ditebak.
	med, err := h.Svc.Get(r.Context(), id.UserID, chi.URLParam(r, "id"))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	data := make([]map[string]any, 0, len(med.Times))
	for i := range med.Times {
		data = append(data, timeResponse(&med.Times[i]))
	}

	pohttp.WriteList(w, r, data, pohttp.Page{
		NextCursor: nil,
		HasMore:    false,
		Limit:      len(data),
	})
}

// CreateTime menangani POST /api/v1/medications/{id}/times.
func (h *Medications) CreateTime(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	var req timeRequest
	if err := decodeBodyWithRules(w, r, &req, nil); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	in, err := toTimeInput(req)
	if err != nil {
		writeMedicationValidation(w, r, err)
		return
	}

	t, err := h.Svc.CreateTime(r.Context(), id.UserID, chi.URLParam(r, "id"), in, metaOf(r))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusCreated, timeResponse(t))
}

// UpdateTime menangani PATCH /api/v1/medications/{id}/times/{tid}.
func (h *Medications) UpdateTime(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	var req timeRequest
	if err := decodeBodyWithRules(w, r, &req, nil); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	in := service.UpdateTimeInput{}
	if req.TimeLocal != "" {
		hh, mm, err := domain.ParseClock(req.TimeLocal)
		if err != nil {
			writeMedicationValidation(w, r, err)
			return
		}
		normalized := domain.FormatClock(hh, mm)
		in.TimeLocal = &normalized
	}
	if req.Label != nil {
		l := domain.TimeLabel(*req.Label)
		if !l.Valid() {
			writeMedicationValidation(w, r,
				errInvalidField("label", "label jam tidak dikenal"))
			return
		}
		in.Label = &l
	}
	in.Weekdays = req.Weekdays
	in.DoseOverride = req.DoseOverride

	t, err := h.Svc.UpdateTime(r.Context(), id.UserID,
		chi.URLParam(r, "id"), chi.URLParam(r, "tid"), in, metaOf(r))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, timeResponse(t))
}

// DeleteTime menangani DELETE /api/v1/medications/{id}/times/{tid}.
func (h *Medications) DeleteTime(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	err := h.Svc.DeleteTime(r.Context(), id.UserID,
		chi.URLParam(r, "id"), chi.URLParam(r, "tid"), metaOf(r))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

// -----------------------------------------------------------------------------
// Pemetaan dan balasan
// -----------------------------------------------------------------------------

// toCreateInput memetakan bentuk kabel ke input service.
//
// Tanggal dan jam diurai di sini, di batas transport, supaya lapisan service
// hanya menerima tipe yang sudah pasti benar.
func toCreateInput(req createMedicationRequest) (service.CreateMedicationInput, error) {
	in := service.CreateMedicationInput{
		Name:                 req.Name,
		DoseValue:            req.DoseValue,
		DoseUnitCode:         req.DoseUnitCode,
		StrengthNote:         req.StrengthNote,
		RouteCode:            req.RouteCode,
		DosageFormCode:       req.DosageFormCode,
		MealRelation:         domain.MealRelation(req.MealRelation),
		CatalogID:            req.CatalogID,
		AsNeeded:             req.AsNeeded,
		MustFinish:           req.MustFinish,
		StartTime:            req.StartTime,
		Notes:                req.Notes,
		NotesReminder:        req.NotesReminder,
		PrescriptionPhotoURL: req.PrescriptionPhotoURL,
		ColorTag:             req.ColorTag,
	}

	if req.StartDate != "" {
		d, err := time.Parse(time.DateOnly, req.StartDate)
		if err != nil {
			return in, errInvalidField("start_date", "tanggal mulai harus berbentuk YYYY-MM-DD")
		}
		in.StartDate = d
	}
	if req.EndDate != nil && *req.EndDate != "" {
		d, err := time.Parse(time.DateOnly, *req.EndDate)
		if err != nil {
			return in, errInvalidField("end_date", "tanggal selesai harus berbentuk YYYY-MM-DD")
		}
		in.EndDate = &d
	}

	in.Frequency = service.FrequencyInput{
		Pattern:          domain.FrequencyPattern(req.Frequency.Pattern),
		TimesPerDay:      req.Frequency.TimesPerDay,
		IntervalHours:    req.Frequency.IntervalHours,
		DaysOfWeek:       req.Frequency.DaysOfWeek,
		IntervalMinHours: req.Frequency.IntervalMinHours,
		CycleIntakeDays:  req.Frequency.CycleIntakeDays,
		CyclePauseDays:   req.Frequency.CyclePauseDays,
		CycleAnchorDOM:   req.Frequency.CycleAnchorDOM,
		PRNMaxPerDay:     req.Frequency.PRNMaxPerDay,
		ToleranceMinutes: req.Frequency.ToleranceMinutes,
		StartTime:        req.Frequency.StartTime,
		ScheduleTimezone: req.Frequency.ScheduleTimezone,
	}
	if req.Frequency.CycleAnchor != nil && *req.Frequency.CycleAnchor != "" {
		a := domain.CycleAnchor(*req.Frequency.CycleAnchor)
		in.Frequency.CycleAnchor = &a
	}

	for i, t := range req.Times {
		ti, err := toTimeInput(t)
		if err != nil {
			return in, err
		}
		in.Times = append(in.Times, ti)
		_ = i
	}

	if req.Stock != nil {
		in.Stock = &service.StockInput{
			UnitCode:           req.Stock.UnitCode,
			InitialQty:         req.Stock.InitialQty,
			QtyPerDose:         req.Stock.QtyPerDose,
			RefillThresholdQty: req.Stock.RefillThresholdQty,
			RefillReminder:     req.Stock.RefillReminder,
		}
	}

	return in, nil
}

// toTimeInput memetakan satu jam minum.
func toTimeInput(req timeRequest) (service.TimeInput, error) {
	hh, mm, err := domain.ParseClock(req.TimeLocal)
	if err != nil {
		return service.TimeInput{}, errInvalidField("time_local", "jam harus berbentuk HH:MM")
	}

	out := service.TimeInput{
		TimeLocal:    domain.FormatClock(hh, mm),
		Weekdays:     req.Weekdays,
		DoseOverride: req.DoseOverride,
	}
	if req.Label != nil && *req.Label != "" {
		l := domain.TimeLabel(*req.Label)
		if !l.Valid() {
			return out, errInvalidField("label", "label jam tidak dikenal")
		}
		out.Label = &l
	}
	return out, nil
}

// toUpdateInput memetakan perubahan obat.
func toUpdateInput(req updateMedicationRequest) (service.UpdateMedicationInput, error) {
	in := service.UpdateMedicationInput{
		Name:           req.Name,
		DoseValue:      req.DoseValue,
		DoseUnitCode:   req.DoseUnitCode,
		StrengthNote:   req.StrengthNote,
		RouteCode:      req.RouteCode,
		DosageFormCode: req.DosageFormCode,
		AsNeeded:       req.AsNeeded,
		MustFinish:     req.MustFinish,
		StartTime:      req.StartTime,
		Notes:          req.Notes,
		NotesReminder:  req.NotesReminder,
		ColorTag:       req.ColorTag,
		IsActive:       req.IsActive,
	}

	if req.MealRelation != nil {
		m := domain.MealRelation(*req.MealRelation)
		if !m.Valid() {
			return in, errInvalidField("meal_relation", "waktu makan tidak dikenal")
		}
		in.MealRelation = &m
	}
	if req.StartDate != nil {
		d, err := time.Parse(time.DateOnly, *req.StartDate)
		if err != nil {
			return in, errInvalidField("start_date", "tanggal mulai harus berbentuk YYYY-MM-DD")
		}
		in.StartDate = &d
	}
	if req.EndDate != nil {
		if *req.EndDate == "" {
			// String kosong berarti user ingin mengosongkan tanggal selesai,
			// sehingga terapi kembali tanpa batas.
			in.ClearEndDate = true
		} else {
			d, err := time.Parse(time.DateOnly, *req.EndDate)
			if err != nil {
				return in, errInvalidField("end_date", "tanggal selesai harus berbentuk YYYY-MM-DD")
			}
			in.EndDate = &d
		}
	}

	return in, nil
}

// medicationResponse menyusun balasan obat.
//
// includeDetail menentukan apakah jam dan stok ikut dikirim. Daftar tidak
// perlu, karena melengkapinya berarti satu query tambahan per baris.
func medicationResponse(m *domain.Medication, includeDetail bool) map[string]any {
	out := map[string]any{
		"id":                     m.ID,
		"owner_id":               m.OwnerID,
		"catalog_id":             m.CatalogID,
		"name":                   m.Name,
		"dose_value":             m.DoseValue,
		"dose_unit_code":         m.DoseUnitCode,
		"strength_note":          m.StrengthNote,
		"route_code":             m.RouteCode,
		"dosage_form_code":       m.DosageFormCode,
		"meal_relation":          string(m.MealRelation),
		"as_needed":              m.AsNeeded,
		"must_finish":            m.MustFinish,
		"start_date":             m.StartDate.Format(time.DateOnly),
		"start_time":             normalizeClock(m.StartTime),
		"end_date":               formatDatePtr(m.EndDate),
		"notes":                  m.Notes,
		"notes_reminder":         m.NotesReminder,
		"prescription_photo_url": m.PrescriptionPhotoURL,
		"color_tag":              m.ColorTag,
		"is_active":              m.IsActive,
		"created_at":             m.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":             m.UpdatedAt.UTC().Format(time.RFC3339),
	}

	if includeDetail {
		if m.Frequency != nil {
			out["frequency"] = frequencyResponse(m.Frequency)
		} else {
			out["frequency"] = nil
		}
		times := make([]map[string]any, 0, len(m.Times))
		for i := range m.Times {
			times = append(times, timeResponse(&m.Times[i]))
		}
		out["times"] = times
		if m.Stock != nil {
			out["stock"] = stockResponse(m.Stock)
		} else {
			out["stock"] = nil
		}
	}

	return out
}

// frequencyResponse menyusun balasan aturan frekuensi.
//
// Parameter yang tidak relevan untuk pattern-nya dikirim sebagai null,
// sesuai konvensi api-v2 bagian 1 butir 8. Frontend bisa menampilkan hanya
// yang terisi tanpa perlu menebak.
func frequencyResponse(f *domain.MedicationFrequency) map[string]any {
	return map[string]any{
		"id":                 f.ID,
		"pattern":            string(f.Pattern),
		"times_per_day":      f.TimesPerDay,
		"interval_hours":     f.IntervalHours,
		"days_of_week":       f.DaysOfWeek,
		"interval_min_hours": f.IntervalMinHours,
		"cycle_intake_days":  f.CycleIntakeDays,
		"cycle_pause_days":   f.CyclePauseDays,
		"cycle_anchor":       anchorValue(f.CycleAnchor),
		"cycle_anchor_dom":   f.CycleAnchorDOM,
		"prn_max_per_day":    f.PRNMaxPerDay,
		"tolerance_minutes":  f.ToleranceMinutes,
		"start_time":         normalizeClock(f.StartTime),
		"schedule_timezone":  f.ScheduleTimezone,
	}
}

func timeResponse(t *domain.MedicationTime) map[string]any {
	return map[string]any{
		"id":            t.ID,
		"medication_id": t.MedicationID,
		"time_local":    normalizeClock(t.TimeLocal),
		"label":         labelValue(t.Label),
		"weekdays":      t.Weekdays,
		"dose_override": t.DoseOverride,
		"sort_order":    t.SortOrder,
		"is_active":     t.IsActive,
	}
}

func stockResponse(s *domain.MedicationStock) map[string]any {
	return map[string]any{
		"unit_code":            s.UnitCode,
		"initial_qty":          s.InitialQty,
		"current_qty":          s.CurrentQty,
		"qty_per_dose":         s.QtyPerDose,
		"refill_threshold_qty": s.RefillThresholdQty,
		"refill_reminder":      s.RefillReminder,
		"doses_remaining":      s.DosesRemaining(),
		"needs_refill":         s.NeedsRefill(),
		"last_restocked_at":    formatTimePtr(s.LastRestockedAt),
	}
}

// normalizeClock memotong detik dari nilai TIME PostgreSQL.
//
// PostgreSQL mengembalikan "07:00:00", sedangkan kontrak API memakai "07:00".
// Normalisasi dilakukan di lapisan response supaya service dan domain tetap
// memakai bentuk apa adanya dari database.
func normalizeClock(s string) string {
	if hh, mm, err := domain.ParseClock(s); err == nil {
		return domain.FormatClock(hh, mm)
	}
	return s
}

func formatDatePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Format(time.DateOnly)
}

func formatTimePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

func anchorValue(a *domain.CycleAnchor) any {
	if a == nil {
		return nil
	}
	return string(*a)
}

func labelValue(l *domain.TimeLabel) any {
	if l == nil {
		return nil
	}
	return string(*l)
}

// unauthorized membalas 401 untuk request tanpa identitas.
func unauthorized(w http.ResponseWriter, r *http.Request) {
	pohttp.WriteError(w, r, http.StatusUnauthorized, pohttp.CodeUnauthenticated,
		"sesi tidak valid, silakan masuk lagi", nil)
}

// errInvalidField menyusun error validasi yang menyebut field bermasalah.
type fieldError struct {
	field   string
	message string
}

func (e *fieldError) Error() string { return e.message }

func errInvalidField(field, message string) error {
	return &fieldError{field: field, message: message}
}

// writeMedicationValidation membalas 400 dengan detail field.
func writeMedicationValidation(w http.ResponseWriter, r *http.Request, err error) {
	var fe *fieldError
	if ok := asFieldError(err, &fe); ok {
		pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeValidationFailed,
			fe.message, []pohttp.FieldIssue{{Field: fe.field, Issue: "invalid"}})
		return
	}
	pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeValidationFailed,
		err.Error(), nil)
}

func asFieldError(err error, target **fieldError) bool {
	if err == nil {
		return false
	}
	if fe, ok := err.(*fieldError); ok {
		*target = fe
		return true
	}
	return false
}

// writeErr memetakan error service ke balasan HTTP.
func (h *Medications) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	if resp, ok := validationResponse(err); ok {
		pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeValidationFailed,
			resp.message, resp.details)
		return
	}
	if !isKnownDomainError(err) {
		h.Logger.ErrorContext(r.Context(), "kesalahan tak terduga pada obat",
			slog.String("error", err.Error()))
	}
	pohttp.WriteDomainError(w, r, err)
}
