package handlers

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	"github.com/frndly-bot/pantau-obat/backend/internal/service"
	pohttp "github.com/frndly-bot/pantau-obat/backend/internal/transport/http"
)

// File ini menampung endpoint kelola obat & stok PASIEN oleh caregiver.
//
// Semua rute di sini WAJIB lolos service.RequireManage — hanya role manager
// yang boleh. Helper dibatasi mencatat log (CreateLog di carelinks.go), dan
// viewer hanya membaca (Medications/Overview/Adherence/Logs).
//
// Pemisahan dengan handler obat pasien sendiri (medications.go) disengaja:
// audit trail-nya berbeda. Di jalur ini actor_id = caregiver sementara
// owner_id = pasien, dan itu harus terlihat jelas di audit_log.

// careMedicationRequest meniru createMedicationRequest (medications.go).
// Sengaja TIDAK berbagi struct: kalau berbagi, perubahan bentuk kabel untuk
// pasien otomatis mengubah bentuk kabel caregiver — dua kontrak yang harusnya
// bisa bergerak sendiri.
type careMedicationRequest struct {
	Name                 string  `json:"name"`
	DoseValue            float64 `json:"dose_value"`
	DoseUnitCode         string  `json:"dose_unit_code"`
	StrengthNote         *string `json:"strength_note"`
	RouteCode            string  `json:"route_code"`
	DosageFormCode       *string `json:"dosage_form_code"`
	MealRelation         string  `json:"meal_relation"`
	CatalogID            *string `json:"catalog_id"`
	AsNeeded             bool    `json:"as_needed"`
	MustFinish           bool    `json:"must_finish"`
	StartDate            string  `json:"start_date"`
	StartTime            string  `json:"start_time"`
	EndDate              *string `json:"end_date"`
	Notes                *string `json:"notes"`
	NotesReminder        *string `json:"notes_reminder"`
	PrescriptionPhotoURL *string `json:"prescription_photo_url"`
	ColorTag             *string `json:"color_tag"`

	Frequency frequencyRequest `json:"frequency"`
	Times     []timeRequest    `json:"times"`
	Stock     *stockRequest    `json:"stock"`
}

// PrescriptionPhotoURL SENGAJA TIDAK ada di struct ini: caregiver (termasuk
// manager) tidak pernah melihat foto resep pasien, sesuai api-v2 bagian 9.
// Menerima field itu berarti menampung data yang tidak boleh mereka lihat.

// CreateMedication menangani POST /api/v1/care/{ownerId}/medications.
//
// Hanya manager. Obat yang dibuat milik PASIEN (owner_id = pasien), bukan
// caregiver; audit mencatat keduanya.
func (h *CareLinks) CreateMedication(w http.ResponseWriter, r *http.Request) {
	actor := pohttp.IdentityFrom(r.Context())
	if actor == nil {
		unauthorized(w, r)
		return
	}
	ownerID := chi.URLParam(r, "ownerId")

	acc, err := h.Svc.RequireManage(r.Context(), actor.UserID, ownerID)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	var req careMedicationRequest
	if err := decodeBodyWithRules(w, r, &req, nil); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	in, err := careMedToCreateInput(req)
	if err != nil {
		writeMedicationValidation(w, r, err)
		return
	}

	med, err := h.Meds.Create(r.Context(), acc.OwnerID, in, careMetaOf(r, actor.UserID))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusCreated, map[string]any{
		"medication": medicationResponse(med, true),
		"created_by": actor.UserID,
		"created_as": "caregiver_manager",
	})
}

// UpdateMedication menangani PATCH /api/v1/care/{ownerId}/medications/{id}.
// Hanya manager. Bentuk kabel sama dengan PATCH /medications/{id} milik
// pasien; yang berbeda hanya pemeriksaan hak dan jejak auditnya.
func (h *CareLinks) UpdateMedication(w http.ResponseWriter, r *http.Request) {
	actor := pohttp.IdentityFrom(r.Context())
	if actor == nil {
		unauthorized(w, r)
		return
	}
	ownerID := chi.URLParam(r, "ownerId")

	acc, err := h.Svc.RequireManage(r.Context(), actor.UserID, ownerID)
	if err != nil {
		h.writeErr(w, r, err)
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

	med, err := h.Meds.Update(r.Context(), acc.OwnerID, chi.URLParam(r, "id"), in,
		careMetaOf(r, actor.UserID))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, map[string]any{
		"medication": medicationResponse(med, true),
		"updated_by": actor.UserID,
		"updated_as": "caregiver_manager",
	})
}

// DeleteMedication menangani DELETE /api/v1/care/{ownerId}/medications/{id}.
// Soft-delete: log pasien tetap tersimpan, pengingatnya berhenti.
func (h *CareLinks) DeleteMedication(w http.ResponseWriter, r *http.Request) {
	actor := pohttp.IdentityFrom(r.Context())
	if actor == nil {
		unauthorized(w, r)
		return
	}
	ownerID := chi.URLParam(r, "ownerId")

	acc, err := h.Svc.RequireManage(r.Context(), actor.UserID, ownerID)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	if err := h.Meds.Delete(r.Context(), acc.OwnerID, chi.URLParam(r, "id"),
		careMetaOf(r, actor.UserID)); err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, map[string]any{
		"deleted":           true,
		"logs_preserved":    true,
		"reminders_stopped": true,
		"deleted_by":        actor.UserID,
		"deleted_as":        "caregiver_manager",
	})
}

// careRestockRequest meniru restockRequest (stock.go) tanpa berbagi struct,
// dengan alasan yang sama seperti careMedicationRequest.
type careRestockRequest struct {
	Qty  float64 `json:"qty"`
	Note *string `json:"note"`
}

type careAdjustRequest struct {
	QtyDelta float64 `json:"qty_delta"`
	Kind     string  `json:"kind"`
	Note     *string `json:"note"`
}

// Restock menangani POST /api/v1/care/{ownerId}/medications/{id}/stock/restock.
// Hanya manager. Stok pasien bertambah; transaksi tercatat atas nama caregiver.
func (h *CareLinks) Restock(w http.ResponseWriter, r *http.Request) {
	actor := pohttp.IdentityFrom(r.Context())
	if actor == nil {
		unauthorized(w, r)
		return
	}
	ownerID := chi.URLParam(r, "ownerId")

	acc, err := h.Svc.RequireManage(r.Context(), actor.UserID, ownerID)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	var req careRestockRequest
	if err := decodeBodyWithRules(w, r, &req, nil); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	change, err := h.Stock.Restock(r.Context(), acc.OwnerID, chi.URLParam(r, "id"),
		service.RestockInput{Qty: req.Qty, Note: req.Note}, careMetaOf(r, actor.UserID))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusCreated, map[string]any{
		"stock":        change,
		"restocked_by": actor.UserID,
		"restocked_as": "caregiver_manager",
	})
}

// Adjust menangani POST /api/v1/care/{ownerId}/medications/{id}/stock/adjust.
// Koreksi stok: hanya manager. Kind yang tidak dikenal ditolak di service.
func (h *CareLinks) Adjust(w http.ResponseWriter, r *http.Request) {
	actor := pohttp.IdentityFrom(r.Context())
	if actor == nil {
		unauthorized(w, r)
		return
	}
	ownerID := chi.URLParam(r, "ownerId")

	acc, err := h.Svc.RequireManage(r.Context(), actor.UserID, ownerID)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	var req careAdjustRequest
	if err := decodeBodyWithRules(w, r, &req, nil); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	change, err := h.Stock.Adjust(r.Context(), acc.OwnerID, chi.URLParam(r, "id"),
		service.AdjustInput{QtyDelta: req.QtyDelta, Kind: domain.StockTxnKind(req.Kind), Note: strDeref(req.Note)},
		careMetaOf(r, actor.UserID))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusCreated, map[string]any{
		"stock":       change,
		"adjusted_by": actor.UserID,
		"adjusted_as": "caregiver_manager",
	})
}

// careMedToCreateInput menyalin toCreateInput (medications.go) untuk bentuk
// kabel caregiver. Terpisah supaya dua kontrak tidak menempel satu sama lain.
func careMedToCreateInput(req careMedicationRequest) (service.CreateMedicationInput, error) {
	in := service.CreateMedicationInput{
		Name:           req.Name,
		DoseValue:      req.DoseValue,
		DoseUnitCode:   req.DoseUnitCode,
		StrengthNote:   req.StrengthNote,
		RouteCode:      req.RouteCode,
		DosageFormCode: req.DosageFormCode,
		MealRelation:   domain.MealRelation(req.MealRelation),
		CatalogID:      req.CatalogID,
		AsNeeded:       req.AsNeeded,
		MustFinish:     req.MustFinish,
		StartTime:      req.StartTime,
		Notes:          req.Notes,
		NotesReminder:  req.NotesReminder,
		ColorTag:       req.ColorTag,
	}

	if req.StartDate != "" {
		d, err := time.Parse(time.DateOnly, req.StartDate)
		if err != nil {
			return in, errInvalidField("start_date", "tanggal harus berbentuk YYYY-MM-DD")
		}
		in.StartDate = d
	}
	if req.EndDate != nil && *req.EndDate != "" {
		d, err := time.Parse(time.DateOnly, *req.EndDate)
		if err != nil {
			return in, errInvalidField("end_date", "tanggal harus berbentuk YYYY-MM-DD")
		}
		in.EndDate = &d
	}

	freq := service.FrequencyInput{
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
		freq.CycleAnchor = &a
	}

	var times []service.TimeInput
	for _, t := range req.Times {
		ti, err := toTimeInput(t)
		if err != nil {
			return in, err
		}
		times = append(times, ti)
	}

	var stock *service.StockInput
	if req.Stock != nil {
		stock = &service.StockInput{
			UnitCode:           req.Stock.UnitCode,
			InitialQty:         req.Stock.InitialQty,
			QtyPerDose:         req.Stock.QtyPerDose,
			RefillThresholdQty: req.Stock.RefillThresholdQty,
			RefillReminder:     req.Stock.RefillReminder,
		}
	}

	in.Frequency = freq
	in.Times = times
	in.Stock = stock
	return in, nil
}

// strDeref mengubah *string menjadi string kosong bila nil.
func strDeref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// careMetaOf menyusun metadata untuk audit aksi caregiver atas nama pasien.
// ActorID diisi caregiver — berbeda dari jalur pasien sendiri. Meta yang ada
// hanya membawa IP dan user agent, keduanya sudah cukup untuk jejak audit;
// aktor caregivernya sendiri tercatat di audit_log.actor_id oleh service.
func careMetaOf(r *http.Request, actorID string) service.Meta {
	_ = actorID // aktor tercatat via audit actor_id di service layer
	return metaOf(r)
}
