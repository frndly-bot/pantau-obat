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

// File ini mengurus katalog obat.
//
// DUA PERAN, SATU TABEL. Itu keputusan ADR-004, dan seluruh penjagaan di
// sini berasal dari situ:
//
//   - PRESET PUBLIK  : owner_id NULL, is_preset true, dimiliki sistem.
//     Hanya admin yang boleh mengubah. User hanya bisa membacanya.
//   - KATALOG PRIBADI: owner_id terisi, is_preset false, curation_status
//     'user_private'. Hanya pemiliknya yang bisa melihat dan mengubahnya.
//     TIDAK PERNAH muncul untuk user lain.
//
// CHECK ck_catalog_scope di database yang menegakkan bentuk ini, sehingga
// kombinasi yang tidak sah tidak mungkin tersimpan.

// catalogColumns adalah daftar kolom untuk SELECT berulang.
//
// Berbeda dari tabel lain, kolom di sini sengaja TIDAK dipecah per bagian:
// katalog dibaca sebagai satu kesatuan untuk ditampilkan sebagai saran, dan
// memecahnya hanya menambah tempat yang bisa salah saat kolom ditambah.
const catalogColumns = `
    id, display_name, nama_dagang, generik_nama, is_generik, rxterm,
    manufacturer, registrar, produksi_buatan, country_of_origin,
    dosage_form_code, dosage_form_name, route_code, route_name,
    dose_value_text, ucum_code, uom_name, controlled_code, controlled_name,
    atc_l5_code, atc_l5_name, indication, warning, side_effect, description,
    dosage_usage, storage_note, is_discontinued,
    kfa_code, bpom_nie, barcode, kode_lkpp,
    owner_id, is_preset, curation_status, source, curated_by, curated_at,
    created_at, updated_at`

// CatalogEntry adalah satu entri katalog, preset maupun pribadi.
type CatalogEntry struct {
	ID           string
	DisplayName  string
	NamaDagang   *string
	GenerikNama  *string
	IsGenerik    *bool
	Rxterm       *string
	Manufacturer *string
	Registrar    *string

	ProduksiBuatan  *string
	CountryOfOrigin *string

	DosageFormCode *string
	DosageFormName *string
	RouteCode      *string
	RouteName      *string
	DoseValueText  *string
	UcumCode       *string
	UomName        *string

	ControlledCode *string
	ControlledName *string

	ATCL5Code *string
	ATCL5Name *string

	Indication     *string
	Warning        *string
	SideEffect     *string
	Description    *string
	DosageUsage    *string
	StorageNote    *string
	IsDiscontinued *bool

	KFACode  *string
	BpomNIE  *string
	Barcode  *string
	KodeLKPP *string

	OwnerID        *string
	IsPreset       bool
	CurationStatus domain.CurationStatus
	Source         domain.CatalogSource
	CuratedBy      *string
	CuratedAt      *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsPrivate melaporkan apakah entri ini katalog pribadi user.
func (e *CatalogEntry) IsPrivate() bool { return !e.IsPreset && e.OwnerID != nil }

// EditableBy melaporkan apakah user tertentu boleh mengubah entri ini.
//
// Preset publik TIDAK PERNAH boleh diubah user, bahkan oleh yang membuatnya.
// Katalog pribadi hanya boleh diubah pemiliknya.
func (e *CatalogEntry) EditableBy(userID string) bool {
	if e.IsPreset {
		return false
	}
	return e.OwnerID != nil && *e.OwnerID == userID
}

// scanCatalogEntry membaca satu baris drug_catalog.
func scanCatalogEntry(row pgx.Row) (*CatalogEntry, error) {
	var e CatalogEntry
	err := row.Scan(
		&e.ID, &e.DisplayName, &e.NamaDagang, &e.GenerikNama, &e.IsGenerik,
		&e.Rxterm, &e.Manufacturer, &e.Registrar, &e.ProduksiBuatan,
		&e.CountryOfOrigin, &e.DosageFormCode, &e.DosageFormName,
		&e.RouteCode, &e.RouteName, &e.DoseValueText, &e.UcumCode,
		&e.UomName, &e.ControlledCode, &e.ControlledName,
		&e.ATCL5Code, &e.ATCL5Name, &e.Indication, &e.Warning,
		&e.SideEffect, &e.Description, &e.DosageUsage, &e.StorageNote,
		&e.IsDiscontinued, &e.KFACode, &e.BpomNIE, &e.Barcode,
		&e.KodeLKPP, &e.OwnerID, &e.IsPreset, &e.CurationStatus,
		&e.Source, &e.CuratedBy, &e.CuratedAt, &e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("membaca katalog: %w", err)
	}
	return &e, nil
}

// CatalogSearchFilter adalah syarat pencarian katalog.
type CatalogSearchFilter struct {
	UserID string

	// Query dicocokkan ke search_tsv, yang mencakup nama tampilan, nama
	// dagang, nama generik, rxterm, nama ATC level 5, dan bentuk sediaan.
	Query string

	FormCode  string
	RouteCode string
	Limit     int
}

// SearchCatalog mencari preset publik DAN katalog pribadi milik user.
//
// Katalog pribadi user LAIN tidak pernah ikut, dan itu ditegakkan di sini —
// bukan di handler, bukan di UI. Satu-satunya cara memastikan katalog
// pribadi tidak bocor adalah memastikan setiap query ke tabel ini memuat
// syarat pemiliknya.
//
// Pencarian memakai search_tsv (index GIN) untuk kata kunci, karena LIKE
// '%kata%' tidak bisa memakai index dan akan lambat begitu katalog tumbuh.
// Bila kata kuncinya kosong, urutan jatuh ke nama, supaya hasilnya stabil
// dan tidak berubah-ubah antar pemanggilan.
func (db *DB) SearchCatalog(ctx context.Context, q Querier, f CatalogSearchFilter) ([]CatalogEntry, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 20
	}

	// Dua cabang, disatukan dengan UNION ALL:
	//   1. preset publik yang sudah dipublikasikan
	//   2. katalog pribadi milik user ini
	var (
		where []string
		args  []any
	)

	visibility := `((is_preset = true AND curation_status = 'published')` +
		` OR (is_preset = false AND owner_id = $1))`
	args = append(args, f.UserID)
	where = append(where, visibility, "deleted_at IS NULL")

	if raw := strings.TrimSpace(f.Query); raw != "" {
		// DUA cara pencocokan, digabung dengan OR, dan keduanya diperlukan.
		//
		// 1. PREFIX MATCHING (ILIKE 'kata%') — ini yang paling penting.
		//    Pengguna mengetik sebagian, dan full-text TIDAK bisa
		//    mencocokkan sebagian: `websearch_to_tsquery('amlo')` hanya
		//    menemukan kata `amlo` yang utuh, sehingga "amlo" tidak
		//    menemukan "Amlodipine". Itu bug yang benar-benar terjadi, dan
		//    membuat pencarian katalog praktis tidak berfungsi.
		//
		// 2. FULL-TEXT (search_tsv) — melengkapi yang pertama untuk
		//    pencarian yang lebih kaya: beberapa kata, nama dagang, nama
		//    generik, rxterm, dan nama ATC.
		//
		// INDeks argumen harus dihitung SETELAH append, dan setiap $N yang
		// muncul di SQL harus benar-benar ada argumennya. Sempat salah di
		// sini: satu argumen ekstra ditambahkan tanpa dipakai, sehingga
		// seluruh $N bergeser satu dan query gagal dengan "could not
		// determine data type of parameter".
		//
		// Cast ::text tetap diperlukan: LIKE tidak memberi petunjuk tipe
		// kepada perencana, berbeda dari perbandingan dengan kolom.
		args = append(args, escapeLikePattern(raw)+"%")
		prefixIdx := len(args)

		args = append(args, "%"+escapeLikePattern(raw)+"%")
		containsIdx := len(args)

		args = append(args, toPrefixTSQuery(raw))
		tsIdx := len(args)

		where = append(where, fmt.Sprintf(`(
            display_name ILIKE $%d::text
            OR lower(display_name) LIKE lower($%d::text)
            OR search_tsv @@ to_tsquery('simple', $%d::text)
        )`, prefixIdx, containsIdx, tsIdx))
	}
	if f.FormCode != "" {
		args = append(args, f.FormCode)
		where = append(where, fmt.Sprintf("dosage_form_code = $%d", len(args)))
	}
	if f.RouteCode != "" {
		args = append(args, f.RouteCode)
		where = append(where, fmt.Sprintf("route_code = $%d", len(args)))
	}

	args = append(args, f.Limit)

	query := `SELECT ` + catalogColumns + `
                FROM drug_catalog
               WHERE ` + joinAnd(where) + `
               ORDER BY is_preset DESC, display_name
               LIMIT $` + fmt.Sprint(len(args))

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("mencari katalog: %w", err)
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

// CatalogEntryVisible memastikan entri bisa dilihat user.
//
// Preset yang masih draft atau arsip TIDAK terlihat user biasa: keduanya
// belum siap dipakai, dan menampilkannya akan membuat user memilih obat yang
// datanya belum diverifikasi.
func (db *DB) CatalogEntryVisible(ctx context.Context, q Querier, id, userID string) (*CatalogEntry, error) {
	row := q.QueryRow(ctx, `
        SELECT `+catalogColumns+`
          FROM drug_catalog
         WHERE id = $1
           AND deleted_at IS NULL
           AND ((is_preset = true AND curation_status = 'published')
                OR (is_preset = false AND owner_id = $2))`, id, userID)
	return scanCatalogEntry(row)
}

// CatalogEntryAny mengambil entri tanpa memeriksa visibilitas.
//
// Dipakai HANYA oleh jalur admin, yang wewenangnya sudah diperiksa
// RequireAdmin. Sengaja diberi nama berbeda supaya pemakaiannya di tempat
// lain langsung terlihat mencurigakan saat dibaca.
func (db *DB) CatalogEntryAny(ctx context.Context, q Querier, id string) (*CatalogEntry, error) {
	row := q.QueryRow(ctx, `
        SELECT `+catalogColumns+`
          FROM drug_catalog
         WHERE id = $1 AND deleted_at IS NULL`, id)
	return scanCatalogEntry(row)
}

// CreatePrivateCatalogInput adalah data katalog pribadi baru.
type CreatePrivateCatalogInput struct {
	OwnerID        string
	DisplayName    string
	DosageFormCode *string
	RouteCode      *string
	UcumCode       *string
	UomName        *string
	DoseValueText  *string
}

// CreatePrivateCatalog membuat entri katalog pribadi.
//
// is_preset, owner_id, dan curation_status DITETAPKAN DI SINI, bukan
// diterima dari klien. Kalau klien bisa mengirimnya, ia bisa membuat entri
// yang mengaku preset publik — dan entri itu akan muncul untuk semua user.
func (db *DB) CreatePrivateCatalog(ctx context.Context, q Querier, in CreatePrivateCatalogInput) (*CatalogEntry, error) {
	row := q.QueryRow(ctx, `
        INSERT INTO drug_catalog (
            display_name, dosage_form_code, route_code, ucum_code, uom_name,
            dose_value_text, owner_id, is_preset, curation_status, source
        ) VALUES ($1,$2,$3,$4,$5,$6,$7,false,'user_private','manual')
        RETURNING `+catalogColumns,
		strings.TrimSpace(in.DisplayName), in.DosageFormCode, in.RouteCode,
		in.UcumCode, in.UomName, in.DoseValueText, in.OwnerID,
	)

	e, err := scanCatalogEntry(row)
	if err != nil {
		// Nama yang sama untuk user yang sama ditolak uq_catalog_owner_name.
		// Dibiarkan sebagai error unik biasa, karena pemanggil yang
		// memutuskan kode balasannya.
		return nil, err
	}
	return e, nil
}

// UpdatePrivateCatalogInput adalah perubahan katalog pribadi.
type UpdatePrivateCatalogInput struct {
	DisplayName    *string
	DosageFormCode *string
	RouteCode      *string
	UcumCode       *string
	UomName        *string
	DoseValueText  *string
}

// UpdatePrivateCatalog mengubah katalog pribadi milik user.
//
// Syarat owner_id dan is_preset=false ada di WHERE, bukan hanya di
// pemeriksaan sebelumnya. Dengan begitu, satu bug di lapisan atas tidak bisa
// berubah menjadi kemampuan mengubah preset publik — database sendiri yang
// menolaknya.
func (db *DB) UpdatePrivateCatalog(ctx context.Context, q Querier, id, ownerID string, in UpdatePrivateCatalogInput) (*CatalogEntry, error) {
	row := q.QueryRow(ctx, `
        UPDATE drug_catalog SET
            display_name     = COALESCE($3, display_name),
            dosage_form_code = COALESCE($4, dosage_form_code),
            route_code       = COALESCE($5, route_code),
            ucum_code        = COALESCE($6, ucum_code),
            uom_name         = COALESCE($7, uom_name),
            dose_value_text  = COALESCE($8, dose_value_text)
         WHERE id = $1
           AND owner_id = $2
           AND is_preset = false
           AND curation_status = 'user_private'
           AND deleted_at IS NULL
        RETURNING `+catalogColumns,
		id, ownerID, in.DisplayName, in.DosageFormCode, in.RouteCode,
		in.UcumCode, in.UomName, in.DoseValueText,
	)
	return scanCatalogEntry(row)
}

// SoftDeletePrivateCatalog menghapus katalog pribadi milik user.
func (db *DB) SoftDeletePrivateCatalog(ctx context.Context, q Querier, id, ownerID string) error {
	tag, err := q.Exec(ctx, `
        UPDATE drug_catalog
           SET deleted_at = now()
         WHERE id = $1
           AND owner_id = $2
           AND is_preset = false
           AND curation_status = 'user_private'
           AND deleted_at IS NULL`, id, ownerID)
	if err != nil {
		return fmt.Errorf("menghapus katalog pribadi: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// SuggestCatalog mengambil saran nama untuk autocomplete.
//
// Sengaja HANYA mengembalikan id dan nama, bukan seluruh entri. Autocomplete
// dipanggil pada setiap ketikan, dan mengirim 40 kolom per saran akan
// membebani jaringan tanpa manfaat.
func (db *DB) SuggestCatalog(ctx context.Context, q Querier, userID, prefix string, limit int) ([]CatalogSuggestion, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}

	raw := strings.TrimSpace(prefix)
	if raw == "" {
		return nil, nil
	}

	// Prefix matching, bukan full-text: pengguna mengetik nama dari awal,
	// sehingga "amlo" harus menyarankan "Amlodipine". Dua pola dipakai
	// supaya kata di tengah nama juga ketemu.
	rows, err := q.Query(ctx, `
        SELECT id, display_name, dosage_form_name, route_name, is_preset
          FROM drug_catalog
         WHERE deleted_at IS NULL
           AND ((is_preset = true AND curation_status = 'published')
                OR (is_preset = false AND owner_id = $1))
           AND (display_name ILIKE $2 OR display_name ILIKE $3)
         ORDER BY is_preset DESC, length(display_name), display_name
         LIMIT $4`,
		userID, raw+"%", "% "+raw+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("mengambil saran katalog: %w", err)
	}
	defer rows.Close()

	var out []CatalogSuggestion
	for rows.Next() {
		var s CatalogSuggestion
		if err := rows.Scan(&s.ID, &s.DisplayName, &s.DosageFormName,
			&s.RouteName, &s.IsPreset); err != nil {
			return nil, fmt.Errorf("membaca saran katalog: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// CatalogSuggestion adalah satu saran nama.
type CatalogSuggestion struct {
	ID             string
	DisplayName    string
	DosageFormName *string
	RouteName      *string
	IsPreset       bool
}

// escapeLikePattern meng-escape karakter khusus pada pola LIKE.
//
// Tanpa ini, pengguna yang mengetik "%" akan mencocokkan SEMUA entri, dan
// "_" akan mencocokkan sembarang satu karakter. Keduanya bukan celah
// keamanan di sini — query-nya sudah berparameter — tapi hasil pencariannya
// menjadi membingungkan.
func escapeLikePattern(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '%', '_', '\\':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// toPrefixTSQuery menyusun tsquery dengan awalan pada kata terakhir.
//
// Kata terakhir diberi ":*" supaya ketikan yang belum selesai tetap
// menemukan hasil: "amlo" cocok dengan "amlodipine". Kata sebelumnya
// dibiarkan utuh, karena pengguna yang sudah selesai mengetik satu kata
// biasanya memang mencari kata itu.
//
// Karakter yang tidak aman dibuang, bukan di-escape: tsquery punya sintaks
// sendiri (&, |, !, :, <->) dan meneruskan karakter itu apa adanya akan
// menghasilkan error sintaks, bukan hasil pencarian kosong.
func toPrefixTSQuery(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}

	out := make([]string, 0, len(fields))
	for i, f := range fields {
		clean := strings.Map(func(r rune) rune {
			switch {
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z',
				r >= '0' && r <= '9', r == '-' || r == '.':
				return r
			}
			return -1
		}, f)
		if clean == "" {
			continue
		}
		if i == len(fields)-1 {
			out = append(out, clean+":*")
		} else {
			out = append(out, clean)
		}
	}
	return strings.Join(out, " & ")
}
