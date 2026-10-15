package worker

import (
	"context"
	"errors"
	"fmt"
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
	// queue dipakai membuat eskalasi ke caregiver. Boleh nil: pass tetap
	// berguna tanpa eskalasi, dan lebih baik dosis terlewat tercatat
	// daripada seluruh pass gagal karena antrean tidak tersedia.
	queue *DBQueue

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

// WithQueue memasang antrean supaya eskalasi caregiver bisa dibuat.
func (p *MissedPass) WithQueue(q *DBQueue) *MissedPass {
	p.queue = q
	return p
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

	// Eskalasi dijalankan SETIAP kali pass berjalan, bukan hanya saat ada
	// yang baru ditandai.
	//
	// Alasannya: pass bisa berjalan berkali-kali dalam sehari (saat start,
	// lalu tengah malam), dan justru pada saat start itulah rangkaian
	// terlewat selama proses mati baru terlihat. Menggantungkannya pada
	// `marked > 0` akan melewatkan kasus itu.
	if err := p.escalate(ctx, now); err != nil {
		// Kegagalan eskalasi tidak menggagalkan pass: penandaan dosis
		// terlewat sudah tersimpan, dan itu yang lebih penting untuk
		// perhitungan kepatuhan.
		p.logger.ErrorContext(ctx, "eskalasi caregiver gagal", slog.String("error", err.Error()))
	}
	return nil
}

// escalate memberi tahu caregiver pasien yang melewatkan beberapa hari
// berturut-turut.
//
// INI JANJI YANG BELUM PERNAH DIJALANKAN. notification-v2 bagian 9 menulis
// "kalau streak >= N (default 3): buat job missed_escalation untuk
// caregiver", dan `MissedEscalationThreshold = 3` sudah dideklarasikan di
// domain — tetapi tidak ada satu baris pun yang membuat job itu. Akibatnya
// caregiver baru tahu ada masalah kalau ia membuka aplikasi sendiri,
// padahal justru merekalah yang bisa menindak.
//
// Batas "maksimal 1 kali per hari per pasien" (bagian 12) ditegakkan lewat
// dedup_key: kunci yang sama tidak akan membuat baris kedua.
func (p *MissedPass) escalate(ctx context.Context, now time.Time) error {
	if p.queue == nil {
		return nil
	}

	owners, err := p.db.OwnersWithMissedDoses(ctx, p.db.Pool(), now)
	if err != nil {
		return err
	}

	dikirim := 0
	for _, ownerID := range owners {
		// Tanggal acuan memakai zona pasien: "hari ini" bagi caregiver yang
		// memantau dari kota lain harus sama dengan hari pasiennya.
		loc, err := time.LoadLocation(p.db.TimezoneOf(ctx, p.db.Pool(), ownerID))
		if err != nil {
			loc = time.UTC
		}
		hari := now.In(loc)
		slotDate := time.Date(hari.Year(), hari.Month(), hari.Day(), 0, 0, 0, 0, loc)

		streak, err := p.db.MissedStreak(ctx, p.db.Pool(), ownerID, slotDate.Format(time.DateOnly))
		if err != nil {
			p.logger.WarnContext(ctx, "gagal menghitung streak terlewat",
				slog.String("owner_id", ownerID), slog.String("error", err.Error()))
			continue
		}
		if streak < domain.MissedEscalationThreshold {
			continue
		}

		// Batas 1× per hari per pasien.
		sudah, err := p.db.EscalationAlreadySentToday(ctx, p.db.Pool(), ownerID, slotDate.Format(time.DateOnly))
		if err != nil {
			p.logger.WarnContext(ctx, "gagal memeriksa eskalasi hari ini",
				slog.String("owner_id", ownerID), slog.String("error", err.Error()))
			continue
		}
		if sudah {
			continue
		}

		caregivers, err := p.db.AcceptedCaregivers(ctx, p.db.Pool(), ownerID)
		if err != nil {
			p.logger.WarnContext(ctx, "gagal mencari caregiver",
				slog.String("owner_id", ownerID), slog.String("error", err.Error()))
			continue
		}
		if len(caregivers) == 0 {
			// Tidak ada yang bisa diberi tahu. Bukan kegagalan — pasien
			// memang belum mengundang siapa pun.
			continue
		}

		for _, cg := range caregivers {
			ok, err := p.queue.EnqueueBump(ctx, p.queue.txOrPool(), EnqueueInput{
				Kind:    domain.BumpEscalation,
				OwnerID: cg.CaregiverID, // PENERIMA: caregiver
				// PEMILIK DATA: pasien. Dipisah supaya pesannya bisa
				// menyebut siapa yang dimaksud tanpa menebak dari konteks.
				SubjectOwnerID: &ownerID,
				ScheduledAt:    now,
				SlotDate:       slotDate,
				// Kunci memuat tanggal, jadi satu eskalasi per pasien per
				// hari. Kunci yang sama pada pass berikutnya tidak membuat
				// baris kedua.
				DedupKey: fmt.Sprintf("escalation:%s:%s:%s",
					ownerID, cg.CaregiverID, slotDate.Format(time.DateOnly)),
			})
			if err != nil {
				p.logger.WarnContext(ctx, "gagal membuat eskalasi",
					slog.String("caregiver_id", cg.CaregiverID),
					slog.String("error", err.Error()))
				continue
			}
			if ok {
				dikirim++
			}
		}
	}

	if dikirim > 0 {
		p.logger.InfoContext(ctx, "eskalasi caregiver dibuat", slog.Int("jumlah", dikirim))
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
