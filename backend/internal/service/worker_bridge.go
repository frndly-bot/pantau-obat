package service

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	"github.com/frndly-bot/pantau-obat/backend/internal/repo/postgres"
	"github.com/frndly-bot/pantau-obat/backend/internal/worker"
)

// File ini menjembatani worker dengan service.
//
// Ini SATU-SATUNYA tempat paket service mengimpor worker, dan arahnya
// service -> worker (bukan sebaliknya). Worker mendefinisikan interface-nya
// sendiri, dan service yang memenuhinya. Dengan begitu paket worker tidak
// pernah mengimpor service, sehingga memindahkannya jadi binary kedua nanti
// tidak menyeret lapisan bisnis ikut berpindah.
//
// Kenapa perlu jembatan sama sekali: dispatcher butuh dua hal yang berasal
// dari tabel berbeda — preferensi notifikasi user, dan langganan push
// perangkatnya. Keduanya sudah punya repo masing-masing, dan service ini
// yang menyatukannya.

// WorkerBridge memenuhi worker.PrefsProvider.
type WorkerBridge struct {
	notif *Notifications
	db    *postgres.DB
}

// NewWorkerBridge membuat jembatan untuk dispatcher.
func NewWorkerBridge(n *Notifications, db *postgres.DB) *WorkerBridge {
	return &WorkerBridge{notif: n, db: db}
}

// PrefsFor meneruskan pembacaan preferensi efektif satu kanal.
func (b *WorkerBridge) PrefsFor(ctx context.Context, userID string, channel domain.Channel) (domain.ChannelPrefs, error) {
	return b.notif.PrefsFor(ctx, userID, channel)
}

// ActivePushSubscriptions menyediakan langganan push aktif milik user.
//
// Nama methodnya sama persis dengan worker.SubscriptionStore supaya satu
// metode ini memenuhi DUA interface sekaligus: yang dipakai PushSender, dan
// yang dipakai Dispatcher. Kalau namanya berbeda, dispatcher akan butuh
// method pembungkus yang isinya sama — dan itu dua tempat yang bisa berbeda
// perilaku.
//
// Bentuknya berbeda dari DeviceToken di repo: di sini langganan sudah dipecah
// menjadi endpoint, kunci P256DH, dan auth, karena itulah yang dibutuhkan
// pustaka web push. Pemecahan itu dilakukan DI SINI, bukan di worker, supaya
// worker tidak perlu tahu bagaimana token perangkat disimpan.
//
// Token yang tidak bisa dipecah DILEWATI, bukan menggagalkan seluruh
// pengiriman. Satu perangkat dengan token rusak tidak boleh menghalangi
// pengingat sampai ke perangkat lain yang sehat.
func (b *WorkerBridge) ActivePushSubscriptions(ctx context.Context, ownerID string) ([]worker.PushSubscription, error) {
	tokens, err := b.db.ListDeviceTokens(ctx, b.db.Pool(), ownerID)
	if err != nil {
		return nil, err
	}

	out := make([]worker.PushSubscription, 0, len(tokens))
	for _, t := range tokens {
		// Hanya platform web yang memakai web push. Android dan iOS memakai
		// FCM/APNs lewat kanal tersendiri, yang belum dibangun.
		if t.Platform != domain.PlatformWeb {
			continue
		}

		sub, ok := parsePushSubscription(t.Token)
		if !ok {
			// Token rusak: dilewati, tidak menggagalkan pengiriman lain.
			// Dibersihkan lewat worker retensi, bukan di jalur pengiriman.
			continue
		}
		out = append(out, sub)
	}
	return out, nil
}

// pushSubscriptionJSON adalah bentuk yang dikirim browser lewat
// PushSubscription.toJSON(). Disimpan apa adanya di device_tokens.token.
type pushSubscriptionJSON struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256DH string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// parsePushSubscription mengurai token perangkat web menjadi langganan push.
//
// Mengembalikan ok=false untuk token yang tidak bisa diurai. Pemanggil
// memutuskan untuk melewatinya; fungsi ini tidak mengembalikan error supaya
// satu token rusak tidak memaksa pemanggil menghentikan seluruh pengiriman.
func parsePushSubscription(raw string) (worker.PushSubscription, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "{") {
		return worker.PushSubscription{}, false
	}

	var parsed pushSubscriptionJSON
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return worker.PushSubscription{}, false
	}

	// Ketiga bagian wajib ada. Endpoint tanpa kunci akan selalu ditolak
	// service push, dan meneruskannya hanya membuang satu percobaan.
	if parsed.Endpoint == "" || parsed.Keys.P256DH == "" || parsed.Keys.Auth == "" {
		return worker.PushSubscription{}, false
	}

	return worker.PushSubscription{
		Endpoint: parsed.Endpoint,
		P256DH:   parsed.Keys.P256DH,
		Auth:     parsed.Keys.Auth,
	}, true
}
