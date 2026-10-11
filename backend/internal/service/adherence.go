package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	"github.com/frndly-bot/pantau-obat/backend/internal/repo/postgres"
)

// Adherence menghitung kepatuhan.
//
// PENYEBUT dihitung dari ATURAN jadwal lewat mesin aturan domain, bukan dari
// baris log yang ada. Ini perbedaan yang menentukan: kalau penyebutnya berasal
// dari log, pasien yang tidak pernah membuka aplikasi akan selalu tampak
// patuh 100%, karena tidak ada catatan yang bisa dihitung sebagai terlewat.
type Adherence struct {
	db     *postgres.DB
	logger *slog.Logger
}

// NewAdherence membuat layanan kepatuhan.
func NewAdherence(db *postgres.DB, logger *slog.Logger) *Adherence {
	return &Adherence{db: db, logger: logger}
}

// Range adalah rentang tanggal laporan.
type Range struct {
	From time.Time
	To   time.Time
}

// Summary menghitung ringkasan kepatuhan seluruh obat milik user.
func (s *Adherence) Summary(ctx context.Context, ownerID string, r Range) (*domain.AdherenceSummary, error) {
	byMed, err := s.ByMedication(ctx, ownerID, r)
	if err != nil {
		return nil, err
	}

	out := &domain.AdherenceSummary{From: r.From, To: r.To}
	for _, m := range byMed {
		out.ScheduledDoses += m.ScheduledDoses
		out.TakenDoses += m.TakenDoses
		out.LateDoses += m.LateDoses
		out.SkippedDoses += m.SkippedDoses
		out.MissedDoses += m.MissedDoses
		out.PendingDoses += m.PendingDoses
	}
	out.Ratio = domain.ComputeRatio(out.TakenDoses, out.ScheduledDoses, out.PendingDoses)
	return out, nil
}

// ByMedication menghitung kepatuhan per obat.
//
// Seluruhnya dikerjakan dalam satu transaksi read-only supaya obat, jam, dan
// log dibaca dari keadaan yang konsisten. Tanpa itu, obat yang dihapus di
// tengah perhitungan bisa membuat penyebut dan pembilang berasal dari dua
// keadaan berbeda.
func (s *Adherence) ByMedication(ctx context.Context, ownerID string, r Range) ([]domain.AdherenceByMedication, error) {
	if r.To.Before(r.From) {
		return nil, fmt.Errorf("%w: tanggal akhir tidak boleh sebelum tanggal awal", domain.ErrValidation)
	}

	var out []domain.AdherenceByMedication
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		u, err := s.db.UserByID(ctx, tx, ownerID)
		if err != nil {
			return err
		}
		loc, err := time.LoadLocation(u.Timezone)
		if err != nil {
			loc = time.FixedZone("WIB", 7*3600)
		}

		meds, err := s.db.ListMedicationsWithSchedule(ctx, tx, ownerID)
		if err != nil {
			return err
		}

		for i := range meds {
			m := &meds[i]

			params := toFrequencyParams(m.Frequency)

			// Obat tanpa frekuensi atau tanpa jam tidak punya dosis
			// terjadwal, sehingga tidak ada yang bisa dinilai. Dilewatkan,
			// bukan dihitung sebagai 0%, karena 0% berarti "semua terlewat".
			if params == nil || params.Pattern == domain.PatternPRN || len(m.Times) == 0 {
				continue
			}

			scheduled, pending := domain.ScheduledDosesInRange(
				&m.Medication, params, m.Times, r.From, r.To, loc, time.Now(), false)
			if scheduled == 0 {
				continue
			}

			counts, err := s.db.CountLogsForRange(ctx, tx, m.ID, r.From, r.To)
			if err != nil {
				return err
			}

			taken := counts["taken"] + counts["late"]
			missed, err := s.db.CountMissedForRange(ctx, tx, m.ID, r.From, r.To)
			if err != nil {
				return err
			}

			late := counts["late"]
			skipped := counts["skipped"]

			// Pembilang dibatasi tidak melebihi penyebut. Selisih bisa
			// muncul dari dosis PRN yang tercatat, atau dari koreksi yang
			// menambah catatan di luar rentang aturan.
			if taken+pending > scheduled {
				taken = scheduled - pending
				if taken < 0 {
					taken = 0
				}
			}

			out = append(out, domain.AdherenceByMedication{
				MedicationID:   m.ID,
				MedicationName: m.Name,
				ColorTag:       m.ColorTag,
				ScheduledDoses: scheduled,
				TakenDoses:     taken,
				LateDoses:      late,
				SkippedDoses:   skipped,
				MissedDoses:    missed,
				PendingDoses:   pending,
				Ratio:          domain.ComputeRatio(taken, scheduled, pending),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Daily menghitung kepatuhan harian.
//
// Memakai view v_adherence_daily untuk sisi CATATAN (berapa yang tercatat,
// berapa yang terlewat), lalu melengkapinya dengan penyebut dari aturan.
//
// Kenapa tidak sepenuhnya dari view: view tidak bisa tahu berapa dosis yang
// seharusnya ada, karena itu bergantung pada pola frekuensi. View hanya tahu
// apa yang sudah terjadi.
func (s *Adherence) Daily(ctx context.Context, ownerID string, r Range) ([]DailyAdherence, error) {
	if r.To.Before(r.From) {
		return nil, fmt.Errorf("%w: tanggal akhir tidak boleh sebelum tanggal awal", domain.ErrValidation)
	}

	// Batasi rentang supaya tidak ada yang meminta laporan 10 tahun sekaligus.
	const maxDays = 366
	if r.To.Sub(r.From) > maxDays*24*time.Hour {
		return nil, fmt.Errorf("%w: rentang laporan maksimal %d hari", domain.ErrValidation, maxDays)
	}

	byMed, err := s.ByMedication(ctx, ownerID, r)
	if err != nil {
		return nil, err
	}

	// Baris per hari disusun dari catatan yang ada, lalu penyebut per hari
	// dihitung dari aturan. Hasilnya digabung supaya satu hari hanya muncul
	// sekali dengan angka lengkap.
	daily, err := s.db.AdherenceDailyRows(ctx, s.db.Pool(), ownerID, r.From, r.To)
	if err != nil {
		return nil, err
	}

	// Peta penyebut per hari dari perhitungan aturan.
	denominator, err := s.dailyDenominators(ctx, ownerID, r, byMed)
	if err != nil {
		return nil, err
	}

	seen := map[string]*DailyAdherence{}
	for _, row := range daily {
		key := row.SlotDate.Format(time.DateOnly)
		item, ok := seen[key]
		if !ok {
			item = &DailyAdherence{Date: row.SlotDate}
			seen[key] = item
		}
		item.TakenDoses += row.TakenDoses
		item.SkippedDoses += row.SkippedDoses
		item.MissedDoses += row.MissedDoses
		item.LoggedDoses += row.LoggedDoses
	}

	// Hari yang punya dosis terjadwal tapi belum punya catatan tetap harus
	// muncul. Tanpa ini, hari yang benar-benar terlewat justru hilang dari
	// laporan — persis hari yang paling perlu terlihat.
	for dateKey, d := range denominator {
		item, ok := seen[dateKey]
		if !ok {
			parsed, err := time.ParseInLocation(time.DateOnly, dateKey, time.UTC)
			if err != nil {
				continue
			}
			item = &DailyAdherence{Date: parsed}
			seen[dateKey] = item
		}
		item.ScheduledDoses = d.scheduled
		item.PendingDoses = d.pending
		item.Ratio = domain.ComputeRatio(item.TakenDoses, d.scheduled, d.pending)
	}

	out := make([]DailyAdherence, 0, len(seen))
	for _, item := range seen {
		out = append(out, *item)
	}
	sortDailyDesc(out)
	return out, nil
}

// DailyAdherence adalah kepatuhan pada satu hari.
type DailyAdherence struct {
	Date time.Time

	ScheduledDoses int
	TakenDoses     int
	SkippedDoses   int
	MissedDoses    int
	PendingDoses   int
	LoggedDoses    int

	Ratio *float64
}

// denominatorPerDay adalah penyebut hasil perhitungan aturan untuk satu hari.
type denominatorPerDay struct {
	scheduled int
	pending   int
}

// dailyDenominators menghitung penyebut per hari dari aturan jadwal.
func (s *Adherence) dailyDenominators(
	ctx context.Context, ownerID string, r Range, byMed []domain.AdherenceByMedication,
) (map[string]denominatorPerDay, error) {
	out := map[string]denominatorPerDay{}

	// Obat yang ikut dihitung: yang punya dosis terjadwal pada rentang ini.
	// Obat PRN tidak masuk, dan itu memang benar: obat "bila perlu" tidak
	// punya jadwal, jadi tidak ada dosis yang bisa dinilai terlewat.
	active := map[string]bool{}
	for _, m := range byMed {
		if m.ScheduledDoses > 0 {
			active[m.MedicationID] = true
		}
	}
	if len(active) == 0 {
		return out, nil
	}

	var meds []postgres.MedicationWithSchedule
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		u, err := s.db.UserByID(ctx, tx, ownerID)
		if err != nil {
			return err
		}
		loc, err := time.LoadLocation(u.Timezone)
		if err != nil {
			loc = time.FixedZone("WIB", 7*3600)
		}

		all, err := s.db.ListMedicationsWithSchedule(ctx, tx, ownerID)
		if err != nil {
			return err
		}
		meds = all
		_ = loc
		return nil
	})
	if err != nil {
		return nil, err
	}

	u, err := s.db.UserByID(ctx, s.db.Pool(), ownerID)
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(u.Timezone)
	if err != nil {
		loc = time.FixedZone("WIB", 7*3600)
	}

	now := time.Now()
	for i := range meds {
		m := &meds[i]
		if !active[m.ID] {
			continue
		}
		params := toFrequencyParams(m.Frequency)
		if params == nil {
			continue
		}

		for d := r.From; !d.After(r.To); d = d.AddDate(0, 0, 1) {
			day := time.Date(d.Year(), d.Month(), d.Day(), 12, 0, 0, 0, loc)
			sched, pend := domain.ScheduledDosesInRange(
				&m.Medication, params, m.Times, day, day, loc, now, false)
			if sched == 0 {
				continue
			}
			key := d.Format(time.DateOnly)
			cur := out[key]
			cur.scheduled += sched
			cur.pending += pend
			out[key] = cur
		}
	}

	return out, nil
}

// toFrequencyParams memetakan frekuensi dari repo ke parameter mesin aturan.
func toFrequencyParams(f *domain.MedicationFrequency) *domain.FrequencyParams {
	if f == nil {
		return nil
	}
	return &domain.FrequencyParams{
		Pattern:          f.Pattern,
		TimesPerDay:      f.TimesPerDay,
		IntervalHours:    f.IntervalHours,
		DaysOfWeek:       f.DaysOfWeek,
		IntervalMinHours: f.IntervalMinHours,
		CycleIntakeDays:  f.CycleIntakeDays,
		CyclePauseDays:   f.CyclePauseDays,
		CycleAnchor:      f.CycleAnchor,
		CycleAnchorDOM:   f.CycleAnchorDOM,
		PRNMaxPerDay:     f.PRNMaxPerDay,
		StartTime:        f.StartTime,
	}
}

// sortDailyDesc mengurutkan hasil harian dari terbaru ke terlama.
func sortDailyDesc(items []DailyAdherence) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].Date.After(items[j-1].Date); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}

// TherapySpan adalah rentang tanggal saat user punya dosis terjadwal.
//
// Dipakai untuk membatasi rentang laporan bawaan, supaya laporan tidak
// menghitung hari-hari di luar masa terapi.
type TherapySpan struct {
	Earliest time.Time
	Latest   time.Time

	// Ended menandakan SEMUA obat terjadwal milik user sudah melewati
	// tanggal selesainya. Bila true, tidak ada lagi dosis yang seharusnya
	// diminum, sehingga rentang laporan boleh dipotong di akhir terapi.
	//
	// Bila false, setidaknya ada satu terapi yang masih berjalan. Dalam
	// kasus itu rentang TIDAK dipotong, supaya pasien yang berhenti
	// mencatat tetap mendapat rasio rendah — dan itu memang sinyal yang
	// dibutuhkan caregiver.
	Ended bool
}

// TherapySpan menghitung kapan terapi user mulai dan berakhir, memakai
// aturan frekuensi yang sudah dihitung.
//
// Mengembalikan nil bila user tidak punya obat dengan dosis terjadwal sama
// sekali. Itu bukan error: user yang belum menambah obat memang tidak punya
// rentang terapi.
//
// Batas atas dibatasi `today`: kalau terapi berjalan tanpa batas (end_date
// NULL), rentangnya tidak boleh melebar ke masa depan. Menghitung hari yang
// belum terjadi akan membuat penyebut membengkak dan rasio merosot tanpa
// sebab yang nyata.
func (s *Adherence) TherapySpan(ctx context.Context, ownerID string, today time.Time) (*TherapySpan, error) {
	var span *TherapySpan

	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		meds, err := s.db.ListMedicationsWithSchedule(ctx, tx, ownerID)
		if err != nil {
			return err
		}

		// running menandai apakah ADA terapi yang masih berjalan. Selama
		// minimal satu masih berjalan, rentang laporan tidak boleh dipotong:
		// pasien yang berhenti mencatat harus tetap terlihat tidak patuh.
		running := false
		anyEnded := false

		for i := range meds {
			m := &meds[i]
			params := toFrequencyParams(m.Frequency)
			if params == nil || params.Pattern == domain.PatternPRN || len(m.Times) == 0 {
				continue
			}

			start := m.StartDate
			if start.After(today) {
				// Terapi belum mulai. Tidak menyumbang rentang.
				continue
			}

			end := today
			ended := false
			if m.EndDate != nil {
				if m.EndDate.Before(today) {
					end = *m.EndDate
					ended = true
				}
				// EndDate setelah hari ini berarti masih berjalan.
			}
			if end.Before(start) {
				continue
			}

			if ended {
				anyEnded = true
			} else {
				running = true
			}

			if span == nil {
				span = &TherapySpan{Earliest: start, Latest: end}
				continue
			}
			if start.Before(span.Earliest) {
				span.Earliest = start
			}
			if end.After(span.Latest) {
				span.Latest = end
			}
		}

		if span != nil {
			// Ended hanya true bila SEMUA terapi sudah selesai. Kalau ada
			// satu saja yang berjalan, rentangnya tidak boleh dipotong.
			span.Ended = anyEnded && !running
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return span, nil
}
