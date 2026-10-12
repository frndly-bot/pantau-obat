package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	"github.com/frndly-bot/pantau-obat/backend/internal/repo/postgres"
)

// CareLinks adalah layanan relasi caregiver.
//
// Seluruh hak akses diperiksa ULANG di sini pada setiap panggilan, bukan
// disimpan dari pemeriksaan sebelumnya. Alasannya bukan kehati-hatian
// berlebihan: caregiver yang aksesnya baru dicabut harus langsung kehilangan
// akses. Kalau statusnya dipegang UI atau di-cache di token, pencabutan baru
// terasa setelah user menutup aplikasinya.
type CareLinks struct {
	db     *postgres.DB
	logger *slog.Logger
}

// NewCareLinks membuat layanan caregiver.
func NewCareLinks(db *postgres.DB, logger *slog.Logger) *CareLinks {
	return &CareLinks{db: db, logger: logger}
}

// InviteValidity adalah masa berlaku undangan caregiver, sesuai TRD bagian F1.
const InviteValidity = 7 * 24 * time.Hour

// CreateInviteInput adalah permintaan undangan.
type CreateInviteInput struct {
	InviteEmail string
	Relation    domain.CareRelation
	Role        domain.CareRole
}

// CreateInvite membuat undangan caregiver.
//
// Mengembalikan token MENTAH, dan hanya sekali ini ia bisa dilihat. Yang
// disimpan di database hanya SHA-256-nya, jadi bocornya isi tabel tidak
// memberi penyerang token yang bisa dipakai.
func (s *CareLinks) CreateInvite(ctx context.Context, ownerID string, in CreateInviteInput, meta Meta) (*InviteResult, error) {
	email, err := domain.NormalizeEmail(in.InviteEmail)
	if err != nil {
		return nil, err
	}
	if in.Role == "" {
		// Default viewer, sesuai TRD bagian F5: tingkat izin paling aman
		// sebagai titik awal.
		in.Role = domain.CareViewer
	}
	if !in.Role.Valid() {
		return nil, fmt.Errorf("%w: tingkat izin %q tidak dikenal", domain.ErrValidation, in.Role)
	}
	if in.Relation == "" {
		in.Relation = domain.RelationOther
	}
	if !in.Relation.Valid() {
		return nil, fmt.Errorf("%w: hubungan %q tidak dikenal", domain.ErrValidation, in.Relation)
	}

	// Tidak boleh mengundang diri sendiri. Ditegakkan CHECK ck_care_not_self
	// di database, tapi diperiksa di sini supaya pesannya jelas.
	if strings.EqualFold(email, "") {
		return nil, fmt.Errorf("%w: email undangan wajib diisi", domain.ErrValidation)
	}

	token, err := newInviteToken()
	if err != nil {
		return nil, err
	}
	expires := time.Now().Add(InviteValidity)

	var link *postgres.CareLink
	err = s.db.WithTx(ctx, func(tx pgx.Tx) error {
		// Kalau email yang diundang sudah terdaftar, pastikan bukan diri
		// sendiri. Kalau belum terdaftar, undangan tetap boleh dikirim.
		if existing, err := s.db.UserByEmail(ctx, tx, email); err == nil {
			if existing.ID == ownerID {
				return fmt.Errorf("%w: tidak bisa mengundang diri sendiri", domain.ErrValidation)
			}
		} else if !isNotFound(err) {
			return err
		}

		created, err := s.db.CreateCareLink(ctx, tx, postgres.CreateCareLinkInput{
			OwnerID:        ownerID,
			InviteEmail:    email,
			Relation:       in.Relation,
			Role:           in.Role,
			TokenHash:      hashInviteToken(token),
			TokenExpiresAt: expires,
			InvitedBy:      ownerID,
		})
		if err != nil {
			return err
		}
		link = created

		// Nama pasien TIDAK masuk audit. Yang dicatat hanya email yang
		// diundang, karena itu memang perlu untuk menelusuri siapa diundang.
		return s.db.WriteAudit(ctx, tx, postgres.AuditEntry{
			ActorID:      &ownerID,
			OwnerID:      &ownerID,
			ActorRole:    string(domain.ActorUser),
			Action:       postgres.ActionInvite,
			ResourceType: postgres.ResourceCare,
			ResourceID:   &created.ID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			AfterState: map[string]any{
				"invite_email": email,
				"role":         string(in.Role),
				"relation":     string(in.Relation),
			},
		})
	})
	if err != nil {
		return nil, err
	}

	return &InviteResult{Link: link, Token: token, ExpiresAt: expires}, nil
}

// InviteResult adalah hasil pembuatan undangan.
type InviteResult struct {
	Link      *postgres.CareLink
	Token     string
	ExpiresAt time.Time
}

// AcceptInvite menerima undangan.
func (s *CareLinks) AcceptInvite(ctx context.Context, caregiverID, token string, meta Meta) (*postgres.CareLink, error) {
	return s.decideInvite(ctx, caregiverID, token, true, meta)
}

// RejectInvite menolak undangan.
func (s *CareLinks) RejectInvite(ctx context.Context, caregiverID, token string, meta Meta) (*postgres.CareLink, error) {
	return s.decideInvite(ctx, caregiverID, token, false, meta)
}

// decideInvite menangani penerimaan maupun penolakan undangan.
//
// Digabung karena kedua jalur memeriksa hal yang sama: token harus ada,
// belum kedaluwarsa, dan seorang user tidak boleh menerima undangan yang
// ditujukan untuknya sendiri.
func (s *CareLinks) decideInvite(ctx context.Context, caregiverID, token string, accept bool, meta Meta) (*postgres.CareLink, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("%w: token undangan wajib diisi", domain.ErrValidation)
	}

	var out *postgres.CareLink
	action := postgres.ActionReject
	if accept {
		action = postgres.ActionAccept
	}

	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		link, err := s.db.CareLinkByToken(ctx, tx, hashInviteToken(token))
		if err != nil {
			// Token tidak ada, atau statusnya bukan pending lagi.
			// Dijawab sebagai "tidak ditemukan" supaya tidak membocorkan
			// bahwa undangan itu memang pernah ada.
			return domain.ErrNotFound
		}

		if link.InviteExpiresAt != nil && time.Now().After(*link.InviteExpiresAt) {
			// Bertanda kedaluwarsa, dan undangan baru bisa dibuat. Tanpa
			// ini, undangan basi menahan undangan berikutnya untuk email
			// yang sama.
			if _, err := s.db.Pool().Exec(ctx, `
                UPDATE care_links SET status = 'expired' WHERE id = $1`, link.ID); err != nil {
				s.logger.WarnContext(ctx, "gagal menandai undangan kedaluwarsa",
					slog.String("link_id", link.ID))
			}
			return fmt.Errorf("%w: undangan sudah kedaluwarsa", domain.ErrValidation)
		}

		// Pasien tidak bisa menerima undangannya sendiri.
		if link.OwnerID == caregiverID {
			return fmt.Errorf("%w: tidak bisa menerima undangan sendiri", domain.ErrValidation)
		}

		// Kalau undangan ditujukan ke email tertentu, hanya pemilik email
		// itu yang boleh menerimanya. Tanpa pemeriksaan ini, siapa pun yang
		// menemukan tokennya bisa memantau data kesehatan orang lain.
		if link.InviteEmail != nil {
			user, err := s.db.UserByID(ctx, tx, caregiverID)
			if err != nil {
				return err
			}
			if !strings.EqualFold(user.Email, *link.InviteEmail) {
				return domain.ErrForbidden
			}
		}

		var decided *postgres.CareLink
		if accept {
			decided, err = s.db.AcceptCareLink(ctx, tx, link.ID, caregiverID)
		} else {
			decided, err = s.db.RejectCareLink(ctx, tx, link.ID, caregiverID)
		}
		if err != nil {
			return err
		}
		out = decided

		return s.db.WriteAudit(ctx, tx, postgres.AuditEntry{
			ActorID:      &caregiverID,
			OwnerID:      &link.OwnerID,
			ActorRole:    string(domain.ActorUser),
			Action:       action,
			ResourceType: postgres.ResourceCare,
			ResourceID:   &link.ID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			AfterState:   map[string]any{"status": string(decided.Status)},
		})
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListLinks mengambil daftar relasi.
//
// asOwner=true berarti "siapa yang memantau saya"; false berarti "siapa yang
// saya pantau". Keduanya dibedakan karena tampilannya berbeda di aplikasi.
func (s *CareLinks) ListLinks(ctx context.Context, userID string, asOwner bool) ([]postgres.CareLink, error) {
	return s.db.ListCareLinks(ctx, s.db.Pool(), postgres.ListCareLinksFilter{
		AsOwner: asOwner,
		UserID:  userID,
	})
}

// UpdateLink mengubah tingkat izin atau hubungan.
//
// Hanya PEMILIK yang boleh mengubah, bukan caregiver. Kalau caregiver bisa
// menaikkan perannya sendiri dari viewer jadi helper, batas izinnya tidak
// berarti apa-apa.
func (s *CareLinks) UpdateLink(ctx context.Context, ownerID, linkID string, role *domain.CareRole, relation *domain.CareRelation, meta Meta) (*postgres.CareLink, error) {
	if role == nil && relation == nil {
		return nil, fmt.Errorf("%w: tidak ada field yang diubah", domain.ErrValidation)
	}
	if role != nil && !role.Valid() {
		return nil, fmt.Errorf("%w: tingkat izin %q tidak dikenal", domain.ErrValidation, *role)
	}
	if relation != nil && !relation.Valid() {
		return nil, fmt.Errorf("%w: hubungan %q tidak dikenal", domain.ErrValidation, *relation)
	}

	var out *postgres.CareLink
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		link, err := s.db.CareLinkByID(ctx, tx, linkID)
		if err != nil {
			return err
		}
		// Bukan pemiliknya: dijawab "tidak ditemukan", bukan "tidak
		// berhak", supaya tidak membocorkan bahwa relasi itu ada.
		if link.OwnerID != ownerID {
			return domain.ErrNotFound
		}
		if link.Status == domain.CareRevoked {
			return fmt.Errorf("%w: relasi ini sudah dicabut", domain.ErrValidation)
		}

		updated, err := s.db.UpdateCareLinkRole(ctx, tx, linkID, role, relation)
		if err != nil {
			return err
		}
		out = updated

		return s.db.WriteAudit(ctx, tx, postgres.AuditEntry{
			ActorID:      &ownerID,
			OwnerID:      &ownerID,
			ActorRole:    string(domain.ActorUser),
			Action:       postgres.ActionUpdate,
			ResourceType: postgres.ResourceCare,
			ResourceID:   &linkID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			BeforeState:  map[string]any{"role": string(link.Role), "relation": string(link.Relation)},
			AfterState:   map[string]any{"role": string(updated.Role), "relation": string(updated.Relation)},
		})
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RevokeLink mencabut relasi.
//
// Boleh dilakukan PEMILIK maupun CAREGIVER: pasien berhak berhenti dipantau,
// dan caregiver berhak berhenti memantau. Keduanya menghasilkan status yang
// sama, dan aksesnya langsung hilang.
func (s *CareLinks) RevokeLink(ctx context.Context, userID, linkID string, meta Meta) error {
	return s.db.WithTx(ctx, func(tx pgx.Tx) error {
		link, err := s.db.CareLinkByID(ctx, tx, linkID)
		if err != nil {
			return err
		}

		isOwner := link.OwnerID == userID
		isCaregiver := link.CaregiverID != nil && *link.CaregiverID == userID
		if !isOwner && !isCaregiver {
			return domain.ErrNotFound
		}

		if err := s.db.RevokeCareLink(ctx, tx, linkID); err != nil {
			return err
		}

		return s.db.WriteAudit(ctx, tx, postgres.AuditEntry{
			ActorID:      &userID,
			OwnerID:      &link.OwnerID,
			ActorRole:    string(domain.ActorUser),
			Action:       postgres.ActionRevoke,
			ResourceType: postgres.ResourceCare,
			ResourceID:   &linkID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			AfterState: map[string]any{
				"status":    string(domain.CareRevoked),
				"by":        map[bool]string{true: "owner", false: "caregiver"}[isOwner],
				"caregiver": link.CaregiverID != nil,
			},
		})
	})
}

// RequireAccess memastikan user berhak mengakses data pemilik tertentu.
//
// INI GERBANG UTAMA seluruh jalur caregiver. Setiap endpoint /care/{ownerId}/*
// memanggilnya lebih dulu, dan hasilnya memberi tahu peran apa yang berlaku.
//
// Mengembalikan galat "tidak ditemukan" bila user tidak punya relasi aktif.
// Bukan "tidak berhak": membedakannya akan memberi tahu penyerang bahwa
// seorang user tertentu memang ada dan bisa dipantau.
//
// Pemilik datanya SENDIRI selalu lolos sebagai pemilik penuh, supaya endpoint
// yang sama bisa dipakai pasien untuk melihat datanya sendiri.
func (s *CareLinks) RequireAccess(ctx context.Context, actorID, ownerID string) (*Access, error) {
	if actorID == ownerID {
		return &Access{
			OwnerID:  ownerID,
			IsOwner:  true,
			CanRead:  true,
			CanWrite: true,
		}, nil
	}

	link, err := s.db.ActiveLink(ctx, s.db.Pool(), ownerID, actorID)
	if err != nil {
		if isNotFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	return &Access{
		OwnerID: ownerID,
		Link:    link,
		IsOwner: false,
		CanRead: true,
		// Hanya helper yang boleh mencatat log. Viewer tidak boleh menulis
		// apa pun, dan keduanya tidak boleh mengelola obat maupun jadwal.
		CanWrite: link.CanWriteLog(),
	}, nil
}

// Access adalah hasil pemeriksaan hak akses.
type Access struct {
	OwnerID  string
	Link     *postgres.CareLink
	IsOwner  bool
	CanRead  bool
	CanWrite bool
}

// RequireWrite memastikan aktor boleh MENULIS.
//
// Dipakai endpoint pencatatan dosis atas nama pasien. Pesannya dibedakan
// dari "tidak berhak membaca", karena artinya berbeda: user ini memang
// memantau, hanya saja perannya viewer.
func (s *CareLinks) RequireWrite(ctx context.Context, actorID, ownerID string) (*Access, error) {
	acc, err := s.RequireAccess(ctx, actorID, ownerID)
	if err != nil {
		return nil, err
	}
	if !acc.CanWrite {
		return nil, fmt.Errorf("%w: peran viewer tidak boleh mencatat dosis", domain.ErrForbidden)
	}
	return acc, nil
}

// RequireManage memastikan aktor boleh MENGELOLA obat & stok pasien.
//
// Dipakai endpoint /care/{ownerId}/medications* (tambah, ubah, arsip obat,
// restock, koreksi stok). Hanya manager yang lolos — helper dibatasi mencatat
// log, viewer hanya membaca. Sesuai api-v2 bagian 9.
//
// Pesan penolakan menyebut peran yang benar: caregiver helper yang mencoba
// mengelola obat harus tahu bahwa yang dibutuhkan adalah manager, bukan
// sekadar "tidak berhak".
func (s *CareLinks) RequireManage(ctx context.Context, actorID, ownerID string) (*Access, error) {
	acc, err := s.RequireAccess(ctx, actorID, ownerID)
	if err != nil {
		return nil, err
	}
	if acc.IsOwner {
		// Pemilik datanya mengelola obatnya sendiri lewat endpoint biasa;
		// rute care tidak pernah dipakai untuk diri sendiri.
		return nil, fmt.Errorf("%w: gunakan endpoint /medications untuk obat milik sendiri",
			domain.ErrForbidden)
	}
	if acc.Link == nil || !acc.Link.Role.CanManageMedications() {
		return nil, fmt.Errorf("%w: mengelola obat pasien hanya untuk peran manager",
			domain.ErrForbidden)
	}
	return acc, nil
}

// newInviteToken membuat token undangan acak 32 byte.
func newInviteToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("membuat token undangan: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// hashInviteToken menghitung SHA-256 dari token.
//
// SHA-256, bukan Argon2, dan itu disengaja: token sudah punya 256 bit entropi
// acak, sehingga tidak rentan ditebak. Hashing lambat hanya akan membebani
// jalur penerimaan undangan tanpa menambah keamanan.
func hashInviteToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
