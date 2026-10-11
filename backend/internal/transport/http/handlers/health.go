// Package handlers memuat handler HTTP. Satu file per kelompok resource,
// mengikuti pembagian di docs/design-v2/architecture-v2.md bagian 3.
package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	pohttp "github.com/frndly-bot/pantau-obat/backend/internal/transport/http"
)

// Pinger adalah irisan minimal koneksi database yang dibutuhkan health check.
// Dibuat sebagai interface supaya handler ini bisa diuji tanpa database nyata,
// dan supaya transport tidak mengimpor repo secara langsung.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Health menangani liveness dan readiness.
type Health struct {
	DB     Pinger
	Logger *slog.Logger
}

// NewHealth membuat handler health check.
func NewHealth(db Pinger, logger *slog.Logger) *Health {
	return &Health{DB: db, Logger: logger}
}

// Live menjawab GET /healthz — "proses hidup".
//
// Selalu 200 selama server bisa menjawab. TIDAK memeriksa database:
// kalau DB sedang down, prosesnya tetap hidup, dan yang gagal adalah
// readiness. Membedakan keduanya mencegah Docker me-restart container
// hanya karena DB belum siap.
func (h *Health) Live(w http.ResponseWriter, r *http.Request) {
	pohttp.WriteJSON(w, r, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// Ready menjawab GET /readyz — "siap melayani".
//
// 200 kalau database bisa di-ping, 503 kalau tidak. Inilah yang dipakai
// docker-compose healthcheck dan Caddy sebelum mengarahkan trafik.
func (h *Health) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.DB.Ping(ctx); err != nil {
		h.Logger.WarnContext(r.Context(), "readiness gagal: database tidak terjangkau",
			slog.String("error", err.Error()))
		pohttp.WriteError(w, r, http.StatusServiceUnavailable, "NOT_READY",
			"database belum siap", nil)
		return
	}

	pohttp.WriteJSON(w, r, http.StatusOK, map[string]any{
		"status":   "ready",
		"database": "ok",
		"time":     time.Now().UTC().Format(time.RFC3339),
	})
}
