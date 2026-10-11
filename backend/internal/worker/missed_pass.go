package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	"github.com/frndly-bot/pantau-obat/backend/internal/repo/postgres"
)

// MissedPass menandai dosis yang lewat jam tanpa catatan.
//
// INI BAGIAN YANG PALING MUDAH TERLUPA, dan akibatnya serius: tanpa pass ini,
// dosis terlewat TIDAK PERNAH punya baris di medication_dose_events. Karena
// perhitungan kepatuhan membaca penyimpangan dari sana, seorang pasien yang
// berhenti minum obat sepenuhnya akan terlihat patuh sempurna — tidak ada
// catatan yang bisa dihitung sebagai terlewat.
//
// Pass ini dijalankan sekali sehari, lewat tengah malam waktu pemilik,
// sehingga hari yang baru selesai bisa dinilai utuh.
type MissedPass struct {
	db     *postgres.DB
	logger *slog.Logger

	// graceMinutes adalah tenggang setelah jam dosis sebelum dinyatakan
	// terlewat. Tanpa tenggang, dosis yang baru lewat satu menit sudah
	// dicap terlewat, padahal user mungkin sedang membuka aplikasinya.
	graceMinutes int
}

// NewMissedPass membuat pass harian.
func NewMissedPass(db *postgres.DB, logger *slog.Logger) *MissedPass {
	return &MissedPass{
		db:           db,
		logger:       logger,
		graceMinutes: 60,
	}
}

// Run menjalankan loop harian sampai context dibatalkan.
//
// Tidur panjang dipecah jadi potongan pendek supaya pembatalan context
// terasa cepat. Tidur 24 jam sekaligus akan membuat proses menolak berhenti
// sampai tidurnya selesai, dan itu membuat deploy terasa menggantung.
func (p *MissedPass) Run(ctx context.Context) error {
	// Jalankan sekali saat start: kalau proses mati semalaman, dosis
	// terlewat selama mati itu tetap harus tercatat.
	if err := p.RunOnce(ctx, time.Now()); err != nil {
		p.logger.ErrorContext(ctx, "missed pass awal gagal", slog.String("error", err.Error()))
	}

	for {
		// Target: 00:30 waktu server. Dipilih 30 menit setelah tengah malam
		// supaya transaksi yang masih berjalan di menit terakhir hari
		// sebelumnya sudah selesai.
		next := nextRunTime(time.Now(), 0, 30)
		wait := time.Until(next)

		select {
		case <-ctx.Done():
			return ErrWorkerStopped
		case <-time.After(wait):
			if err := p.RunOnce(ctx, time.Now()); err != nil {
				p.logger.ErrorContext(ctx, "missed pass gagal",
					slog.String("error", err.Error()))
			}
		}
	}
}

// nextRunTime menghitung waktu jalan berikutnya pada jam dan menit tertentu.
func nextRunTime(now time.Time, hour, minute int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// RunOnce menjalankan satu siklus pemeriksaan.
func (p *MissedPass) RunOnce(ctx context.Context, now time.Time) error {
	rows, err := p.db.ScanDueCandidates(ctx, p.db.Pool())
	if err != nil {
		return err
	}

	marked := 0
	failed := 0

	for i := range rows {
		r := &rows[i]
		if r.Frequency == nil || r.Time == nil {
			continue
		}

		n, err := p.markMissedForTime(ctx, r, now)
		if err != nil {
			failed++
			p.logger.WarnContext(ctx, "gagal menandai dosis terlewat",
				slog.String("medication_id", r.Medication.ID),
				slog.String("schedule_id", r.Time.ID),
				slog.String("error", err.Error()))
			continue
		}
		marked += n
	}

	if marked > 0 {
		p.logger.InfoContext(ctx, "dosis terlewat ditandai",
			slog.Int("jumlah", marked), slog.Int("gagal", failed))
	}
	return nil
}

// markMissedForTime menandai dosis terlewat untuk satu jam pada satu obat.
//
// Menyusuri 8 hari ke belakang. Rentang ini dipilih karena:
//
//   - Cukup menutupi proses yang mati beberapa hari.
//   - Tidak terlalu jauh sehingga menulis ribuan baris sekaligus pada
//     sistem yang baru pertama kali dijalankan.
//
// Idempotensi dijamin event_key sebagai PRIMARY KEY. Menjalankan pass ini dua
// kali tidak menggandakan apa pun, sehingga aman dijalankan ulang setelah
// crash.
func (p *MissedPass) markMissedForTime(ctx context.Context, r *postgres.DueScanRow, now time.Time) (int, error) {
	freq := r.Frequency
	if freq == nil || r.Time == nil {
		return 0, nil
	}

	tz := freq.ScheduleTimezone
	if tz == "" {
		tz = domain.DefaultTimezone
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return 0, nil
	}

	params := &domain.FrequencyParams{
		Pattern:          freq.Pattern,
		TimesPerDay:      freq.TimesPerDay,
		IntervalHours:    freq.IntervalHours,
		DaysOfWeek:       freq.DaysOfWeek,
		IntervalMinHours: freq.IntervalMinHours,
		CycleIntakeDays:  freq.CycleIntakeDays,
		CyclePauseDays:   freq.CyclePauseDays,
		CycleAnchor:      freq.CycleAnchor,
		CycleAnchorDOM:   freq.CycleAnchorDOM,
		PRNMaxPerDay:     freq.PRNMaxPerDay,
		StartTime:        freq.StartTime,
	}

	// Obat PRN tidak pernah terlewat. Obat "bila perlu" memang sering tidak
	// diminum, dan menandainya terlewat akan membuat semua obat PRN selalu
	// tampak gagal.
	if params.Pattern == domain.PatternPRN {
		return 0, nil
	}
	if r.Medication.AsNeeded {
		return 0, nil
	}

	hh, mm, err := domain.ParseClock(r.Time.TimeLocal)
	if err != nil {
		return 0, nil
	}

	localNow := now.In(loc)
	marked := 0

	for dayOffset := 1; dayOffset <= 8; dayOffset++ {
		day := localNow.AddDate(0, 0, -dayOffset)
		local := time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, loc)

		// Hari sebelum terapi dimulai bukan terlewat.
		if !withinTherapy(&r.Medication, local, loc) {
			continue
		}
		// Hari dalam masa jeda siklus bukan terlewat: memang tidak ada
		// dosis yang dijadwalkan.
		if params.IsPaused(local) {
			continue
		}
		if !params.MatchesOn(local) {
			continue
		}
		if !r.Time.MatchesWeekday(local.Weekday()) {
			continue
		}

		dueAt := time.Date(day.Year(), day.Month(), day.Day(), hh, mm, 0, 0, loc)
		cutoff := dueAt.Add(time.Duration(p.graceMinutes) * time.Minute)
		if cutoff.After(localNow) {
			// Belum lewat tenggang. Bisa terjadi pada hari yang baru saja
			// berganti; dosis malamnya belum tentu terlewat.
			continue
		}

		slotDate := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)

		// Kalau sudah ada log untuk dosis ini, bukan terlewat.
		existing, err := p.db.LogByOccurrence(ctx, p.db.Pool(),
			r.Medication.ID, r.Time.ID, slotDate, hh, mm)
		if err != nil && !isNotFound(err) {
			return marked, err
		}
		if existing != nil {
			continue
		}

		key := domain.OccurrenceKey(r.Time.ID, slotDate, hh, mm)
		if err := p.db.InsertDoseEvent(ctx, p.db.Pool(), postgres.DoseEventInput{
			EventKey:        key,
			OwnerID:         r.Medication.OwnerID,
			MedicationID:    r.Medication.ID,
			ScheduleID:      &r.Time.ID,
			EventDate:       slotDate,
			ScheduledHour:   hh,
			ScheduledMinute: mm,
			ScheduledTZ:     tz,
			Kind:            domain.EventMissed,
			Reason:          "tidak dicatat sampai batas waktu",
		}); err != nil {
			return marked, err
		}
		marked++
	}

	return marked, nil
}

// isNotFound melaporkan apakah error berarti data tidak ada.
//
// Memakai errors.Is, bukan pembandingan teks: sentinel domain bisa
// terbungkus beberapa lapis oleh fmt.Errorf("%w"), dan errors.Is yang
// menelusuri seluruh rantainya. Mencocokkan potongan teks akan gagal begitu
// ada yang membungkusnya dengan cara berbeda.
func isNotFound(err error) bool {
	return errors.Is(err, domain.ErrNotFound)
}
