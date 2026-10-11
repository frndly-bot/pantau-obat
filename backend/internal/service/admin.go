package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	"github.com/frndly-bot/pantau-obat/backend/internal/repo/postgres"
)

// Admin adalah layanan operasi admin.
//
// ATURAN YANG MENOPANG SELURUH FILE INI, dari TRD bagian 5.3 dan 7.2:
//
//  1. admin_scope diperiksa di SETIAP operasi, bukan sekali di awal sesi.
//     Admin yang cakupannya diubah harus langsung terkena perubahan itu.
//
//  2. Setiap tulisan menulis audit dengan actor_role='admin' dan
//     before_state/after_state.
//
//  3. Admin TIDAK BISA membaca isi obat atau log pasien individual. Yang
//     tersedia hanya data akun dan agregat. Batas ini ditegakkan di lapisan
//     query, bukan di tampilan.
type Admin struct {
	db     *postgres.DB
	logger *slog.Logger
}

// NewAdmin membuat layanan admin.
func NewAdmin(db *postgres.DB, logger *slog.Logger) *Admin {
	return &Admin{db: db, logger: logger}
}

// AdminActor adalah identitas admin yang melakukan aksi.
//
// Dipisah dari Identity transport karena paket service tidak boleh
// bergantung pada bentuk kawat. Yang dibutuhkan hanya tiga hal: siapa,
// peran apa yang disalin ke audit, dan cakupannya.
type AdminActor struct {
	UserID string
	Scope  []string // nil = akses penuh
}

// RequireScope memastikan admin punya cakupan tertentu.
//
// Dipanggil di setiap operasi. Bukan di-cache di sesi, karena pencabutan
// cakupan harus langsung berlaku — dan karena admin_scope adalah batas
// keamanan, bukan petunjuk tampilan.
func (a *Admin) RequireScope(actor AdminActor, want domain.AdminScope) error {
	if actor.Scope == nil {
		return nil // NULL = akses penuh
	}
	for _, s := range actor.Scope {
		if s == string(want) {
			return nil
		}
	}
	return domain.ErrAdminScopeDenied
}

// ListUsers mengambil daftar akun.
func (a *Admin) ListUsers(ctx context.Context, actor AdminActor, f ListUsersInput) ([]postgres.AdminUser, int, error) {
	if err := a.RequireScope(actor, domain.ScopeUsers); err != nil {
		return nil, 0, err
	}

	dbFilter := postgres.AdminListUsersFilter{
		Query:  f.Query,
		Role:   f.Role,
		Status: f.Status,
		Limit:  f.Limit,
		Offset: f.Offset,
	}

	users, err := a.db.ListUsers(ctx, a.db.Pool(), dbFilter)
	if err != nil {
		return nil, 0, err
	}
	total, err := a.db.CountUsers(ctx, a.db.Pool(), dbFilter)
	if err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

// ListUsersInput adalah syarat pencarian akun.
type ListUsersInput struct {
	Query  string
	Role   string
	Status string
	Limit  int
	Offset int
}

// GetUser mengambil satu akun.
//
// Mengembalikan data akun dan JUMLAH obat, bukan daftar obatnya. Admin tidak
// perlu tahu obat apa yang diminum seseorang.
func (a *Admin) GetUser(ctx context.Context, actor AdminActor, userID string) (*postgres.AdminUser, error) {
	if err := a.RequireScope(actor, domain.ScopeUsers); err != nil {
		return nil, err
	}
	return a.db.AdminUserByID(ctx, a.db.Pool(), userID)
}

// SetUserActive menonaktifkan atau mengaktifkan akun.
//
// DUA HAL terjadi bersamaan saat menonaktifkan, dan keduanya wajib:
//
//  1. is_active = false, sehingga user tidak bisa login lagi.
//  2. Seluruh akses dicabut, sehingga access token yang SUDAH beredar
//     langsung mati. Tanpa ini, user yang dinonaktifkan tetap bisa memakai
//     aplikasi sampai tokennya kedaluwarsa — dan itu membuat tindakan admin
//     terasa tidak berlaku.
func (a *Admin) SetUserActive(ctx context.Context, actor AdminActor, userID string, active bool, meta Meta) error {
	if err := a.RequireScope(actor, domain.ScopeUsers); err != nil {
		return err
	}
	if userID == actor.UserID && !active {
		// Admin menonaktifkan dirinya sendiri akan langsung kehilangan
		// akses, dan tidak ada yang bisa mengembalikannya lewat API.
		return fmt.Errorf("%w: tidak bisa menonaktifkan akun sendiri", domain.ErrValidation)
	}

	return a.db.WithTx(ctx, func(tx pgx.Tx) error {
		before, err := a.db.AdminUserByID(ctx, tx, userID)
		if err != nil {
			return err
		}
		if before.IsActive == active {
			return fmt.Errorf("%w: akun sudah dalam keadaan itu", domain.ErrValidation)
		}

		if err := a.db.SetUserActive(ctx, tx, userID, active); err != nil {
			return err
		}

		action := postgres.ActionUserDisable
		reason := domain.RevokeReasonAdminDisable
		if active {
			action = postgres.ActionUserEnable
			reason = ""
		}

		if active {
			// Mengaktifkan kembali: garis potong pencabutan dihapus, supaya
			// token baru berlaku normal. Token LAMA yang sudah ditolak tetap
			// tidak berlaku, karena iat-nya tetap sebelum garis potong lama.
			if err := a.db.ClearRevocation(ctx, tx, userID); err != nil {
				return err
			}
		} else {
			// Menonaktifkan: cabut seluruh akses, termasuk yang aktif.
			if _, err := a.db.RevokeAccessAndSessions(ctx, tx, userID, reason, &actor.UserID); err != nil {
				return err
			}
		}

		return a.writeAdminAudit(ctx, tx, actor, postgres.AuditEntry{
			ActorID:      &actor.UserID,
			OwnerID:      &userID,
			Action:       action,
			ResourceType: postgres.ResourceUser,
			ResourceID:   &userID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			BeforeState:  map[string]any{"is_active": before.IsActive},
			AfterState:   map[string]any{"is_active": active},
			Meta:         map[string]any{"access_revoked": !active},
		})
	})
}

// EraseUser menghapus akun atas permintaan.
//
// Berbeda dari user menghapus akunnya sendiri: di sini admin yang bertindak,
// dan auditnya mencatat actor_role='admin'. Identitas dianonimkan, dan log
// kepatuhan DIPERTAHANKAN — riwayat medis tidak boleh hilang hanya karena
// akunnya dihapus.
func (a *Admin) EraseUser(ctx context.Context, actor AdminActor, userID string, reason string, meta Meta) error {
	if err := a.RequireScope(actor, domain.ScopeUsers); err != nil {
		return err
	}
	if userID == actor.UserID {
		return fmt.Errorf("%w: tidak bisa menghapus akun sendiri", domain.ErrValidation)
	}
	if strings.TrimSpace(reason) == "" {
		// Alasan wajib. Penghapusan akun adalah tindakan yang tidak bisa
		// dibatalkan, dan tanpa alasan tertulis tidak ada cara menjelaskan
		// kenapa itu dilakukan.
		return fmt.Errorf("%w: alasan penghapusan wajib diisi", domain.ErrValidation)
	}

	return a.db.WithTx(ctx, func(tx pgx.Tx) error {
		before, err := a.db.AdminUserByID(ctx, tx, userID)
		if err != nil {
			return err
		}

		if err := a.db.AnonymizeUser(ctx, tx, userID); err != nil {
			return err
		}
		if _, err := a.db.RevokeAccessAndSessions(ctx, tx, userID,
			domain.RevokeReasonAdminErase, &actor.UserID); err != nil {
			return err
		}

		return a.writeAdminAudit(ctx, tx, actor, postgres.AuditEntry{
			ActorID:      &actor.UserID,
			OwnerID:      &userID,
			Action:       postgres.ActionUserErase,
			ResourceType: postgres.ResourceUser,
			ResourceID:   &userID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			BeforeState:  map[string]any{"email": before.Email, "is_active": before.IsActive},
			AfterState:   map[string]any{"anonymized": true, "access_revoked": true},
			Meta: map[string]any{
				"reason":           reason,
				"logs_preserved":   true,
				"purge_after_days": 30,
			},
		})
	})
}

// GrantAdminInput adalah permintaan pemberian peran admin.
type GrantAdminInput struct {
	UserID string
	Scope  []domain.AdminScope
	Reason string
}

// GrantAdmin memberi peran admin beserta cakupannya.
//
// Cakupan divalidasi terhadap daftar yang dikenal. Cakupan asing akan membuat
// RequireScope selalu menolak, dan admin itu tidak bisa berbuat apa pun —
// keadaan yang membingungkan dan sulit dilacak.
func (a *Admin) GrantAdmin(ctx context.Context, actor AdminActor, in GrantAdminInput, meta Meta) error {
	if err := a.RequireScope(actor, domain.ScopeUsers); err != nil {
		return err
	}
	if err := domain.ValidScopeList(in.Scope); err != nil {
		return fmt.Errorf("%w: %s", domain.ErrValidation, err.Error())
	}
	if in.UserID == actor.UserID {
		return fmt.Errorf("%w: tidak bisa mengubah peran sendiri", domain.ErrValidation)
	}
	if strings.TrimSpace(in.Reason) == "" {
		return fmt.Errorf("%w: alasan pemberian peran wajib diisi", domain.ErrValidation)
	}

	return a.db.WithTx(ctx, func(tx pgx.Tx) error {
		before, err := a.db.AdminUserByID(ctx, tx, in.UserID)
		if err != nil {
			return err
		}

		if err := a.db.GrantAdmin(ctx, tx, postgres.GrantAdminInput{
			UserID:    in.UserID,
			Scope:     in.Scope,
			GrantedBy: &actor.UserID,
		}); err != nil {
			return err
		}

		// Peran berubah, jadi token lama tidak boleh lagi berlaku: klaim
		// peran di token lama masih 'user', dan token itu seharusnya tidak
		// bisa dipakai untuk mengakses rute admin. Mencabut akses memaksa
		// login ulang, dan token baru yang terbit akan memuat peran baru.
		if _, err := a.db.RevokeAccessAndSessions(ctx, tx, in.UserID,
			domain.RevokeReasonAdminRevokeRole, &actor.UserID); err != nil {
			return err
		}

		return a.writeAdminAudit(ctx, tx, actor, postgres.AuditEntry{
			ActorID:      &actor.UserID,
			OwnerID:      &in.UserID,
			Action:       postgres.ActionAdminGrant,
			ResourceType: postgres.ResourceAdmin,
			ResourceID:   &in.UserID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			BeforeState:  map[string]any{"role": string(before.Role), "admin_scope": scopeStrings(before.AdminScope)},
			AfterState:   map[string]any{"role": string(domain.RoleAdmin), "admin_scope": scopeStrings(in.Scope)},
			Meta:         map[string]any{"reason": in.Reason, "relogin_required": true},
		})
	})
}

// RevokeAdmin mencabut peran admin.
func (a *Admin) RevokeAdmin(ctx context.Context, actor AdminActor, userID, reason string, meta Meta) error {
	if err := a.RequireScope(actor, domain.ScopeUsers); err != nil {
		return err
	}
	if userID == actor.UserID {
		return fmt.Errorf("%w: tidak bisa mencabut peran sendiri", domain.ErrValidation)
	}
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("%w: alasan pencabutan peran wajib diisi", domain.ErrValidation)
	}

	return a.db.WithTx(ctx, func(tx pgx.Tx) error {
		before, err := a.db.AdminUserByID(ctx, tx, userID)
		if err != nil {
			return err
		}

		if err := a.db.RevokeAdmin(ctx, tx, userID); err != nil {
			return err
		}
		// Akses dicabut karena peran berubah. Perangkat yang masih memegang
		// token admin TIDAK boleh terus punya kemampuan itu.
		if _, err := a.db.RevokeAccessAndSessions(ctx, tx, userID,
			domain.RevokeReasonAdminRevokeRole, &actor.UserID); err != nil {
			return err
		}

		return a.writeAdminAudit(ctx, tx, actor, postgres.AuditEntry{
			ActorID:      &actor.UserID,
			OwnerID:      &userID,
			Action:       postgres.ActionAdminRevoke,
			ResourceType: postgres.ResourceAdmin,
			ResourceID:   &userID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			BeforeState:  map[string]any{"role": string(before.Role), "admin_scope": scopeStrings(before.AdminScope)},
			AfterState:   map[string]any{"role": string(domain.RoleUser), "admin_scope": nil},
			Meta:         map[string]any{"reason": reason, "relogin_required": true},
		})
	})
}

// Stats mengambil agregat sistem.
func (a *Admin) Stats(ctx context.Context, actor AdminActor) (postgres.AdminStats, error) {
	if err := a.RequireScope(actor, domain.ScopeReports); err != nil {
		return postgres.AdminStats{}, err
	}
	return a.db.Stats(ctx, a.db.Pool())
}

// ListCatalog mengambil katalog termasuk draft dan arsip.
func (a *Admin) ListCatalog(ctx context.Context, actor AdminActor, f AdminCatalogInput) ([]postgres.CatalogEntry, error) {
	if err := a.RequireScope(actor, domain.ScopeCatalog); err != nil {
		return nil, err
	}
	return a.db.ListCatalogForAdmin(ctx, a.db.Pool(), postgres.AdminCatalogFilter{
		CurationStatus: f.CurationStatus,
		Query:          f.Query,
		Limit:          f.Limit,
	})
}

// AdminCatalogInput adalah syarat pencarian katalog oleh admin.
type AdminCatalogInput struct {
	CurationStatus string
	Query          string
	Limit          int
}

// CreatePreset membuat preset publik baru.
//
// Selalu bermula sebagai draft. Preset yang langsung published akan muncul
// ke seluruh user sebelum sempat diperiksa, dan itu sulit ditarik kembali.
func (a *Admin) CreatePreset(ctx context.Context, actor AdminActor, in CreatePresetInput, meta Meta) (*postgres.CatalogEntry, error) {
	if err := a.RequireScope(actor, domain.ScopeCatalog); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.DisplayName)
	if name == "" {
		return nil, fmt.Errorf("%w: nama obat wajib diisi", domain.ErrValidation)
	}

	var out *postgres.CatalogEntry
	err := a.db.WithTx(ctx, func(tx pgx.Tx) error {
		entry, err := a.db.CreatePreset(ctx, tx, postgres.AdminCreatePresetInput{
			DisplayName:    name,
			NamaDagang:     in.NamaDagang,
			GenerikNama:    in.GenerikNama,
			DosageFormCode: in.DosageFormCode,
			RouteCode:      in.RouteCode,
			UcumCode:       in.UcumCode,
			UomName:        in.UomName,
			DoseValueText:  in.DoseValueText,
			CreatedBy:      actor.UserID,
		})
		if err != nil {
			return err
		}
		out = entry

		return a.writeAdminAudit(ctx, tx, actor, postgres.AuditEntry{
			ActorID:      &actor.UserID,
			Action:       postgres.ActionCatalogCreate,
			ResourceType: postgres.ResourceCat,
			ResourceID:   &entry.ID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			AfterState: map[string]any{
				"display_name":    entry.DisplayName,
				"curation_status": string(entry.CurationStatus),
			},
		})
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CreatePresetInput adalah data preset baru.
type CreatePresetInput struct {
	DisplayName    string
	NamaDagang     *string
	GenerikNama    *string
	DosageFormCode *string
	RouteCode      *string
	UcumCode       *string
	UomName        *string
	DoseValueText  *string
}

// SetCurationStatus mengubah status kurasi preset.
//
// Transisi yang diizinkan dibatasi, dan itu bukan sekadar formalitas:
// mengarsipkan preset yang masih draft, atau mempublikasikan yang sudah
// arsip, akan menghasilkan keadaan yang membingungkan bagi user.
func (a *Admin) SetCurationStatus(ctx context.Context, actor AdminActor, id string, status domain.CurationStatus, reason string, meta Meta) (*postgres.CatalogEntry, error) {
	if err := a.RequireScope(actor, domain.ScopeCatalog); err != nil {
		return nil, err
	}
	if strings.TrimSpace(reason) == "" {
		// Alasan wajib, sesuai api-v2 bagian 11: perubahan status katalog
		// memengaruhi apa yang dilihat semua user, jadi harus bisa
		// dipertanggungjawabkan.
		return nil, fmt.Errorf("%w: alasan perubahan status wajib diisi", domain.ErrValidation)
	}

	var out *postgres.CatalogEntry
	err := a.db.WithTx(ctx, func(tx pgx.Tx) error {
		before, err := a.db.CatalogEntryAny(ctx, tx, id)
		if err != nil {
			return err
		}
		if !before.IsPreset {
			// Katalog pribadi user tidak punya siklus kurasi. Mengubahnya
			// akan merusak CHECK ck_catalog_scope.
			return fmt.Errorf("%w: entri ini bukan preset publik", domain.ErrValidation)
		}
		if err := validCurationTransition(before.CurationStatus, status); err != nil {
			return err
		}

		action := actionForCuration(status)
		updated, err := a.db.SetCurationStatus(ctx, tx, id, status, actor.UserID)
		if err != nil {
			return err
		}
		out = updated

		return a.writeAdminAudit(ctx, tx, actor, postgres.AuditEntry{
			ActorID:      &actor.UserID,
			Action:       action,
			ResourceType: postgres.ResourceCat,
			ResourceID:   &id,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			BeforeState:  map[string]any{"curation_status": string(before.CurationStatus)},
			AfterState:   map[string]any{"curation_status": string(status)},
			Meta:         map[string]any{"reason": reason},
		})
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// validCurationTransition memeriksa apakah perpindahan status diizinkan.
//
// Aturannya dari TRD bagian I2:
//
//	draft     -> published, archived
//	published -> draft, archived
//	archived  -> draft
//
// Mempublikasikan yang sudah arsip tidak diizinkan lewat jalur ini:
// ia harus dikembalikan ke draft lebih dulu, supaya ada kesempatan meninjau
// ulang sebelum muncul lagi ke user.
func validCurationTransition(from, to domain.CurationStatus) error {
	if from == to {
		return fmt.Errorf("%w: status sudah %s", domain.ErrValidation, to)
	}
	switch from {
	case domain.CurationDraft:
		if to == domain.CurationPublished || to == domain.CurationArchived {
			return nil
		}
	case domain.CurationPublished:
		if to == domain.CurationDraft || to == domain.CurationArchived {
			return nil
		}
	case domain.CurationArchived:
		if to == domain.CurationDraft {
			return nil
		}
	}
	return fmt.Errorf("%w: tidak bisa mengubah status dari %s ke %s", domain.ErrValidation, from, to)
}

// actionForCuration memetakan status baru ke aksi audit.
func actionForCuration(status domain.CurationStatus) string {
	switch status {
	case domain.CurationPublished:
		return postgres.ActionCatalogPublish
	case domain.CurationArchived:
		return postgres.ActionCatalogArchive
	case domain.CurationDraft:
		return postgres.ActionCatalogUnpublish
	}
	return postgres.ActionCatalogUpdate
}

// ListJobs mengambil daftar job notifikasi.
func (a *Admin) ListJobs(ctx context.Context, actor AdminActor, status, kind string, limit int) ([]postgres.NotificationJob, error) {
	if err := a.RequireScope(actor, domain.ScopeNotifications); err != nil {
		return nil, err
	}
	return a.db.ListJobs(ctx, a.db.Pool(), postgres.ListJobsFilter{
		Status: status,
		Kind:   kind,
		Limit:  limit,
	})
}

// RetryJob mengulang job notifikasi yang gagal.
func (a *Admin) RetryJob(ctx context.Context, actor AdminActor, jobID int64, meta Meta) error {
	if err := a.RequireScope(actor, domain.ScopeNotifications); err != nil {
		return err
	}

	return a.db.WithTx(ctx, func(tx pgx.Tx) error {
		before, err := a.db.JobByID(ctx, tx, jobID)
		if err != nil {
			return err
		}

		if err := a.db.RetryJob(ctx, tx, jobID); err != nil {
			return err
		}

		return a.writeAdminAudit(ctx, tx, actor, postgres.AuditEntry{
			ActorID:      &actor.UserID,
			Action:       postgres.ActionNotifRetry,
			ResourceType: postgres.ResourceNotif,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			BeforeState: map[string]any{
				"status":   before.Status,
				"attempts": before.Attempts,
			},
			AfterState: map[string]any{"status": string(domain.JobPending), "attempts": 0},
			// Payload TIDAK disalin ke audit. Isinya bisa memuat detail
			// pesan, dan halaman admin tidak boleh menyimpan isi notifikasi
			// pasien.
			Meta: map[string]any{"job_id": jobID},
		})
	})
}

// CancelJob membatalkan job notifikasi yang belum selesai.
func (a *Admin) CancelJob(ctx context.Context, actor AdminActor, jobID int64, meta Meta) error {
	if err := a.RequireScope(actor, domain.ScopeNotifications); err != nil {
		return err
	}

	return a.db.WithTx(ctx, func(tx pgx.Tx) error {
		before, err := a.db.JobByID(ctx, tx, jobID)
		if err != nil {
			return err
		}

		if err := a.db.CancelJob(ctx, tx, jobID); err != nil {
			return err
		}

		return a.writeAdminAudit(ctx, tx, actor, postgres.AuditEntry{
			ActorID:      &actor.UserID,
			Action:       postgres.ActionNotifCancel,
			ResourceType: postgres.ResourceNotif,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			BeforeState:  map[string]any{"status": before.Status},
			AfterState:   map[string]any{"status": string(domain.JobCancelled)},
			Meta:         map[string]any{"job_id": jobID},
		})
	})
}

// writeAdminAudit menulis jejak audit dengan actor_role='admin'.
//
// Peran DISALIN, bukan di-join: kalau admin itu kemudian diturunkan jadi
// user biasa, jejak lamanya harus tetap menunjukkan bahwa aksi itu dilakukan
// SAAT ia berperan admin. Tanpa ini, riwayat admin yang sudah diturunkan
// jadi menyesatkan.
func (a *Admin) writeAdminAudit(ctx context.Context, q pgx.Tx, _ AdminActor, e postgres.AuditEntry) error {
	e.ActorRole = string(domain.ActorAdmin)
	if err := a.db.WriteAudit(ctx, q, e); err != nil {
		// Audit admin WAJIB berhasil. Berbeda dari audit user yang boleh
		// gagal dengan peringatan, aksi admin menyentuh akun orang lain dan
		// mengubah apa yang dilihat semua user. Tanpa jejaknya, tidak ada
		// cara mempertanggungjawabkannya.
		return fmt.Errorf("menulis audit admin: %w", err)
	}
	return nil
}

// scopeStrings mengubah daftar cakupan menjadi bentuk yang bisa ditulis ke
// JSON audit.
func scopeStrings(scopes []domain.AdminScope) any {
	if scopes == nil {
		return nil // NULL = akses penuh, dan itu harus terlihat berbeda
	}
	out := make([]string, len(scopes))
	for i, s := range scopes {
		out[i] = string(s)
	}
	return out
}

// unusedTimeGuard menjaga impor time tetap terpakai bila suatu saat
// pemakaiannya berpindah. Dihapus begitu ada pemakaian nyata.
var _ = time.Time{}
