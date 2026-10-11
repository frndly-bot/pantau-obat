package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// File ini mengurus ATURAN jadwal: satu baris frekuensi + N baris jam per obat.
//
// Tidak ada tabel baris-per-hari di sini. Itu keputusan inti v2: terapi
// "selamanya" 2x sehari memakai 3 baris, bukan 730 baris setahun.

// frequencyColumns adalah daftar kolom untuk SELECT berulang.
//
// PENTING: TIDAK BOLEH memuat literal string atau tanda kutip. Daftar ini
// dilewatkan ke prefixed() saat query gabungan, dan fungsi itu memecah daftar
// berdasarkan spasi lalu menempelkan alias tabel di depan setiap potongan.
// Literal seperti 'Asia/Jakarta' akan terpecah dan berubah menjadi
// f.'Asia/Jakarta' — dan PostgreSQL menolak seluruh query dengan
// "syntax error at or near".
//
// Nilai bawaan untuk zona waktu karena itu TIDAK diambil dari SQL, melainkan
// diisi di Go saat membacanya. Kolomnya sendiri NOT NULL di skema, jadi
// NULL hanya mungkin muncul dari baris lama.
const frequencyColumns = `
    id, medication_id, owner_id, pattern, times_per_day, interval_hours,
    days_of_week, interval_min_hours, cycle_intake_days, cycle_pause_days,
    cycle_anchor, cycle_anchor_dom, prn_max_per_day, tolerance_minutes,
    start_time::text, schedule_timezone,
    created_at, updated_at`

// CreateFrequencyInput adalah data aturan frekuensi baru.
type CreateFrequencyInput struct {
	MedicationID string
	OwnerID      string

	Pattern domain.FrequencyPattern

	TimesPerDay      *int
	IntervalHours    *int
	DaysOfWeek       *int
	IntervalMinHours *int

	CycleIntakeDays *int
	CyclePauseDays  *int
	CycleAnchor     *domain.CycleAnchor
	CycleAnchorDOM  *int

	PRNMaxPerDay *int

	ToleranceMinutes int
	StartTime        string
	ScheduleTimezone string
}

// scanFrequency membaca satu baris medication_frequency.
func scanFrequency(row pgx.Row) (*domain.MedicationFrequency, error) {
	var f domain.MedicationFrequency
	var tz *string

	err := row.Scan(
		&f.ID, &f.MedicationID, &f.OwnerID, &f.Pattern, &f.TimesPerDay,
		&f.IntervalHours, &f.DaysOfWeek, &f.IntervalMinHours,
		&f.CycleIntakeDays, &f.CyclePauseDays, &f.CycleAnchor,
		&f.CycleAnchorDOM, &f.PRNMaxPerDay, &f.ToleranceMinutes,
		&f.StartTime, &tz, &f.CreatedAt, &f.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("membaca frekuensi: %w", err)
	}

	// Bawaan diisi di Go, bukan lewat coalesce() di SQL. Alasannya ada di
	// komentar frequencyColumns: literal string tidak boleh masuk daftar
	// kolom karena akan dipecah oleh prefixed().
	if tz != nil && *tz != "" {
		f.ScheduleTimezone = *tz
	} else {
		f.ScheduleTimezone = domain.DefaultTimezone
	}
	return &f, nil
}

// CreateFrequency menyimpan aturan frekuensi.
//
// Satu obat hanya punya satu aturan (UNIQUE pada medication_id). Mengubah
// pola berarti mengubah baris ini, bukan menambah baris baru, supaya tidak
// ada dua aturan yang berlaku bersamaan dan saling bertentangan.
func (db *DB) CreateFrequency(ctx context.Context, q Querier, in CreateFrequencyInput) (*domain.MedicationFrequency, error) {
	startTime := in.StartTime
	if startTime == "" {
		startTime = "08:00"
	}
	tz := in.ScheduleTimezone
	if tz == "" {
		tz = domain.DefaultTimezone
	}

	row := q.QueryRow(ctx, `
        INSERT INTO medication_frequency (
            medication_id, owner_id, pattern, times_per_day, interval_hours,
            days_of_week, interval_min_hours, cycle_intake_days, cycle_pause_days,
            cycle_anchor, cycle_anchor_dom, prn_max_per_day, tolerance_minutes,
            start_time, schedule_timezone
        ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
        RETURNING `+frequencyColumns,
		in.MedicationID, in.OwnerID, in.Pattern, in.TimesPerDay, in.IntervalHours,
		in.DaysOfWeek, in.IntervalMinHours, in.CycleIntakeDays, in.CyclePauseDays,
		in.CycleAnchor, in.CycleAnchorDOM, in.PRNMaxPerDay, in.ToleranceMinutes,
		startTime, tz,
	)
	return scanFrequency(row)
}

// FrequencyByMedication mengambil aturan frekuensi sebuah obat.
func (db *DB) FrequencyByMedication(ctx context.Context, q Querier, medicationID string) (*domain.MedicationFrequency, error) {
	row := q.QueryRow(ctx, `
        SELECT `+frequencyColumns+`
          FROM medication_frequency
         WHERE medication_id = $1 AND deleted_at IS NULL`, medicationID)
	return scanFrequency(row)
}

// UpdateFrequencyPattern mengubah aturan frekuensi yang sudah ada.
//
// CATATAN PENTING soal schedule_timezone: kolom ini TIDAK ikut diubah di
// sini. Zona disimpan saat blok jadwal dibuat, dan perubahannya punya
// konsekuensi pada tanggal dosis yang sudah tercatat. Perubahan zona harus
// lewat jalur tersendiri supaya jejaknya bisa dicatat sebagai dose_shift.
func (db *DB) UpdateFrequencyPattern(ctx context.Context, q Querier, medicationID string, in CreateFrequencyInput) (*domain.MedicationFrequency, error) {
	row := q.QueryRow(ctx, `
        UPDATE medication_frequency SET
            pattern           = $2,
            times_per_day     = $3,
            interval_hours    = $4,
            days_of_week      = $5,
            interval_min_hours= $6,
            cycle_intake_days = $7,
            cycle_pause_days  = $8,
            cycle_anchor      = $9,
            cycle_anchor_dom  = $10,
            prn_max_per_day   = $11,
            tolerance_minutes = $12,
            start_time        = COALESCE(NULLIF($13,''), start_time)
         WHERE medication_id = $1 AND deleted_at IS NULL
        RETURNING `+frequencyColumns,
		medicationID, in.Pattern, in.TimesPerDay, in.IntervalHours,
		in.DaysOfWeek, in.IntervalMinHours, in.CycleIntakeDays, in.CyclePauseDays,
		in.CycleAnchor, in.CycleAnchorDOM, in.PRNMaxPerDay, in.ToleranceMinutes,
		in.StartTime,
	)
	return scanFrequency(row)
}

const timeColumns = `
    id, medication_id, frequency_id, owner_id, time_local::text, label,
    weekdays, dose_override, sort_order, is_active, created_at, updated_at`

// scanMedicationTime membaca satu baris medication_times.
func scanMedicationTime(row pgx.Row) (*domain.MedicationTime, error) {
	var t domain.MedicationTime
	err := row.Scan(
		&t.ID, &t.MedicationID, &t.FrequencyID, &t.OwnerID, &t.TimeLocal,
		&t.Label, &t.Weekdays, &t.DoseOverride, &t.SortOrder, &t.IsActive,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("membaca jam minum: %w", err)
	}
	return &t, nil
}

// CreateMedicationTimeInput adalah data satu jam minum baru.
type CreateMedicationTimeInput struct {
	MedicationID string
	FrequencyID  *string
	OwnerID      string
	TimeLocal    string
	Label        *domain.TimeLabel
	Weekdays     int
	DoseOverride *string
	SortOrder    int
}

// CreateMedicationTime menambah satu jam minum.
//
// Duplikat ditentukan uq_times_med_time, yaitu kombinasi
// (medication_id, time_local, weekdays). Jadi jam yang sama BOLEH ada dua
// kali bila pola harinya berbeda, dan itu memang benar: 07:00 hari kerja
// dan 09:00 akhir pekan adalah dua jadwal yang berbeda.
func (db *DB) CreateMedicationTime(ctx context.Context, q Querier, in CreateMedicationTimeInput) (*domain.MedicationTime, error) {
	weekdays := in.Weekdays
	if weekdays == 0 {
		weekdays = domain.EveryDay
	}

	row := q.QueryRow(ctx, `
        INSERT INTO medication_times (
            medication_id, frequency_id, owner_id, time_local, label,
            weekdays, dose_override, sort_order
        ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
        RETURNING `+timeColumns,
		in.MedicationID, in.FrequencyID, in.OwnerID, in.TimeLocal, in.Label,
		weekdays, in.DoseOverride, in.SortOrder,
	)

	t, err := scanMedicationTime(row)
	if err != nil {
		if IsUniqueViolation(err) {
			return nil, domain.ErrDuplicateTimes
		}
		return nil, err
	}
	return t, nil
}

// TimesByMedication mengambil semua jam minum sebuah obat.
func (db *DB) TimesByMedication(ctx context.Context, q Querier, medicationID string) ([]domain.MedicationTime, error) {
	rows, err := q.Query(ctx, `
        SELECT `+timeColumns+`
          FROM medication_times
         WHERE medication_id = $1 AND deleted_at IS NULL
         ORDER BY sort_order, time_local`, medicationID)
	if err != nil {
		return nil, fmt.Errorf("mendaftar jam minum: %w", err)
	}
	defer rows.Close()

	var out []domain.MedicationTime
	for rows.Next() {
		t, err := scanMedicationTime(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// MedicationTimeByID mengambil satu jam minum milik user.
func (db *DB) MedicationTimeByID(ctx context.Context, q Querier, id, ownerID string) (*domain.MedicationTime, error) {
	row := q.QueryRow(ctx, `
        SELECT `+timeColumns+`
          FROM medication_times
         WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL`, id, ownerID)
	return scanMedicationTime(row)
}

// UpdateMedicationTimeInput adalah perubahan pada satu jam minum.
type UpdateMedicationTimeInput struct {
	TimeLocal    *string
	Label        *domain.TimeLabel
	Weekdays     *int
	DoseOverride *string
	SortOrder    *int
	IsActive     *bool
}

// UpdateMedicationTime mengubah satu jam minum.
//
// Mengubah time_local hanya berlaku untuk dosis KE DEPAN. Log yang sudah
// tercatat dipertahankan apa adanya, karena occurrence key-nya memuat jam
// yang berlaku saat dosis itu dicatat. Service yang mencatat perubahannya
// sebagai dose_shift.
func (db *DB) UpdateMedicationTime(ctx context.Context, q Querier, id, ownerID string, in UpdateMedicationTimeInput) (*domain.MedicationTime, error) {
	row := q.QueryRow(ctx, `
        UPDATE medication_times SET
            time_local    = COALESCE($3, time_local),
            label         = COALESCE($4, label),
            weekdays      = COALESCE($5, weekdays),
            dose_override = COALESCE($6, dose_override),
            sort_order    = COALESCE($7, sort_order),
            is_active     = COALESCE($8, is_active)
         WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL
        RETURNING `+timeColumns,
		id, ownerID, in.TimeLocal, in.Label, in.Weekdays,
		in.DoseOverride, in.SortOrder, in.IsActive,
	)

	t, err := scanMedicationTime(row)
	if err != nil {
		if IsUniqueViolation(err) {
			return nil, domain.ErrDuplicateTimes
		}
		return nil, err
	}
	return t, nil
}

// SoftDeleteMedicationTime menghapus satu jam minum.
//
// Soft-delete, bukan DELETE fisik: log dosis lama merujuk schedule_id ini
// lewat FK RESTRICT, jadi menghapus fisiknya akan ditolak database. Yang
// benar adalah menandainya terhapus, sehingga tidak lagi menghasilkan
// jadwal baru sementara riwayatnya tetap utuh.
func (db *DB) SoftDeleteMedicationTime(ctx context.Context, q Querier, id, ownerID string) error {
	tag, err := q.Exec(ctx, `
        UPDATE medication_times
           SET deleted_at = now(), is_active = false
         WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL`, id, ownerID)
	if err != nil {
		return fmt.Errorf("menghapus jam minum: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// ActiveSchedulesForDueScan mengambil semua obat beserta aturan dan jamnya,
// untuk dipakai scheduler mencari dosis yang jatuh tempo.
//
// Sengaja mengambil semuanya sekaligus, bukan satu query per obat: jumlah
// obat aktif per user kecil, dan satu query besar jauh lebih murah daripada
// N+1 query. Baris yang dikembalikan adalah gabungan obat + frekuensi + jam,
// dan baris dengan jam NULL berarti obat tanpa jam aktif.
type DueScanRow struct {
	Medication domain.Medication
	Frequency  *domain.MedicationFrequency
	Time       *domain.MedicationTime
}

// ScanDueCandidates mengambil kandidat dosis jatuh tempo.
//
// Filter yang dilakukan di SQL: obat hidup, aktif, dan punya jam aktif.
// Filter yang TIDAK dilakukan di SQL: pencocokan pola dan zona waktu,
// karena `schedule_timezone` adalah teks bebas dan Postgres tidak bisa
// menghitungnya secepat Go. Penyaringan pola dilakukan di domain, yang
// sudah punya test sendiri.
func (db *DB) ScanDueCandidates(ctx context.Context, q Querier) ([]DueScanRow, error) {
	rows, err := q.Query(ctx, `
        SELECT
            m.id, m.owner_id, m.catalog_id, m.name, m.dose_value, m.dose_unit_code,
            m.strength_note, m.route_code, m.dosage_form_code, m.meal_relation,
            m.as_needed, m.must_finish, m.start_date, m.start_time::text, m.end_date,
            m.notes, m.notes_reminder, m.prescription_photo_url, m.color_tag,
            m.is_active, m.created_at, m.updated_at,
            `+prefixed("f", frequencyColumns)+`,
            `+prefixed("t", timeColumns)+`
          FROM medications m
          JOIN medication_frequency f ON f.medication_id = m.id AND f.deleted_at IS NULL
          JOIN medication_times t     ON t.medication_id = m.id
                                     AND t.deleted_at IS NULL AND t.is_active = true
         WHERE m.deleted_at IS NULL AND m.is_active = true
         ORDER BY m.id, t.sort_order, t.time_local`)
	if err != nil {
		return nil, fmt.Errorf("memindai kandidat dosis: %w", err)
	}
	defer rows.Close()

	var out []DueScanRow
	for rows.Next() {
		var (
			row DueScanRow
			m   domain.Medication
			f   domain.MedicationFrequency
			t   domain.MedicationTime
		)
		err := rows.Scan(
			&m.ID, &m.OwnerID, &m.CatalogID, &m.Name, &m.DoseValue, &m.DoseUnitCode,
			&m.StrengthNote, &m.RouteCode, &m.DosageFormCode, &m.MealRelation,
			&m.AsNeeded, &m.MustFinish, &m.StartDate, &m.StartTime, &m.EndDate,
			&m.Notes, &m.NotesReminder, &m.PrescriptionPhotoURL, &m.ColorTag,
			&m.IsActive, &m.CreatedAt, &m.UpdatedAt,

			&f.ID, &f.MedicationID, &f.OwnerID, &f.Pattern, &f.TimesPerDay,
			&f.IntervalHours, &f.DaysOfWeek, &f.IntervalMinHours,
			&f.CycleIntakeDays, &f.CyclePauseDays, &f.CycleAnchor,
			&f.CycleAnchorDOM, &f.PRNMaxPerDay, &f.ToleranceMinutes,
			&f.StartTime, &f.ScheduleTimezone, &f.CreatedAt, &f.UpdatedAt,

			&t.ID, &t.MedicationID, &t.FrequencyID, &t.OwnerID, &t.TimeLocal,
			&t.Label, &t.Weekdays, &t.DoseOverride, &t.SortOrder, &t.IsActive,
			&t.CreatedAt, &t.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("membaca kandidat dosis: %w", err)
		}
		row.Medication = m
		row.Frequency = &f
		row.Time = &t
		out = append(out, row)
	}
	return out, rows.Err()
}

// prefixed menambahkan alias tabel ke daftar kolom.
//
// Diperlukan karena query gabungan memuat kolom bernama sama dari beberapa
// tabel (id, owner_id, created_at). Tanpa alias, PostgreSQL menolak dengan
// "column reference is ambiguous".
func prefixed(alias, columns string) string {
	// Penjagaan terhadap kesalahan yang pernah benar-benar terjadi: daftar
	// kolom yang memuat tanda kutip akan dipecah di tengah literal, dan
	// query-nya gagal dengan "syntax error at or near". Lebih baik gagal di
	// sini dengan pesan yang menjelaskan daripada menghasilkan SQL rusak.
	if stringsContains(columns, "'") {
		panic("prefixed: daftar kolom tidak boleh memuat tanda kutip; " +
			"pindahkan literal ke parameter query")
	}

	out := make([]string, 0, 32)
	cur := make([]byte, 0, 32)
	flush := func() {
		s := string(cur)
		cur = cur[:0]
		if s == "" {
			return
		}
		out = append(out, alias+"."+s)
	}
	for i := 0; i < len(columns); i++ {
		c := columns[i]
		switch c {
		case ' ', '\n', '\t', ',':
			flush()
			if c == ',' {
				out = append(out, ",")
			}
		default:
			cur = append(cur, c)
		}
	}
	flush()
	return stringsJoin(out)
}

// stringsJoin menyatukan potongan kolom dengan spasi, kecuali koma yang
// menempel ke elemen sebelumnya.
func stringsJoin(parts []string) string {
	var b []byte
	for _, p := range parts {
		if p == "," {
			b = append(b, ',')
			continue
		}
		if len(b) > 0 {
			last := b[len(b)-1]
			if last != ',' {
				b = append(b, ' ')
			}
		}
		b = append(b, p...)
	}
	return string(b)
}

// stringsContains memeriksa keberadaan substring tanpa memanggil strings.
//
// Disediakan supaya paket ini tidak perlu menambah impor hanya untuk satu
// pemeriksaan sederhana di jalur penyiapan query.
func stringsContains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	n := len(needle)
	for i := 0; i+n <= len(haystack); i++ {
		if haystack[i:i+n] == needle {
			return true
		}
	}
	return false
}
