package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	"github.com/frndly-bot/pantau-obat/backend/internal/repo/postgres"
)

// WebhookProcessor memproses payload webhook yang sudah lolos verifikasi.
//
// Pemisahan dari handler disengaja: handler hanya memastikan "permintaan ini
// asli", sedangkan makna bisnisnya ada di sini. Service juga tidak tahu
// header HTTP apa pun — payload yang diterima sudah bersih.
type WebhookProcessor struct {
	db     *postgres.DB
	logger *slog.Logger
}

// NewWebhookProcessor membuat processor webhook.
func NewWebhookProcessor(db *postgres.DB, logger *slog.Logger) *WebhookProcessor {
	return &WebhookProcessor{db: db, logger: logger}
}

// EmailStatusPayload adalah laporan status pengiriman dari penyedia email.
//
// Bentuknya sengaja minimal: provider_id dan status wajib, sisanya pelengkap
// yang boleh tidak dikirim penyedia. Field yang tidak dikenal diabaikan, agar
// penyedia bisa menambah kolom tanpa merusak integrasi.
type EmailStatusPayload struct {
	ProviderID string `json:"provider_id"`
	Status     string `json:"status"`
	Recipient  string `json:"recipient"`
	Timestamp  string `json:"timestamp"`
}

// TelegramUpdatePayload adalah subset update Telegram yang relevan.
//
// Hanya pesan pribadi yang diproses. Field lain (edited_message,
// callback_query, dan semacamnya) diabaikan: parser JSON mengabaikan field
// yang tidak dipetakan, sehingga struktur update lengkap dari Telegram
// tetap diterima tanpa perlu dimodelkan seluruhnya.
type TelegramUpdatePayload struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		MessageID int64 `json:"message_id"`
		From      *struct {
			ID int64 `json:"id"`
		} `json:"from"`
		Chat struct {
			ID   int64  `json:"id"`
			Type string `json:"type"`
		} `json:"chat"`
		Text string `json:"text"`
	} `json:"message"`
}

// emailStatusMap memetakan istilah penyedia ke status delivery internal.
//
// Penyedia email memakai nama yang bervariasi ("delivered", "sent",
// "delivered, 2026-10-11"). Dipetakan di satu tempat supaya istilah baru
// cukup ditambahkan di sini.
var emailStatusMap = map[string]domain.DeliveryStatus{
	"sent":         domain.DeliverySent,
	"delivered":    domain.DeliverySent,
	"failed":       domain.DeliveryFailed,
	"bounced":      domain.DeliveryFailed,
	"complained":   domain.DeliverySkipped,
	"unsubscribed": domain.DeliverySkipped,
	"opened":       domain.DeliverySent,
}

// EmailStatus mencatat laporan status dari penyedia email.
//
// Laporan hanya bermakna kalau delivery-nya memang pernah dicatat oleh
// dispatcher. provider_id yang tidak dikenal TIDAK dianggap error: bisa jadi
// laporan untuk delivery lama yang sudah melewati masa retensi 30 hari.
// Menolaknya hanya membuat penyedia mengulang laporan yang sama tanpa guna.
func (s *WebhookProcessor) EmailStatus(ctx context.Context, p EmailStatusPayload) error {
	if strings.TrimSpace(p.ProviderID) == "" {
		return fmt.Errorf("%w: provider_id wajib diisi", domain.ErrValidation)
	}

	status, ok := emailStatusMap[strings.ToLower(strings.TrimSpace(p.Status))]
	if !ok {
		// Status yang tidak dikenal dicatat, bukan ditolak. Kegagalan di
		// sini bukan salah penyedia, dan permintaannya sudah sah.
		s.logger.Warn("webhook email: status tidak dikenal",
			slog.String("provider_id", p.ProviderID),
			slog.String("status", p.Status))
		return nil
	}

	_, err := s.db.DeliveryByProvider(ctx, s.db.Pool(), p.ProviderID)
	if err != nil {
		if isNotFound(err) {
			s.logger.Info("webhook email: provider_id tidak dikenal, diabaikan",
				slog.String("provider_id", p.ProviderID))
			return nil
		}
		return err
	}

	// Provider_id sudah ada — laporan ulang. Status delivery sudah diatur
	// dispatcher saat mengirim; laporan ini tidak mengubah jalur pengiriman
	// apa pun. V0.1 cukup mencatat bahwa laporannya datang.
	s.logger.Info("webhook email: status diterima",
		slog.String("provider_id", p.ProviderID),
		slog.String("status", string(status)))
	return nil
}

// TelegramUpdate memproses satu update dari bot Telegram.
//
// V0.1: hanya mengenali perintah `/start` untuk mencatat bahwa seseorang
// mencoba menghubungkan akun. Pemetaan chat ke user aplikasi butuh alur
// kode tautan yang belum dibangun, jadi update apa pun selain itu hanya
// dicatat tanpa aksi.
//
// Error hanya dikembalikan untuk kegagalan infrastruktur (DB mati, dan
// semacamnya). Chat yang belum terhubung BUKAN error — balasan 200 tetap
// dikirim supaya Telegram tidak mengirim ulang tanpa henti.
func (s *WebhookProcessor) TelegramUpdate(ctx context.Context, p TelegramUpdatePayload) error {
	if p.Message == nil {
		// Bukan message (mis. callback_query) — diabaikan.
		return nil
	}
	if p.Message.Chat.Type != "private" {
		// Bot ini hanya melayani chat pribadi; grup diabaikan diam-diam.
		return nil
	}

	text := strings.TrimSpace(p.Message.Text)
	if !strings.HasPrefix(text, "/start") {
		s.logger.Info("webhook telegram: pesan non-perintah diabaikan",
			slog.Int64("chat_id", p.Message.Chat.ID))
		return nil
	}

	externalID := itoa64(p.Message.Chat.ID)
	_, err := s.db.IntegrationByExternalID(ctx, s.db.Pool(), "telegram", externalID)
	if err != nil && !isNotFound(err) {
		return err
	}
	if err == nil {
		// Chat ini sudah terhubung ke sebuah akun. Tidak ada aksi tambahan;
		// pengingat kanal telegram baru aktif di v0.2.
		s.logger.Info("webhook telegram: /start dari chat yang sudah terhubung",
			slog.Int64("chat_id", p.Message.Chat.ID))
		return nil
	}

	// Chat belum terhubung. Alur kode tautan ada di v0.2; di sini cukup
	// dicatat supaya operator bisa melihat minat pemakaian dari log.
	s.logger.Info("webhook telegram: /start dari chat yang belum terhubung (v0.2: kirim kode tautan)",
		slog.Int64("chat_id", p.Message.Chat.ID),
		slog.Int64("update_id", p.UpdateID),
		slog.String("waktu", time.Now().UTC().Format(time.RFC3339)))
	return nil
}

// itoa64 mengubah int64 menjadi string tanpa menyertakan strconv di banyak
// tempat. Dipakai untuk chat ID Telegram.
func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
