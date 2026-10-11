package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	"github.com/frndly-bot/pantau-obat/backend/internal/repo/postgres"
)

// Scheduler menghitung dosis yang jatuh tempo lalu mengisi bump_queue.
//
// Ini pengganti "scan tabel slot": karena jadwal disimpan sebagai aturan,
// scheduler menghitung sendiri dosis mana yang jatuh pada menit ini. Karena
// itu pula loop ini boleh berjalan tiap menit tanpa menghasilkan baris
// berlebih — dedup_key menjaga idempotensinya.
type Scheduler struct {
	db        *postgres.DB
	queue     *DBQueue
	logger    *slog.Logger
	interval  time.Duration
	lookahead time.Duration
}

// NewScheduler membuat scheduler.
//
// lookahead adalah seberapa jauh ke depan dosis dicari. Nilai 15 menit
// diambil dari TRD bagian 7.1: worker memindai dosis jatuh tempo 15 menit
// ke depan. Dengan begitu pengingat sudah siap sebelum waktunya, dan
// dispatcher tinggal mengirim saat scheduled_at-nya tiba.
func NewScheduler(db *postgres.DB, queue *DBQueue, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		db:        db,
		queue:     queue,
		logger:    logger,
		interval:  time.Minute,
		lookahead: 15 * time.Minute,
	}
}

// Run menjalankan loop sampai context dibatalkan.
func (s *Scheduler) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// Catch-up saat start: kalau proses mati beberapa jam, dosis yang
	// terlewat selama mati tidak boleh hilang tanpa jejak. Tanpa ini,
	// jadwal "at rule" kehilangan pengingat begitu saja.
	if err := s.RunOnce(ctx, time.Now()); err != nil {
		s.logger.ErrorContext(ctx, "scheduler catch-up gagal", slog.String("error", err.Error()))
	}

	for {
		select {
		case <-ctx.Done():
			return ErrWorkerStopped
		case <-ticker.C:
			if err := s.RunOnce(ctx, time.Now()); err != nil {
				// Kegagalan satu siklus tidak menghentikan loop. Kalau
				// berhenti, satu error sementara akan mematikan seluruh
				// pengingat sampai proses di-restart.
				s.logger.ErrorContext(ctx, "scheduler gagal satu siklus",
					slog.String("error", err.Error()))
			}
		}
	}
}

// RunOnce menjalankan satu siklus pemindaian.
func (s *Scheduler) RunOnce(ctx context.Context, now time.Time) error {
	rows, err := s.db.ScanDueCandidates(ctx, s.db.Pool())
	if err != nil {
		return err
	}

	// Obat dikelompokkan per (medication, jam) supaya satu aturan hanya
	// dihitung sekali, bukan sekali per baris gabungan.
	type key struct {
		medicationID string
		timeID       string
	}
	grouped := map[key]*postgres.DueScanRow{}
	for i := range rows {
		r := &rows[i]
		if r.Frequency == nil || r.Time == nil {
			continue
		}
		k := key{r.Medication.ID, r.Time.ID}
		if _, ok := grouped[k]; !ok {
			grouped[k] = r
		}
	}

	enqueued := 0
	for _, r := range grouped {
		n, err := s.enqueueForMedication(ctx, r, now)
		if err != nil {
			// Satu obat yang bermasalah tidak boleh menghentikan sisanya.
			s.logger.WarnContext(ctx, "gagal menyiapkan pengingat untuk satu obat",
				slog.String("medication_id", r.Medication.ID),
				slog.String("error", err.Error()))
			continue
		}
		enqueued += n
	}

	if enqueued > 0 {
		s.logger.InfoContext(ctx, "pengingat diantrekan", slog.Int("jumlah", enqueued))
	}
	return nil
}

// enqueueForMedication menyiapkan pengingat untuk satu jam pada satu obat.
//
// Menyusuri dua hari: hari ini dan besok. Alasannya bukan kemalasan: pada
// pukul 23:50, dosis pukul 00:10 besok sudah masuk jendela 15 menit, dan
// tanpa menyusuri besok pengingat itu tidak akan pernah dibuat.
func (s *Scheduler) enqueueForMedication(ctx context.Context, r *postgres.DueScanRow, now time.Time) (int, error) {
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
		// Zona tak dikenal: dilewati, bukan ditebak. Menebak UTC akan
		// menggeser pengingat 7 jam untuk pengguna WIB.
		s.logger.WarnContext(ctx, "zona jadwal tidak dikenal, pengingat dilewati",
			slog.String("medication_id", r.Medication.ID),
			slog.String("timezone", tz))
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

	hh, mm, err := domain.ParseClock(r.Time.TimeLocal)
	if err != nil {
		return 0, nil
	}

	enqueued := 0
	localNow := now.In(loc)
	deadline := localNow.Add(s.lookahead)

	for dayOffset := 0; dayOffset <= 1; dayOffset++ {
		day := localNow.AddDate(0, 0, dayOffset)
		local := time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, loc)

		if !withinTherapy(&r.Medication, local, loc) {
			continue
		}
		if !params.MatchesOn(local) {
			continue
		}
		if !r.Time.MatchesWeekday(local.Weekday()) {
			continue
		}

		dueAt := time.Date(day.Year(), day.Month(), day.Day(), hh, mm, 0, 0, loc)

		// Hanya dosis dalam jendela lookahead yang diantrekan. Dosis yang
		// sudah lewat jauh tidak diantrekan di sini: yang mengurus dosis
		// terlewat adalah missed_pass, bukan scheduler. Tanpa pemisahan ini,
		// pengingat untuk dosis berhari-hari lalu akan menumpuk dan dikirim
		// sekaligus.
		if dueAt.After(deadline) {
			continue
		}
		if dueAt.Before(localNow.Add(-time.Minute)) {
			continue
		}
		if dueAt.Before(r.Medication.StartDate.In(loc)) {
			continue
		}

		slotDate := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)

		ok, err := s.queue.EnqueueBump(ctx, s.queue.txOrPool(), EnqueueInput{
			Kind:         domain.BumpDose,
			OwnerID:      r.Medication.OwnerID,
			MedicationID: &r.Medication.ID,
			ScheduleID:   &r.Time.ID,
			ScheduledAt:  dueAt,
			SlotDate:     slotDate,
			DedupKey:     domain.OccurrenceKey(r.Time.ID, slotDate, hh, mm),
		})
		if err != nil {
			return enqueued, err
		}
		if ok {
			enqueued++
		}
	}

	return enqueued, nil
}

// withinTherapy melaporkan apakah tanggal berada dalam rentang terapi obat.
func withinTherapy(m *domain.Medication, local time.Time, loc *time.Location) bool {
	start := m.StartDate.In(loc)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	startDay := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)

	if day.Before(startDay) {
		return false
	}
	if m.EndDate != nil {
		end := m.EndDate.In(loc)
		endDay := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, loc)
		if day.After(endDay) {
			return false
		}
	}
	return true
}
