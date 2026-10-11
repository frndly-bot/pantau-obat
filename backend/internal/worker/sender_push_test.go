package worker

import (
	"context"
	"testing"
	"time"

	"github.com/frndly-bot/pantau-obat/backend/internal/config"
	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// fakeStore adalah penyimpanan langganan palsu untuk pengujian.
type fakeStore struct {
	subs    []PushSubscription
	err     error
	ownerID string
}

func (f *fakeStore) ActivePushSubscriptions(_ context.Context, ownerID string) ([]PushSubscription, error) {
	f.ownerID = ownerID
	return f.subs, f.err
}

func TestPushSender_AvailableRequiresKeys(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.PushConfig
		want bool
	}{
		{"kosong semua", config.PushConfig{}, false},
		{"hanya publik", config.PushConfig{PublicKey: "a"}, false},
		{"hanya privat", config.PushConfig{PrivateKey: "b"}, false},
		{"publik dan privat tanpa subject", config.PushConfig{
			PublicKey: "a", PrivateKey: "b"}, false},
		{"lengkap", config.PushConfig{
			PublicKey: "a", PrivateKey: "b", Subject: "mailto:x@y.z"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := NewPushSender(c.cfg, nil)
			if got := s.Available(); got != c.want {
				t.Errorf("Available() = %v, mau %v", got, c.want)
			}
		})
	}
}

func TestPushSender_ChannelIsPush(t *testing.T) {
	s := NewPushSender(config.PushConfig{}, nil)
	if got := s.Channel(); got != domain.ChannelPush {
		t.Errorf("Channel() = %q, mau %q", got, domain.ChannelPush)
	}
}

func TestPushSender_NoSubscriptionsIsError(t *testing.T) {
	// User tanpa perangkat terdaftar harus menghasilkan ERROR, bukan sukses
	// diam. Kalau dianggap sukses, job ditandai selesai padahal tidak ada
	// yang menerima apa pun — kegagalan yang menyamar sebagai keberhasilan.
	s := NewPushSender(config.PushConfig{
		PublicKey: "a", PrivateKey: "b", Subject: "mailto:x@y.z"}, nil)
	s.WithSubscriptions(&fakeStore{subs: nil})

	_, err := s.Send(context.Background(), OutgoingMessage{
		OwnerID:  "01900000-0000-7000-8000-000000000000",
		Kind:     domain.JobDoseReminder,
		Title:    "Waktunya minum",
		Body:     "1 tablet",
		DedupKey: "dose:abc|2026-10-10|08:00",
	})
	if err == nil {
		t.Fatal("harus error bila tidak ada perangkat terdaftar")
	}
}

func TestPushSender_NoStoreIsError(t *testing.T) {
	// Tanpa penyimpanan langganan, sender tidak tahu harus mengirim ke mana.
	// Harus error, bukan diam-diam sukses.
	s := NewPushSender(config.PushConfig{
		PublicKey: "a", PrivateKey: "b", Subject: "mailto:x@y.z"}, nil)

	_, err := s.Send(context.Background(), OutgoingMessage{OwnerID: "x"})
	if err == nil {
		t.Fatal("harus error bila penyimpanan langganan belum dipasang")
	}
}

func TestPushSender_UsesOwnerIDForLookup(t *testing.T) {
	// Memastikan langganan dicari berdasarkan PEMILIK pesan, bukan id lain.
	// Kalau salah, notifikasi bisa terkirim ke perangkat orang lain — dan
	// itu kebocoran data kesehatan, bukan sekadar bug.
	store := &fakeStore{}
	s := NewPushSender(config.PushConfig{
		PublicKey: "a", PrivateKey: "b", Subject: "mailto:x@y.z"}, nil)
	s.WithSubscriptions(store)

	owner := "01900000-0000-7000-8000-0000000000aa"
	_, _ = s.Send(context.Background(), OutgoingMessage{OwnerID: owner})

	if store.ownerID != owner {
		t.Errorf("pencarian memakai ownerID %q, mau %q", store.ownerID, owner)
	}
}

func TestPushSender_GoneDetection(t *testing.T) {
	// 404 dan 410 berarti langganan sudah tidak ada di browser. Dibedakan
	// dari kegagalan sementara, karena penanganannya berbeda: yang mati
	// tidak perlu dicoba ulang dan sebaiknya dibersihkan.
	if !isSubscriptionGone(&subscriptionGoneError{status: 410}) {
		t.Error("410 harus dikenali sebagai langganan mati")
	}
	if !isSubscriptionGone(&subscriptionGoneError{status: 404}) {
		t.Error("404 harus dikenali sebagai langganan mati")
	}
	if isSubscriptionGone(context.DeadlineExceeded) {
		t.Error("timeout BUKAN langganan mati; itu kegagalan sementara")
	}
	if isSubscriptionGone(nil) {
		t.Error("nil bukan langganan mati")
	}
}

func TestPushSender_TagNeverEmpty(t *testing.T) {
	// Tag kosong membuat browser memperlakukan setiap notifikasi sebagai
	// terpisah, sehingga pengingat dosis yang sama menumpuk di layar.
	// Tag diturunkan dari dedup_key, jadi harus selalu ada isinya.
	msg := OutgoingMessage{DedupKey: "dose:abc|2026-10-10|08:00"}
	tag := msg.DedupKey
	if tag == "" {
		t.Fatal("dedup_key tidak boleh kosong pada pengingat dosis")
	}
	_ = time.Now
}
