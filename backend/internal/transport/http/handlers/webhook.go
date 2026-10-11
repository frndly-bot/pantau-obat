package handlers

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/config"
	"github.com/frndly-bot/pantau-obat/backend/internal/service"
	pohttp "github.com/frndly-bot/pantau-obat/backend/internal/transport/http"
)

// Batas ukuran body webhook. Update Telegram dan laporan status email
// keduanya jauh di bawah ini; batasnya hanya mencegah body raksasa dibaca
// sebelum verifikasi.
const maxWebhookBody = 64 << 10 // 64 KiB

// Toleransi cap waktu webhook, sesuai api-v2 bagian 12.
const webhookTimestampTolerance = 5 * time.Minute

// Webhook adalah handler untuk /webhooks/*.
//
// Handler ini TIDAK bagian dari kelompok terautentikasi: yang datang bukan
// user dengan sesi, melainkan penyedia eksternal yang membuktikan dirinya
// lewat tanda tangan atau secret token. Karena itu jalurnya sendiri di
// router, dan verifikasinya dilakukan di sini, bukan lewat middleware Auth.
type Webhook struct {
	EmailSecret    string
	TelegramSecret string
	Processor      *service.WebhookProcessor
	Logger         *slog.Logger
}

// NewWebhook membuat handler webhook.
//
// Secret dibaca dari konfigurasi saat startup, bukan per request, supaya
// kesalahan konfigurasi ketahuan sejak proses naik. Secret kosong berarti
// kanal itu nonaktif: permintaan apa pun akan ditolak dengan status 500,
// bukan diterima tanpa pemeriksaan.
func NewWebhook(cfg config.Config, processor *service.WebhookProcessor, logger *slog.Logger) *Webhook {
	return &Webhook{
		EmailSecret:    cfg.Webhook.EmailSecret,
		TelegramSecret: cfg.Telegram.WebhookSecret,
		Processor:      processor,
		Logger:         logger,
	}
}

// Routes mendaftarkan endpoint webhook masuk.
func (h *Webhook) Routes(r chi.Router) {
	r.Post("/webhooks/email-status", h.EmailStatus)
	r.Post("/webhooks/telegram-update", h.TelegramUpdate)
}

// webhookReceived adalah bentuk balasan sukses kedua webhook.
//
// `received: true` berarti verifikasi lolos dan body sudah diterima — bukan
// janji bahwa pemrosesannya berhasil. Dua webhook memakai balasan yang sama
// supaya klien eksternal hanya perlu menangani satu bentuk.
func webhookReceived() map[string]any {
	return map[string]any{"received": true}
}

// readRawBody membaca body mentah dengan batas ukuran.
//
// Verifikasi HMAC WAJIB memakai byte mentah persis seperti dikirim; body
// yang sudah diurai lalu disusun ulang bisa berbeda urutan kuncinya dan
// tanda tangannya tidak akan pernah cocok. Karena itu pembacaan dilakukan
// sekali di sini, lalu byte yang sama dipakai untuk verifikasi DAN parsing.
func (h *Webhook) readRawBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBody+1))
	if err != nil {
		pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeInvalidJSON,
			"body webhook tidak bisa dibaca", nil)
		return nil, false
	}
	if len(body) > maxWebhookBody {
		pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeInvalidJSON,
			"body webhook terlalu besar", nil)
		return nil, false
	}
	return body, true
}

// EmailStatus menangani POST /api/v1/webhooks/email-status.
//
// Alur: verifikasi HMAC atas raw body -> parse -> teruskan ke processor.
// Kegagalan verifikasi membalas 401/400 sesuai WebhookErrorStatus; kegagalan
// pemrosesan TETAP membalas 200 supaya penyedia tidak mengirim ulang tanpa
// henti — errornya sudah tercatat di log.
func (h *Webhook) EmailStatus(w http.ResponseWriter, r *http.Request) {
	body, ok := h.readRawBody(w, r)
	if !ok {
		return
	}

	sig := r.Header.Get("X-Signature")
	ts := r.Header.Get("X-Timestamp")
	if err := pohttp.VerifyWebhookHMAC(h.EmailSecret, body, sig, ts,
		time.Now(), webhookTimestampTolerance); err != nil {
		h.rejectVerification(w, r, "email-status", err)
		return
	}

	var payload service.EmailStatusPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		pohttp.WriteError(w, r, http.StatusBadRequest, pohttp.CodeInvalidJSON,
			"body webhook bukan JSON yang sah", nil)
		return
	}

	h.processAndRespond(w, r, "email-status", func() error {
		return h.Processor.EmailStatus(r.Context(), payload)
	})
}

// TelegramUpdate menangani POST /api/v1/webhooks/telegram-update.
//
// Telegram tidak menandatangani body; autentikasinya secret token statis di
// header X-Telegram-Bot-Api-Secret-Token, dibandingkan waktu-tetap.
//
// Setelah verifikasi lolos, balasan SELALU 200 walau pemrosesannya gagal.
// Kalau membalas error, Telegram akan mengirim ulang update yang sama
// berulang kali — dan itu bisa menumpuk.
func (h *Webhook) TelegramUpdate(w http.ResponseWriter, r *http.Request) {
	body, ok := h.readRawBody(w, r)
	if !ok {
		return
	}

	provided := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
	if !pohttp.SecureTokenEqual(provided, h.TelegramSecret) {
		pohttp.WriteError(w, r, http.StatusUnauthorized, pohttp.CodeUnauthenticated,
			"secret token tidak valid", nil)
		return
	}

	var payload service.TelegramUpdatePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		// Body yang bukan JSON tetap dibalas 200: Telegram memperlakukan
		// error sebagai kegagalan pengiriman dan akan mencoba lagi, padahal
		// mencoba lagi tidak akan mengubah isi body-nya.
		h.Logger.Warn("webhook telegram: body bukan JSON",
			slog.String("error", err.Error()))
		pohttp.WriteJSON(w, r, http.StatusOK, webhookReceived())
		return
	}

	h.processAndRespond(w, r, "telegram-update", func() error {
		return h.Processor.TelegramUpdate(r.Context(), payload)
	})
}

// processAndRespond menjalankan pemrosesan dan menyusun balasannya.
//
// Pemisahan ini membuat aturan "200 walau gagal" ditulis satu kali untuk
// kedua webhook, bukan dua kali dengan sedikit perbedaan.
func (h *Webhook) processAndRespond(w http.ResponseWriter, r *http.Request, name string, run func() error) {
	if err := run(); err != nil {
		// Error pemrosesan TIDAK mengubah status: verifikasi sudah lolos,
		// body sudah diterima. Membalas 4xx/5xx hanya membuat pengirim
		// eksternal mengulang permintaan yang isinya sudah tercatat.
		h.Logger.Error("webhook gagal diproses",
			slog.String("webhook", name),
			slog.String("error", err.Error()))
	}
	pohttp.WriteJSON(w, r, http.StatusOK, webhookReceived())
}

// rejectVerification mencatat dan membalas kegagalan verifikasi HMAC.
func (h *Webhook) rejectVerification(w http.ResponseWriter, r *http.Request, name string, err error) {
	status, msg := pohttp.WebhookErrorStatus(err)
	if status == http.StatusInternalServerError {
		// Secret belum diatur = masalah konfigurasi server, bukan klien.
		h.Logger.Error("webhook belum dikonfigurasi", slog.String("webhook", name))
	} else {
		h.Logger.Warn("webhook ditolak",
			slog.String("webhook", name),
			slog.String("alasan", pohttp.FormatWebhookReason(err)))
	}
	pohttp.WriteError(w, r, status, pohttp.CodeUnauthenticated, msg, nil)
}
