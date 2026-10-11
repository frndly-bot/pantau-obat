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

// Adherence adalah handler untuk laporan kepatuhan.
type Adherence struct {
	Svc    *service.Adherence
	Users  *service.Users
	Logger *slog.Logger
}

// NewAdherence membuat handler kepatuhan.
func NewAdherence(svc *service.Adherence, users *service.Users, logger *slog.Logger) *Adherence {
	return &Adherence{Svc: svc, Users: users, Logger: logger}
}

// Routes mendaftarkan endpoint kepatuhan.
func (h *Adherence) Routes(r chi.Router) {
	r.Get("/medications/{id}/adherence", h.ByMedication)
	r.Get("/adherence", h.Summary)
	r.Get("/adherence/daily", h.Daily)
}

// Summary menangani GET /api/v1/adherence.
func (h *Adherence) Summary(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	rng, err := h.parseRange(r, id.UserID)
	if err != nil {
		writeMedicationValidation(w, r, err)
		return
	}

	sum, err := h.Svc.Summary(r.Context(), id.UserID, rng)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, summaryResponse(sum))
}

// ByMedication menangani GET /api/v1/medications/{id}/adherence.
func (h *Adherence) ByMedication(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	rng, err := h.parseRange(r, id.UserID)
	if err != nil {
		writeMedicationValidation(w, r, err)
		return
	}

	target := chi.URLParam(r, "id")

	// Kepatuhan dihitung untuk SELURUH obat, lalu disaring ke satu obat.
	//
	// Memakai satu perhitungan untuk semua, bukan perhitungan khusus satu
	// obat, supaya angkanya dijamin sama dengan yang muncul di ringkasan.
	// Kalau ada dua jalur perhitungan, cepat atau lambat keduanya berbeda,
	// dan user melihat angka yang bertentangan untuk obat yang sama.
	all, err := h.Svc.ByMedication(r.Context(), id.UserID, rng)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	for i := range all {
		if all[i].MedicationID == target {
			pohttp.WriteJSON(w, r, http.StatusOK, byMedicationResponse(&all[i]))
			return
		}
	}

	// Obat tidak muncul di hasil: bisa karena bukan milik user, atau karena
	// tidak punya dosis terjadwal pada rentang itu (mis. PRN). Dibedakan
	// dengan memeriksa keberadaan obatnya.
	if _, err := h.Users.Profile(r.Context(), id.UserID); err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteError(w, r, http.StatusNotFound, pohttp.CodeNotFound,
		"obat tidak ditemukan atau tidak punya dosis terjadwal pada rentang itu", nil)
}

// Daily menangani GET /api/v1/adherence/daily.
func (h *Adherence) Daily(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	rng, err := h.parseRange(r, id.UserID)
	if err != nil {
		writeMedicationValidation(w, r, err)
		return
	}

	rows, err := h.Svc.Daily(r.Context(), id.UserID, rng)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	data := make([]map[string]any, 0, len(rows))
	for _, d := range rows {
		item := map[string]any{
			"date":            d.Date.Format(time.DateOnly),
			"scheduled_doses": d.ScheduledDoses,
			"taken_doses":     d.TakenDoses,
			"skipped_doses":   d.SkippedDoses,
			"missed_doses":    d.MissedDoses,
			"pending_doses":   d.PendingDoses,
			"logged_doses":    d.LoggedDoses,
		}
		if d.Ratio != nil {
			item["adherence_ratio"] = *d.Ratio
		} else {
			item["adherence_ratio"] = nil
		}
		data = append(data, item)
	}

	pohttp.WriteList(w, r, data, pohttp.Page{
		NextCursor: nil,
		HasMore:    false,
		Limit:      len(data),
	})
}

// parseRange mengurai rentang tanggal dari query string.
//
// Rentang bawaan 7 hari terakhir, dihitung memakai zona waktu user. Tanpa
// itu, pukul 06:00 WIB tanggal 10 Oktober masih dianggap 9 Oktober karena
// UTC-nya belum lewat tengah malam.
//
// Rentang bawaan DISESUAIKAN ke masa terapi dalam dua hal saja:
//
//  1. Kalau terapi baru dimulai di dalam rentang, awal rentang dimajukan
//     ke tanggal itu. Menghitung hari sebelum terapi dimulai akan menambah
//     penyebut untuk dosis yang belum pernah dijadwalkan.
//
//  2. Kalau SEMUA terapi sudah berakhir, akhir rentang dimundurkan ke hari
//     terakhir terapi. Setelah terapi usai, tidak ada dosis yang seharusnya
//     diminum, jadi menghitung hari sesudahnya sebagai terlewat adalah
//     tuduhan yang salah.
//
// Yang TIDAK dilakukan: memundurkan akhir rentang untuk terapi yang masih
// berjalan. Pasien dengan obat harian yang berhenti mencatat sejak berhari
// hari lalu memang pantas mendapat rasio rendah — itu sinyal yang justru
// dibutuhkan caregiver untuk tahu pasiennya berhenti minum obat.
func (h *Adherence) parseRange(r *http.Request, userID string) (service.Range, error) {
	loc := time.UTC
	if u, err := h.Users.Profile(r.Context(), userID); err == nil && u.Timezone != "" {
		if l, err := time.LoadLocation(u.Timezone); err == nil {
			loc = l
		}
	}

	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	out := service.Range{
		From: today.AddDate(0, 0, -6),
		To:   today,
	}

	q := r.URL.Query()
	fromSet := q.Get("from") != ""
	toSet := q.Get("to") != ""

	if fromSet {
		d, err := time.Parse(time.DateOnly, q.Get("from"))
		if err != nil {
			return out, errInvalidField("from", "tanggal harus berbentuk YYYY-MM-DD")
		}
		out.From = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc)
	}
	if toSet {
		d, err := time.Parse(time.DateOnly, q.Get("to"))
		if err != nil {
			return out, errInvalidField("to", "tanggal harus berbentuk YYYY-MM-DD")
		}
		out.To = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc)
	}

	if out.To.Before(out.From) {
		return out, errInvalidField("to", "tanggal akhir tidak boleh sebelum tanggal awal")
	}

	// Pembatasan rentang hanya berlaku untuk TERAPI YANG SUDAH BERAKHIR,
	// bukan untuk semua rentang bawaan.
	//
	// Alasannya penting dan sempat saya salah pahami: pasien yang punya obat
	// harian berjalan tapi berhenti mencatat sejak berhari-hari lalu
	// MENDAPAT rasio 0%. Itu jawaban yang BENAR, bukan bug. Terapinya masih
	// berjalan, dosisnya memang seharusnya diminum, dan ia tidak mencatatnya.
	// Justru angka itulah yang membuat caregiver bisa melihat pasiennya
	// berhenti minum obat — nilai utama aplikasi ini.
	//
	// Yang benar-benar perlu dipotong adalah terapi yang endpoint_date-nya
	// sudah lewat. Setelah terapi selesai, tidak ada lagi dosis yang
	// seharusnya diminum, sehingga menghitung hari-hari sesudahnya sebagai
	// terlewat adalah tuduhan yang salah.
	if !fromSet || !toSet {
		span := h.therapySpan(r, userID, today)
		if span != nil {
			// Mulai dari tanggal terapi pertama yang masih di dalam rentang
			// bawaan, TAPI hanya bila terapi sudah berakhir. Untuk terapi
			// berjalan, rentangnya dibiarkan utuh.
			if span.Ended && !fromSet && span.Earliest.After(out.From) {
				out.From = span.Earliest
			}

			// Akhiri rentang di hari terakhir terapi, tapi hanya bila
			// terapi itu memang sudah selesai.
			if span.Ended && !toSet && span.Latest.Before(out.To) {
				out.To = span.Latest
			}

			// Terapi berjalan: batas atas tetap "hari ini", dan batas bawah
			// tetap awal rentang bawaan. Rasio 0% untuk pasien yang berhenti
			// mencatat itu disengaja.
			if !span.Ended && !toSet && span.Earliest.After(out.From) {
				// Kecuali terapi baru dimulai di dalam rentang. Menghitung hari
				// sebelum terapi dimulai akan menambah penyebut untuk dosis
				// yang belum pernah dijadwalkan.
				out.From = span.Earliest
			}
		}
	}

	if out.To.Before(out.From) {
		// Bisa terjadi kalau terapi terakhir sudah lewat jauh sebelum awal
		// rentang bawaan. Dalam kasus itu, tampilkan rentang satu hari di
		// sekitar dosis terakhir, bukan rentang kosong yang membingungkan.
		out.From = out.To
	}

	return out, nil
}

// therapySpan menghitung kapan terapi user mulai dan berakhir.
//
// Mengembalikan nil bila user tidak punya obat dengan jadwal terjadwal.
// Dalam kasus itu, rentang bawaan dibiarkan apa adanya — tidak ada gunanya
// mempersempit rentang untuk data yang memang tidak ada.
//
// Kegagalan membaca rentang TIDAK menggagalkan laporan: rentang bawaan
// dipakai apa adanya. Laporan yang sedikit lebih lebar jauh lebih berguna
// daripada tidak ada laporan sama sekali.
func (h *Adherence) therapySpan(r *http.Request, userID string, today time.Time) *service.TherapySpan {
	span, err := h.Svc.TherapySpan(r.Context(), userID, today)
	if err != nil {
		h.Logger.WarnContext(r.Context(), "tidak bisa membaca rentang terapi",
			slog.String("error", err.Error()))
		return nil
	}
	return span
}

// summaryResponse menyusun balasan ringkasan kepatuhan.
func summaryResponse(s *domain.AdherenceSummary) map[string]any {
	out := map[string]any{
		"from":            s.From.Format(time.DateOnly),
		"to":              s.To.Format(time.DateOnly),
		"scheduled_doses": s.ScheduledDoses,
		"taken_doses":     s.TakenDoses,
		"late_doses":      s.LateDoses,
		"skipped_doses":   s.SkippedDoses,
		"missed_doses":    s.MissedDoses,
		"pending_doses":   s.PendingDoses,
	}
	if s.Ratio != nil {
		out["adherence_ratio"] = *s.Ratio
	} else {
		// null, bukan 0. Nol berarti "semua terlewat"; null berarti
		// "belum ada dosis yang keadaannya pasti".
		out["adherence_ratio"] = nil
	}
	return out
}

// byMedicationResponse menyusun balasan kepatuhan satu obat.
func byMedicationResponse(m *domain.AdherenceByMedication) map[string]any {
	out := map[string]any{
		"medication_id":   m.MedicationID,
		"medication_name": m.MedicationName,
		"color_tag":       m.ColorTag,
		"scheduled_doses": m.ScheduledDoses,
		"taken_doses":     m.TakenDoses,
		"late_doses":      m.LateDoses,
		"skipped_doses":   m.SkippedDoses,
		"missed_doses":    m.MissedDoses,
		"pending_doses":   m.PendingDoses,
	}
	if m.Ratio != nil {
		out["adherence_ratio"] = *m.Ratio
	} else {
		out["adherence_ratio"] = nil
	}
	return out
}

// writeErr memetakan error service ke balasan HTTP.
func (h *Adherence) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	if !isKnownDomainError(err) {
		h.Logger.ErrorContext(r.Context(), "kesalahan tak terduga pada kepatuhan",
			slog.String("error", err.Error()))
	}
	pohttp.WriteDomainError(w, r, err)
}
