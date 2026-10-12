package worker

import (
	"context"
	"log/slog"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// LogSender adalah sender kanal in-app untuk v0.1.
//
// "Mengirim" di sini berarti mencatat job `sent` yang dibaca inbox
// aplikasi sendiri (GET /notifications) — tidak ada pengiriman ke luar
// sistem, karena isinya memang ditampilkan DI DALAM aplikasi. Dengan ini
// seluruh rantai worker teruji end-to-end tanpa kredensial apa pun, dan
// pengingat user benar-benar muncul di lonceng header.
//
// Yang PENTING: sender ini TIDAK menulis nama obat ke log. Ia mencatat
// medication_id, sesuai aturan privasi yang berlaku untuk seluruh aplikasi.
// Nama obat boleh muncul di ISI notifikasi (memang dibutuhkan user), tapi
// tidak di log aplikasi.
type LogSender struct {
	logger *slog.Logger

	// Available melaporkan kesiapan. Diisi true untuk v0.1: in-app adalah
	// satu-satunya kanal aktif sesuai keputusan produk. Kalau false,
	// dispatcher menjatuhkan semua job ke `skipped` NO_CHANNEL dan inbox
	// menampilkan pengingat yang tidak pernah terkirim.
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
