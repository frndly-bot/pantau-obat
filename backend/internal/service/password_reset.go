package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/auth"
	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	"github.com/frndly-bot/pantau-obat/backend/internal/repo/postgres"
)

// Masa berlaku token pemulihan sandi.
//
// Lima belas menit: cukup untuk membuka email dan mengeklik tautannya, tapi
// tidak cukup untuk ditinggalkan di kotak masuk yang sudah lama tidak dibuka.
// Token pemulihan adalah kunci akun — umurnya harus sependek yang masih
// nyaman dipakai.
const resetTokenTTL = 15 * time.Minute

// Jeda minimum antar permintaan pemulihan untuk alamat yang sama.
//
// Tanpa jeda, alamat email siapa pun bisa dibanjiri surat hanya dengan
// memanggil endpoint ini berulang kali — dan itu gangguan terhadap orang
// lain, bukan terhadap sistem. Satu menit cukup untuk menahan penyalahgunaan
// tanpa menghalangi pengguna yang salah ketik alamat lalu memperbaikinya.
const resetRequestCooldown = time.Minute

// ErrResetCooldown = permintaan pemulihan terlalu sering untuk alamat itu.
var ErrResetCooldown = errors.New("permintaan pemulihan baru saja dikirim")

// PasswordResetter menangani alur lupa sandi.
//
// Dipisah dari `Auth` karena tanggung jawabnya berbeda: `Auth` memeriksa
// kredensial yang SUDAH terbukti, sedangkan di sini pemanggilnya belum
// membuktikan apa pun — ia hanya punya akses ke email.
type PasswordResetter struct {
	db     *postgres.DB
	mailer Mailer
	logger *slog.Logger
	// publicURL dipakai menyusun tautan reset. Diambil dari konfigurasi,
	// BUKAN dari header permintaan: tautan yang disusun dari header Host
	// bisa diarahkan penyerang ke domainnya sendiri, dan pengguna yang
	// mengekliknya akan menyerahkan token resetnya.
	publicURL string
}

// Mailer mengirim satu email. Dipenuhi oleh worker.EmailSender.
type Mailer interface {
	Available() bool
	Send(ctx context.Context, to, subject, body string) (string, error)
}

// NewPasswordResetter membuat layanan pemulihan sandi.
func NewPasswordResetter(db *postgres.DB, mailer Mailer, publicURL string, logger *slog.Logger) *PasswordResetter {
	return &PasswordResetter{db: db, mailer: mailer, logger: logger, publicURL: publicURL}
}

// RequestReset membuat token pemulihan dan mengirimnya lewat email.
//
// PENTING: fungsi ini TIDAK PERNAH memberi tahu pemanggil apakah alamatnya
// terdaftar. Ia mengembalikan sukses untuk alamat yang tidak ada sama sekali.
// Kalau tidak, endpoint ini menjadi alat untuk memeriksa email mana yang
// punya akun di sini — dan pada aplikasi kesehatan, keanggotaan itu sendiri
// sudah informasi pribadi.
func (s *PasswordResetter) RequestReset(ctx context.Context, email string, meta Meta) error {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return fmt.Errorf("%w: email wajib diisi", domain.ErrValidation)
	}

	user, err := s.db.UserByEmail(ctx, s.db.Pool(), email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// Tidak ada akun: tetap dilaporkan sukses, dan tidak ada surat
			// yang dikirim. Lihat catatan di atas.
			s.logger.InfoContext(ctx, "permintaan pemulihan untuk email tak terdaftar",
				slog.String("to_domain", domainOf(email)))
			return nil
		}
		return err
	}

	// Jeda antar permintaan.
	last, err := s.db.LastResetRequestAt(ctx, s.db.Pool(), user.ID)
	if err != nil {
		return err
	}
	if !last.IsZero() && time.Since(last) < resetRequestCooldown {
		return ErrResetCooldown
	}

	// Token mentah dibuat dari sumber acak kriptografis. Yang disimpan hanya
	// hash-nya, sehingga isi database tidak cukup untuk memakai token.
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Errorf("membuat token pemulihan: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := hashResetToken(token)

	expires := time.Now().Add(resetTokenTTL)
	if err := s.db.CreatePasswordResetToken(ctx, s.db.Pool(), user.ID, hash, expires); err != nil {
		return err
	}

	link := strings.TrimRight(s.publicURL, "/") + "/reset-sandi?token=" + token
	body := "Seseorang meminta pengaturan ulang sandi untuk akun Pantau Obat ini.\n\n" +
		"Buka tautan berikut untuk membuat sandi baru:\n\n" + link + "\n\n" +
		"Tautan berlaku " + "15 menit" + " dan hanya bisa dipakai sekali.\n\n" +
		"Kalau Anda tidak memintanya, abaikan email ini — sandi Anda tidak berubah."

	if err := s.sendMail(ctx, user.Email, "Atur ulang sandi Pantau Obat", body, meta); err != nil {
		return err
	}

	_ = s.db.WriteAudit(ctx, s.db.Pool(), postgres.AuditEntry{
		ActorID:      &user.ID,
		OwnerID:      &user.ID,
		ActorRole:    string(domain.ActorUser),
		Action:       postgres.ActionUpdate,
		ResourceType: postgres.ResourceAuth,
		ResourceID:   &user.ID,
		IP:           meta.IP,
		UserAgent:    meta.UserAgent,
		Meta:         map[string]any{"field": "password_reset_requested"},
	})

	return nil
}

// ResetPassword memakai token dan mengganti sandi.
//
// Token dipakai SEKALI, dalam transaksi yang sama dengan pembaruan sandi:
// kalau penulisan sandi gagal, token tidak ikut hangus, dan kalau berhasil,
// token tidak bisa dipakai lagi.
func (s *PasswordResetter) ResetPassword(ctx context.Context, token, newPassword string, meta Meta) error {
	if token == "" {
		return fmt.Errorf("%w: token wajib diisi", domain.ErrValidation)
	}
	if err := auth.ValidatePasswordStrength(newPassword); err != nil {
		return err
	}

	newHash, err := auth.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("menyandikan password: %w", err)
	}

	return s.db.WithTx(ctx, func(tx pgx.Tx) error {
		userID, err := s.db.ConsumePasswordResetToken(ctx, tx, hashResetToken(token))
		if err != nil {
			return err
		}

		if err := s.db.UpdatePassword(ctx, tx, userID, newHash); err != nil {
			return err
		}

		// Seluruh sesi lama dicabut.
		//
		// Orang mengatur ulang sandi justru ketika ia tidak bisa masuk —
		// sering karena mencurigai akunnya dipakai orang lain. Membiarkan
		// sesi lama tetap hidup akan membuat pemulihan itu sia-sia.
		if _, err := tx.Exec(ctx, `
            INSERT INTO user_token_revocations (user_id, reason, revoked_by)
            VALUES ($1, 'password_changed', $1)
            ON CONFLICT (user_id) DO UPDATE
               SET revoked_at = now(), reason = 'password_changed', revoked_by = $1`,
			userID); err != nil {
			return fmt.Errorf("mencabut sesi lama: %w", err)
		}

		return s.db.WriteAudit(ctx, tx, postgres.AuditEntry{
			ActorID:      &userID,
			OwnerID:      &userID,
			ActorRole:    string(domain.ActorUser),
			Action:       postgres.ActionUpdate,
			ResourceType: postgres.ResourceAuth,
			ResourceID:   &userID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			Meta:         map[string]any{"field": "password", "via": "reset_token", "all_sessions_revoked": true},
		})
	})
}

// sendMail mengirim surat, atau mengembalikan error yang menjelaskan kenapa
// tidak bisa.
//
// Bila SMTP belum dikonfigurasi, permintaan DIANGGAP GAGAL — bukan diam-diam
// sukses. Pengguna yang diminta menunggu email yang tidak akan pernah datang
// akan menyimpulkan aplikasinya rusak, dan tidak punya cara tahu kenapa.
func (s *PasswordResetter) sendMail(ctx context.Context, to, subject, body string, meta Meta) error {
	if s.mailer == nil || !s.mailer.Available() {
		s.logger.WarnContext(ctx, "permintaan pemulihan sandi ditolak: SMTP belum dikonfigurasi",
			slog.String("to_domain", domainOf(to)))
		return fmt.Errorf("%w: pengiriman email belum dikonfigurasi di server ini", domain.ErrValidation)
	}

	if _, err := s.mailer.Send(ctx, to, subject, body); err != nil {
		s.logger.ErrorContext(ctx, "gagal mengirim email pemulihan",
			slog.String("to_domain", domainOf(to)),
			slog.String("error", err.Error()))
		return fmt.Errorf("mengirim email pemulihan: %w", err)
	}
	return nil
}

// hashResetToken menghasilkan hash token untuk disimpan.
//
// SHA-256, bukan Argon2 seperti password. Alasannya: token ini acak 256 bit,
// jadi tidak ada yang bisa ditebak — memperlambat verifikasinya justru
// membuat endpoint pemulihan mudah dijadikan sasaran pembebanan.
func hashResetToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// domainOf mengambil bagian domain dari alamat, untuk LOG.
//
// Alamat lengkap tidak pernah dicatat: log berpindah tempat, dan alamat email
// pengguna bukan sesuatu yang perlu ikut berpindah.
func domainOf(email string) string {
	if i := strings.LastIndex(email, "@"); i >= 0 && i+1 < len(email) {
		return email[i+1:]
	}
	return "unknown"
}
