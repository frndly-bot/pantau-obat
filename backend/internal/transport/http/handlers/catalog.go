package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/repo/postgres"
	"github.com/frndly-bot/pantau-obat/backend/internal/service"
	pohttp "github.com/frndly-bot/pantau-obat/backend/internal/transport/http"
)

// Catalog adalah handler untuk /catalog/*.
type Catalog struct {
	Svc    *service.Catalog
	Logger *slog.Logger
}

// NewCatalog membuat handler katalog.
func NewCatalog(svc *service.Catalog, logger *slog.Logger) *Catalog {
	return &Catalog{Svc: svc, Logger: logger}
}

// Routes mendaftarkan endpoint katalog.
func (h *Catalog) Routes(r chi.Router) {
	r.Get("/catalog", h.Search)
	r.Get("/catalog/suggest", h.Suggest)
	r.Post("/catalog", h.Create)
	r.Get("/catalog/{id}", h.Get)
	r.Patch("/catalog/{id}", h.Update)
	r.Delete("/catalog/{id}", h.Delete)
}

// -----------------------------------------------------------------------------
// Bentuk kabel
// -----------------------------------------------------------------------------

type createCatalogRequest struct {
	DisplayName    string  `json:"display_name"`
	DosageFormCode *string `json:"dosage_form_code"`
	RouteCode      *string `json:"route_code"`
	UcumCode       *string `json:"ucum_code"`
	UomName        *string `json:"uom_name"`
	DoseValueText  *string `json:"dose_value_text"`
}

type updateCatalogRequest struct {
	DisplayName    *string `json:"display_name"`
	DosageFormCode *string `json:"dosage_form_code"`
	RouteCode      *string `json:"route_code"`
	UcumCode       *string `json:"ucum_code"`
	UomName        *string `json:"uom_name"`
	DoseValueText  *string `json:"dose_value_text"`
}

// Search menangani GET /api/v1/catalog.
func (h *Catalog) Search(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	q := r.URL.Query()
	entries, err := h.Svc.Search(r.Context(), id.UserID, service.SearchInput{
		Query:     q.Get("q"),
		FormCode:  q.Get("form"),
		RouteCode: q.Get("route"),
		Limit:     parsePositiveInt(q.Get("limit"), 20),
	})
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	data := make([]map[string]any, 0, len(entries))
	for i := range entries {
		// Daftar pencarian TIDAK membawa seluruh 40 kolom. Yang dibutuhkan
		// untuk memilih obat hanya nama, bentuk, rute, dan dosisnya.
		// Mengirim deskripsi, indikasi, dan peringatan untuk 20 hasil
		// sekaligus akan membebani jaringan tanpa manfaat.
		data = append(data, catalogSummary(&entries[i]))
	}

	pohttp.WriteList(w, r, data, pohttp.Page{
		NextCursor: nil,
		HasMore:    len(entries) == parsePositiveInt(q.Get("limit"), 20),
		Limit:      parsePositiveInt(q.Get("limit"), 20),
	})
}

// Suggest menangani GET /api/v1/catalog/suggest.
func (h *Catalog) Suggest(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	items, err := h.Svc.Suggest(r.Context(), id.UserID,
		r.URL.Query().Get("q"),
		parsePositiveInt(r.URL.Query().Get("limit"), 10))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	data := make([]map[string]any, 0, len(items))
	for _, s := range items {
		data = append(data, map[string]any{
			"id":               s.ID,
			"display_name":     s.DisplayName,
			"dosage_form_name": s.DosageFormName,
			"route_name":       s.RouteName,
			"is_preset":        s.IsPreset,
		})
	}

	pohttp.WriteList(w, r, data, pohttp.Page{
		NextCursor: nil,
		HasMore:    false,
		Limit:      len(data),
	})
}

// Create menangani POST /api/v1/catalog.
//
// Entri yang dibuat SELALU katalog pribadi. Tidak ada field is_preset,
// owner_id, atau curation_status yang dibaca dari body — ketiganya
// ditetapkan server. Kalau klien bisa mengirimnya, ia bisa membuat entri
// yang mengaku preset publik dan entri itu akan muncul untuk semua user.
func (h *Catalog) Create(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	var req createCatalogRequest
	if err := decodeAuthBody(w, r, &req); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	entry, err := h.Svc.Create(r.Context(), id.UserID, service.CreateInput{
		DisplayName:    req.DisplayName,
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

	pohttp.WriteJSON(w, r, http.StatusCreated, catalogDetail(entry))
}

// Get menangani GET /api/v1/catalog/{id}.
func (h *Catalog) Get(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	entry, err := h.Svc.Get(r.Context(), id.UserID, chi.URLParam(r, "id"))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, catalogDetail(entry))
}

// Update menangani PATCH /api/v1/catalog/{id}.
//
// Mengubah preset publik membalas 409 CATALOG_IMMUTABLE dengan pesan yang
// menjelaskan, bukan diabaikan diam-diam. Perubahan yang tidak terjadi tanpa
// penjelasan adalah pengalaman yang membingungkan.
func (h *Catalog) Update(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	var req updateCatalogRequest
	if err := decodeAuthBody(w, r, &req); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	entry, err := h.Svc.Update(r.Context(), id.UserID, chi.URLParam(r, "id"),
		service.UpdateInput{
			DisplayName:    req.DisplayName,
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

	pohttp.WriteJSON(w, r, http.StatusOK, catalogDetail(entry))
}

// Delete menangani DELETE /api/v1/catalog/{id}.
func (h *Catalog) Delete(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	if err := h.Svc.Delete(r.Context(), id.UserID,
		chi.URLParam(r, "id"), metaOf(r)); err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, map[string]any{
		"deleted":          true,
		"medications_safe": true,
		"note":             "obat yang dibuat dari entri ini tidak terpengaruh",
	})
}

// -----------------------------------------------------------------------------
// Balasan
// -----------------------------------------------------------------------------

// catalogSummary menyusun ringkasan entri untuk daftar pencarian.
func catalogSummary(e *postgres.CatalogEntry) map[string]any {
	return map[string]any{
		"id":               e.ID,
		"display_name":     e.DisplayName,
		"nama_dagang":      e.NamaDagang,
		"generik_nama":     e.GenerikNama,
		"is_preset":        e.IsPreset,
		"curation_status":  string(e.CurationStatus),
		"source":           string(e.Source),
		"dosage_form_name": e.DosageFormName,
		"route_name":       e.RouteName,
		"dose_value_text":  e.DoseValueText,
		"ucum_code":        e.UcumCode,
		"uom_name":         e.UomName,
		"atc_l5_code":      e.ATCL5Code,
	}
}

// catalogDetail menyusun balasan lengkap satu entri.
//
// Kolom null tetap dikirim bila kolomnya ada di skema, sesuai konvensi
// api-v2 bagian 1 butir 8, sehingga klien tidak perlu menebak field mana
// yang ada.
func catalogDetail(e *postgres.CatalogEntry) map[string]any {
	out := catalogSummary(e)

	out["is_generik"] = e.IsGenerik
	out["rxterm"] = e.Rxterm
	out["manufacturer"] = e.Manufacturer
	out["registrar"] = e.Registrar
	out["produksi_buatan"] = e.ProduksiBuatan
	out["country_of_origin"] = e.CountryOfOrigin
	out["dosage_form_code"] = e.DosageFormCode
	out["route_code"] = e.RouteCode
	out["controlled_code"] = e.ControlledCode
	out["controlled_name"] = e.ControlledName
	out["atc_l5_name"] = e.ATCL5Name
	out["indication"] = e.Indication
	out["warning"] = e.Warning
	out["side_effect"] = e.SideEffect
	out["description"] = e.Description
	out["dosage_usage"] = e.DosageUsage
	out["storage_note"] = e.StorageNote
	out["is_discontinued"] = e.IsDiscontinued

	// Kode identitas hanya ditampilkan untuk preset publik.
	//
	// Katalog pribadi tidak punya kode KFA/BPOM yang sah — ia dibuat user
	// sendiri. Menampilkan kolomnya sebagai null akan mengundang pertanyaan
	// "kenapa kosong", padahal memang tidak berlaku.
	if e.IsPreset {
		out["kfa_code"] = e.KFACode
		out["bpom_nie"] = e.BpomNIE
		out["barcode"] = e.Barcode
		out["kode_lkpp"] = e.KodeLKPP
	}

	out["owner_id"] = e.OwnerID
	out["created_at"] = e.CreatedAt.UTC().Format(time.RFC3339)
	out["updated_at"] = e.UpdatedAt.UTC().Format(time.RFC3339)

	return out
}

// writeErr memetakan error service ke balasan HTTP.
func (h *Catalog) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	if resp, ok := validationResponse(err); ok {
		pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeValidationFailed,
			resp.message, resp.details)
		return
	}
	if !isKnownDomainError(err) {
		h.Logger.ErrorContext(r.Context(), "kesalahan tak terduga pada katalog",
			slog.String("error", err.Error()))
	}
	pohttp.WriteDomainError(w, r, err)
}
