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

// Medications adalah layanan pengelolaan obat.
type Medications struct {
	db     *postgres.DB
	logger *slog.Logger
}

// NewMedications membuat layanan obat.
func NewMedications(db *postgres.DB, logger *slog.Logger) *Medications {
	return &Medications{db: db, logger: logger}
}

// OwnerOfSchedule mengembalikan pemilik sebuah jam jadwal.
//
// Dipakai jalur caregiver untuk melengkapi `medication_id` ketika klien
// hanya mengirim `schedule_id` — bentuk yang paling wajar, karena id jadwal
// sudah menunjuk satu obat tertentu.
//
// Nilai baliknya adalah owner_id, supaya pemanggil bisa memastikan jadwal itu
// memang milik pasien yang bersangkutan. Tanpa pemeriksaan itu, seorang
// caregiver bisa mencatat dosis pada jadwal pasien yang bukan tanggung
// jawabnya hanya dengan menebak id.
func (s *Medications) OwnerOfSchedule(ctx context.Context, scheduleID string) (ownerID, medicationID string, err error) {
	var owner, med string
	err = s.db.Pool().QueryRow(ctx, `
        SELECT owner_id, medication_id
          FROM medication_times
         WHERE id = $1 AND deleted_at IS NULL`, scheduleID).Scan(&owner, &med)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", domain.ErrNotFound
		}
		return "", "", fmt.Errorf("membaca pemilik jadwal: %w", err)
	}
	return owner, med, nil
}

// FrequencyInput adalah aturan frekuensi yang diminta klien.
type FrequencyInput struct {
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

// TimeInput adalah satu jam minum yang diminta klien.
type TimeInput struct {
	TimeLocal    string
	Label        *domain.TimeLabel
	Weekdays     *int
	DoseOverride *string
}

// StockInput adalah stok awal saat obat dibuat.
type StockInput struct {
	UnitCode           string
	InitialQty         float64
	QtyPerDose         float64
	RefillThresholdQty *float64
	RefillReminder     bool
}

// CreateMedicationInput adalah permintaan pembuatan obat lengkap.
type CreateMedicationInput struct {
	Name           string
	DoseValue      float64
	DoseUnitCode   string
	StrengthNote   *string
	RouteCode      string
	DosageFormCode *string
	MealRelation   domain.MealRelation
	CatalogID      *string

	AsNeeded   bool
	MustFinish bool

	StartDate time.Time
	StartTime string
	EndDate   *time.Time

	Notes                *string
	NotesReminder        *string
	PrescriptionPhotoURL *string
	ColorTag             *string

	Frequency FrequencyInput
	Times     []TimeInput
	Stock     *StockInput
}

// Create membuat obat beserta frekuensi, jam, dan stoknya.
//
// Semuanya dalam SATU transaksi. Ini penting: obat tanpa jam tidak akan
// pernah memunculkan pengingat, dan obat dengan jam tetapi tanpa aturan
// frekuensi akan membuat scheduler gagal. Setengah jadi lebih buruk daripada
// gagal seluruhnya.
func (s *Medications) Create(ctx context.Context, ownerID string, in CreateMedicationInput, meta Meta) (*domain.Medication, error) {
	if err := s.validateCreate(ownerID, &in); err != nil {
		return nil, err
	}

	// Toleransi default 60 menit sesuai TRD bagian D1: status `taken` bila
	// selisih <= 60 menit, `late` bila lebih.
	tolerance := in.Frequency.ToleranceMinutes
	if tolerance == 0 {
		tolerance = 60
	}

	tz := in.Frequency.ScheduleTimezone
	if tz == "" {
		tz = domain.DefaultTimezone
	}

	var created *domain.Medication

	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		med, err := s.db.CreateMedication(ctx, tx, postgres.CreateMedicationInput{
			OwnerID:              ownerID,
			CatalogID:            in.CatalogID,
			Name:                 in.Name,
			DoseValue:            in.DoseValue,
			DoseUnitCode:         in.DoseUnitCode,
			StrengthNote:         in.StrengthNote,
			RouteCode:            in.RouteCode,
			DosageFormCode:       in.DosageFormCode,
			MealRelation:         in.MealRelation,
			AsNeeded:             in.AsNeeded,
			MustFinish:           in.MustFinish,
			StartDate:            in.StartDate,
			StartTime:            in.StartTime,
			EndDate:              in.EndDate,
			Notes:                in.Notes,
			NotesReminder:        in.NotesReminder,
			PrescriptionPhotoURL: in.PrescriptionPhotoURL,
			ColorTag:             in.ColorTag,
		})
		if err != nil {
			return err
		}

		freq, err := s.db.CreateFrequency(ctx, tx, postgres.CreateFrequencyInput{
			MedicationID:     med.ID,
			OwnerID:          ownerID,
			Pattern:          in.Frequency.Pattern,
			TimesPerDay:      in.Frequency.TimesPerDay,
			IntervalHours:    in.Frequency.IntervalHours,
			DaysOfWeek:       in.Frequency.DaysOfWeek,
			IntervalMinHours: in.Frequency.IntervalMinHours,
			CycleIntakeDays:  in.Frequency.CycleIntakeDays,
			CyclePauseDays:   in.Frequency.CyclePauseDays,
			CycleAnchor:      in.Frequency.CycleAnchor,
			CycleAnchorDOM:   in.Frequency.CycleAnchorDOM,
			PRNMaxPerDay:     in.Frequency.PRNMaxPerDay,
			ToleranceMinutes: tolerance,
			StartTime:        in.Frequency.StartTime,
			ScheduleTimezone: tz,
		})
		if err != nil {
			return err
		}

		times := make([]domain.MedicationTime, 0, len(in.Times))
		for i, t := range in.Times {
			weekdays := domain.EveryDay
			if t.Weekdays != nil {
				weekdays = *t.Weekdays
			}
			mt, err := s.db.CreateMedicationTime(ctx, tx, postgres.CreateMedicationTimeInput{
				MedicationID: med.ID,
				FrequencyID:  &freq.ID,
				OwnerID:      ownerID,
				TimeLocal:    t.TimeLocal,
				Label:        t.Label,
				Weekdays:     weekdays,
				DoseOverride: t.DoseOverride,
				SortOrder:    i,
			})
			if err != nil {
				return err
			}
			times = append(times, *mt)
		}

		if in.Stock != nil {
			stock, err := s.createStock(ctx, tx, med.ID, ownerID, *in.Stock)
			if err != nil {
				return err
			}
			// Ikut dikembalikan supaya balasan 201 langsung memuat saldo
			// akhir. Tanpa ini, klien harus GET ulang hanya untuk tahu
			// stok awalnya berapa — padahal itu baru saja ditentukan
			// oleh request yang sama.
			med.Stock = stock
		}

		if err := s.db.WriteAudit(ctx, tx, postgres.AuditEntry{
			ActorID:      &ownerID,
			OwnerID:      &ownerID,
			ActorRole:    string(domain.ActorUser),
			Action:       postgres.ActionCreate,
			ResourceType: postgres.ResourceMed,
			ResourceID:   &med.ID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			AfterState: map[string]any{
				"name":       med.Name,
				"dose_value": med.DoseValue,
				"pattern":    string(freq.Pattern),
				"times":      len(times),
			},
		}); err != nil {
			return err
		}

		med.Frequency = freq
		med.Times = times
		created = med
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// validateCreate memeriksa input sebelum menyentuh database.
//
// Mengumpulkan SEMUA masalah, bukan berhenti di yang pertama, supaya user
// tidak memperbaiki satu per satu. Pesan yang menyebut semuanya sekaligus
// jauh lebih berguna pada form yang panjang seperti form obat.
func (s *Medications) validateCreate(ownerID string, in *CreateMedicationInput) error {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	name := strings.TrimSpace(in.Name)
	if name == "" {
		add("nama obat wajib diisi")
	} else if len([]rune(name)) > 120 {
		add("nama obat maksimal 120 karakter")
	}
	in.Name = name

	if in.DoseValue <= 0 {
		add("nilai dosis harus lebih dari nol")
	}
	if strings.TrimSpace(in.DoseUnitCode) == "" {
		add("satuan dosis wajib diisi")
	}
	if strings.TrimSpace(in.RouteCode) == "" {
		add("rute pemberian wajib diisi")
	}

	if in.MealRelation != "" && !in.MealRelation.Valid() {
		add("waktu makan %q tidak dikenal", in.MealRelation)
	} else if in.MealRelation == "" {
		in.MealRelation = domain.MealNotRelated
	}

	if in.StartDate.IsZero() {
		add("tanggal mulai wajib diisi")
	} else if in.EndDate != nil && in.EndDate.Before(in.StartDate) {
		add("tanggal selesai tidak boleh sebelum tanggal mulai")
	}

	if in.ColorTag != nil && *in.ColorTag != "" && !validColorTag(*in.ColorTag) {
		add("warna harus berbentuk #rrggbb")
	}

	// Pola frekuensi divalidasi oleh domain, sehingga aturannya sama persis
	// dengan yang dipakai unit test dan konsisten dengan CHECK di database.
	params := domain.FrequencyParams{
		Pattern:          in.Frequency.Pattern,
		TimesPerDay:      in.Frequency.TimesPerDay,
		IntervalHours:    in.Frequency.IntervalHours,
		DaysOfWeek:       in.Frequency.DaysOfWeek,
		IntervalMinHours: in.Frequency.IntervalMinHours,
		CycleIntakeDays:  in.Frequency.CycleIntakeDays,
		CyclePauseDays:   in.Frequency.CyclePauseDays,
		CycleAnchor:      in.Frequency.CycleAnchor,
		CycleAnchorDOM:   in.Frequency.CycleAnchorDOM,
		PRNMaxPerDay:     in.Frequency.PRNMaxPerDay,
		StartTime:        in.Frequency.StartTime,
	}
	if err := params.Validate(); err != nil {
		add("%s", validationText(err))
	}

	// PRN tidak dijadwalkan, jadi jam minum untuk PRN tidak bermakna.
	// Membiarkannya ada akan menyesatkan: seolah obat ini punya jadwal.
	if in.Frequency.Pattern == domain.PatternPRN && len(in.Times) > 0 {
		add("obat PRN tidak memakai jam minum terjadwal")
	}

	// Pola selain PRN WAJIB punya minimal satu jam. Tanpa jam, aturan
	// frekuensinya benar tapi tidak akan pernah menghasilkan dosis.
	if in.Frequency.Pattern != domain.PatternPRN && len(in.Times) == 0 {
		add("wajib ada minimal satu jam minum")
	}

	seen := map[string]bool{}
	totalPerDay := 0
	for i, t := range in.Times {
		hh, mm, err := domain.ParseClock(t.TimeLocal)
		if err != nil {
			add("jam minum ke-%d: %s", i+1, err.Error())
			continue
		}
		normalized := domain.FormatClock(hh, mm)
		in.Times[i].TimeLocal = normalized

		weekdays := domain.EveryDay
		if t.Weekdays != nil {
			weekdays = *t.Weekdays
			if weekdays < 1 || weekdays > 127 {
				add("jam %s: weekdays harus antara 1 dan 127", normalized)
			}
		}

		// Kombinasi yang sama akan ditolak uq_times_med_time. Diperiksa di
		// sini supaya pesannya jelas dan tidak berupa pelanggaran constraint.
		key := fmt.Sprintf("%s|%d", normalized, weekdays)
		if seen[key] {
			add("jam %s untuk pola hari itu sudah ada", normalized)
		}
		seen[key] = true

		if t.Label != nil && !t.Label.Valid() {
			add("label jam %s tidak dikenal", normalized)
		}
		if weekdays == domain.EveryDay {
			totalPerDay++
		}
	}

	// Untuk pola daily, jumlah jam harus konsisten dengan times_per_day.
	// Kalau tidak, aturannya saling bertentangan dan hasilnya membingungkan:
	// times_per_day bilang 2, tapi hanya ada 1 jam, sehingga pengingat yang
	// diharapkan tidak pernah muncul.
	if in.Frequency.Pattern == domain.PatternDaily && in.Frequency.TimesPerDay != nil {
		if totalPerDay != *in.Frequency.TimesPerDay {
			add("pola daily dengan times_per_day=%d butuh %d jam harian, tapi ada %d",
				*in.Frequency.TimesPerDay, *in.Frequency.TimesPerDay, totalPerDay)
		}
	}

	if in.Stock != nil {
		if strings.TrimSpace(in.Stock.UnitCode) == "" {
			add("satuan stok wajib diisi")
		}
		if in.Stock.InitialQty < 0 {
			add("stok awal tidak boleh negatif")
		}
		if in.Stock.QtyPerDose <= 0 {
			add("pemakaian per dosis harus lebih dari nol")
		}
		if in.Stock.RefillThresholdQty != nil && *in.Stock.RefillThresholdQty < 0 {
			add("ambang restock tidak boleh negatif")
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("%w: %s", domain.ErrValidation, strings.Join(problems, "; "))
	}
	return nil
}

// createStock membuat baris stok beserta transaksi awalnya.
//
// Mengembalikan baris stok yang sudah dibaca ulang, supaya pemanggil bisa
// menyertakan saldo akhir di balasan. Pembacaan ulang diperlukan karena
// saldo TIDAK diisi langsung saat insert — trigger apply_stock_txn yang
// menaikkannya dari transaksi `initial`.
//
// Dua baris ditulis, dan keduanya diperlukan: medication_stock menyimpan
// saldo berjalan, sedangkan stock_transactions menyimpan riwayat. Saldo
// tidak boleh diubah tanpa transaksi, karena trigger yang menghitungnya.
func (s *Medications) createStock(ctx context.Context, q pgx.Tx, medicationID, ownerID string, in StockInput) (*domain.MedicationStock, error) {
	stock, err := s.db.CreateStock(ctx, q, postgres.CreateStockInput{
		MedicationID:       medicationID,
		OwnerID:            ownerID,
		UnitCode:           in.UnitCode,
		InitialQty:         in.InitialQty,
		QtyPerDose:         in.QtyPerDose,
		RefillThresholdQty: in.RefillThresholdQty,
		RefillReminder:     in.RefillReminder,
	})
	if err != nil {
		return nil, err
	}

	// Transaksi awal hanya dibuat bila memang ada stok. Stok nol tanpa
	// transaksi lebih jujur daripada transaksi bernilai nol.
	if in.InitialQty > 0 {
		if err := s.db.InsertStockTransaction(ctx, q, postgres.StockTxnInput{
			StockID:      stock.ID,
			MedicationID: medicationID,
			OwnerID:      ownerID,
			Kind:         domain.StockInitial,
			QtyDelta:     in.InitialQty,
			Note:         strPtr("stok awal"),
			RecordedBy:   &ownerID,
		}); err != nil {
			return nil, err
		}
	}

	// Baca ulang supaya current_qty memuat hasil perhitungan trigger.
	return s.db.StockByMedication(ctx, q, medicationID)
}

// List mengembalikan daftar obat milik user.
func (s *Medications) List(ctx context.Context, ownerID string, f ListFilter) ([]*domain.Medication, error) {
	return s.db.ListMedications(ctx, s.db.Pool(), postgres.ListMedicationsFilter{
		OwnerID: ownerID,
		Status:  f.Status,
		Query:   f.Query,
		Limit:   f.Limit,
		Cursor:  f.Cursor,
	})
}

// ListFilter adalah syarat pencarian daftar obat.
type ListFilter struct {
	Status string
	Query  string
	Limit  int
	Cursor *postgres.ListCursor
}

// Get mengambil satu obat lengkap beserta frekuensi, jam, dan stoknya.
//
// Ketiga bagian diambil berturut-turut dalam satu transaksi read-only supaya
// hasilnya konsisten. Tanpa transaksi, obat bisa berubah di antara dua query
// dan hasilnya jadi campuran dua keadaan.
func (s *Medications) Get(ctx context.Context, ownerID, medicationID string) (*domain.Medication, error) {
	var out *domain.Medication
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		med, err := s.db.MedicationByID(ctx, tx, medicationID, ownerID)
		if err != nil {
			return err
		}

		if freq, err := s.db.FrequencyByMedication(ctx, tx, medicationID); err == nil {
			med.Frequency = freq
		} else if !isNotFound(err) {
			return err
		}

		times, err := s.db.TimesByMedication(ctx, tx, medicationID)
		if err != nil {
			return err
		}
		med.Times = times

		if stock, err := s.db.StockByMedication(ctx, tx, medicationID); err == nil {
			med.Stock = stock
		} else if !isNotFound(err) {
			return err
		}

		out = med
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Update mengubah data obat.
//
// Jadwal TIDAK ikut diubah di sini; ia punya endpoint sendiri. Memisahkannya
// penting karena mengubah jadwal punya konsekuensi pada dosis yang sudah
// tercatat, dan itu perlu ditangani berbeda.
func (s *Medications) Update(ctx context.Context, ownerID, medicationID string, in UpdateMedicationInput, meta Meta) (*domain.Medication, error) {
	if in.IsEmpty() {
		return nil, fmt.Errorf("%w: tidak ada field yang diubah", domain.ErrValidation)
	}

	var problems []string
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if n == "" {
			problems = append(problems, "nama obat tidak boleh kosong")
		} else if len([]rune(n)) > 120 {
			problems = append(problems, "nama obat maksimal 120 karakter")
		}
		in.Name = &n
	}
	if in.DoseValue != nil && *in.DoseValue <= 0 {
		problems = append(problems, "nilai dosis harus lebih dari nol")
	}
	if in.MealRelation != nil && !in.MealRelation.Valid() {
		problems = append(problems, fmt.Sprintf("waktu makan %q tidak dikenal", *in.MealRelation))
	}
	if in.ColorTag != nil && *in.ColorTag != "" && !validColorTag(*in.ColorTag) {
		problems = append(problems, "warna harus berbentuk #rrggbb")
	}
	if in.StartTime != nil {
		hh, mm, err := domain.ParseClock(*in.StartTime)
		if err != nil {
			problems = append(problems, "jam mulai: "+err.Error())
		} else {
			formatted := domain.FormatClock(hh, mm)
			in.StartTime = &formatted
		}
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("%w: %s", domain.ErrValidation, strings.Join(problems, "; "))
	}

	var out *domain.Medication
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		before, err := s.db.MedicationByID(ctx, tx, medicationID, ownerID)
		if err != nil {
			return err
		}

		after, err := s.db.UpdateMedication(ctx, tx, medicationID, ownerID, postgres.UpdateMedicationInput{
			Name:           in.Name,
			DoseValue:      in.DoseValue,
			DoseUnitCode:   in.DoseUnitCode,
			StrengthNote:   in.StrengthNote,
			RouteCode:      in.RouteCode,
			DosageFormCode: in.DosageFormCode,
			MealRelation:   in.MealRelation,
			AsNeeded:       in.AsNeeded,
			MustFinish:     in.MustFinish,
			StartDate:      in.StartDate,
			StartTime:      in.StartTime,
			EndDate:        in.EndDate,
			ClearEndDate:   in.ClearEndDate,
			Notes:          in.Notes,
			NotesReminder:  in.NotesReminder,
			ColorTag:       in.ColorTag,
			IsActive:       in.IsActive,
		})
		if err != nil {
			return err
		}

		if err := s.db.WriteAudit(ctx, tx, postgres.AuditEntry{
			ActorID:      &ownerID,
			OwnerID:      &ownerID,
			ActorRole:    string(domain.ActorUser),
			Action:       postgres.ActionUpdate,
			ResourceType: postgres.ResourceMed,
			ResourceID:   &medicationID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			BeforeState:  medicationSnapshot(before),
			AfterState:   medicationSnapshot(after),
		}); err != nil {
			// Audit di sini tidak boleh menggagalkan perubahan yang sah.
			s.logger.WarnContext(ctx, "gagal menulis audit perubahan obat",
				slog.String("medication_id", medicationID),
				slog.String("error", err.Error()))
		}

		out = after
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Delete menghapus obat dengan soft-delete.
//
// Log kepatuhan dipertahankan. Obat yang sudah dihapus boleh didaftarkan
// ulang dengan nama sama, karena index uniknya partial.
func (s *Medications) Delete(ctx context.Context, ownerID, medicationID string, meta Meta) error {
	return s.db.WithTx(ctx, func(tx pgx.Tx) error {
		med, err := s.db.MedicationByID(ctx, tx, medicationID, ownerID)
		if err != nil {
			return err
		}

		if err := s.db.SoftDeleteMedication(ctx, tx, medicationID, ownerID); err != nil {
			return err
		}

		if err := s.db.WriteAudit(ctx, tx, postgres.AuditEntry{
			ActorID:      &ownerID,
			OwnerID:      &ownerID,
			ActorRole:    string(domain.ActorUser),
			Action:       postgres.ActionDelete,
			ResourceType: postgres.ResourceMed,
			ResourceID:   &medicationID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			BeforeState:  medicationSnapshot(med),
		}); err != nil {
			s.logger.WarnContext(ctx, "gagal menulis audit penghapusan obat",
				slog.String("medication_id", medicationID),
				slog.String("error", err.Error()))
		}
		return nil
	})
}

// medicationSnapshot mengambil bagian obat yang layak diaudit.
func medicationSnapshot(m *domain.Medication) map[string]any {
	if m == nil {
		return nil
	}
	return map[string]any{
		"name":       m.Name,
		"dose_value": m.DoseValue,
		"dose_unit":  m.DoseUnitCode,
		"route":      m.RouteCode,
		"is_active":  m.IsActive,
		"start_date": m.StartDate.Format(time.DateOnly),
	}
}

// validColorTag memeriksa format warna #rrggbb.
func validColorTag(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for i := 1; i < 7; i++ {
		c := s[i]
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
		if !isHex {
			return false
		}
	}
	return true
}

// validationText mengambil bagian yang berguna dari error validasi.
//
// domain.FrequencyParams.Validate membungkus pesannya dengan ErrValidation,
// dan yang berguna bagi user adalah bagian setelah titik dua.
func validationText(err error) string {
	msg := err.Error()
	if i := strings.LastIndex(msg, ": "); i >= 0 && i+2 < len(msg) {
		return msg[i+2:]
	}
	return msg
}

func strPtr(s string) *string { return &s }
