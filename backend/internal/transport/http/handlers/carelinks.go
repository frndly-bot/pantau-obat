package handlers

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	"github.com/frndly-bot/pantau-obat/backend/internal/repo/postgres"
	"github.com/frndly-bot/pantau-obat/backend/internal/service"
	pohttp "github.com/frndly-bot/pantau-obat/backend/internal/transport/http"
)

// CareLinks adalah handler untuk /care-links/* dan /care/{ownerId}/*.
type CareLinks struct {
	Svc    *service.CareLinks
	LogSvc *service.Logs
	Meds   *service.Medications
	Stock  *service.Stock
	Adh    *service.Adherence
	Users  *service.Users
	// Sched dipakai untuk menampilkan JADWAL pasien, bukan hanya catatan
	// yang sudah ada. Tanpa ini caregiver tidak bisa melihat dosis yang
	// belum dicatat — dan justru itulah yang paling perlu ditindak.
	Sched  *service.Schedule
	Logger *slog.Logger

	// PublicURL dipakai untuk menyusun tautan undangan. Nilainya dari
	// konfigurasi, tidak pernah dari header permintaan.
	PublicURL string
}

// NewCareLinks membuat handler caregiver.
func NewCareLinks(
	svc *service.CareLinks, logSvc *service.Logs, meds *service.Medications,
	stock *service.Stock, adh *service.Adherence, users *service.Users,
	sched *service.Schedule, logger *slog.Logger,
) *CareLinks {
	return &CareLinks{
		Svc: svc, LogSvc: logSvc, Meds: meds, Stock: stock,
		Adh: adh, Users: users, Sched: sched, Logger: logger,
	}
}

// Routes mendaftarkan endpoint caregiver.
func (h *CareLinks) Routes(r chi.Router) {
	r.Post("/care-links", h.CreateInvite)
	r.Get("/care-links", h.ListLinks)
	r.Post("/care-links/accept", h.AcceptInvite)
	r.Post("/care-links/reject", h.RejectInvite)
	r.Patch("/care-links/{id}", h.UpdateLink)
	r.Delete("/care-links/{id}", h.RevokeLink)

	// Dashboard caregiver: seluruh jalur di bawah /care/{ownerId}
	r.Get("/care/{ownerId}/overview", h.Overview)
	r.Get("/care/{ownerId}/schedule", h.Schedule)
	r.Get("/care/{ownerId}/medications", h.Medications)
	r.Get("/care/{ownerId}/adherence", h.Adherence)
	r.Get("/care/{ownerId}/logs", h.ListLogs)
	r.Get("/care/{ownerId}/logs/export", h.ExportForOwner)
	r.Post("/care/{ownerId}/logs", h.CreateLog)

	// Kelola obat & stok pasien. HANYA role manager yang lolos (lihat
	// service.RequireManage). Sesuai api-v2 bagian 9.
	r.Post("/care/{ownerId}/medications", h.CreateMedication)
	r.Patch("/care/{ownerId}/medications/{id}", h.UpdateMedication)
	r.Delete("/care/{ownerId}/medications/{id}", h.DeleteMedication)
	r.Post("/care/{ownerId}/medications/{id}/stock/restock", h.Restock)
	r.Post("/care/{ownerId}/medications/{id}/stock/adjust", h.Adjust)
}

// -----------------------------------------------------------------------------
// Bentuk kabel
// -----------------------------------------------------------------------------

type createInviteRequest struct {
	InviteEmail string `json:"invite_email"`
	Relation    string `json:"relation"`
	Role        string `json:"role"`
}

type inviteTokenRequest struct {
	Token string `json:"token"`
}

type updateLinkRequest struct {
	Role     *string `json:"role"`
	Relation *string `json:"relation"`
}

type careLogRequest struct {
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

// CreateInvite menangani POST /api/v1/care-links.
func (h *CareLinks) CreateInvite(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	var req createInviteRequest
	if err := decodeBodyWithRules(w, r, &req, nil); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	res, err := h.Svc.CreateInvite(r.Context(), id.UserID, service.CreateInviteInput{
		InviteEmail: req.InviteEmail,
		Relation:    domain.CareRelation(req.Relation),
		Role:        domain.CareRole(req.Role),
	}, metaOf(r))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	// Token MENTAH dikembalikan sekali ini saja. Yang tersimpan hanya
	// SHA-256-nya, jadi setelah balasan ini tidak ada cara mengambilnya lagi.
	pohttp.WriteJSON(w, r, http.StatusCreated, map[string]any{
		"id":         res.Link.ID,
		"status":     string(res.Link.Status),
		"role":       string(res.Link.Role),
		"relation":   string(res.Link.Relation),
		"invite_url": h.inviteURL(r, res.Token),
		"expires_at": res.ExpiresAt.UTC().Format(time.RFC3339),
		"note":       "tautan undangan hanya ditampilkan sekali dan tidak bisa diambil ulang",
	})
}

// inviteURL menyusun tautan undangan untuk dikirim ke caregiver.
//
// Memakai APP_PUBLIC_URL dari konfigurasi, BUKAN nilai dari header Host.
// Header itu dikirim klien dan bisa dipalsukan, sehingga tautan undangan
// bisa diarahkan ke domain penyerang — dan caregiver yang mengkliknya akan
// menyerahkan akses ke data kesehatan pasien.
func (h *CareLinks) inviteURL(_ *http.Request, token string) string {
	base := h.PublicURL
	if base == "" {
		base = "http://localhost:8080"
	}
	return strings.TrimRight(base, "/") + "/terima-undangan?token=" + token
}

// ListLinks menangani GET /api/v1/care-links.
func (h *CareLinks) ListLinks(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	// ?as=caregiver berarti "siapa yang saya pantau"; bawaannya
	// "siapa yang memantau saya".
	asOwner := r.URL.Query().Get("as") != "caregiver"

	links, err := h.Svc.ListLinks(r.Context(), id.UserID, asOwner)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	data := make([]map[string]any, 0, len(links))
	for _, l := range links {
		data = append(data, careLinkResponse(&l))
	}
	pohttp.WriteList(w, r, data, pohttp.Page{
		NextCursor: nil,
		HasMore:    false,
		Limit:      len(data),
	})
}

// AcceptInvite menangani POST /api/v1/care-links/accept.
func (h *CareLinks) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	var req inviteTokenRequest
	if err := decodeBodyWithRules(w, r, &req, nil); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	link, err := h.Svc.AcceptInvite(r.Context(), id.UserID, req.Token, metaOf(r))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, careLinkResponse(link))
}

// RejectInvite menangani POST /api/v1/care-links/reject.
func (h *CareLinks) RejectInvite(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	var req inviteTokenRequest
	if err := decodeBodyWithRules(w, r, &req, nil); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	link, err := h.Svc.RejectInvite(r.Context(), id.UserID, req.Token, metaOf(r))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, careLinkResponse(link))
}

// UpdateLink menangani PATCH /api/v1/care-links/{id}.
func (h *CareLinks) UpdateLink(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	var req updateLinkRequest
	if err := decodeBodyWithRules(w, r, &req, nil); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	var role *domain.CareRole
	if req.Role != nil {
		v := domain.CareRole(*req.Role)
		role = &v
	}
	var relation *domain.CareRelation
	if req.Relation != nil {
		v := domain.CareRelation(*req.Relation)
		relation = &v
	}

	link, err := h.Svc.UpdateLink(r.Context(), id.UserID, chi.URLParam(r, "id"),
		role, relation, metaOf(r))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, careLinkResponse(link))
}

// RevokeLink menangani DELETE /api/v1/care-links/{id}.
func (h *CareLinks) RevokeLink(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	if err := h.Svc.RevokeLink(r.Context(), id.UserID,
		chi.URLParam(r, "id"), metaOf(r)); err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, map[string]any{
		"revoked":        true,
		"access_removed": true,
	})
}

// -----------------------------------------------------------------------------
// Dashboard caregiver
// -----------------------------------------------------------------------------

// Overview menangani GET /api/v1/care/{ownerId}/overview.
func (h *CareLinks) Overview(w http.ResponseWriter, r *http.Request) {
	acc, owner, ok := h.requireAccess(w, r)
	if !ok {
		return
	}

	rng, err := h.parseRange(r, owner)
	if err != nil {
		writeMedicationValidation(w, r, err)
		return
	}

	// Seluruh angka disusun dari perhitungan YANG SAMA dengan yang dipakai
	// pasien melihat laporannya sendiri. Kalau caregiver punya jalur
	// perhitungan tersendiri, cepat atau lambat angkanya berbeda dan
	// keduanya sama-sama merasa benar.
	sum, err := h.Adh.Summary(r.Context(), acc.OwnerID, rng)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	byMed, err := h.Adh.ByMedication(r.Context(), acc.OwnerID, rng)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	items := make([]map[string]any, 0, len(byMed))
	for _, m := range byMed {
		item := map[string]any{
			"medication_id":   m.MedicationID,
			"medication_name": m.MedicationName,
			"color_tag":       m.ColorTag,
			"scheduled_doses": m.ScheduledDoses,
			"taken_doses":     m.TakenDoses,
			"missed_doses":    m.MissedDoses,
			"pending_doses":   m.PendingDoses,
		}
		if m.Ratio != nil {
			item["adherence_ratio"] = *m.Ratio
		} else {
			item["adherence_ratio"] = nil
		}
		items = append(items, item)
	}

	pohttp.WriteJSON(w, r, http.StatusOK, map[string]any{
		"owner_id":   acc.OwnerID,
		"owner_name": owner.FullName,
		"timezone":   owner.Timezone,
		"from":       rng.From.Format(time.DateOnly),
		"to":         rng.To.Format(time.DateOnly),
		"summary": map[string]any{
			"scheduled_doses": sum.ScheduledDoses,
			"taken_doses":     sum.TakenDoses,
			"late_doses":      sum.LateDoses,
			"skipped_doses":   sum.SkippedDoses,
			"missed_doses":    sum.MissedDoses,
			"pending_doses":   sum.PendingDoses,
			"adherence_ratio": ratioOrNil(sum.Ratio),
		},
		"medications": items,
		"permissions": map[string]any{
			"can_read":      acc.CanRead,
			"can_write_log": acc.CanWrite,
			// Dihitung dari peran, BUKAN dipatok false.
			//
			// Sebelumnya baris ini menulis `false` dengan komentar "caregiver
			// tidak pernah boleh kelola obat". Itu tidak lagi benar: peran
			// `manager` sudah ada sejak migrasi 00026, dan endpoint kelola
			// obat untuk manager sudah terpasang. Nilai yang dipatok membuat
			// UI menyembunyikan tombol yang seharusnya ada — pengguna tidak
			// bisa mengelola obat pasien meski perannya mengizinkan.
			//
			// `acc.Link` bisa nil (mis. pemilik melihat datanya sendiri),
			// jadi diperiksa dulu seperti di `careRoleOf`.
			"can_manage_meds": acc.Link != nil && acc.Link.Role.CanManageMedications(),
			"role":            careRoleOf(acc),
		},
	})
}

// Medications menangani GET /api/v1/care/{ownerId}/medications.
func (h *CareLinks) Medications(w http.ResponseWriter, r *http.Request) {
	acc, _, ok := h.requireAccess(w, r)
	if !ok {
		return
	}

	meds, err := h.Meds.List(r.Context(), acc.OwnerID, service.ListFilter{
		Status: r.URL.Query().Get("status"),
		Query:  r.URL.Query().Get("q"),
		Limit:  parsePositiveInt(r.URL.Query().Get("limit"), 50),
	})
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	data := make([]map[string]any, 0, len(meds))
	for _, m := range meds {
		// CATATAN PRIVASI: prescription_photo_url dan notes pribadi pasien
		// TIDAK ikut dikirim, sesuai TRD bagian 5.2. Penyaringan dilakukan
		// di sini, bukan di UI: kalau dikirim lalu disembunyikan tampilan,
		// datanya tetap ada di jaringan dan bisa dibaca siapa pun yang
		// memeriksa lalu lintas.
		data = append(data, map[string]any{
			"id":             m.ID,
			"name":           m.Name,
			"dose_value":     m.DoseValue,
			"dose_unit_code": m.DoseUnitCode,
			"strength_note":  m.StrengthNote,
			"route_code":     m.RouteCode,
			"meal_relation":  string(m.MealRelation),
			"as_needed":      m.AsNeeded,
			"start_date":     m.StartDate.Format(time.DateOnly),
			"end_date":       formatDatePtr(m.EndDate),
			"is_active":      m.IsActive,
			"color_tag":      m.ColorTag,
		})
	}

	pohttp.WriteList(w, r, data, pohttp.Page{
		NextCursor: nil,
		HasMore:    false,
		Limit:      len(data),
	})
}

// Adherence menangani GET /api/v1/care/{ownerId}/adherence.
func (h *CareLinks) Adherence(w http.ResponseWriter, r *http.Request) {
	acc, _, ok := h.requireAccess(w, r)
	if !ok {
		return
	}

	rng, err := h.parseRange(r, nil)
	if err != nil {
		writeMedicationValidation(w, r, err)
		return
	}

	rows, err := h.Adh.Daily(r.Context(), acc.OwnerID, rng)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	data := make([]map[string]any, 0, len(rows))
	for _, d := range rows {
		data = append(data, map[string]any{
			"date":            d.Date.Format(time.DateOnly),
			"scheduled_doses": d.ScheduledDoses,
			"taken_doses":     d.TakenDoses,
			"skipped_doses":   d.SkippedDoses,
			"missed_doses":    d.MissedDoses,
			"pending_doses":   d.PendingDoses,
			"adherence_ratio": ratioOrNil(d.Ratio),
		})
	}

	pohttp.WriteList(w, r, data, pohttp.Page{
		NextCursor: nil,
		HasMore:    false,
		Limit:      len(data),
	})
}

// ListLogs menangani GET /api/v1/care/{ownerId}/logs.
func (h *CareLinks) ListLogs(w http.ResponseWriter, r *http.Request) {
	acc, _, ok := h.requireAccess(w, r)
	if !ok {
		return
	}

	f, err := h.parseLogFilter(r)
	if err != nil {
		writeMedicationValidation(w, r, err)
		return
	}

	logs, err := h.LogSvc.List(r.Context(), acc.OwnerID, f)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	data := make([]map[string]any, 0, len(logs))
	for _, l := range logs {
		// catatan bebas (note) pasien TIDAK ikut dikirim ke caregiver,
		// sesuai TRD bagian 5.2. Yang dikirim hanya waktunya, statusnya,
		// dan siapa yang mencatat.
		entry := map[string]any{
			"id":            l.ID,
			"medication_id": l.MedicationID,
			"schedule_id":   l.ScheduleID,
			"slot_date":     l.SlotDate.Format(time.DateOnly),
			"status":        string(l.Status),
			"source":        string(l.Source),
			"recorded_by":   l.RecordedBy,
			"taken_at":      formatTimePtr(l.TakenAt),
			"created_at":    l.CreatedAt.UTC().Format(time.RFC3339),
		}
		if l.ScheduledHour != nil && l.ScheduledMinute != nil {
			entry["scheduled_time"] = domain.FormatClock(*l.ScheduledHour, *l.ScheduledMinute)
		}
		data = append(data, entry)
	}

	pohttp.WriteList(w, r, data, pohttp.Page{
		NextCursor: nil,
		HasMore:    false,
		Limit:      f.Limit,
	})
}

// Schedule menangani GET /api/v1/care/{ownerId}/schedule.
//
// KENAPA ENDPOINT INI ADA
//
// `GET /care/{ownerId}/logs` hanya mengembalikan dosis yang SUDAH dicatat.
// Tanpa jadwal, caregiver tidak bisa melihat dosis yang belum dicatat —
// padahal justru itulah yang perlu ditindak: pasien lupa minum, dan
// pendampingnya tidak punya cara mengetahuinya.
//
// Endpoint `POST /care/{ownerId}/logs` juga butuh `schedule_id`, yang hanya
// bisa didapat dari sini. Jadi tanpa endpoint ini, mencatat atas nama pasien
// tidak mungkin dilakukan sama sekali.
//
// Perhitungannya memakai service Schedule yang sama dengan jadwal milik
// pengguna sendiri, dengan `OwnerID` = pasien. Bentuk responsnya identik
// (`slotResponse`), sehingga klien bisa memakai tipe `DaySlot` yang sama.
func (h *CareLinks) Schedule(w http.ResponseWriter, r *http.Request) {
	acc, owner, ok := h.requireAccess(w, r)
	if !ok {
		return
	}

	// Dosis terlewat TIDAK dihitung ulang di sini: penandaan itu tugas
	// worker, dan melakukannya saat membaca akan mengubah data pada operasi
	// GET. Yang ditampilkan adalah keadaan apa adanya.
	in := service.DayInput{OwnerID: acc.OwnerID, Timezone: owner.Timezone}

	if raw := r.URL.Query().Get("date"); raw != "" {
		d, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			writeMedicationValidation(w, r, errInvalidField("date", "tanggal harus berbentuk YYYY-MM-DD"))
			return
		}
		in.Date = d
	}

	slots, err := h.Sched.Day(r.Context(), in)
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

// CreateLog menangani POST /api/v1/care/{ownerId}/logs.
//
// Hanya helper yang boleh. Viewer ditolak 403 dengan pesan yang membedakan
// "bukan caregiver" dari "caregiver tapi viewer".
func (h *CareLinks) CreateLog(w http.ResponseWriter, r *http.Request) {
	actor := pohttp.IdentityFrom(r.Context())
	if actor == nil {
		unauthorized(w, r)
		return
	}

	ownerID := chi.URLParam(r, "ownerId")

	acc, err := h.Svc.RequireWrite(r.Context(), actor.UserID, ownerID)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	var req careLogRequest
	if err := decodeBodyWithRules(w, r, &req, nil); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	// `medication_id` boleh tidak dikirim: ia bisa diambil dari `schedule_id`.
	//
	// Sebelumnya field ini WAJIB, dan klien yang hanya mengirim `schedule_id`
	// — bentuk yang paling wajar, karena id jadwal sudah menunjuk satu obat —
	// mendapat galat 500 yang menyalahkan server untuk parameter yang kurang.
	// Sekarang jalur ini melengkapinya sendiri, dan hanya menolak bila
	// KEDUANYA kosong.
	if req.MedicationID == "" {
		if req.ScheduleID == "" {
			writeMedicationValidation(w, r,
				errInvalidField("schedule_id", "schedule_id atau medication_id wajib diisi"))
			return
		}
		ownerOfSlot, medOfSlot, err := h.Meds.OwnerOfSchedule(r.Context(), req.ScheduleID)
		if err != nil {
			h.writeErr(w, r, err)
			return
		}
		if ownerOfSlot != acc.OwnerID {
			// Jadwal milik orang lain tidak boleh dipakai, meski caregiver
			// punya akses ke pasien lain. Tanpa pemeriksaan ini, seorang
			// caregiver bisa mencatat dosis pada jadwal pasien yang bukan
			// tanggung jawabnya hanya dengan menebak id.
			h.writeErr(w, r, domain.ErrNotFound)
			return
		}
		req.MedicationID = medOfSlot
	}

	in, err := toCareLogInput(req)
	if err != nil {
		writeMedicationValidation(w, r, err)
		return
	}

	// recorded_by dan source diisi oleh jalur ini, bukan oleh klien.
	// Kalau klien yang menentukan, catatan atas nama pasien bisa menyamar
	// sebagai catatan pasien sendiri.
	res, err := h.LogSvc.RecordFor(r.Context(), acc.OwnerID, actor.UserID, req.MedicationID, in, metaOf(r))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusCreated, map[string]any{
		"log": logResponseForCare(res.Log),
		"stock": map[string]any{
			"consumed":      res.Stock != nil && res.Stock.Consumed,
			"qty_delta":     stockDelta(res),
			"balance_after": stockBalance(res),
		},
		"recorded_as": "caregiver",
	})
}

// -----------------------------------------------------------------------------
// Helper
// -----------------------------------------------------------------------------

// requireAccess memeriksa hak akses dan mengembalikan data pemilik.
func (h *CareLinks) requireAccess(w http.ResponseWriter, r *http.Request) (*service.Access, *domain.User, bool) {
	actor := pohttp.IdentityFrom(r.Context())
	if actor == nil {
		unauthorized(w, r)
		return nil, nil, false
	}

	acc, err := h.Svc.RequireAccess(r.Context(), actor.UserID, chi.URLParam(r, "ownerId"))
	if err != nil {
		h.writeErr(w, r, err)
		return nil, nil, false
	}

	owner, err := h.Users.Profile(r.Context(), acc.OwnerID)
	if err != nil {
		h.writeErr(w, r, err)
		return nil, nil, false
	}
	return acc, owner, true
}

// parseRange mengurai rentang tanggal, memakai zona waktu pemilik.
func (h *CareLinks) parseRange(r *http.Request, owner *domain.User) (service.Range, error) {
	loc := time.UTC
	if owner != nil && owner.Timezone != "" {
		if l, err := time.LoadLocation(owner.Timezone); err == nil {
			loc = l
		}
	}

	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	out := service.Range{From: today.AddDate(0, 0, -6), To: today}

	q := r.URL.Query()
	if raw := q.Get("from"); raw != "" {
		d, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			return out, errInvalidField("from", "tanggal harus berbentuk YYYY-MM-DD")
		}
		out.From = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc)
	}
	if raw := q.Get("to"); raw != "" {
		d, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			return out, errInvalidField("to", "tanggal harus berbentuk YYYY-MM-DD")
		}
		out.To = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc)
	}
	if out.To.Before(out.From) {
		return out, errInvalidField("to", "tanggal akhir tidak boleh sebelum tanggal awal")
	}
	return out, nil
}

// parseLogFilter mengurai filter daftar log.
func (h *CareLinks) parseLogFilter(r *http.Request) (service.LogListFilter, error) {
	q := r.URL.Query()
	f := service.LogListFilter{
		Status: q.Get("status"),
		Limit:  parsePositiveInt(q.Get("limit"), 50),
	}
	if f.Status != "" && !domain.LogStatus(f.Status).Valid() {
		return f, errInvalidField("status", "status tidak dikenal")
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
	return f, nil
}

// careLinkResponse menyusun balasan relasi caregiver.
//
// invite_email dibiarkan tampil: itu email yang diundang, bukan data
// kesehatan, dan pemilik perlu tahu siapa yang diundang.
func careLinkResponse(c *postgres.CareLink) map[string]any {
	out := map[string]any{
		"id":           c.ID,
		"owner_id":     c.OwnerID,
		"caregiver_id": c.CaregiverID,
		"invite_email": c.InviteEmail,
		"relation":     string(c.Relation),
		"role":         string(c.Role),
		"status":       string(c.Status),
		"created_at":   c.CreatedAt.UTC().Format(time.RFC3339),
	}
	if c.InviteExpiresAt != nil {
		out["invite_expires_at"] = c.InviteExpiresAt.UTC().Format(time.RFC3339)
	} else {
		out["invite_expires_at"] = nil
	}
	if c.DecidedAt != nil {
		out["decided_at"] = c.DecidedAt.UTC().Format(time.RFC3339)
	} else {
		out["decided_at"] = nil
	}
	return out
}

func careRoleOf(acc *service.Access) any {
	if acc.IsOwner {
		return "owner"
	}
	if acc.Link != nil {
		return string(acc.Link.Role)
	}
	return nil
}

func ratioOrNil(r *float64) any {
	if r == nil {
		return nil
	}
	return *r
}

func stockDelta(res *service.RecordResult) float64 {
	if res.Stock == nil {
		return 0
	}
	return res.Stock.QtyDelta
}

func stockBalance(res *service.RecordResult) float64 {
	if res.Stock == nil {
		return 0
	}
	return res.Stock.BalanceAfter
}

// writeErr memetakan error service ke balasan HTTP.
func (h *CareLinks) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	if resp, ok := validationResponse(err); ok {
		pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeValidationFailed,
			resp.message, resp.details)
		return
	}
	if !isKnownDomainError(err) {
		h.Logger.ErrorContext(r.Context(), "kesalahan tak terduga pada caregiver",
			slog.String("error", err.Error()))
	}
	pohttp.WriteDomainError(w, r, err)
}

// toCareLogInput memetakan bentuk kabel ke input service.
func toCareLogInput(req careLogRequest) (service.RecordInput, error) {
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

// logResponseForCare menyusun balasan catatan dosis untuk jalur caregiver.
//
// Berbeda dari logResponse biasa: catatan bebas pasien (note) TIDAK ikut
// dikirim. Caregiver perlu tahu APAKAH dosis diminum dan kapan, bukan apa
// yang ditulis pasien tentang dirinya sendiri.
func logResponseForCare(l *domain.MedicationLog) map[string]any {
	out := map[string]any{
		"id":            l.ID,
		"medication_id": l.MedicationID,
		"schedule_id":   l.ScheduleID,
		"owner_id":      l.OwnerID,
		"recorded_by":   l.RecordedBy,
		"slot_date":     l.SlotDate.Format(time.DateOnly),
		"status":        string(l.Status),
		"source":        string(l.Source),
		"taken_at":      formatTimePtr(l.TakenAt),
		"created_at":    l.CreatedAt.UTC().Format(time.RFC3339),
	}
	if l.ScheduledHour != nil && l.ScheduledMinute != nil {
		out["scheduled_time"] = domain.FormatClock(*l.ScheduledHour, *l.ScheduledMinute)
	}
	return out
}
