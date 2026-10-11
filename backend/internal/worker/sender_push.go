package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/frndly-bot/pantau-obat/backend/internal/config"
	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// PushSender mengirim notifikasi web push lewat VAPID.
//
// Mengirim ke SEMUA perangkat user, bukan hanya satu: pengingat minum obat
// tidak boleh hilang hanya karena ponselnya sedang mati sementara tabletnya
// menyala.
type PushSender struct {
	cfg    config.PushConfig
	logger *slog.Logger
	client *http.Client
	store  SubscriptionStore
	maxAge time.Duration
}

// PushSubscription adalah langganan push satu perangkat.
//
// Bentuknya mengikuti apa yang dikembalikan browser lewat
// PushSubscription.toJSON(): endpoint plus dua kunci terenkode base64.
type PushSubscription struct {
	Endpoint string
	P256DH   string
	Auth     string
}

// SubscriptionStore menyediakan langganan push milik user.
//
// Interface, bukan tipe konkret, supaya sender ini tidak mengimpor repo
// langsung dan pengujian bisa memakai penyimpanan palsu.
type SubscriptionStore interface {
	ActivePushSubscriptions(ctx context.Context, ownerID string) ([]PushSubscription, error)
}

// NewPushSender membuat sender push.
func NewPushSender(cfg config.PushConfig, logger *slog.Logger) *PushSender {
	return &PushSender{
		cfg:    cfg,
		logger: logger,
		// Timeout eksplisit: tanpa ini, satu endpoint yang menggantung bisa
		// menahan siklus dispatcher, dan pengingat untuk user lain ikut
		// tertunda.
		client: &http.Client{Timeout: 15 * time.Second},
		maxAge: 12 * time.Hour,
	}
}

// WithSubscriptions memasang penyimpanan langganan.
func (s *PushSender) WithSubscriptions(store SubscriptionStore) *PushSender {
	s.store = store
	return s
}

// Channel melaporkan kanal yang dilayani.
func (s *PushSender) Channel() domain.Channel { return domain.ChannelPush }

// Available melaporkan apakah kunci VAPID sudah lengkap.
func (s *PushSender) Available() bool { return s.cfg.Enabled() }

// pushPayload adalah isi pesan yang diterima service worker.
//
// Struktur ini mengikuti notification-v2 bagian 14: judul, isi, tag untuk
// menggabungkan notifikasi yang sama, dan blok data berisi id — bukan nama
// obat.
type pushPayload struct {
	Notification pushNotification `json:"notification"`
	Data         map[string]any   `json:"data,omitempty"`
}

type pushNotification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Tag   string `json:"tag,omitempty"`
}

// Send mengirim notifikasi ke seluruh perangkat user.
//
// Mengembalikan:
//   - id perangkat pertama yang berhasil, bila minimal satu terkirim;
//   - error bila SEMUA perangkat gagal, atau bila user tidak punya perangkat.
//
// "Tidak punya perangkat" diperlakukan sebagai error, bukan sukses diam.
// Kalau dianggap sukses, job ditandai selesai padahal tidak ada yang
// menerima apa pun — dan kegagalan seperti itu tidak akan pernah ketahuan.
func (s *PushSender) Send(ctx context.Context, msg OutgoingMessage) (string, error) {
	if s.store == nil {
		return "", errors.New("penyimpanan langganan push belum dipasang")
	}

	subs, err := s.store.ActivePushSubscriptions(ctx, msg.OwnerID)
	if err != nil {
		return "", fmt.Errorf("membaca langganan push: %w", err)
	}
	if len(subs) == 0 {
		return "", errors.New("user tidak punya perangkat terdaftar untuk notifikasi")
	}

	// tag dipakai browser untuk menggabungkan notifikasi yang sama, sehingga
	// pengingat dosis yang sama tidak menumpuk di layar.
	tag := strings.ReplaceAll(msg.DedupKey, ":", "-")
	if len(tag) > 100 {
		tag = tag[:100]
	}

	body, err := json.Marshal(pushPayload{
		Notification: pushNotification{
			Title: msg.Title,
			Body:  msg.Body,
			Tag:   tag,
		},
		Data: msg.Data,
	})
	if err != nil {
		return "", fmt.Errorf("menyusun payload push: %w", err)
	}

	var (
		sent      int
		firstID   string
		lastErr   error
		goneCount int
	)
	for _, sub := range subs {
		id, err := s.sendOne(ctx, sub, body, tag)
		if err != nil {
			lastErr = err
			// 404 dan 410 berarti langganan sudah tidak ada di sisi browser:
			// user mencabut izin, atau perangkatnya diganti. Bukan kegagalan
			// sementara, jadi tidak perlu dicoba ulang.
			if isSubscriptionGone(err) {
				goneCount++
			}
			s.logger.WarnContext(ctx, "pengiriman push ke satu perangkat gagal",
				slog.String("owner_id", msg.OwnerID),
				slog.String("error", err.Error()))
			continue
		}
		sent++
		if firstID == "" {
			firstID = id
		}
	}

	if goneCount > 0 {
		// Dilaporkan supaya pemanggil bisa membersihkan langganan mati.
		// Pembersihan sendiri dilakukan di luar, karena sender tidak
		// mengimpor repo.
		s.logger.InfoContext(ctx, "langganan push sudah tidak berlaku",
			slog.String("owner_id", msg.OwnerID),
			slog.Int("jumlah", goneCount))
	}

	if sent == 0 {
		if lastErr != nil {
			return "", fmt.Errorf("semua perangkat gagal: %w", lastErr)
		}
		return "", errors.New("tidak ada perangkat yang berhasil dikirimi")
	}
	return firstID, nil
}

// sendOne mengirim ke satu perangkat.
func (s *PushSender) sendOne(ctx context.Context, sub PushSubscription, body []byte, topic string) (string, error) {
	resp, err := webpush.SendNotificationWithContext(ctx, body, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			P256dh: sub.P256DH,
			Auth:   sub.Auth,
		},
	}, &webpush.Options{
		HTTPClient:      s.client,
		Subscriber:      s.cfg.Subject,
		VAPIDPublicKey:  s.cfg.PublicKey,
		VAPIDPrivateKey: s.cfg.PrivateKey,
		VapidExpiration: time.Now().Add(s.maxAge),

		// TTL 1 jam: kalau perangkat sedang mati, pengingat ini masih
		// berguna saat perangkat menyala dalam satu jam ke depan. Lebih dari
		// itu, mengirim pengingat minum obat yang sudah lewat justru
		// membingungkan.
		TTL: 3600,

		// UrgencyHigh supaya pengingat minum obat tidak ditahan oleh
		// penghemat baterai. Ini satu-satunya pesan yang layak memakai
		// prioritas tinggi.
		Urgency: webpush.UrgencyHigh,

		// Topic menggabungkan notifikasi dengan tag sama, sehingga pengingat
		// dosis yang sama tidak menumpuk.
		Topic: topic,
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		// Push service tidak memberi id pesan yang berguna. Yang dicatat
		// adalah kode statusnya sebagai bukti pengiriman diterima.
		return fmt.Sprintf("push:%d", resp.StatusCode), nil
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return "", &subscriptionGoneError{status: resp.StatusCode}
	default:
		return "", fmt.Errorf("push service membalas %d", resp.StatusCode)
	}
}

// subscriptionGoneError menandai langganan yang sudah tidak berlaku.
type subscriptionGoneError struct{ status int }

func (e *subscriptionGoneError) Error() string {
	return fmt.Sprintf("langganan push sudah tidak berlaku (HTTP %d)", e.status)
}

// isSubscriptionGone melaporkan apakah error berarti langganan mati.
//
// Dipakai untuk membedakan dua hal yang perlakuannya berbeda: kegagalan
// SEMENTARA (jaringan, server push sedang bermasalah) vs langganan yang
// memang sudah tidak ada. Yang kedua tidak perlu dicoba ulang, dan
// langganannya sebaiknya dibersihkan.
func isSubscriptionGone(err error) bool {
	var gone *subscriptionGoneError
	return errors.As(err, &gone)
}

// IsSubscriptionGone adalah bentuk ekspor untuk pemanggil di luar paket.
func IsSubscriptionGone(err error) bool { return isSubscriptionGone(err) }
