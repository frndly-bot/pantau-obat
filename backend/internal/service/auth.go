// Package service memuat aturan bisnis. Berada di antara transport dan repo.
//
// ATURAN IMPORT: service boleh mengimpor `repo`, `domain`, dan `auth`.
// Tidak boleh mengimpor `transport`.
package service

import (
	"context"
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

// AuthConfig adalah pengaturan yang dibutuhkan layanan autentikasi.
type AuthConfig struct {
	JWTSecret     string
	JWTTTL        time.Duration
	RefreshTTL    time.Duration
	PublicURL     string
	Argon2Configd struct{} // penanda tempat; parameter ada di paket auth
}

// Meta berisi metadata request untuk audit dan pelacakan.
type Meta struct {
	IP        *string
	UserAgent *string
}

// Auth adalah layanan autentikasi.
type Auth struct {
	db     *postgres.DB
	issuer *auth.TokenIssuer
	cfg    AuthConfig
	logger *slog.Logger
}

// NewAuth membuat layanan autentikasi.
func NewAuth(db *postgres.DB, cfg AuthConfig, logger *slog.Logger) (*Auth, error) {
	issuer, err := auth.NewTokenIssuer(cfg.JWTSecret, cfg.JWTTTL)
	if err != nil {
		return nil, fmt.Errorf("menyiapkan penerbit token: %w", err)
	}
	if cfg.RefreshTTL <= 0 {
		return nil, errors.New("refresh ttl harus positif")
	}
	return &Auth{db: db, issuer: issuer, cfg: cfg, logger: logger}, nil
}

// TTL mengembalikan umur access token dalam detik, untuk field expires_in.
func (s *Auth) TTL() int { return int(s.issuer.TTL().Seconds()) }

// Register mendaftar akun baru dan langsung membuat sesi.
//
// Peran SELALU 'user'. Tidak ada parameter role di sepanjang jalur ini,
// sehingga tidak ada cara bagi klien memberi dirinya peran admin.
func (s *Auth) Register(ctx context.Context, in domain.RegisterInput, meta Meta) (*domain.AuthResult, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	// Kekuatan password diperiksa SEBELUM hashing, supaya password lemah
	// tidak membuang 64 MiB memori dan 3 iterasi Argon2 tanpa guna.
	if err := auth.ValidatePasswordStrength(in.Password); err != nil {
		return nil, err
	}

	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return nil, fmt.Errorf("menyandikan password: %w", err)
	}

	var result *domain.AuthResult
	err = s.db.WithTx(ctx, func(tx pgx.Tx) error {
		q := tx

		// Cek dulu supaya pesannya jelas. Tetap ada balapan dengan request
		// lain, dan itu ditangani oleh unique constraint saat insert.
		if _, err := s.db.UserByEmail(ctx, q, in.Email); err == nil {
			return fmt.Errorf("%w: email sudah terdaftar", domain.ErrValidation)
		} else if !errors.Is(err, domain.ErrNotFound) {
			return err
		}

		user, err := s.db.CreateUser(ctx, q, in, hash)
		if err != nil {
			if postgres.IsUniqueViolation(errors.Unwrap(err)) {
				return fmt.Errorf("%w: email sudah terdaftar", domain.ErrValidation)
			}
			return err
		}

		res, err := s.startSession(ctx, q, user, meta)
		if err != nil {
			return err
		}
		result = res

		// Audit ditulis di dalam transaksi yang sama. Untuk registrasi,
		// kegagalan audit sebaiknya ikut membatalkan: ini titik masuk akun,
		// dan jejaknya perlu ada.
		return s.auditOrFail(ctx, q, postgres.AuditEntry{
			ActorID:      &user.ID,
			OwnerID:      &user.ID,
			ActorRole:    string(domain.ActorUser),
			Action:       postgres.ActionCreate,
			ResourceType: postgres.ResourceUser,
			ResourceID:   &user.ID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			AfterState:   map[string]any{"email": user.Email, "full_name": user.FullName},
		})
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Login menukar kredensial menjadi sesi.
func (s *Auth) Login(ctx context.Context, in domain.LoginInput, meta Meta) (*domain.AuthResult, error) {
	email, err := domain.NormalizeEmail(in.Email)
	if err != nil {
		// Email yang formatnya salah tetap dijawab dengan pesan kredensial
		// yang sama, supaya tidak ada perbedaan perilaku yang bisa dipakai
		// untuk menebak email mana yang terdaftar.
		return nil, domain.ErrInvalidCredentials
	}
	if in.Password == "" {
		return nil, domain.ErrInvalidCredentials
	}

	creds, err := s.db.CredentialsByEmail(ctx, s.db.Pool(), email)
	if err != nil {
		return nil, err
	}

	if err := auth.VerifyPassword(in.Password, creds.PasswordHash); err != nil {
		if errors.Is(err, auth.ErrPasswordMismatch) {
			return nil, domain.ErrInvalidCredentials
		}
		// Hash rusak = masalah data, bukan salah password.
		s.logger.ErrorContext(ctx, "hash password tidak bisa dibaca",
			slog.String("user_id", creds.UserID))
		return nil, domain.ErrInvalidCredentials
	}

	// Diperiksa SETELAH password benar. Kalau diperiksa sebelum, pesan
	// "akun dinonaktifkan" bisa dipakai untuk memastikan sebuah akun ada.
	if !creds.IsActive {
		return nil, domain.ErrAccountInactive
	}

	var result *domain.AuthResult
	err = s.db.WithTx(ctx, func(tx pgx.Tx) error {
		q := tx

		user, err := s.db.UserByID(ctx, q, creds.UserID)
		if err != nil {
			return err
		}

		res, err := s.startSession(ctx, q, user, meta)
		if err != nil {
			return err
		}
		result = res

		if err := s.db.TouchLastLogin(ctx, q, user.ID); err != nil {
			// Kegagalan mencatat waktu login tidak boleh menggagalkan login.
			s.logger.WarnContext(ctx, "gagal mencatat waktu login",
				slog.String("user_id", user.ID))
		}

		return s.auditOrFail(ctx, q, postgres.AuditEntry{
			ActorID:      &user.ID,
			OwnerID:      &user.ID,
			ActorRole:    actorRoleOf(user),
			Action:       postgres.ActionLogin,
			ResourceType: postgres.ResourceAuth,
			ResourceID:   &user.ID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
		})
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Refresh menukar refresh token menjadi sesi baru, dengan rotasi.
//
// Rotasi berlaku: token yang dipakai langsung dicabut dan diganti yang baru.
// Kalau token yang sudah dipakai muncul lagi, itu tanda kebocoran, dan
// seluruh rantai sesi dicabut.
func (s *Auth) Refresh(ctx context.Context, refreshToken string, meta Meta) (*domain.AuthResult, error) {
	if refreshToken == "" {
		return nil, domain.ErrUnauthenticated
	}
	tokenHash := auth.HashRefreshToken(refreshToken)

	var result *domain.AuthResult
	// WithTxCommitOnError, BUKAN WithTx.
	//
	// Alasannya penting: saat penggunaan ulang terdeteksi, kita mencabut
	// seluruh akses user DAN mengembalikan error supaya request dibalas 401.
	// Dengan WithTx biasa, error itu akan membatalkan transaksi, sehingga
	// pencabutannya ikut hilang dan token penyerang tetap hidup. Persis
	// kebalikan dari yang diinginkan.
	err := s.db.WithTxCommitOnError(ctx, func(tx pgx.Tx) error {
		q := tx

		sess, err := s.db.SessionByTokenHash(ctx, q, tokenHash)
		if err != nil {
			return err
		}

		// Sesi yang SUDAH dicabut tapi masih dipakai = tanda token bocor.
		// Bukan hanya ditolak: seluruh rantai dimatikan, supaya token milik
		// user yang asli dan token penyerang sama-sama tidak berlaku lagi.
		if sess.RevokedAt != nil {
			s.logger.WarnContext(ctx, "refresh token yang sudah dicabut dipakai ulang",
				slog.String("user_id", sess.UserID),
				slog.String("session_id", sess.ID))
			// Penggunaan ulang refresh token adalah tanda kebocoran, jadi
			// yang dimatikan bukan hanya rantai sesi ini, tapi SELURUH
			// akses user: access token yang beredar dan semua sesi lain.
			if _, err := s.db.RevokeAccessAndSessions(ctx, q, sess.UserID,
				domain.RevokeReasonTokenReuse, nil); err != nil {
				s.logger.ErrorContext(ctx, "gagal mencabut akses setelah deteksi penggunaan ulang",
					slog.String("error", err.Error()))
			}
			return domain.ErrTokenRevoked
		}

		if !sess.Active(time.Now()) {
			return domain.ErrTokenRevoked
		}

		user, err := s.db.UserByID(ctx, q, sess.UserID)
		if err != nil {
			return err
		}
		if !user.IsActive {
			// Akun dinonaktifkan setelah sesi dibuat. Cabut sesinya.
			_ = s.db.RevokeSession(ctx, q, sess.ID)
			return domain.ErrAccountInactive
		}

		// Rotasi: sesi lama dicabut, sesi baru dibuat dengan rotated_from
		// menunjuk sesi lama. Rantai inilah yang ditelusuri saat ada
		// penggunaan ulang.
		if err := s.db.RevokeSession(ctx, q, sess.ID); err != nil {
			return err
		}

		res, err := s.startSessionRotated(ctx, q, user, meta, sess.ID)
		if err != nil {
			return err
		}
		result = res
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Logout mencabut sesi ini, atau seluruh sesi user.
func (s *Auth) Logout(ctx context.Context, userID, sessionID string, allDevices bool, meta Meta) error {
	return s.db.WithTx(ctx, func(tx pgx.Tx) error {
		q := tx

		if allDevices {
			// Keluar dari semua perangkat harus mematikan access token yang
			// sudah beredar juga, bukan hanya refresh token. Kalau hanya
			// sesi yang dicabut, access token yang dipegang perangkat lain
			// tetap hidup sampai 24 jam, dan permintaan "keluar dari semua
			// perangkat" tidak benar-benar dipenuhi.
			_, err := s.db.RevokeAccessAndSessions(ctx, q, userID,
				domain.RevokeReasonLogoutAll, &userID)
			if err != nil {
				return err
			}
		} else {
			if err := s.db.RevokeSession(ctx, q, sessionID); err != nil {
				return err
			}
		}

		action := postgres.ActionLogout
		metaExtra := map[string]any{"all_devices": allDevices}
		return s.auditOrFail(ctx, q, postgres.AuditEntry{
			ActorID:      &userID,
			OwnerID:      &userID,
			ActorRole:    string(domain.ActorUser),
			Action:       action,
			ResourceType: postgres.ResourceAuth,
			ResourceID:   &userID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			Meta:         metaExtra,
		})
	})
}

// Me mengembalikan profil pemanggil.
//
// Me TIDAK memeriksa pencabutan token. Pemeriksaan itu dilakukan di
// middleware, yang memegang klaim `iat` dari token. Memeriksanya di sini
// tidak mungkin, karena fungsi ini hanya menerima id user, bukan tokennya.
func (s *Auth) Me(ctx context.Context, userID string) (*domain.User, error) {
	u, err := s.db.UserByID(ctx, s.db.Pool(), userID)
	if err != nil {
		return nil, err
	}
	if !u.IsActive {
		return nil, domain.ErrAccountInactive
	}
	return u, nil
}

// ChangePassword mengganti password dan mencabut sesi lain.
//
// Sesi yang sedang dipakai TIDAK dicabut, supaya user tidak terlempar keluar
// tepat setelah mengganti password. Perangkat lain gugur, karena password
// mungkin diganti justru karena dicurigai bocor.
func (s *Auth) ChangePassword(ctx context.Context, userID, sessionID string, in domain.ChangePasswordInput, meta Meta) error {
	if in.NewPassword == "" {
		return fmt.Errorf("%w: password baru wajib diisi", domain.ErrValidation)
	}
	if err := auth.ValidatePasswordStrength(in.NewPassword); err != nil {
		return err
	}
	if in.NewPassword == in.CurrentPassword {
		return fmt.Errorf("%w: password baru harus berbeda dari yang lama", domain.ErrValidation)
	}

	newHash, err := auth.HashPassword(in.NewPassword)
	if err != nil {
		return fmt.Errorf("menyandikan password: %w", err)
	}

	return s.db.WithTx(ctx, func(tx pgx.Tx) error {
		q := tx

		user, err := s.db.UserByID(ctx, q, userID)
		if err != nil {
			return err
		}
		creds, err := s.db.CredentialsByEmail(ctx, q, user.Email)
		if err != nil {
			return err
		}
		if err := auth.VerifyPassword(in.CurrentPassword, creds.PasswordHash); err != nil {
			if errors.Is(err, auth.ErrPasswordMismatch) {
				return fmt.Errorf("%w: password lama salah", domain.ErrValidation)
			}
			return err
		}

		if err := s.db.UpdatePassword(ctx, q, userID, newHash); err != nil {
			return err
		}
		// Ganti password mencabut SELURUH akses, termasuk access token yang
		// sedang dipakai. Perangkat yang sedang dipakai user juga harus
		// login ulang; itu memang disengaja, karena orang mengganti password
		// justru ketika mencurigai akunnya dipakai orang lain.
		if err := s.revokeForUserTx(ctx, q, userID,
			domain.RevokeReasonPasswordChange, &userID); err != nil {
			return err
		}

		return s.auditOrFail(ctx, q, postgres.AuditEntry{
			ActorID:      &userID,
			OwnerID:      &userID,
			ActorRole:    actorRoleOf(user),
			Action:       postgres.ActionUpdate,
			ResourceType: postgres.ResourceAuth,
			ResourceID:   &userID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			Meta:         map[string]any{"field": "password", "other_sessions_revoked": true},
		})
	})
}

// VerifyAccessToken memeriksa access token dan mengembalikan klaimnya.
func (s *Auth) VerifyAccessToken(token string) (auth.AccessClaims, error) {
	return s.issuer.Verify(token, time.Now())
}

// startSession membuat sesi baru dan tokennya.
func (s *Auth) startSession(ctx context.Context, q postgresQuerier, user *domain.User, meta Meta) (*domain.AuthResult, error) {
	return s.startSessionRotated(ctx, q, user, meta, "")
}

// startSessionRotated membuat sesi baru, opsional menaut ke sesi sebelumnya.
func (s *Auth) startSessionRotated(
	ctx context.Context, q postgresQuerier, user *domain.User, meta Meta, rotatedFromID string,
) (*domain.AuthResult, error) {
	token, err := auth.NewRefreshToken()
	if err != nil {
		return nil, err
	}

	var rotatedFrom *string
	if rotatedFromID != "" {
		rotatedFrom = &rotatedFromID
	}

	sess, err := s.db.CreateSession(
		ctx, q, user.ID, auth.HashRefreshToken(token),
		time.Now().Add(s.cfg.RefreshTTL), meta.IP, meta.UserAgent, rotatedFrom,
	)
	if err != nil {
		return nil, err
	}

	access, err := s.issuer.Issue(user.ID, sess.ID, time.Now())
	if err != nil {
		return nil, fmt.Errorf("menerbitkan access token: %w", err)
	}

	return &domain.AuthResult{
		User:         user,
		AccessToken:  access,
		RefreshToken: token,
		ExpiresIn:    s.TTL(),
	}, nil
}

// auditOrFail menulis audit dan mengembalikan error bila gagal.
//
// Dipakai pada aksi yang jejaknya WAJIB ada (registrasi, login, logout,
// ganti password). Aksi lain memakai auditBestEffort.
func (s *Auth) auditOrFail(ctx context.Context, q postgresQuerier, e postgres.AuditEntry) error {
	if err := s.db.WriteAudit(ctx, q, e); err != nil {
		return fmt.Errorf("menulis audit untuk %s: %w", e.Action, err)
	}
	return nil
}

// actorRoleOf menentukan peran yang disalin ke audit.
//
// Disalin, bukan di-join, supaya jejak lama tetap benar walau peran user
// berubah di kemudian hari.
func actorRoleOf(u *domain.User) string {
	if u.Role == domain.RoleAdmin {
		return string(domain.ActorAdmin)
	}
	return string(domain.ActorUser)
}

// NormalizeUserAgent memotong user agent yang terlalu panjang sebelum
// disimpan ke kolom audit atau sesi.
func NormalizeUserAgent(ua string) *string {
	ua = strings.TrimSpace(ua)
	if ua == "" {
		return nil
	}
	if len(ua) > 500 {
		ua = ua[:500]
	}
	return &ua
}
