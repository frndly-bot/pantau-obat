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

// Schedule adalah handler jadwal dosis harian.
type Schedule struct {
	Svc    *service.Schedule
	Users  *service.Users
	Logger *slog.Logger
}

// NewSchedule membuat handler jadwal.
func NewSchedule(svc *service.Schedule, users *service.Users, logger *slog.Logger) *Schedule {
	return &Schedule{Svc: svc, Users: users, Logger: logger}
}

// Routes mendaftarkan endpoint jadwal.
//
// Diletakkan di kelompok terautentikasi: jadwal berisi nama obat, dan itu
// data kesehatan.
func (h *Schedule) Routes(r chi.Router) {
	r.Get("/schedule", h.Day)
}

// Day menangani GET /api/v1/schedule.
//
// Parameter `date` opsional (YYYY-MM-DD, waktu lokal pemilik). Bila kosong,
// dipakai hari ini menurut zona waktu pemilik — bukan UTC. Tanpa itu, pengguna
// WIB yang membuka aplikasi pukul 06:00 akan melihat jadwal hari kemarin.
func (h *Schedule) Day(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	tz := h.timezoneFor(r, id.UserID)

	in := service.DayInput{OwnerID: id.UserID, Timezone: tz}

	if raw := r.URL.Query().Get("date"); raw != "" {
		d, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			writeMedicationValidation(w, r, errInvalidField("date", "tanggal harus berbentuk YYYY-MM-DD"))
			return
		}
		in.Date = d
	}

	slots, err := h.Svc.Day(r.Context(), in)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	data := make([]map[string]any, 0, len(slots))
	for i := range slots {
		data = append(data, slotResponse(&slots[i]))
	}

	pohttp.WriteList(w, r, data, pohttp.Page{
		NextCursor: nil,
		HasMore:    false,
		Limit:      len(data),
	})
}

// timezoneFor membaca zona waktu user, dengan cadangan WIB.
func (h *Schedule) timezoneFor(r *http.Request, userID string) string {
	u, err := h.Users.Profile(r.Context(), userID)
	if err != nil || u.Timezone == "" {
		return domain.DefaultTimezone
	}
	return u.Timezone
}

func (h *Schedule) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	if resp, ok := validationResponse(err); ok {
		pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeValidationFailed,
			resp.message, resp.details)
		return
	}
	if !isKnownDomainError(err) {
		h.Logger.ErrorContext(r.Context(), "kesalahan tak terduga pada jadwal",
			slog.String("error", err.Error()))
	}
	pohttp.WriteDomainError(w, r, err)
}

// slotResponse menyusun representasi JSON satu slot jadwal.
//
// `schedule_id` sengaja diberi nama yang sama dengan yang diharapkan
// POST /medications/{id}/logs, supaya klien bisa mencatat langsung dari slot
// tanpa menerjemahkan id.
func slotResponse(s *service.DaySlot) map[string]any {
	out := map[string]any{
		"medication_id":   s.MedicationID,
		"medication_name": s.MedicationName,
		"dose_value":      s.DoseValue,
		"dose_unit_code":  s.DoseUnitCode,
		"meal_relation":   string(s.MealRelation),
		"color_tag":       s.ColorTag,

		// normalizeClock memotong detik: PostgreSQL mengembalikan kolom `time`
		// sebagai "07:00:00", sedangkan kontrak API memakai "HH:MM". Tanpa ini
		// klien menerima dua bentuk berbeda untuk jam yang sama.
		"schedule_id":   s.ScheduleID,
		"time_local":    normalizeClock(s.TimeLocal),
		"time_label":    labelOrNil(s.TimeLabel),
		"dose_override": s.DoseOverride,

		"slot_date":        s.SlotDate.Format(time.DateOnly),
		"scheduled_at":     s.DueAt.Format(time.RFC3339),
		"scheduled_tz":     s.ScheduledTZ,
		"scheduled_hour":   s.DueAt.Hour(),
		"scheduled_minute": s.DueAt.Minute(),

		"status":   string(s.Status),
		"log_id":   s.LogID,
		"taken_at": formatTimePtr(s.TakenAt),
		"note":     s.Note,

		"stock_remaining": s.StockRemaining,
		"stock_unit_code": s.StockUnitCode,
	}
	return out
}

func labelOrNil(l *domain.TimeLabel) any {
	if l == nil {
		return nil
	}
	return string(*l)
}
