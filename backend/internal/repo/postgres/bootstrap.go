package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// BootstrapAdminInput adalah permintaan pembuatan admin pertama.
type BootstrapAdminInput struct {
	Email     string
	Password  string
	FullName  string
	Scope     []domain.AdminScope
	GrantedBy *string
}

// BootstrapAdminResult melaporkan apa yang terjadi, bukan sekadar berhasil
// atau gagal.
//
// Perbedaan Created penting bagi pemanggil: menjalankan bootstrap pada
// database yang sudah punya admin BUKAN kegagalan, dan pesan yang menyamakan
// keduanya akan menyesatkan operator yang menjalankannya dua kali.
type BootstrapAdminResult struct {
	UserID   string
	Created  bool // true = akun baru dibuat; false = akun lama dinaikkan/dilewati
	Promoted bool // true = akun lama dinaikkan jadi admin
}

// BootstrapAdmin membuat admin pertama, atau menaikkan akun yang sudah ada.
//
// Kenapa ada di sini dan bukan lewat API: peran admin TIDAK PERNAH diterima
// dari klien (api-v2 bagian 4). Tanpa jalur ini, database baru tidak punya
// satu pun admin, dan seluruh rute /admin/* mustahil diuji — uji e2e berhenti
// di 401 selamanya.
//
// Penjagaan "hanya di luar produksi" ada di pemanggil (cmd/admin-bootstrap),
// bukan di sini, supaya fungsi ini tetap bisa diuji tanpa mengubah
// environment.
//
// Idempoten: akun yang sudah admin cukup dilaporkan, tidak digandakan dan
// scope-nya tidak ditimpa.
func (db *DB) BootstrapAdmin(ctx context.Context, in BootstrapAdminInput, passwordHash string) (*BootstrapAdminResult, error) {
	var out BootstrapAdminResult

	// GrantedBy kosong harus dikirim sebagai NULL, bukan string kosong:
	// kolom admin_granted_by bertipe uuid, dan '' tidak bisa diurai sebagai
	// UUID (SQLSTATE 22P02). Bootstrap dijalankan dari shell tanpa aktor,
	// jadi GrantedBy-nya NULL — kolom itu memang nullable untuk akun pertama.
	var grantedByPtr *string
	if in.GrantedBy != nil && *in.GrantedBy != "" {
		grantedByPtr = in.GrantedBy
	}

	// Peran admin menetapkan scope; validasinya dilakukan sebelum menyentuh
	// database supaya cakupan yang salah ketik tidak pernah tersimpan.
	scope := in.Scope
	if err := domain.ValidScopeList(scope); err != nil {
		return nil, fmt.Errorf("%w: %s", domain.ErrValidation, err.Error())
	}

	err := db.WithTx(ctx, func(tx pgx.Tx) error {
		existing, err := db.UserByEmail(ctx, tx, in.Email)
		switch {
		case err == nil:
			out.UserID = existing.ID
			if existing.Role == domain.RoleAdmin {
				// Sudah admin. TIDAK menimpa scope: operator mungkin sudah
				// mempersempitnya dengan sengaja.
				return nil
			}
			out.Promoted = true
			return db.GrantAdmin(ctx, tx, GrantAdminInput{
				UserID:    existing.ID,
				Scope:     scope,
				GrantedBy: grantedByPtr,
			})
		case errors.Is(err, domain.ErrNotFound):
			// Akun belum ada; lanjut ke pembuatan.
		default:
			return err
		}

		created, err := db.CreateUser(ctx, tx, domain.RegisterInput{
			Email:    in.Email,
			FullName: in.FullName,
			Timezone: domain.DefaultTimezone,
		}, passwordHash)
		if err != nil {
			return err
		}

		out.UserID = created.ID
		out.Created = true
		return db.GrantAdmin(ctx, tx, GrantAdminInput{
			UserID:    created.ID,
			Scope:     scope,
			GrantedBy: grantedByPtr,
		})
	})
	if err != nil {
		return nil, fmt.Errorf("bootstrap admin: %w", err)
	}
	return &out, nil
}
