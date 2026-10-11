package worker

import (
	"context"
	"log/slog"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// LogSender adalah sender yang hanya mencatat, tidak benar-benar mengirim.
//
// Dipakai saat kanal notifikasi belum dikonfigurasi (VAPID belum diisi).
// Gunanya bukan sekadar pengganti sementara: dengan sender ini, seluruh
// rantai worker bisa diuji end-to-end tanpa kredensial push, dan yang
// dihasilkan tetap jejak nyata di notification_jobs dan
// notification_deliveries.
//
// Yang PENTING: sender ini TIDAK menulis nama obat ke log. Ia mencatat
// medication_id, sesuai aturan privasi yang berlaku untuk seluruh aplikasi.
// Nama obat boleh muncul di ISI notifikasi (memang dibutuhkan user), tapi
// tidak di log aplikasi.
type LogSender struct {
	logger *slog.Logger

	// Enabled menentukan apakah sender ini mengaku tersedia. Diisi false
	// secara bawaan supaya tidak ada yang tanpa sadar menjalankan produksi
	// dengan sender yang tidak mengirim apa pun.
	Enabled bool
}

// NewLogSender membuat sender pencatat.
func NewLogSender(logger *slog.Logger, enabled bool) *LogSender {
	return &LogSender{logger: logger, Enabled: enabled}
}

// Channel melaporkan kanal yang dilayani.
func (s *LogSender) Channel() domain.Channel { return domain.ChannelInApp }

// Available melaporkan apakah sender siap dipakai.
func (s *LogSender) Available() bool { return s.Enabled }

// Send mencatat pesan lalu mengembalikan id palsu.
//
// Id dikembalikan supaya kolom provider_id tetap terisi dan alurnya sama
// dengan sender sungguhan. Mengembalikan string kosong akan membuat baris
// delivery terlihat seperti gagal sebagian.
func (s *LogSender) Send(ctx context.Context, msg OutgoingMessage) (string, error) {
	// medication_id diambil dari data, bukan nama obat. Ini batas privasi.
	var medID any
	if msg.Data != nil {
		medID = msg.Data["medication_id"]
	}

	s.logger.InfoContext(ctx, "notifikasi tercatat (kanal belum dikonfigurasi)",
		slog.String("job_kind", string(msg.Kind)),
		slog.String("owner_id", msg.OwnerID),
		slog.Any("medication_id", medID),
		slog.String("dedup_key", msg.DedupKey),
	)
	return "log-local", nil
}
