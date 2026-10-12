package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	pohttp "github.com/frndly-bot/pantau-obat/backend/internal/transport/http"
)

// File ini menangani ekspor CSV riwayat pemakaian (TRD H2).
//
// Dua rute, satu bentuk file:
//   - GET /logs/export            — pasien mengekspor miliknya sendiri.
//   - GET /care/{ownerId}/logs/export — caregiver mengekspor data pasien.
//
// Jalur caregiver DIBERIKAN LEBIH KETAT dari yang bisa dibayangkan: ekspor
// mengeluarkan riwayat kesehatan dari sistem, jadi hanya manager yang boleh,
// dan setiap ekspor menulis audit_log dengan alasan (reason) yang wajib
// diisi. Tanpa alasan, jejak "siapa mengekspor apa" tidak bisa diaudit.
//
// Format file: CSV + BOM UTF-8, dibuka di Excel/Spreadsheet tanpa dialog.
// Nama file menyertakan rentangnya supaya tidak tertukar saat beberapa
// ekspor diunduh berurutan.

// exportRequest adalah parameter kedua jalur ekspor.
type exportRequest struct {
	From   time.Time
	To     time.Time
	Reason *string // wajib untuk jalur caregiver
}

// parseExportRequest mengurai dan memvalidasi parameter ekspor.
//
// from/to wajib keduanya. Batas 366 hari ditegakkan di service; di sini
// cukup memastikan bentuk dan urutannya benar.
func parseExportRequest(w http.ResponseWriter, r *http.Request) (exportRequest, bool) {
	q := r.URL.Query()
	var req exportRequest

	if raw := q.Get("from"); raw == "" {
		pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeValidationFailed,
			"parameter from wajib diisi", []pohttp.FieldIssue{{Field: "from", Issue: "invalid"}})
		return req, false
	} else {
		d, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeValidationFailed,
				"parameter from harus berbentuk YYYY-MM-DD", []pohttp.FieldIssue{{Field: "from", Issue: "invalid"}})
			return req, false
		}
		req.From = d
	}
	if raw := q.Get("to"); raw == "" {
		pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeValidationFailed,
			"parameter to wajib diisi", []pohttp.FieldIssue{{Field: "to", Issue: "invalid"}})
		return req, false
	} else {
		d, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeValidationFailed,
				"parameter to harus berbentuk YYYY-MM-DD", []pohttp.FieldIssue{{Field: "to", Issue: "invalid"}})
			return req, false
		}
		req.To = d
	}
	if req.To.Before(req.From) {
		pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeValidationFailed,
			"tanggal akhir tidak boleh sebelum tanggal awal", []pohttp.FieldIssue{{Field: "to", Issue: "invalid"}})
		return req, false
	}
	if raw := q.Get("reason"); raw != "" {
		req.Reason = &raw
	}
	return req, true
}

// writeCSVHeaders memasang header HTTP untuk file CSV unduhan.
func writeCSVHeaders(w http.ResponseWriter, filename string) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
}

// Export menangani GET /api/v1/logs/export.
//
// HANYA milik sendiri (TRD H2 AC kedua). Rentang dibatasi 366 hari di
// service; di sini cukup memastikan parameter bentuknya benar.
func (h *Logs) Export(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	req, ok := parseExportRequest(w, r)
	if !ok {
		return
	}

	writeCSVHeaders(w, "pantau-obat-riwayat-"+req.From.Format("20060102")+"-"+req.To.Format("20060102")+".csv")
	if err := h.Svc.ExportCSV(r.Context(), w, id.UserID, req.From, req.To); err != nil {
		// Header sudah terkirim; tidak ada yang bisa diperbaiki selain
		// memutus koneksi. Log untuk diselidiki.
		h.Logger.ErrorContext(r.Context(), "ekspor CSV terputus di tengah",
			slog.String("error", err.Error()))
		return
	}
	h.Logger.InfoContext(r.Context(), "riwayat diekspor",
		slog.String("owner", id.UserID),
		slog.String("rentang", req.From.Format(time.DateOnly)+".."+req.To.Format(time.DateOnly)))
}

// ExportForOwner menangani GET /api/v1/care/{ownerId}/logs/export.
//
// HANYA manager, dan WAJIB menyertakan ?reason= — ekspor mengeluarkan data
// kesehatan dari sistem atas nama orang lain, dan itu harus berjejak dengan
// alasan yang bisa dibaca manusia.
func (h *CareLinks) ExportForOwner(w http.ResponseWriter, r *http.Request) {
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

	req, ok := parseExportRequest(w, r)
	if !ok {
		return
	}
	if req.Reason == nil || *req.Reason == "" {
		pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeValidationFailed,
			"parameter reason wajib diisi untuk ekspor data pasien",
			[]pohttp.FieldIssue{{Field: "reason", Issue: "invalid"}})
		return
	}

	if err := h.LogSvc.WriteExportAudit(r.Context(), actor.UserID, acc.OwnerID,
		req.From, req.To, *req.Reason); err != nil {
		h.writeErr(w, r, err)
		return
	}

	writeCSVHeaders(w, "pantau-obat-riwayat-pasien-"+req.From.Format("20060102")+"-"+req.To.Format("20060102")+".csv")
	if err := h.LogSvc.ExportCSV(r.Context(), w, acc.OwnerID, req.From, req.To); err != nil {
		h.Logger.ErrorContext(r.Context(), "ekspor CSV caregiver terputus di tengah",
			slog.String("error", err.Error()))
		return
	}
	h.Logger.InfoContext(r.Context(), "riwayat pasien diekspor caregiver",
		slog.String("actor", actor.UserID),
		slog.String("owner", acc.OwnerID),
		slog.String("rentang", req.From.Format(time.DateOnly)+".."+req.To.Format(time.DateOnly)))
}
