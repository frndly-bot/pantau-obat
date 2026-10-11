package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	"github.com/frndly-bot/pantau-obat/backend/internal/repo/postgres"
)

// Users adalah layanan pengelolaan profil.
//
// Terpisah dari Auth karena tanggung jawabnya berbeda: Auth mengurus
// kredensial dan sesi, sedangkan ini mengurus data profil. Keduanya
// memakai tabel `users`, tapi jalur tulisnya tidak pernah beririsan.
type Users struct {
	db     *postgres.DB
	logger *slog.Logger
}

// NewUsers membuat layanan profil.
func NewUsers(db *postgres.DB, logger *slog.Logger) *Users {
	return &Users{db: db, logger: logger}
}

// ProfileUpdate adalah perubahan profil yang diminta.
//
// Memakai pointer untuk membedakan "tidak dikirim" dari "dikirim kosong".
// Tanpa ini, PATCH yang hanya ingin mengubah nama akan ikut mengosongkan
// telepon, dan itu merusak data tanpa disadari user.
type ProfileUpdate struct {
	FullName *string
	Phone    *string
	Timezone *string
}

// IsEmpty melaporkan apakah tidak ada field yang diubah.
func (u *ProfileUpdate) IsEmpty() bool {
	return u.FullName == nil && u.Phone == nil && u.Timezone == nil
}

// Profile mengembalikan profil user beserta ringkasan preferensinya.
func (s *Users) Profile(ctx context.Context, userID string) (*domain.User, error) {
	u, err := s.db.UserByID(ctx, s.db.Pool(), userID)
	if err != nil {
		return nil, err
	}
	if !u.IsActive {
		return nil, domain.ErrAccountInactive
	}
	return u, nil
}

// UpdateProfile mengubah nama, telepon, dan zona waktu.
//
// Zona waktu punya aturan khusus yang penting: mengubahnya TIDAK mengubah
// log lama, dan juga tidak mengubah blok jadwal yang sudah berjalan.
// Alasannya ada di data-model-v2 bagian 10 butir 6: slot_date dihitung dari
// zona, sehingga mengubah zona akan menggeser tanggal dosis yang sudah
// tercatat kalau diterapkan surut. Yang berubah hanya dosis ke depan.
//
// Karena itu perubahan zona dicatat di audit, supaya pergeseran waktu
// pengingat di kemudian hari bisa ditelusuri penyebabnya.
func (s *Users) UpdateProfile(ctx context.Context, userID string, in ProfileUpdate, meta Meta) (*domain.User, error) {
	if in.IsEmpty() {
		return nil, fmt.Errorf("%w: tidak ada field yang diubah", domain.ErrValidation)
	}

	// Validasi dilakukan sebelum menyentuh database, supaya pesannya jelas
	// dan tidak berupa pelanggaran CHECK yang samar.
	if in.FullName != nil {
		if err := domain.ValidateFullName(*in.FullName); err != nil {
			return nil, err
		}
		trimmed := strings.TrimSpace(*in.FullName)
		in.FullName = &trimmed
	}
	if in.Phone != nil && *in.Phone != "" {
		if err := domain.ValidatePhone(*in.Phone); err != nil {
			return nil, err
		}
		trimmed := strings.TrimSpace(*in.Phone)
		in.Phone = &trimmed
	}
	if in.Timezone != nil {
		if err := domain.ValidateTimezone(*in.Timezone); err != nil {
			return nil, err
		}
	}

	var before *domain.User
	var after *domain.User

	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		before, err = s.db.UserByID(ctx, tx, userID)
		if err != nil {
			return err
		}

		after, err = s.db.UpdateProfile(ctx, tx, userID, in.FullName, in.Phone, in.Timezone)
		if err != nil {
			return err
		}

		changed := changedProfileFields(in, before, after)
		if len(changed) == 0 {
			// Tidak ada yang benar-benar berubah. Tidak perlu menulis audit:
			// jejak untuk perubahan kosong hanya mengotori tabel.
			return nil
		}

		return s.db.WriteAudit(ctx, tx, postgres.AuditEntry{
			ActorID:      &userID,
			OwnerID:      &userID,
			ActorRole:    actorRoleOf(before),
			Action:       postgres.ActionUpdate,
			ResourceType: postgres.ResourceUser,
			ResourceID:   &userID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			BeforeState:  profileSnapshot(before),
			AfterState:   profileSnapshot(after),
			Meta:         map[string]any{"changed_fields": changed},
		})
	})
	if err != nil {
		return nil, err
	}
	return after, nil
}

// DeleteProfile melakukan soft-delete akun sekaligus menghapus identitas.
//
// Tiga hal terjadi bersamaan dan semuanya wajib:
//
//  1. Data identitas dianonimkan. Soft-delete saja tidak cukup: email dan
//     nama masih tersimpan, dan itu data pribadi.
//  2. Seluruh akses dicabut. Soft-delete tidak otomatis mematikan access
//     token yang sudah beredar.
//  3. Log kepatuhan DIPERTAHANKAN. Sesuai data-model-v2 bagian 10, riwayat
//     medis tetap ada untuk kepatuhan, dan sudah terputus dari identitas.
func (s *Users) DeleteProfile(ctx context.Context, userID string, meta Meta) error {
	return s.db.WithTx(ctx, func(tx pgx.Tx) error {
		before, err := s.db.UserByID(ctx, tx, userID)
		if err != nil {
			return err
		}

		if err := s.db.AnonymizeUser(ctx, tx, userID); err != nil {
			return err
		}

		if _, err := s.db.RevokeAccessAndSessions(ctx, tx, userID,
			domain.RevokeReasonUserRequest, &userID); err != nil {
			return err
		}

		return s.db.WriteAudit(ctx, tx, postgres.AuditEntry{
			ActorID:      &userID,
			OwnerID:      &userID,
			ActorRole:    actorRoleOf(before),
			Action:       postgres.ActionDelete,
			ResourceType: postgres.ResourceUser,
			ResourceID:   &userID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			BeforeState:  map[string]any{"email": before.Email, "full_name": before.FullName},
			AfterState:   map[string]any{"anonymized": true, "access_revoked": true},
			Meta:         map[string]any{"kept": "medication_logs", "reason": "user_request"},
		})
	})
}

// AuditLog mengembalikan riwayat akses data milik user sendiri.
//
// User hanya melihat jejak atas datanya sendiri. Ini yang menjawab
// kebutuhan "siapa saja yang membuka data saya".
func (s *Users) AuditLog(ctx context.Context, userID string, limit int) ([]AuditItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	rows, err := s.db.Pool().Query(ctx, `
        SELECT id, actor_id, actor_role, action, resource_type,
               resource_id, coalesce(host(ip),''), created_at
          FROM audit_log
         WHERE owner_id = $1
         ORDER BY id DESC
         LIMIT $2`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("membaca riwayat audit: %w", err)
	}
	defer rows.Close()

	var out []AuditItem
	for rows.Next() {
		var it AuditItem
		if err := rows.Scan(&it.ID, &it.ActorID, &it.ActorRole, &it.Action,
			&it.ResourceType, &it.ResourceID, &it.IP, &it.CreatedAt); err != nil {
			return nil, fmt.Errorf("membaca riwayat audit: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// AuditItem adalah satu baris riwayat untuk ditampilkan ke user.
//
// CreatedAt bertipe time.Time, bukan string. Scanning timestamptz ke
// string ditolak pgx dalam format biner, dan memaksa konversi di SQL
// hanya memindahkan masalahnya ke database. Format waktunya diurus di
// lapisan response.
type AuditItem struct {
	ID           int64
	ActorID      *string
	ActorRole    string
	Action       string
	ResourceType string
	ResourceID   *string
	IP           string
	CreatedAt    time.Time
}

// changedProfileFields melaporkan field mana yang n ilainya benar-benar berubah.
//
// Dibandingkan dengan nilai SESUDAH, bukan dengan input. Bedanya penting:
// kalau user mengirim nama yang sama persis, PATCH-nya sukses tapi tidak ada
// yang berubah, dan menulis audit untuk itu hanya menambah kebisingan.
func changedProfileFields(in ProfileUpdate, before, after *domain.User) []string {
	var changed []string
	if in.FullName != nil && before.FullName != after.FullName {
		changed = append(changed, "full_name")
	}
	switch {
	case before.Phone == nil && after.Phone != nil,
		before.Phone != nil && after.Phone == nil,
		before.Phone != nil && after.Phone != nil && *before.Phone != *after.Phone:
		changed = append(changed, "phone")
	}
	if in.Timezone != nil && before.Timezone != after.Timezone {
		changed = append(changed, "timezone")
	}
	return changed
}

// profileSnapshot mengambil bagian profil yang layak diaudit.
//
// Password hash dan data kredensial lain TIDAK pernah masuk sini.
func profileSnapshot(u *domain.User) map[string]any {
	if u == nil {
		return nil
	}
	return map[string]any{
		"full_name": u.FullName,
		"phone":     u.Phone,
		"timezone":  u.Timezone,
	}
}

// errNotFoundIfNoRows memetakan pgx.ErrNoRows ke sentinel domain.
func errNotFoundIfNoRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}
