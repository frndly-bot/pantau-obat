package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	"github.com/frndly-bot/pantau-obat/backend/internal/repo/postgres"
)

// Catalog adalah layanan katalog obat.
//
// Dua peran dalam satu tabel (ADR-004), dan seluruh aturan di sini berasal
// dari situ:
//
//   - Preset publik: hanya dibaca user. Mengubahnya ditolak dengan
//     CATALOG_IMMUTABLE, bukan diam-diam diabaikan.
//   - Katalog pribadi: hanya terlihat pemiliknya. Tidak pernah muncul untuk
//     user lain, dan itu ditegakkan di query, bukan di tampilan.
type Catalog struct {
	db     *postgres.DB
	logger *slog.Logger
}

// NewCatalog membuat layanan katalog.
func NewCatalog(db *postgres.DB, logger *slog.Logger) *Catalog {
	return &Catalog{db: db, logger: logger}
}

// SearchInput adalah syarat pencarian katalog.
type SearchInput struct {
	Query     string
	FormCode  string
	RouteCode string
	Limit     int
}

// Search mencari preset publik dan katalog pribadi user.
//
// Katalog pribadi user LAIN tidak pernah ikut. Ini jaminan privasi yang
// ditegakkan di query, dan alasannya bukan sekadar kerapian: katalog pribadi
// dibuat user untuk obat yang tidak ada di preset, dan nama obat orang lain
// bisa membocorkan kondisi kesehatannya.
func (s *Catalog) Search(ctx context.Context, userID string, in SearchInput) ([]postgres.CatalogEntry, error) {
	limit := in.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	entries, err := s.db.SearchCatalog(ctx, s.db.Pool(), postgres.CatalogSearchFilter{
		UserID:    userID,
		Query:     in.Query,
		FormCode:  in.FormCode,
		RouteCode: in.RouteCode,
		Limit:     limit,
	})
	if err != nil {
		return nil, err
	}

	// Audit pencarian DITULIS, tapi tidak menggagalkan permintaan.
	//
	// Alasannya: pencarian katalog bisa mengungkap apa yang dicari seseorang,
	// dan itu data kesehatan. Jejaknya perlu ada untuk bisa ditelusuri user.
	// Tapi pencarian terjadi pada setiap ketikan, jadi kegagalan menulis
	// audit tidak boleh menghentikan pemakaian normal.
	if err := s.auditSearch(ctx, userID, in, len(entries)); err != nil {
		s.logger.WarnContext(ctx, "gagal menulis audit pencarian katalog",
			slog.String("error", err.Error()))
	}

	return entries, nil
}

// auditSearch menulis jejak pencarian katalog.
//
// Kata kunci yang dicari TIDAK disimpan. Yang dicatat hanya bahwa pencarian
// terjadi dan berapa hasilnya. Menyimpan kata kuncinya berarti menyimpan
// "obat apa yang sedang dicari seseorang", dan itu data kesehatan yang tidak
// perlu ada di jejak audit.
func (s *Catalog) auditSearch(ctx context.Context, userID string, in SearchInput, results int) error {
	meta := map[string]any{
		"results":   results,
		"has_query": strings.TrimSpace(in.Query) != "",
	}
	if in.FormCode != "" {
		meta["form_code"] = in.FormCode
	}
	if in.RouteCode != "" {
		meta["route_code"] = in.RouteCode
	}

	return s.db.WriteAudit(ctx, s.db.Pool(), postgres.AuditEntry{
		ActorID:      &userID,
		OwnerID:      &userID,
		ActorRole:    string(domain.ActorUser),
		Action:       postgres.ActionCatalogSearch,
		ResourceType: postgres.ResourceCat,
		Meta:         meta,
	})
}

// Get mengambil detail satu entri katalog.
//
// Preset yang masih draft atau arsip TIDAK terlihat user biasa: keduanya
// belum siap dipakai. Admin melihatnya lewat endpoint /admin/catalog.
func (s *Catalog) Get(ctx context.Context, userID, catalogID string) (*postgres.CatalogEntry, error) {
	return s.db.CatalogEntryVisible(ctx, s.db.Pool(), catalogID, userID)
}

// CreateInput adalah permintaan pembuatan katalog pribadi.
type CreateInput struct {
	DisplayName    string
	DosageFormCode *string
	RouteCode      *string
	UcumCode       *string
	UomName        *string
	DoseValueText  *string
}

// Create membuat katalog pribadi milik user.
//
// Klien TIDAK bisa membuat entri yang mengaku preset publik. Kolom is_preset,
// owner_id, dan curation_status ditetapkan di repo, dan CHECK
// ck_catalog_scope di database menegakkannya — jadi tidak ada jalur yang
// memungkinkan user menyuntikkan entri ke katalog bersama.
func (s *Catalog) Create(ctx context.Context, userID string, in CreateInput, meta Meta) (*postgres.CatalogEntry, error) {
	name := strings.TrimSpace(in.DisplayName)
	if name == "" {
		return nil, fmt.Errorf("%w: nama obat wajib diisi", domain.ErrValidation)
	}
	if len([]rune(name)) > 200 {
		return nil, fmt.Errorf("%w: nama obat maksimal 200 karakter", domain.ErrValidation)
	}

	var out *postgres.CatalogEntry
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		entry, err := s.db.CreatePrivateCatalog(ctx, tx, postgres.CreatePrivateCatalogInput{
			OwnerID:        userID,
			DisplayName:    name,
			DosageFormCode: in.DosageFormCode,
			RouteCode:      in.RouteCode,
			UcumCode:       in.UcumCode,
			UomName:        in.UomName,
			DoseValueText:  in.DoseValueText,
		})
		if err != nil {
			if postgres.IsUniqueViolation(err) {
				return fmt.Errorf("%w: obat ini sudah ada di katalog pribadi Anda", domain.ErrValidation)
			}
			return err
		}
		out = entry

		return s.db.WriteAudit(ctx, tx, postgres.AuditEntry{
			ActorID:      &userID,
			OwnerID:      &userID,
			ActorRole:    string(domain.ActorUser),
			Action:       postgres.ActionCreate,
			ResourceType: postgres.ResourceCat,
			ResourceID:   &entry.ID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			AfterState:   map[string]any{"display_name": entry.DisplayName},
		})
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateInput adalah perubahan katalog pribadi.
type UpdateInput struct {
	DisplayName    *string
	DosageFormCode *string
	RouteCode      *string
	UcumCode       *string
	UomName        *string
	DoseValueText  *string
}

// IsEmpty melaporkan apakah tidak ada field yang diubah.
func (u *UpdateInput) IsEmpty() bool {
	return u.DisplayName == nil && u.DosageFormCode == nil && u.RouteCode == nil &&
		u.UcumCode == nil && u.UomName == nil && u.DoseValueText == nil
}

// Update mengubah katalog pribadi milik user.
//
// Preset publik DITOLAK dengan CATALOG_IMMUTABLE, bukan diabaikan diam-diam.
// Pesan yang jelas jauh lebih berguna daripada perubahan yang tidak terjadi
// tanpa penjelasan — dan itu juga yang dijanjikan api-v2 bagian 4.
func (s *Catalog) Update(ctx context.Context, userID, catalogID string, in UpdateInput, meta Meta) (*postgres.CatalogEntry, error) {
	if in.IsEmpty() {
		return nil, fmt.Errorf("%w: tidak ada field yang diubah", domain.ErrValidation)
	}
	if in.DisplayName != nil {
		n := strings.TrimSpace(*in.DisplayName)
		if n == "" {
			return nil, fmt.Errorf("%w: nama obat tidak boleh kosong", domain.ErrValidation)
		}
		in.DisplayName = &n
	}

	var out *postgres.CatalogEntry
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		// Entri diambil tanpa syarat visibilitas dulu, supaya preset publik
		// bisa DIBEDAKAN dari entri yang tidak ada. Bedanya penting bagi
		// klien: 409 "hanya admin" vs 404 "tidak ditemukan".
		existing, err := s.db.CatalogEntryAny(ctx, tx, catalogID)
		if err != nil {
			return err
		}

		if existing.IsPreset {
			return domain.ErrCatalogImmutable
		}
		if !existing.EditableBy(userID) {
			// Katalog pribadi milik user lain: dijawab "tidak ditemukan",
			// bukan "tidak berhak". Membedakannya akan memberi tahu bahwa
			// entri itu memang ada.
			return domain.ErrNotFound
		}

		updated, err := s.db.UpdatePrivateCatalog(ctx, tx, catalogID, userID, postgres.UpdatePrivateCatalogInput{
			DisplayName:    in.DisplayName,
			DosageFormCode: in.DosageFormCode,
			RouteCode:      in.RouteCode,
			UcumCode:       in.UcumCode,
			UomName:        in.UomName,
			DoseValueText:  in.DoseValueText,
		})
		if err != nil {
			return err
		}
		out = updated

		return s.db.WriteAudit(ctx, tx, postgres.AuditEntry{
			ActorID:      &userID,
			OwnerID:      &userID,
			ActorRole:    string(domain.ActorUser),
			Action:       postgres.ActionUpdate,
			ResourceType: postgres.ResourceCat,
			ResourceID:   &catalogID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			BeforeState:  map[string]any{"display_name": existing.DisplayName},
			AfterState:   map[string]any{"display_name": updated.DisplayName},
		})
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Delete menghapus katalog pribadi.
//
// Soft delete. Obat yang sudah dibuat dari entri ini TIDAK terpengaruh: ia
// menyimpan catalog_id sebagai rujukan, dan rujukan yang hilang tidak boleh
// membuat obat pasien ikut rusak.
func (s *Catalog) Delete(ctx context.Context, userID, catalogID string, meta Meta) error {
	return s.db.WithTx(ctx, func(tx pgx.Tx) error {
		existing, err := s.db.CatalogEntryAny(ctx, tx, catalogID)
		if err != nil {
			return err
		}
		if existing.IsPreset {
			return domain.ErrCatalogImmutable
		}
		if !existing.EditableBy(userID) {
			return domain.ErrNotFound
		}

		if err := s.db.SoftDeletePrivateCatalog(ctx, tx, catalogID, userID); err != nil {
			return err
		}

		return s.db.WriteAudit(ctx, tx, postgres.AuditEntry{
			ActorID:      &userID,
			OwnerID:      &userID,
			ActorRole:    string(domain.ActorUser),
			Action:       postgres.ActionDelete,
			ResourceType: postgres.ResourceCat,
			ResourceID:   &catalogID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			BeforeState:  map[string]any{"display_name": existing.DisplayName},
		})
	})
}

// Suggest mengambil saran nama untuk autocomplete.
func (s *Catalog) Suggest(ctx context.Context, userID, prefix string, limit int) ([]postgres.CatalogSuggestion, error) {
	return s.db.SuggestCatalog(ctx, s.db.Pool(), userID, prefix, limit)
}
