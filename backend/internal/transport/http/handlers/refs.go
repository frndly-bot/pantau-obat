package handlers

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/repo/postgres"
	"github.com/frndly-bot/pantau-obat/backend/internal/service"
	pohttp "github.com/frndly-bot/pantau-obat/backend/internal/transport/http"
)

// Refs adalah handler tabel referensi.
type Refs struct {
	Svc    *service.Refs
	Logger *slog.Logger
}

// NewRefs membuat handler referensi.
func NewRefs(svc *service.Refs, logger *slog.Logger) *Refs {
	return &Refs{Svc: svc, Logger: logger}
}

// Routes mendaftarkan endpoint referensi.
//
// Diletakkan di kelompok terautentikasi: isinya hanya daftar kode dan nama
// (satuan, rute, bentuk sediaan), tetapi tidak ada alasan membukanya ke publik.
func (h *Refs) Routes(r chi.Router) {
	r.Get("/refs", h.All)
}

// All menangani GET /api/v1/refs.
//
// Mengembalikan satuan, rute, dan bentuk sediaan dalam satu balasan, supaya
// formulir obat cukup sekali memanggil.
func (h *Refs) All(w http.ResponseWriter, r *http.Request) {
	if pohttp.IdentityFrom(r.Context()) == nil {
		unauthorized(w, r)
		return
	}

	refs, err := h.Svc.All(r.Context())
	if err != nil {
		if !isKnownDomainError(err) {
			h.Logger.ErrorContext(r.Context(), "kesalahan tak terduga pada tabel referensi",
				slog.String("error", err.Error()))
		}
		pohttp.WriteDomainError(w, r, err)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, map[string]any{
		"units":        refItems(refs.Units),
		"routes":       refItems(refs.Routes),
		"dosage_forms": refItems(refs.DosageForms),
	})
}

func refItems(items []postgres.RefItem) []map[string]string {
	out := make([]map[string]string, 0, len(items))
	for _, it := range items {
		out = append(out, map[string]string{"code": it.Code, "name": it.Name})
	}
	return out
}
