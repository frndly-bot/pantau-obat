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

// Stock adalah handler untuk /medications/{id}/stock/* dan /stock/low.
type Stock struct {
	Svc    *service.Stock
	Logger *slog.Logger
}

// NewStock membuat handler stok.
func NewStock(svc *service.Stock, logger *slog.Logger) *Stock {
	return &Stock{Svc: svc, Logger: logger}
}

// Routes mendaftarkan endpoint stok.
func (h *Stock) Routes(r chi.Router) {
	r.Get("/medications/{id}/stock", h.Get)
	r.Put("/medications/{id}/stock", h.SetSettings)
	r.Get("/medications/{id}/stock/transactions", h.Transactions)
	r.Post("/medications/{id}/stock/restock", h.Restock)
	r.Post("/medications/{id}/stock/adjust", h.Adjust)
	r.Get("/stock/low", h.LowStock)
}

// -----------------------------------------------------------------------------
// Bentuk kabel
// -----------------------------------------------------------------------------

type setStockRequest struct {
	UnitCode           string   `json:"unit_code"`
	InitialQty         float64  `json:"initial_qty"`
	QtyPerDose         float64  `json:"qty_per_dose"`
	RefillThresholdQty *float64 `json:"refill_threshold_qty"`
	RefillReminder     bool     `json:"refill_reminder"`
}

type restockRequest struct {
	Qty  float64 `json:"qty"`
	Note *string `json:"note"`
}

type adjustRequest struct {
	QtyDelta float64 `json:"qty_delta"`
	Kind     string  `json:"kind"`
	Note     string  `json:"note"`
}

// Get menangani GET /api/v1/medications/{id}/stock.
func (h *Stock) Get(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	detail, err := h.Svc.Get(r.Context(), id.UserID, chi.URLParam(r, "id"))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, stockDetailResponse(detail, chi.URLParam(r, "id")))
}

// SetSettings menangani PUT /api/v1/medications/{id}/stock.
//
// PUT, bukan PATCH, karena endpoint ini menggantikan SELURUH pengaturan
// stok. Saldo tidak termasuk; ia hanya berubah lewat transaksi.
func (h *Stock) SetSettings(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	var req setStockRequest
	if err := decodeAuthBody(w, r, &req); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	// refill_threshold_qty null berarti kosongkan ambang, sedangkan field
	// yang tidak dikirim berarti jangan ubah. Karena body di sini
	// menggantikan seluruh pengaturan, null memang berarti kosong.
	clearThreshold := req.RefillThresholdQty == nil

	stock, err := h.Svc.SetSettings(r.Context(), id.UserID, chi.URLParam(r, "id"), service.SettingsInput{
		UnitCode:           req.UnitCode,
		InitialQty:         req.InitialQty,
		QtyPerDose:         req.QtyPerDose,
		RefillThresholdQty: req.RefillThresholdQty,
		ClearThreshold:     clearThreshold,
		RefillReminder:     req.RefillReminder,
	}, metaOf(r))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	medID := chi.URLParam(r, "id")
	pohttp.WriteJSON(w, r, http.StatusOK, map[string]any{
		"medication_id":        medID,
		"unit_code":            stock.UnitCode,
		"initial_qty":          stock.InitialQty,
		"current_qty":          stock.CurrentQty,
		"qty_per_dose":         stock.QtyPerDose,
		"refill_threshold_qty": stock.RefillThresholdQty,
		"refill_reminder":      stock.RefillReminder,
		"doses_remaining":      stock.DosesRemaining(),
		"needs_refill":         stock.NeedsRefill(),
		"last_restocked_at":    formatTimePtr(stock.LastRestockedAt),
	})
}

// Transactions menangani GET /api/v1/medications/{id}/stock/transactions.
func (h *Stock) Transactions(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	limit := parsePositiveInt(r.URL.Query().Get("limit"), 50)

	txns, err := h.Svc.Transactions(r.Context(), id.UserID, chi.URLParam(r, "id"), limit)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	data := make([]map[string]any, 0, len(txns))
	for _, t := range txns {
		data = append(data, map[string]any{
			"id":            t.ID,
			"kind":          t.Kind,
			"qty_delta":     t.QtyDelta,
			"balance_after": t.BalanceAfter,
			"log_id":        t.LogID,
			"note":          t.Note,
			"recorded_by":   t.RecordedBy,
			"created_at":    t.CreatedAt.UTC().Format(time.RFC3339),
		})
	}

	pohttp.WriteList(w, r, data, pohttp.Page{
		NextCursor: nil,
		HasMore:    len(txns) == limit,
		Limit:      limit,
	})
}

// Restock menangani POST /api/v1/medications/{id}/stock/restock.
func (h *Stock) Restock(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	var req restockRequest
	if err := decodeAuthBody(w, r, &req); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	change, err := h.Svc.Restock(r.Context(), id.UserID, chi.URLParam(r, "id"),
		service.RestockInput{Qty: req.Qty, Note: req.Note}, metaOf(r))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusCreated, map[string]any{
		"kind":              string(domain.StockRestock),
		"qty_delta":         change.QtyDelta,
		"balance_after":     change.BalanceAfter,
		"unit_code":         change.UnitCode,
		"needs_refill":      change.NeedsRefill,
		"last_restocked_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// Adjust menangani POST /api/v1/medications/{id}/stock/adjust.
func (h *Stock) Adjust(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	var req adjustRequest
	if err := decodeAuthBody(w, r, &req); err != nil {
		writeAuthDecodeError(w, r, err)
		return
	}

	kind := domain.StockTxnKind(req.Kind)
	if req.Kind == "" {
		kind = domain.StockAdjust
	}

	change, err := h.Svc.Adjust(r.Context(), id.UserID, chi.URLParam(r, "id"), service.AdjustInput{
		QtyDelta: req.QtyDelta,
		Kind:     kind,
		Note:     req.Note,
	}, metaOf(r))
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusCreated, map[string]any{
		"kind":          string(kind),
		"qty_delta":     change.QtyDelta,
		"balance_after": change.BalanceAfter,
		"unit_code":     change.UnitCode,
		"needs_refill":  change.NeedsRefill,
	})
}

// LowStock menangani GET /api/v1/stock/low.
func (h *Stock) LowStock(w http.ResponseWriter, r *http.Request) {
	id := pohttp.IdentityFrom(r.Context())
	if id == nil {
		unauthorized(w, r)
		return
	}

	rows, err := h.Svc.LowStock(r.Context(), id.UserID)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	data := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		data = append(data, map[string]any{
			"medication_id":        row.MedicationID,
			"medication_name":      row.MedicationName,
			"unit_code":            row.UnitCode,
			"current_qty":          row.CurrentQty,
			"qty_per_dose":         row.QtyPerDose,
			"refill_threshold_qty": row.RefillThresholdQty,
			"doses_remaining":      row.DosesRemaining,
			"needs_refill":         true,
		})
	}

	pohttp.WriteList(w, r, data, pohttp.Page{
		NextCursor: nil,
		HasMore:    false,
		Limit:      len(data),
	})
}

// stockDetailResponse menyusun balasan detail stok.
func stockDetailResponse(d *service.Detail, medicationID string) map[string]any {
	s := d.Stock
	return map[string]any{
		"medication_id":        medicationID,
		"unit_code":            s.UnitCode,
		"initial_qty":          s.InitialQty,
		"current_qty":          s.CurrentQty,
		"qty_per_dose":         s.QtyPerDose,
		"refill_threshold_qty": s.RefillThresholdQty,
		"refill_reminder":      s.RefillReminder,
		"doses_remaining":      d.DosesRemaining,
		"needs_refill":         d.NeedsRefill,
		"estimated_days_left":  d.EstimatedDaysLeft,
		"last_restocked_at":    formatTimePtr(s.LastRestockedAt),
		"last_consumed_at":     formatTimePtr(d.LastConsumedAt),
	}
}

// writeErr memetakan error service ke balasan HTTP.
func (h *Stock) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	if resp, ok := validationResponse(err); ok {
		pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeValidationFailed,
			resp.message, resp.details)
		return
	}
	if !isKnownDomainError(err) {
		h.Logger.ErrorContext(r.Context(), "kesalahan tak terduga pada stok",
			slog.String("error", err.Error()))
	}
	pohttp.WriteDomainError(w, r, err)
}
