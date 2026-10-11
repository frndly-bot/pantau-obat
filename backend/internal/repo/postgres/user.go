package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// userColumns adalah daftar kolom yang dipakai berulang saat SELECT.
// Dikumpulkan di satu tempat supaya urutan scan tidak pernah meleset.
//
// password_hash sengaja TIDAK ada di sini. Hash hanya diambil lewat
// CredentialsByEmail, supaya tidak ada jalur di mana hash ikut ter-scan
// ke dalam struct User dan berpotensi ter-serialize ke JSON.
const userColumns = `
    id, email, full_name, phone, timezone, is_active,
    role, admin_scope, admin_granted_at, admin_granted_by,
    last_login_at, created_at, updated_at`

// scanUser membaca satu baris user.
func scanUser(row pgx.Row) (*domain.User, error) {
	var u domain.User
	var scope []string

	err := row.Scan(
		&u.ID, &u.Email, &u.FullName, &u.Phone, &u.Timezone, &u.IsActive,
		&u.Role, &scope, &u.AdminGrantedAt, &u.AdminGrantedBy,
		&u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("membaca user: %w", err)
	}

	// admin_scope NULL berarti akses penuh. pgx memetakan NULL menjadi
	// slice nil, jadi nil dan []string{} harus dibedakan dengan hati-hati:
	// nil = penuh, kosong = TIDAK punya akses apa pun.
	u.AdminScope = toAdminScopes(scope)
	return &u, nil
}

func toAdminScopes(raw []string) []domain.AdminScope {
	if raw == nil {
		return nil
	}
	out := make([]domain.AdminScope, len(raw))
	for i, s := range raw {
		out[i] = domain.AdminScope(s)
	}
	return out
}

func fromAdminScopes(scopes []domain.AdminScope) []string {
	if scopes == nil {
		return nil
	}
	out := make([]string, len(scopes))
	for i, s := range scopes {
		out[i] = string(s)
	}
	return out
}

// CreateUser menyimpan akun baru.
//
// Peran SELALU 'user' di sini. Tidak ada parameter role, sehingga tidak ada
// jalur di mana handler bisa keliru meneruskan role dari klien. Peran admin
// hanya ditetapkan lewat operasi DB/seed, sesuai TRD bagian 5.3 dan 7.2.
func (db *DB) CreateUser(ctx context.Context, q Querier, in domain.RegisterInput, passwordHash string) (*domain.User, error) {
	var phone *string
	if strings.TrimSpace(in.Phone) != "" {
		p := strings.TrimSpace(in.Phone)
		phone = &p
	}
	tz := in.Timezone
	if tz == "" {
		tz = domain.DefaultTimezone
	}

	row := q.QueryRow(ctx, `
        INSERT INTO users (email, password_hash, full_name, phone, timezone)
        VALUES ($1, $2, $3, $4, $5)
        RETURNING `+userColumns,
		in.Email, passwordHash, strings.TrimSpace(in.FullName), phone, tz)

	u, err := scanUser(row)
	if err != nil {
		// Email digaransi unik oleh constraint di DB. Kalau ini terpicu,
		// ada balapan antara pemeriksaan dan insert, dan pesannya harus
		// tetap jelas untuk user.
		if IsUniqueViolation(err) || IsUniqueViolation(errUnwrap(err)) {
			return nil, domain.ErrValidation
		}
		return nil, err
	}
	return u, nil
}

// errUnwrap membantu mempertahankan rantai error saat dibungkus fmt.Errorf.
func errUnwrap(err error) error { return errors.Unwrap(err) }

// CredentialsByEmail mengambil hash password untuk keperluan login.
//
// Dipisah dari UserByEmail supaya hash hanya muncul di satu tempat, yaitu
// saat benar-benar dibutuhkan untuk verifikasi.
func (db *DB) CredentialsByEmail(ctx context.Context, q Querier, email string) (*domain.Credentials, error) {
	var c domain.Credentials
	err := q.QueryRow(ctx, `
        SELECT id, password_hash, is_active
          FROM users
         WHERE email = $1 AND deleted_at IS NULL`,
		email,
	).Scan(&c.UserID, &c.PasswordHash, &c.IsActive)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Sengaja memakai sentinel yang sama dengan "password salah".
			// Membedakan "email tidak terdaftar" dari "password salah"
			// memudahkan penyerang memetakan email mana yang ada.
			return nil, domain.ErrInvalidCredentials
		}
		return nil, fmt.Errorf("membaca kredensial: %w", err)
	}
	return &c, nil
}

// UserByID mengambil user berdasarkan id.
func (db *DB) UserByID(ctx context.Context, q Querier, id string) (*domain.User, error) {
	row := q.QueryRow(ctx, `
        SELECT `+userColumns+`
          FROM users
         WHERE id = $1 AND deleted_at IS NULL`, id)
	return scanUser(row)
}

// UserByEmail mengambil user berdasarkan email (tidak peduli huruf besar/kecil,
// karena kolomnya CITEXT).
func (db *DB) UserByEmail(ctx context.Context, q Querier, email string) (*domain.User, error) {
	row := q.QueryRow(ctx, `
        SELECT `+userColumns+`
          FROM users
         WHERE email = $1 AND deleted_at IS NULL`, email)
	return scanUser(row)
}

// TouchLastLogin mencatat waktu login terakhir. Kegagalan di sini tidak
// boleh menggagalkan login, jadi pemanggil boleh mengabaikan error.
func (db *DB) TouchLastLogin(ctx context.Context, q Querier, userID string) error {
	_, err := q.Exec(ctx, `UPDATE users SET last_login_at = now() WHERE id = $1`, userID)
	return err
}

// UpdatePassword mengganti hash password.
func (db *DB) UpdatePassword(ctx context.Context, q Querier, userID, newHash string) error {
	tag, err := q.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, userID, newHash)
	if err != nil {
		return fmt.Errorf("mengganti password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// UpdateProfile mengubah nama, telepon, dan zona waktu.
//
// Memakai COALESCE-style dinamis: hanya field yang dikirim yang diubah.
// Ini menghindari PATCH yang menimpa field lain dengan nilai kosong.
func (db *DB) UpdateProfile(ctx context.Context, q Querier, userID string, fullName, phone, timezone *string) (*domain.User, error) {
	row := q.QueryRow(ctx, `
        UPDATE users
           SET full_name = COALESCE($2, full_name),
               phone     = CASE WHEN $3::text IS NULL THEN phone
                                WHEN $3 = '' THEN NULL
                                ELSE $3 END,
               timezone  = COALESCE($4, timezone)
         WHERE id = $1 AND deleted_at IS NULL
        RETURNING `+userColumns,
		userID, fullName, phone, timezone)
	return scanUser(row)
}

// AnonymizeUser melakukan soft-delete sekaligus menghapus data identitas.
//
// Dua hal terjadi bersamaan, dan itu disengaja:
//   - deleted_at diisi, sehingga user tidak bisa login lagi.
//   - email dan nama diganti, supaya data pribadi benar-benar hilang.
//
// Email diganti dengan pola yang tidak mungkin dipakai orang lain, tapi
// tetap unik, karena kolomnya UNIQUE. Log kepatuhan TIDAK dihapus: sesuai
// batasan data-model-v2 bagian 10, riwayat medis dipertahankan untuk
// kepatuhan, dan sudah terputus dari identitas.
func (db *DB) AnonymizeUser(ctx context.Context, q Querier, userID string) error {
	tag, err := q.Exec(ctx, `
        UPDATE users
           SET deleted_at    = now(),
               is_active     = false,
               email         = 'deleted+' || id || '@invalid.local',
               full_name     = 'Akun Dihapus',
               phone         = NULL,
               password_hash = '!' || encode(gen_random_bytes(32), 'hex')
         WHERE id = $1 AND deleted_at IS NULL`, userID)
	if err != nil {
		return fmt.Errorf("menghapus akun: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// IsAdminScopeValid memeriksa bahwa scope yang tersimpan memang dikenal.
// Dipakai sebagai penjagaan tambahan setelah membaca dari DB.
func (db *DB) IsAdminScopeValid(ctx context.Context, q Querier, userID string) (bool, error) {
	var scopes []string
	err := q.QueryRow(ctx, `SELECT admin_scope FROM users WHERE id = $1`, userID).Scan(&scopes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, domain.ErrNotFound
		}
		return false, err
	}
	return domain.ValidScopeList(toAdminScopes(scopes)) == nil, nil
}

// PingQueryer adalah penjagaan kompilasi: pastikan pgxpool dan pgx.Tx
// sama-sama memenuhi Querier.
var (
	_ Querier = (pgx.Tx)(nil)
)
