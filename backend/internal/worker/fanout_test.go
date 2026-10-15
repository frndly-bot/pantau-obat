package worker

import (
	"context"
	"log/slog"
	"testing"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// stubSender mencatat kanal apa saja yang benar-benar dikirimi pesan.
type stubSender struct {
	channel domain.Channel
	tersedia bool
	dikirim int
	gagal   bool
}

func (s *stubSender) Send(context.Context, OutgoingMessage) (string, error) {
	if s.gagal {
		return "", errStubGagal
	}
	s.dikirim++
	return "stub", nil
}
func (s *stubSender) Channel() domain.Channel { return s.channel }
func (s *stubSender) Available() bool         { return s.tersedia }

var errStubGagal = errNoChannelActive

// stubPrefs menjawab preferensi dari peta, dengan bawaan domain.DefaultPrefs.
type stubPrefs struct {
	mati map[domain.Channel]bool
}

func (p *stubPrefs) PrefsFor(_ context.Context, _ string, ch domain.Channel) (domain.ChannelPrefs, error) {
	if p.mati[ch] {
		prefs := domain.DefaultPrefs(ch)
		prefs.Enabled = false
		return prefs, nil
	}
	prefs := domain.DefaultPrefs(ch)
	// Kanal non-in-app diaktifkan supaya uji ini menguji gerbangnya, bukan
	// nilai bawaannya.
	prefs.Enabled = true
	return prefs, nil
}

func (p *stubPrefs) ActivePushSubscriptions(context.Context, string) ([]PushSubscription, error) {
	return nil, nil
}

// TestFanout_KanalInAppSelaluLolos adalah uji regresi untuk bug yang
// mematikan SELURUH notifikasi in-app.
//
// LATAR
//
// `multiSender` dulu mengembalikan kanal sender PERTAMA, dan `dispatch`
// memakainya untuk membaca preferensi sebagai gerbang sebelum mengirim.
// Selama hanya ada LogSender, kanal pertama itu `in_app` yang selalu aktif —
// jadi tidak ada yang menyadari masalahnya. Begitu SMTP diaktifkan untuk
// alur lupa sandi, sender pertama menjadi `email`; preferensi email mati
// secara bawaan, sehingga SETIAP pengingat dijatuhkan ke `skipped`
// CHANNEL_DISABLED. Pengingat obat berhenti masuk ke aplikasi hanya karena
// email dinyalakan.
//
// notification-v2 bagian 6 menetapkan `in_app` SELALU aktif. Uji ini
// mengunci janji itu.
func TestFanout_KanalInAppSelaluLolos(t *testing.T) {
	inApp := &stubSender{channel: domain.ChannelInApp, tersedia: true}
	email := &stubSender{channel: domain.ChannelEmail, tersedia: true}

	// Email DIMATIKAN pengguna. In-app tidak punya pengaturan itu.
	prefs := &stubPrefs{mati: map[domain.Channel]bool{domain.ChannelEmail: true}}

	s := multiSender([]Sender{email, inApp}, prefs, slog.Default())
	if s == nil {
		t.Fatal("multiSender mengembalikan nil padahal ada kanal yang tersedia")
	}

	// Gerbang dispatcher memakai Channel(). Nilainya harus in_app, karena
	// kanal itulah yang selalu boleh dikirim.
	if got := s.Channel(); got != domain.ChannelInApp {
		t.Fatalf("Channel() = %q, ingin %q — gerbang preferensi akan memakai kanal yang salah",
			got, domain.ChannelInApp)
	}

	if _, err := s.Send(context.Background(), OutgoingMessage{OwnerID: "u1"}); err != nil {
		t.Fatalf("Send gagal: %v", err)
	}
	if inApp.dikirim != 1 {
		t.Fatalf("kanal in-app dikirim %d kali, ingin 1 — notifikasi dalam aplikasi hilang", inApp.dikirim)
	}
	if email.dikirim != 0 {
		t.Fatalf("kanal email dikirim %d kali, ingin 0 — pengguna sudah mematikannya", email.dikirim)
	}
}

// TestFanout_MengirimKeSemuaKanalAktif menjaga janji notification-v2 bagian
// 5: "Kalau push dan Telegram sama-sama aktif, keduanya dikirim."
//
// Mengirim hanya ke kanal pertama yang berhasil membuat pengguna yang
// mengaktifkan dua kanal hanya menerima salah satunya — dan ia tidak punya
// cara tahu bahwa pengaturannya diabaikan.
func TestFanout_MengirimKeSemuaKanalAktif(t *testing.T) {
	inApp := &stubSender{channel: domain.ChannelInApp, tersedia: true}
	email := &stubSender{channel: domain.ChannelEmail, tersedia: true}

	prefs := &stubPrefs{mati: map[domain.Channel]bool{}} // semua aktif

	s := multiSender([]Sender{inApp, email}, prefs, slog.Default())
	if _, err := s.Send(context.Background(), OutgoingMessage{OwnerID: "u1"}); err != nil {
		t.Fatalf("Send gagal: %v", err)
	}
	if inApp.dikirim != 1 || email.dikirim != 1 {
		t.Fatalf("in-app=%d email=%d, ingin keduanya 1 — kanal aktif harus dikirim semua",
			inApp.dikirim, email.dikirim)
	}
}

// TestFanout_KanalBelumSiapTidakIkut memastikan sender yang belum
// dikonfigurasi tidak ikut dalam fanout.
//
// Email tanpa SMTP akan selalu gagal. Kalau ia ikut, setiap pengiriman
// mencatat kegagalan yang tidak bisa diperbaiki siapa pun — dan kegagalan
// itu menutupi kanal yang sebenarnya bekerja.
func TestFanout_KanalBelumSiapTidakIkut(t *testing.T) {
	inApp := &stubSender{channel: domain.ChannelInApp, tersedia: true}
	email := &stubSender{channel: domain.ChannelEmail, tersedia: false} // SMTP belum diisi

	s := multiSender([]Sender{inApp, email}, nil, slog.Default())
	if s == nil {
		t.Fatal("multiSender nil padahal in-app tersedia")
	}
	if _, err := s.Send(context.Background(), OutgoingMessage{OwnerID: "u1"}); err != nil {
		t.Fatalf("Send gagal: %v", err)
	}
	if email.dikirim != 0 {
		t.Fatalf("email yang belum siap ikut dikirimi %d kali", email.dikirim)
	}
}

// TestFanout_KegagalanSatuKanalTidakMenahanLain menjaga aturan
// notification-v2 bagian 11: "Kegagalan per kanal tidak menggagalkan job."
//
// Kalau email bermasalah sampai menahan kanal in-app, pengingat obat yang
// paling penting — yang dibaca di aplikasi — ikut hilang karena masalah
// yang tidak berhubungan.
func TestFanout_KegagalanSatuKanalTidakMenahanLain(t *testing.T) {
	inApp := &stubSender{channel: domain.ChannelInApp, tersedia: true}
	email := &stubSender{channel: domain.ChannelEmail, tersedia: true, gagal: true}

	s := multiSender([]Sender{email, inApp}, nil, slog.Default())
	if _, err := s.Send(context.Background(), OutgoingMessage{OwnerID: "u1"}); err != nil {
		t.Fatalf("Send gagal walau in-app berhasil: %v", err)
	}
	if inApp.dikirim != 1 {
		t.Fatalf("in-app dikirim %d kali, ingin 1 — kegagalan email menahan kanal lain", inApp.dikirim)
	}
}
