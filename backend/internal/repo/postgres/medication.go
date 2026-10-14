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

// medicationColumns adalah daftar kolom medications untuk SELECT berulang.
//
// Dikumpulkan di satu tempat supaya urutan scan tidak pernah meleset saat
// kolom ditambah. Setiap penambahan kolom cukup diubah di sini.
const medicationColumns = `
    id, owner_id, catalog_id, name, dose_value, dose_unit_code, strength_note,
    route_code, dosage_form_code, meal_relation, as_needed, must_finish,
    start_date, start_time::text, end_date, notes, notes_reminder,
    prescription_photo_url, color_tag, is_active, created_at, updated_at`

// scanMedication membaca satu baris medications.
func scanMedication(row pgx.Row) (*domain.Medication, error) {
	var m domain.Medication
	err := row.Scan(
		&m.ID, &m.OwnerID, &m.CatalogID, &m.Name, &m.DoseValue, &m.DoseUnitCode,
		&m.StrengthNote, &m.RouteCode, &m.DosageFormCode, &m.MealRelation,
		&m.AsNeeded, &m.MustFinish, &m.StartDate, &m.StartTime, &m.EndDate,
		&m.Notes, &m.NotesReminder, &m.PrescriptionPhotoURL, &m.ColorTag,
		&m.IsActive, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("membaca obat: %w", err)
	}
	return &m, nil
}

// CreateMedicationInput adalah data mentah pembuatan obat.
//
// Berbeda dari domain.Medication karena di sini id belum ada, dan waktu
// belum ditentukan database.
type CreateMedicationInput struct {
	OwnerID              string
	CatalogID            *string
	Name                 string
	DoseValue            float64
	DoseUnitCode         string
	StrengthNote         *string
	RouteCode            string
	DosageFormCode       *string
	MealRelation         domain.MealRelation
	AsNeeded             bool
	MustFinish           bool
	StartDate            time.Time
	StartTime            string
	EndDate              *time.Time
	Notes                *string
	NotesReminder        *string
	PrescriptionPhotoURL *string
	ColorTag             *string
}

// CreateMedication menyimpan obat baru.
//
// Obat TIDAK menyimpan baris per hari. Yang dibuat hanya satu baris obat,
// satu baris aturan frekuensi, dan N baris jam. Itu berlaku untuk terapi
// "selamanya" maupun terapi 5 hari.
func (db *DB) CreateMedication(ctx context.Context, q Querier, in CreateMedicationInput) (*domain.Medication, error) {
	meal := in.MealRelation
	if meal == "" {
		meal = domain.MealNotRelated
	}
	startTime := in.StartTime
	if startTime == "" {
		startTime = "00:00"
	}

	row := q.QueryRow(ctx, `
        INSERT INTO medications (
            owner_id, catalog_id, name, dose_value, dose_unit_code, strength_note,
            route_code, dosage_form_code, meal_relation, as_needed, must_finish,
            start_date, start_time, end_date, notes, notes_reminder,
            prescription_photo_url, color_tag
        ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
        RETURNING `+medicationColumns,
		in.OwnerID, in.CatalogID, strings.TrimSpace(in.Name), in.DoseValue,
		in.DoseUnitCode, in.StrengthNote, in.RouteCode, in.DosageFormCode,
		meal, in.AsNeeded, in.MustFinish, in.StartDate, startTime, in.EndDate,
		in.Notes, in.NotesReminder, in.PrescriptionPhotoURL, in.ColorTag,
	)

	m, err := scanMedication(row)
	if err != nil {
		// Duplikat ditentukan oleh uq_meds_owner_name. Memetakan ke sentinel
		// yang tepat membuat handler bisa membalas 409 dengan pesan yang
		// jelas, bukan 500.
		if IsUniqueViolation(err) {
			return nil, domain.ErrDuplicateMedication
		}
		return nil, err
	}
	return m, nil
}

// MedicationByID mengambil obat milik user tertentu.
//
// ownerID WAJIB disertakan, bukan opsional. Semua query di repo ini memfilter
// owner_id, sehingga tidak ada jalur di mana satu user bisa membaca obat user
// lain karena lupa menambahkan syaratnya. Multi-tenant dijaga di lapisan ini,
// bukan hanya di handler.
func (db *DB) MedicationByID(ctx context.Context, q Querier, id, ownerID string) (*domain.Medication, error) {
	row := q.QueryRow(ctx, `
        SELECT `+medicationColumns+`
          FROM medications
         WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL`, id, ownerID)
	return scanMedication(row)
}

// MedicationByIDAnyOwner mengambil obat tanpa memfilter owner.
//
// Dipakai HANYA oleh jalur caregiver, yang wewenangnya sudah diperiksa
// lewat care_links di lapisan service. Sengaja diberi nama berbeda supaya
// pemakaiannya di tempat lain langsung terlihat mencurigakan saat dibaca.
func (db *DB) MedicationByIDAnyOwner(ctx context.Context, q Querier, id string) (*domain.Medication, error) {
	row := q.QueryRow(ctx, `
        SELECT `+medicationColumns+`
          FROM medications
         WHERE id = $1 AND deleted_at IS NULL`, id)
	return scanMedication(row)
}

// ListMedicationsFilter adalah syarat pencarian daftar obat.
type ListMedicationsFilter struct {
	OwnerID string
	Status  string // "active" | "all"
	Query   string
	Limit   int
	Cursor  *ListCursor
}

// ListMedications mengambil daftar obat milik user.
func (db *DB) ListMedications(ctx context.Context, q Querier, f ListMedicationsFilter) ([]*domain.Medication, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 20
	}

	var (
		where []string
		args  []any
	)
	where = append(where, "owner_id = $1", "deleted_at IS NULL")
	args = append(args, f.OwnerID)

	if f.Status != "all" {
		where = append(where, "is_active = true")
	}

	if f.Query != "" {
		args = append(args, "%"+strings.ToLower(strings.TrimSpace(f.Query))+"%")
		where = append(where, fmt.Sprintf("lower(name) LIKE $%d", len(args)))
	}

	// Pagination berbasis cursor, bukan offset. Offset membuat baris
	// terlewat atau terulang saat data berubah di tengah penelusuran.
	if f.Cursor != nil {
		args = append(args, f.Cursor.CreatedAt, f.Cursor.ID)
		where = append(where,
			fmt.Sprintf("(created_at, id) < ($%d, $%d)", len(args)-1, len(args)))
	}

	args = append(args, f.Limit)

	query := `SELECT ` + medicationColumns + `
                FROM medications
               WHERE ` + strings.Join(where, " AND ") + `
               ORDER BY created_at DESC, id DESC
               LIMIT $` + fmt.Sprint(len(args))

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("mendaftar obat: %w", err)
	}
	defer rows.Close()

	var out []*domain.Medication
	for rows.Next() {
		m, err := scanMedication(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListCursor adalah posisi penelusuran halaman berikutnya.
type ListCursor struct {
	CreatedAt time.Time
	ID        string
}

// UpdateMedicationInput adalah perubahan yang diminta pada obat.
//
// Semua pointer supaya hanya field yang dikirim yang berubah. Tanpa ini,
// PATCH yang hanya ingin mengubah warna akan ikut mengosongkan catatan.
type UpdateMedicationInput struct {
	Name           *string
	DoseValue      *float64
	DoseUnitCode   *string
	StrengthNote   *string
	RouteCode      *string
	DosageFormCode *string
	MealRelation   *domain.MealRelation
	AsNeeded       *bool
	MustFinish     *bool
	StartDate      *time.Time
	StartTime      *string
	EndDate        *time.Time // nil = tidak diubah; pakai ClearEndDate untuk mengosongkan
	ClearEndDate   bool
	Notes          *string
	NotesReminder  *string
	ColorTag       *string
	IsActive       *bool
}

// UpdateMedication mengubah data obat.
//
// Memakai COALESCE untuk setiap kolom, sehingga hanya nilai yang dikirim
// yang berubah. Kolom end_date ditangani terpisah karena "kosongkan" dan
// "jangan ubah" adalah dua hal berbeda yang tidak bisa dibedakan oleh
// COALESCE.
func (db *DB) UpdateMedication(ctx context.Context, q Querier, id, ownerID string, in UpdateMedicationInput) (*domain.Medication, error) {
	row := q.QueryRow(ctx, `
        UPDATE medications SET
            name             = COALESCE($3, name),
            dose_value       = COALESCE($4, dose_value),
            dose_unit_code   = COALESCE($5, dose_unit_code),
            strength_note    = COALESCE($6, strength_note),
            route_code       = COALESCE($7, route_code),
            dosage_form_code = COALESCE($8, dosage_form_code),
            meal_relation    = COALESCE($9, meal_relation),
            as_needed        = COALESCE($10, as_needed),
            must_finish      = COALESCE($11, must_finish),
            start_date       = COALESCE($12, start_date),
            start_time       = COALESCE($13, start_time),
            end_date         = CASE WHEN $14 THEN NULL
                                    ELSE COALESCE($15, end_date) END,
            notes            = COALESCE($16, notes),
            notes_reminder   = COALESCE($17, notes_reminder),
            color_tag        = COALESCE($18, color_tag),
            is_active        = COALESCE($19, is_active)
         WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL
        RETURNING `+medicationColumns,
		id, ownerID,
		in.Name, in.DoseValue, in.DoseUnitCode, in.StrengthNote, in.RouteCode,
		in.DosageFormCode, in.MealRelation, in.AsNeeded, in.MustFinish,
		in.StartDate, in.StartTime, in.ClearEndDate, in.EndDate,
		in.Notes, in.NotesReminder, in.ColorTag, in.IsActive,
	)

	m, err := scanMedication(row)
	if err != nil {
		if IsUniqueViolation(err) {
			return nil, domain.ErrDuplicateMedication
		}
		return nil, err
	}
	return m, nil
}

// SoftDeleteMedication menghapus obat beserta jam-jamnya dalam satu transaksi.
//
// Log kepatuhan TIDAK dihapus. Sesuai data-model-v2 bagian 10 butir 2,
// riwayat pemakaian harus tetap ada walau obatnya dihapus; yang hilang hanya
// jadwalnya, sehingga pengingat berhenti.
//
// Obat yang sudah dihapus boleh didaftarkan ulang dengan nama sama, karena
// uq_meds_owner_name bersifat partial (hanya berlaku untuk baris hidup).
func (db *DB) SoftDeleteMedication(ctx context.Context, q Querier, id, ownerID string) error {
	tag, err := q.Exec(ctx, `
        UPDATE medications
           SET deleted_at = now(), is_active = false
         WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL`, id, ownerID)
	if err != nil {
		return fmt.Errorf("menghapus obat: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	// Jam ikut dihapus supaya pengingat benar-benar berhenti dan tidak ada
	// baris jadwal yang menggantung menunjuk obat yang sudah hilang.
	_, err = q.Exec(ctx, `
        UPDATE medication_times
           SET deleted_at = now(), is_active = false
         WHERE medication_id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("menghapus jam obat: %w", err)
	}

	_, err = q.Exec(ctx, `
        UPDATE medication_frequency
           SET deleted_at = now()
         WHERE medication_id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("menghapus frekuensi obat: %w", err)
	}
	return nil
}

// CountActiveMedications menghitung obat aktif milik user. Dipakai agregat
// admin dan halaman ringkasan.
func (db *DB) CountActiveMedications(ctx context.Context, q Querier, ownerID string) (int, error) {
	var n int
	err := q.QueryRow(ctx, `
        SELECT count(*) FROM medications
         WHERE owner_id = $1 AND deleted_at IS NULL AND is_active = true`, ownerID).Scan(&n)
	return n, err
}

// SetMedicationPhoto menulis kunci objek foto resep untuk satu obat.
//
// Nilai nil mengosongkan kolomnya. Kepemilikan TIDAK diperiksa di sini —
// pemanggil (service) sudah melakukannya, dan klausa owner_id di bawah tetap
// disertakan sebagai lapisan kedua supaya kesalahan pemanggil tidak berubah
// menjadi penulisan lintas pemilik.
func (db *DB) SetMedicationPhoto(ctx context.Context, q Querier, ownerID, medicationID string, key *string) error {
	tag, err := q.Exec(ctx, `
        UPDATE medications
           SET prescription_photo_url = $3,
               updated_at = now()
         WHERE id = $1 AND owner_id = $2`, medicationID, ownerID, key)
	if err != nil {
		return fmt.Errorf("menyimpan kunci foto resep: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
