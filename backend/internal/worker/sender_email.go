package worker

import (
	"context"
	"errors"
	"log/slog"

	"github.com/frndly-bot/pantau-obat/backend/internal/config"
	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// EmailSender mengirim notifikasi lewat SMTP.
//
// Kanal email baru dipakai di v0.2 (TRD bagian 8). Yang ada di sini kerangka
// yang sudah tersambung ke dispatcher, supaya penambahannya nanti tidak
// menyentuh loop sama sekali — cukup mengisi Send.
//
// Sama seperti PushSender: Send mengembalikan error yang jelas, bukan
// mengaku berhasil. Notifikasi yang hilang tanpa jejak adalah kegagalan
// terburuk untuk fitur pengingat.
type EmailSender struct {
	cfg    config.EmailConfig
	logger *slog.Logger
}

// NewEmailSender membuat sender email.
func NewEmailSender(cfg config.EmailConfig, logger *slog.Logger) *EmailSender {
	return &EmailSender{cfg: cfg, logger: logger}
}

// Channel melaporkan kanal yang dilayani.
func (s *EmailSender) Channel() domain.Channel { return domain.ChannelEmail }

// Available melaporkan apakah SMTP sudah dikonfigurasi.
func (s *EmailSender) Available() bool { return s.cfg.Enabled() }

// ErrEmailNotImplemented menandai pengiriman email yang belum tersedia.
var ErrEmailNotImplemented = errors.New(
	"pengiriman email belum diimplementasikan: kanal email dijadwalkan untuk v0.2")

// Send mengirim email notifikasi.
//
// Mengembalikan ErrEmailNotImplemented selama kanal email belum dibangun.
// Error ini sengaja dikembalikan supaya kegagalan terlihat, bukan senyap.
func (s *EmailSender) Send(_ context.Context, _ OutgoingMessage) (string, error) {
	return "", ErrEmailNotImplemented
}
