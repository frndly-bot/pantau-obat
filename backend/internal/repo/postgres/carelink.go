package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// File ini mengurus relasi caregiver.
//
// Satu aturan yang menopang seluruh keamanan di sini: HAK AKSES DIPERIKSA
// ULANG SETIAP REQUEST, bukan disimpan di klien. Relasi yang dicabut harus
// langsung berlaku; kalau UI yang memegang statusnya, pencabutan baru
// terasa setelah user menutup aplikasinya.

const careLinkColumns = `
    id, owner_id, caregiver_id, invite_email, relation, role, status,
    invite_expires_at, decided_at, created_at, updated_at`

// CareLink adalah satu relasi antara pasien dan caregiver.
type CareLink struct {
	ID          string
	OwnerID     string
	CaregiverID *string

	InviteEmail     *string
	InviteExpiresAt *time.Time

	Relation domain.CareRelation
	Role     domain.CareRole
	Status   domain.CareStatus

	DecidedAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Active melaporkan apakah relasi ini memberi akses.
func (c *CareLink) Active() bool { return c != nil && c.Status == domain.CareAccepted }

// CanWriteLog melaporkan apakah peran ini boleh mencatat dosis.
//
// Hanya helper. Viewer tidak boleh menulis apa pun, sesuai TRD bagian 5.2.
// Keduanya sama-sama TIDAK boleh mengelola obat dan jadwal.
func (c *CareLink) CanWriteLog() bool {
	return c.Active() && c.Role == domain.CareHelper
}

// scanCareLink membaca satu baris care_links.
func scanCareLink(row pgx.Row) (*CareLink, error) {
	var c CareLink
	err := row.Scan(
		&c.ID, &c.OwnerID, &c.CaregiverID, &c.InviteEmail, &c.Relation,
		&c.Role, &c.Status, &c.InviteExpiresAt, &c.DecidedAt,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("membaca relasi caregiver: %w", err)
	}
	return &c, nil
}

// CreateCareLinkInput adalah data undangan baru.
type CreateCareLinkInput struct {
	OwnerID        string
	InviteEmail    string
	Relation       domain.CareRelation
	Role           domain.CareRole
	TokenHash      string
	TokenExpiresAt time.Time
	InvitedBy      string
}

// CreateCareLink membuat undangan caregiver.
//
// caregiver_id sengaja dibiarkan NULL: undangan boleh dikirim ke email yang
// belum terdaftar (TRD bagian F1), dan diisi saat undangan diterima.
func (db *DB) CreateCareLink(ctx context.Context, q Querier, in CreateCareLinkInput) (*CareLink, error) {
	row := q.QueryRow(ctx, `
        INSERT INTO care_links (
            owner_id, invite_email, relation, role, status,
            invite_token_hash, invite_expires_at, invited_by
        ) VALUES ($1,$2,$3,$4,'pending',$5,$6,$7)
        RETURNING `+careLinkColumns,
		in.OwnerID, in.InviteEmail, in.Relation, in.Role,
		in.TokenHash, in.TokenExpiresAt, in.InvitedBy,
	)

	c, err := scanCareLink(row)
	if err != nil {
		// Undangan yang masih menganggur untuk email yang sama ditolak
		// uq_care_pending_invite. Dipetakan ke sentinel yang tepat supaya
		// handler bisa membalas 409 dengan pesan yang jelas.
		if IsUniqueViolation(err) {
			return nil, domain.ErrInvitePending
		}
		return nil, err
	}
	return c, nil
}

// CareLinkByID mengambil relasi berdasarkan id.
func (db *DB) CareLinkByID(ctx context.Context, q Querier, id string) (*CareLink, error) {
	row := q.QueryRow(ctx, `
        SELECT `+careLinkColumns+`
          FROM care_links
         WHERE id = $1 AND deleted_at IS NULL`, id)
	return scanCareLink(row)
}

// CareLinkByToken mengambil undangan berdasarkan hash token.
func (db *DB) CareLinkByToken(ctx context.Context, q Querier, tokenHash string) (*CareLink, error) {
	row := q.QueryRow(ctx, `
        SELECT `+careLinkColumns+`
          FROM care_links
         WHERE invite_token_hash = $1 AND status = 'pending' AND deleted_at IS NULL`,
		tokenHash)
	return scanCareLink(row)
}

// ActiveLink antara owner dan caregiver, bila ada.
//
// Inilah pemeriksaan yang dijalankan di setiap request caregiver. Memakai
// index partial ix_care_caregiver, sehingga murah.
func (db *DB) ActiveLink(ctx context.Context, q Querier, ownerID, caregiverID string) (*CareLink, error) {
	row := q.QueryRow(ctx, `
        SELECT `+careLinkColumns+`
          FROM care_links
         WHERE owner_id = $1 AND caregiver_id = $2
           AND status = 'accepted' AND deleted_at IS NULL`, ownerID, caregiverID)
	return scanCareLink(row)
}

// ListCareLinksFilter adalah syarat pencarian relasi.
type ListCareLinksFilter struct {
	// AsOwner=true mencari relasi yang dimiliki user (siapa yang memantau
	// saya), false mencari yang dia pantau (siapa yang saya pantau).
	AsOwner bool
	UserID  string
	Status  []domain.CareStatus
}

// ListCareLinks mengambil daftar relasi.
func (db *DB) ListCareLinks(ctx context.Context, q Querier, f ListCareLinksFilter) ([]CareLink, error) {
	col := "caregiver_id"
	if f.AsOwner {
		col = "owner_id"
	}

	rows, err := q.Query(ctx, `
        SELECT `+careLinkColumns+`
          FROM care_links
         WHERE `+col+` = $1 AND deleted_at IS NULL
         ORDER BY created_at DESC`, f.UserID)
	if err != nil {
		return nil, fmt.Errorf("mendaftar relasi caregiver: %w", err)
	}
	defer rows.Close()

	var out []CareLink
	for rows.Next() {
		c, err := scanCareLink(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// AcceptCareLink menerima undangan.
//
// caregiver_id diisi di sini, karena saat undangan dibuat mungkin belum ada.
// Token langsung dikosongkan supaya tidak bisa dipakai dua kali; index
// uq_care_token yang partial juga berhenti berlaku begitu statusnya berubah.
func (db *DB) AcceptCareLink(ctx context.Context, q Querier, linkID, caregiverID string) (*CareLink, error) {
	row := q.QueryRow(ctx, `
        UPDATE care_links
           SET status = 'accepted',
               caregiver_id = $2,
               invite_token_hash = NULL,
               decided_at = now()
         WHERE id = $1 AND status = 'pending' AND deleted_at IS NULL
        RETURNING `+careLinkColumns, linkID, caregiverID)

	c, err := scanCareLink(row)
	if err != nil {
		if IsUniqueViolation(err) {
			// Sudah ada relasi aktif antara pasangan ini. Bisa terjadi bila
			// undangan dikirim ulang sebelum yang lama diputus.
			return nil, domain.ErrInvitePending
		}
		return nil, err
	}
	return c, nil
}

// RejectCareLink menolak undangan.
func (db *DB) RejectCareLink(ctx context.Context, q Querier, linkID, caregiverID string) (*CareLink, error) {
	row := q.QueryRow(ctx, `
        UPDATE care_links
           SET status = 'rejected',
               caregiver_id = $2,
               invite_token_hash = NULL,
               decided_at = now()
         WHERE id = $1 AND status = 'pending' AND deleted_at IS NULL
        RETURNING `+careLinkColumns, linkID, caregiverID)
	return scanCareLink(row)
}

// UpdateCareLinkRole mengubah tingkat izin atau hubungan.
func (db *DB) UpdateCareLinkRole(ctx context.Context, q Querier, linkID string, role *domain.CareRole, relation *domain.CareRelation) (*CareLink, error) {
	row := q.QueryRow(ctx, `
        UPDATE care_links
           SET role     = COALESCE($2, role),
               relation = COALESCE($3, relation)
         WHERE id = $1 AND deleted_at IS NULL
        RETURNING `+careLinkColumns, linkID, role, relation)
	return scanCareLink(row)
}

// RevokeCareLink mencabut relasi.
//
// Status diubah, bukan baris dihapus: riwayat "pernah ada caregiver" perlu
// tetap ada untuk audit, dan log yang dicatat caregiver itu menunjuk ke
// relasi ini lewat recorded_by.
func (db *DB) RevokeCareLink(ctx context.Context, q Querier, linkID string) error {
	tag, err := q.Exec(ctx, `
        UPDATE care_links
           SET status = 'revoked', decided_at = now()
         WHERE id = $1 AND status IN ('pending','accepted') AND deleted_at IS NULL`, linkID)
	if err != nil {
		return fmt.Errorf("mencabut relasi caregiver: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// ExpireStaleInvites menandai undangan yang sudah lewat masa berlaku.
//
// Dijalankan worker, bukan di jalur request. Undangan yang kedaluwarsa tetapi
// masih berstatus pending akan menghalangi undangan baru untuk email yang
// sama, karena uq_care_pending_invite.
func (db *DB) ExpireStaleInvites(ctx context.Context, q Querier) (int64, error) {
	tag, err := q.Exec(ctx, `
        UPDATE care_links
           SET status = 'expired'
         WHERE status = 'pending'
           AND invite_expires_at IS NOT NULL
           AND invite_expires_at < now()
           AND deleted_at IS NULL`)
	if err != nil {
		return 0, fmt.Errorf("menandai undangan kedaluwarsa: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ListOwnersForCaregiver mengambil pemilik yang dipantau seorang caregiver.
func (db *DB) ListOwnersForCaregiver(ctx context.Context, q Querier, caregiverID string) ([]CareLink, error) {
	return db.ListCareLinks(ctx, q, ListCareLinksFilter{
		AsOwner: false,
		UserID:  caregiverID,
	})
}
