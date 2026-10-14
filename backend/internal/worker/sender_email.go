package worker

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/frndly-bot/pantau-obat/backend/internal/config"
	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// EmailSender mengirim notifikasi lewat SMTP.
//
// KENAPA BARU DIISI SEKARANG
//
// Sebelumnya `Send` selalu mengembalikan `ErrEmailNotImplemented`, dan itu
// disengaja: kanal email ditunda ke v0.2 karena belum ada kredensial SMTP.
// Sekarang ada kebutuhan yang tidak bisa ditunda — **lupa sandi**. Tanpa
// email, pengguna yang lupa sandinya tidak punya jalan masuk kembali sama
// sekali; tidak ada admin yang bisa mereset untuknya.
//
// Untuk pengembangan dipakai MailHog: SMTP palsu yang menampung semua surat
// dan menampilkannya di UI, sehingga alurnya bisa diuji tanpa mengirim surat
// sungguhan ke alamat siapa pun.
//
// Sama seperti PushSender: `Send` mengembalikan error yang jelas, bukan
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

// ErrEmailNotConfigured menandai SMTP yang belum diisi konfigurasinya.
var ErrEmailNotConfigured = errors.New("SMTP belum dikonfigurasi")

// Send mengirim satu email.
//
// Mengembalikan ID pesan sebagai bukti terkirim, atau error yang menjelaskan
// kegagalannya. Tidak pernah mengaku berhasil saat gagal.
func (s *EmailSender) Send(ctx context.Context, msg OutgoingMessage) (string, error) {
	if !s.cfg.Enabled() {
		return "", ErrEmailNotConfigured
	}

	to := msg.Email
	if to == "" {
		return "", errors.New("alamat email tujuan kosong")
	}

	messageID := fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), sanitizeID(msg.DedupKey), domainName(s.cfg.From))
	raw := s.buildMessage(to, messageID, msg)

	// Batas waktu dikendalikan konteks, bukan dial tanpa batas: SMTP yang
	// menggantung akan menahan worker, dan itu menunda SELURUH antrean
	// notifikasi — bukan hanya satu email.
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(15 * time.Second)
	}

	addr := net.JoinHostPort(s.cfg.Host, fmt.Sprint(s.cfg.Port))

	conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return "", fmt.Errorf("menyambung ke SMTP %s: %w", addr, err)
	}
	_ = conn.SetDeadline(deadline)

	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return "", fmt.Errorf("membuka sesi SMTP: %w", err)
	}
	defer func() { _ = client.Close() }()

	// STARTTLS dipakai bila server mendukungnya. MailHog tidak
	// menyediakannya, dan memaksa TLS di sana akan membuat pengembangan
	// lokal mustahil.
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return "", fmt.Errorf("memulai STARTTLS: %w", err)
		}
	}

	// Autentikasi hanya bila kredensial diisi. MailHog menerima tanpa auth,
	// dan mengirim AUTH ke server yang tidak memintanya justru gagal.
	if s.cfg.User != "" {
		auth := smtp.PlainAuth("", s.cfg.User, s.cfg.Password, s.cfg.Host)
		if err := client.Auth(auth); err != nil {
			return "", fmt.Errorf("autentikasi SMTP: %w", err)
		}
	}

	if err := client.Mail(s.cfg.FromAddress()); err != nil {
		return "", fmt.Errorf("menetapkan pengirim: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return "", fmt.Errorf("menetapkan penerima: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return "", fmt.Errorf("membuka isi surat: %w", err)
	}
	if _, err := w.Write([]byte(raw)); err != nil {
		return "", fmt.Errorf("menulis isi surat: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("menutup isi surat: %w", err)
	}
	if err := client.Quit(); err != nil {
		// Surat sudah diterima server pada titik ini. Kegagalan QUIT dicatat
		// sebagai peringatan, bukan error: mengulang pengiriman akan
		// menggandakan surat untuk masalah yang tidak penting.
		s.logger.WarnContext(ctx, "SMTP tidak menutup sesi dengan rapi",
			slog.String("error", err.Error()))
	}

	s.logger.InfoContext(ctx, "email terkirim",
		slog.String("message_id", messageID),
		slog.String("to_domain", domainName(to)))

	return messageID, nil
}

// buildMessage menyusun surat MIME sederhana.
//
// Ditulis manual, bukan memakai pustaka: kebutuhannya hanya teks biasa tanpa
// lampiran, dan menambah dependensi untuk itu tidak sepadan.
func (s *EmailSender) buildMessage(to, messageID string, msg OutgoingMessage) string {
	var b strings.Builder
	b.WriteString("From: " + s.cfg.From + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + encodeSubject(msg.Title) + "\r\n")
	b.WriteString("Message-ID: " + messageID + "\r\n")
	b.WriteString("Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(msg.Body)
	b.WriteString("\r\n")
	return b.String()
}

// encodeSubject menyandikan subjek yang mengandung karakter non-ASCII.
//
// Subjek berbahasa Indonesia sering memuat karakter di luar ASCII. Tanpa
// penyandian, sebagian klien menampilkannya sebagai tanda tanya.
func encodeSubject(s string) string {
	ascii := true
	for _, r := range s {
		if r > 127 {
			ascii = false
			break
		}
	}
	if ascii {
		return s
	}
	return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(s)) + "?="
}

// sanitizeID membuat bagian message-id tetap aman untuk header.
func sanitizeID(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		return "msg"
	}
	if len(out) > 32 {
		out = out[:32]
	}
	return out
}

// domainName mengambil bagian domain dari alamat email.
//
// Dipakai untuk LOG: mencatat alamat lengkap pengguna akan menyimpan data
// pribadi di tempat yang tidak perlu. Domainnya cukup untuk menelusuri
// masalah pengiriman.
func domainName(addr string) string {
	if i := strings.LastIndex(addr, "@"); i >= 0 && i+1 < len(addr) {
		return addr[i+1:]
	}
	return "unknown"
}
