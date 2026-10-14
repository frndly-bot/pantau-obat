package worker

import (
	"context"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// MailerAdapter menyajikan EmailSender dengan bentuk yang dibutuhkan layanan
// pemulihan sandi.
//
// KENAPA ADA ADAPTOR
//
// `EmailSender.Send` menerima `OutgoingMessage` — bentuk yang dipakai
// dispatcher notifikasi, lengkap dengan JobID, DedupKey, dan Data. Layanan
// pemulihan sandi tidak punya konsep itu: ia hanya tahu "kirim surat ini ke
// alamat itu".
//
// Adaptor ini menjaga keduanya tetap sederhana: dispatcher tidak perlu tahu
// tentang reset sandi, dan reset sandi tidak perlu memalsukan job notifikasi
// yang tidak pernah ada.
type MailerAdapter struct {
	sender *EmailSender
}

// NewMailerAdapter membungkus sender email.
func NewMailerAdapter(sender *EmailSender) *MailerAdapter {
	return &MailerAdapter{sender: sender}
}

// Available melaporkan apakah SMTP siap dipakai.
func (a *MailerAdapter) Available() bool {
	return a.sender != nil && a.sender.Available()
}

// Send mengirim satu surat sederhana.
func (a *MailerAdapter) Send(ctx context.Context, to, subject, body string) (string, error) {
	return a.sender.Send(ctx, OutgoingMessage{
		Kind:  domain.JobCareDigest, // jenis tidak dipakai pengirim; hanya untuk log
		Title: subject,
		Body:  body,
		Email: to,
	})
}
