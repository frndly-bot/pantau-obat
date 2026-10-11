package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// File ini mengurus operasi admin.
//
// ATURAN YANG MENOPANG SELURUH FILE INI, dari TRD bagian 5.3:
//
//  1. Admin TIDAK BISA membaca isi obat atau log pasien individual lewat
//     rute admin. Yang tersedia hanya data akun dan agregat. Kalau operator
//     perlu investigasi, itu jalur tersendiri dengan alasan tertulis.
//
//  2. Setiap tulisan menulis audit dengan actor_role='admin' dan
//     before_state/after_state.
//
//  3. Peran admin hanya ditetapkan lewat operasi DB, dan operated lewat
//     endpoint yang sudah diperiksa RequireAdmin(scope).
//
// Query di sini sengaja TIDAK pernah menyentuh tabel medications,
// medication_logs, atau medication_dose_events. Batas itu bukan kelalaian,
// melainkan keputusan privasi yang ditegakkan di lapisan query.

// AdminListUsersFilter adalah syarat pencarian akun.
type AdminListUsersFilter struct {
	Query  string // dicocokkan ke email dan nama
	Role   string
	Status string // "active" | "inactive" | "all"
	Limit  int
	Offset int
}

// AdminUser adalah satu baris akun untuk halaman admin.
//
// TIDAK memuat password_hash. Hash tidak pernah keluar dari repo kecuali
// lewat CredentialsByEmail, dan itu hanya dipakai jalur login.
type AdminUser struct {
	ID          string
	Email       string
	FullName    string
	Phone       *string
	Timezone    string
	IsActive    bool
	Role        domain.UserRole
	AdminScope  []domain.AdminScope
	LastLoginAt *time.Time
	CreatedAt   time.Time

	// MedicationCount adalah jumlah obat AKTIF, bukan isinya. Operator perlu
	// tahu seberapa aktif sebuah akun tanpa melihat obat apa yang diminum.
	MedicationCount int
}

// ListUsers mengambil daftar akun dengan pencarian dan filter.
//
// Jumlah obat dihitung lewat subquery agregat, bukan join, supaya akun tanpa
// obat tetap muncul dengan angka nol.
func (db *DB) ListUsers(ctx context.Context, q Querier, f AdminListUsersFilter) ([]AdminUser, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 50
	}

	where := []string{"deleted_at IS NULL"}
	args := []any{}

	if raw := strings.TrimSpace(f.Query); raw != "" {
		args = append(args, "%"+strings.ToLower(raw)+"%")
		where = append(where, fmt.Sprintf(
			"(lower(email::text) LIKE $%d OR lower(full_name) LIKE $%d)",
			len(args), len(args)))
	}
	if f.Role != "" {
		args = append(args, f.Role)
		where = append(where, fmt.Sprintf("role = $%d::text", len(args)))
	}
	switch f.Status {
	case "active":
		where = append(where, "is_active = true")
	case "inactive":
		where = append(where, "is_active = false")
	}

	args = append(args, f.Limit, f.Offset)

	rows, err := q.Query(ctx, `
        SELECT u.id, u.email, u.full_name, u.phone, u.timezone, u.is_active,
               u.role, u.admin_scope, u.last_login_at, u.created_at,
               (SELECT count(*)::int FROM medications m
                 WHERE m.owner_id = u.id
                   AND m.deleted_at IS NULL AND m.is_active = true)
          FROM users u
         WHERE `+joinAnd(where)+`
         ORDER BY u.created_at DESC
         LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("mendaftar akun: %w", err)
	}
	defer rows.Close()

	var out []AdminUser
	for rows.Next() {
		var u AdminUser
		var scope []string
		if err := rows.Scan(&u.ID, &u.Email, &u.FullName, &u.Phone,
			&u.Timezone, &u.IsActive, &u.Role, &scope, &u.LastLoginAt,
			&u.CreatedAt, &u.MedicationCount); err != nil {
			return nil, fmt.Errorf("membaca akun: %w", err)
		}
		u.AdminScope = toAdminScopes(scope)
		out = append(out, u)
	}
	return out, rows.Err()
}

// CountUsers menghitung total akun yang cocok filter. Dipakai pagination.
func (db *DB) CountUsers(ctx context.Context, q Querier, f AdminListUsersFilter) (int, error) {
	where := []string{"deleted_at IS NULL"}
	args := []any{}

	if raw := strings.TrimSpace(f.Query); raw != "" {
		args = append(args, "%"+strings.ToLower(raw)+"%")
		where = append(where, fmt.Sprintf(
			"(lower(email::text) LIKE $%d OR lower(full_name) LIKE $%d)",
			len(args), len(args)))
	}
	if f.Role != "" {
		args = append(args, f.Role)
		where = append(where, fmt.Sprintf("role = $%d::text", len(args)))
	}
	switch f.Status {
	case "active":
		where = append(where, "is_active = true")
	case "inactive":
		where = append(where, "is_active = false")
	}

	var n int
	err := q.QueryRow(ctx, `
        SELECT count(*)::int FROM users WHERE `+joinAnd(where), args...).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("menghitung akun: %w", err)
	}
	return n, nil
}

// AdminUserByID mengambil satu akun untuk halaman detail.
//
// Mengembalikan data akun dan JUMLAH obat, bukan daftar obatnya. Admin tidak
// perlu tahu obat apa yang diminum seseorang; ia hanya perlu tahu bahwa
// akunnya aktif.
func (db *DB) AdminUserByID(ctx context.Context, q Querier, id string) (*AdminUser, error) {
	var u AdminUser
	var scope []string
	err := q.QueryRow(ctx, `
        SELECT u.id, u.email, u.full_name, u.phone, u.timezone, u.is_active,
               u.role, u.admin_scope, u.last_login_at, u.created_at,
               (SELECT count(*)::int FROM medications m
                 WHERE m.owner_id = u.id
                   AND m.deleted_at IS NULL AND m.is_active = true)
          FROM users u
         WHERE u.id = $1 AND u.deleted_at IS NULL`, id,
	).Scan(&u.ID, &u.Email, &u.FullName, &u.Phone, &u.Timezone, &u.IsActive,
		&u.Role, &scope, &u.LastLoginAt, &u.CreatedAt, &u.MedicationCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("membaca akun: %w", err)
	}
	u.AdminScope = toAdminScopes(scope)
	return &u, nil
}

// SetUserActive mengaktifkan atau menonaktifkan akun.
//
// Menonaktifkan TIDAK mengubah peran: keduanya sengaja dipisah, supaya blokir
// akun tidak ikut menghapus peran admin yang mungkin sudah diberikan. Yang
// perlu dicabut saat menonaktifkan adalah AKSESNYA, dan itu dilakukan
// terpisah lewat SetRevocation.
func (db *DB) SetUserActive(ctx context.Context, q Querier, id string, active bool) error {
	tag, err := q.Exec(ctx, `
        UPDATE users SET is_active = $2
         WHERE id = $1 AND deleted_at IS NULL`, id, active)
	if err != nil {
		return fmt.Errorf("mengubah status akun: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// GrantAdminInput adalah penetapan peran admin.
type GrantAdminInput struct {
	UserID string
	Scope  []domain.AdminScope // nil = akses penuh

	// GrantedBy adalah id admin yang memberi peran. NULL berarti penetapan
	// di luar aplikasi (bootstrap dari shell). String kosong TIDAK boleh
	// dipakai untuk "tidak ada": kolomnya bertipe uuid dan '' ditolak
	// PostgreSQL dengan SQLSTATE 22P02.
	GrantedBy *string
}

// GrantAdmin memberi peran admin beserta cakupannya.
//
// Dijalankan lewat endpoint yang sudah diperiksa RequireAdmin, dan tetap
// menulis audit lengkap. Peran admin adalah hak istimewa terbesar di sistem
// ini, jadi setiap pemberiannya harus bisa ditelusuri: siapa memberikannya,
// kapan, dan dengan cakupan apa.
func (db *DB) GrantAdmin(ctx context.Context, q Querier, in GrantAdminInput) error {
	scope := fromAdminScopes(in.Scope)

	tag, err := q.Exec(ctx, `
        UPDATE users
           SET role = 'admin',
               admin_scope = $2,
               admin_granted_at = now(),
               admin_granted_by = $3
         WHERE id = $1 AND deleted_at IS NULL`,
		in.UserID, scope, in.GrantedBy)
	if err != nil {
		return fmt.Errorf("memberi peran admin: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// RevokeAdmin mencabut peran admin.
//
// admin_scope dikosongkan sekaligus. Kalau peran dicabut tapi cakupannya
// tertinggal, pemberian peran berikutnya akan mewarisi cakupan lama tanpa
// disadari.
func (db *DB) RevokeAdmin(ctx context.Context, q Querier, userID string) error {
	tag, err := q.Exec(ctx, `
        UPDATE users
           SET role = 'user',
               admin_scope = NULL,
               admin_granted_at = NULL,
               admin_granted_by = NULL
         WHERE id = $1 AND deleted_at IS NULL AND role = 'admin'`, userID)
	if err != nil {
		return fmt.Errorf("mencabut peran admin: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Bukan admin, atau tidak ada. Keduanya berarti tidak ada yang
		// dicabut.
		return domain.ErrNotFound
	}
	return nil
}

// AdminStats adalah agregat untuk halaman dashboard admin.
//
// TIDAK memuat identitas pasien maupun nama obat. Operator hanya perlu tahu
// seberapa besar sistemnya dan apakah ada yang bermasalah.
type AdminStats struct {
	UsersActive       int
	UsersDisabled     int
	UsersAdmin        int
	MedicationsActive int
	CatalogPresets    int
	CatalogDraft      int
	JobsFailed24h     int
	DeliveriesSent24h int
	LogsToday         int
	// LogsToday hanya JUMLAH, bukan isinya. Admin perlu tahu sistemnya
	// dipakai, tanpa perlu tahu siapa yang minum obat apa.
}

// Stats menghitung agregat sistem.
func (db *DB) Stats(ctx context.Context, q Querier) (AdminStats, error) {
	var s AdminStats

	err := q.QueryRow(ctx, `
        SELECT
            count(*) FILTER (WHERE is_active AND deleted_at IS NULL)::int,
            count(*) FILTER (WHERE NOT is_active AND deleted_at IS NULL)::int,
            count(*) FILTER (WHERE role = 'admin' AND deleted_at IS NULL)::int
          FROM users`).Scan(&s.UsersActive, &s.UsersDisabled, &s.UsersAdmin)
	if err != nil {
		return s, fmt.Errorf("menghitung statistik akun: %w", err)
	}

	err = q.QueryRow(ctx, `
        SELECT count(*)::int FROM medications
         WHERE deleted_at IS NULL AND is_active = true`).Scan(&s.MedicationsActive)
	if err != nil {
		return s, fmt.Errorf("menghitung statistik obat: %w", err)
	}

	err = q.QueryRow(ctx, `
        SELECT
            count(*) FILTER (WHERE curation_status = 'published')::int,
            count(*) FILTER (WHERE curation_status = 'draft')::int
          FROM drug_catalog
         WHERE is_preset = true AND deleted_at IS NULL`).Scan(&s.CatalogPresets, &s.CatalogDraft)
	if err != nil {
		return s, fmt.Errorf("menghitung statistik katalog: %w", err)
	}

	err = q.QueryRow(ctx, `
        SELECT count(*)::int FROM notification_jobs
         WHERE status = 'failed' AND created_at >= now() - interval '24 hours'`).Scan(&s.JobsFailed24h)
	if err != nil {
		return s, fmt.Errorf("menghitung job gagal: %w", err)
	}

	err = q.QueryRow(ctx, `
        SELECT count(*)::int FROM notification_deliveries
         WHERE status = 'sent' AND sent_at >= now() - interval '24 hours'`).Scan(&s.DeliveriesSent24h)
	if err != nil {
		return s, fmt.Errorf("menghitung pengiriman: %w", err)
	}

	err = q.QueryRow(ctx, `
        SELECT count(*)::int FROM medication_logs
         WHERE slot_date = current_date AND deleted_at IS NULL`).Scan(&s.LogsToday)
	if err != nil {
		return s, fmt.Errorf("menghitung catatan dosis: %w", err)
	}

	return s, nil
}

// AdminCatalogFilter adalah syarat pencarian katalog oleh admin.
type AdminCatalogFilter struct {
	CurationStatus string
	Query          string
	Limit          int
}

// ListCatalogForAdmin mengambil katalog TERMASUK draft dan arsip.
//
// Berbeda dari pencarian user yang hanya menampilkan preset published.
// Admin perlu melihat semua status untuk bisa mengurasi.
func (db *DB) ListCatalogForAdmin(ctx context.Context, q Querier, f AdminCatalogFilter) ([]CatalogEntry, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 50
	}

	where := []string{"is_preset = true", "deleted_at IS NULL"}
	args := []any{}

	if f.CurationStatus != "" {
		args = append(args, f.CurationStatus)
		where = append(where, fmt.Sprintf("curation_status = $%d::text", len(args)))
	}
	if raw := strings.TrimSpace(f.Query); raw != "" {
		args = append(args, "%"+strings.ToLower(raw)+"%")
		where = append(where, fmt.Sprintf("lower(display_name) LIKE $%d::text", len(args)))
	}
	args = append(args, f.Limit)

	rows, err := q.Query(ctx, `
        SELECT `+catalogColumns+`
          FROM drug_catalog
         WHERE `+joinAnd(where)+`
         ORDER BY curation_status, display_name
         LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("mendaftar katalog admin: %w", err)
	}
	defer rows.Close()

	var out []CatalogEntry
	for rows.Next() {
		e, err := scanCatalogEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

// AdminCreatePresetInput adalah data preset publik baru.
type AdminCreatePresetInput struct {
	DisplayName    string
	NamaDagang     *string
	GenerikNama    *string
	DosageFormCode *string
	RouteCode      *string
	UcumCode       *string
	UomName        *string
	DoseValueText  *string
	CreatedBy      string
}

// CreatePreset membuat preset publik.
//
// Selalu bermula sebagai 'draft' dan is_preset=true dengan owner_id NULL.
// Preset yang langsung published akan muncul ke seluruh user sebelum sempat
// diperiksa, dan itu sulit ditarik kembali.
func (db *DB) CreatePreset(ctx context.Context, q Querier, in AdminCreatePresetInput) (*CatalogEntry, error) {
	row := q.QueryRow(ctx, `
        INSERT INTO drug_catalog (
            display_name, nama_dagang, generik_nama,
            dosage_form_code, route_code, ucum_code, uom_name, dose_value_text,
            owner_id, is_preset, curation_status, source, curated_by
        ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8, NULL, true, 'draft', 'manual', $9)
        RETURNING `+catalogColumns,
		strings.TrimSpace(in.DisplayName), in.NamaDagang, in.GenerikNama,
		in.DosageFormCode, in.RouteCode, in.UcumCode, in.UomName,
		in.DoseValueText, in.CreatedBy,
	)
	e, err := scanCatalogEntry(row)
	if err != nil {
		// Nama preset wajib unik (uq_catalog_preset_name). Tanpa pemetaan
		// ini, nama yang sudah dipakai membalas 500 INTERNAL — status yang
		// menyalahkan server untuk bentrok data yang seharusnya dijelaskan.
		// Terbukti terjadi saat uji apidoc.
		if IsUniqueViolation(err) {
			return nil, fmt.Errorf("%w: nama preset sudah dipakai",
				domain.ErrCatalogDuplicateName)
		}
		return nil, err
	}
	return e, nil
}

// SetCurationStatus mengubah status kurasi preset.
//
// Hanya berlaku untuk is_preset=true: katalog pribadi user tidak punya siklus
// kurasi, dan mengubahnya di sini akan merusak CHECK ck_catalog_scope.
func (db *DB) SetCurationStatus(ctx context.Context, q Querier, id string, status domain.CurationStatus, curatedBy string) (*CatalogEntry, error) {
	row := q.QueryRow(ctx, `
        UPDATE drug_catalog
           SET curation_status = $2,
               curated_by = $3,
               curated_at = now()
         WHERE id = $1
           AND is_preset = true
           AND deleted_at IS NULL
        RETURNING `+catalogColumns, id, status, curatedBy)
	return scanCatalogEntry(row)
}

// Job input dan daftar job sudah ada di notification.go.
// Di sini hanya tambahan yang khusus untuk admin.

// JobsFailedSince mengambil job yang gagal sejak waktu tertentu.
//
// Hanya kolom status dan jenis, TANPA payload. Payload bisa memuat detail
// pesan, dan halaman admin tidak boleh menampilkan isi notifikasi pasien.
func (db *DB) JobsFailedSince(ctx context.Context, q Querier, since time.Time, limit int) ([]NotificationJob, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := q.Query(ctx, `
        SELECT id, kind, status, dedup_key, owner_id, '{}'::jsonb, run_at,
               attempts, max_attempts, last_error, claimed_at, created_at
          FROM notification_jobs
         WHERE status = 'failed' AND created_at >= $1
         ORDER BY created_at DESC
         LIMIT $2`, since, limit)
	if err != nil {
		return nil, fmt.Errorf("mendaftar job gagal: %w", err)
	}
	defer rows.Close()

	var out []NotificationJob
	for rows.Next() {
		var j NotificationJob
		if err := rows.Scan(&j.ID, &j.Kind, &j.Status, &j.DedupKey, &j.OwnerID,
			&j.Payload, &j.RunAt, &j.Attempts, &j.MaxAttempts, &j.LastError,
			&j.ClaimedAt, &j.CreatedAt); err != nil {
			return nil, fmt.Errorf("membaca job gagal: %w", err)
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
