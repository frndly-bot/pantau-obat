package worker

import (
	"testing"
	"time"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// Uji ini menguji ATURAN missed_pass secara murni, tanpa database.
//
// Caranya: logika keputusan "apakah dosis ini terlewat" diekstrak ke fungsi
// murni (shouldMarkMissed), sehingga bisa diuji dengan waktu apa pun.
// Memanggil RunOnce yang menyentuh database tidak bisa diuji deterministik,
// karena hasilnya bergantung pada waktu sistem saat uji dijalankan.
//
// Yang diuji di sini adalah bagian yang paling mudah salah: penyaringan
// hari, tenggang waktu, dan kaitannya dengan rentang terapi serta masa jeda
// siklus.

// missedContext adalah input keputusan untuk satu hari.
type missedContext struct {
	local    time.Time
	params   *domain.FrequencyParams
	medStart time.Time
	medEnd   *time.Time
	weekday  time.Weekday
	timeMask int
	now      time.Time
	hour     int
	minute   int
	grace    int
}

// shouldMarkMissed memutuskan apakah dosis pada satu hari dianggap terlewat.
//
// INI SALINAN dari keputusan yang ada di markMissedForTime, dipakai untuk
// menguji aturannya. Disalin, bukan dipanggil, karena versi aslinya
// terikat pada pembacaan database.
func shouldMarkMissed(c missedContext) bool {
	med := &domain.Medication{StartDate: c.medStart, EndDate: c.medEnd}
	if !withinTherapy(med, c.local, c.local.Location()) {
		return false
	}
	if c.params.IsPaused(c.local) {
		return false
	}
	if !c.params.MatchesOn(c.local) {
		return false
	}
	if c.timeMask&bitFor(c.weekday) == 0 {
		return false
	}
	dueAt := time.Date(c.local.Year(), c.local.Month(), c.local.Day(),
		c.hour, c.minute, 0, 0, c.local.Location())
	if dueAt.Add(time.Duration(c.grace) * time.Minute).After(c.now) {
		return false
	}
	return true
}

func bitFor(w time.Weekday) int {
	switch w {
	case time.Monday:
		return 1
	case time.Tuesday:
		return 2
	case time.Wednesday:
		return 4
	case time.Thursday:
		return 8
	case time.Friday:
		return 16
	case time.Saturday:
		return 32
	case time.Sunday:
		return 64
	}
	return 0
}

func localDay(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 12, 0, 0, 0, testLoc)
}

var testLoc = time.FixedZone("WIB", 7*3600)

func dailyParams() *domain.FrequencyParams {
	n := 1
	return &domain.FrequencyParams{Pattern: domain.PatternDaily, TimesPerDay: &n}
}

func TestShouldMarkMissed_NormalDayIsMarked(t *testing.T) {
	// Jadwal 08:00, sekarang 10:00. Tenggang 60 menit sudah lewat,
	// tidak ada log -> terlewat.
	day := localDay(2026, time.October, 9)
	got := shouldMarkMissed(missedContext{
		local:    day,
		params:   dailyParams(),
		medStart: localDay(2026, time.October, 1),
		weekday:  day.Weekday(),
		timeMask: domain.EveryDay,
		now:      time.Date(2026, time.October, 9, 10, 0, 0, 0, testLoc),
		hour:     8, minute: 0, grace: 60,
	})
	if !got {
		t.Fatal("dosis 08:00 pada pukul 10:00 harus dianggap terlewat")
	}
}

func TestShouldMarkMissed_WithinGraceNotMarked(t *testing.T) {
	// Jadwal 08:00, sekarang 08:30, tenggang 60 menit. Belum lewat tenggang,
	// jadi BELUM boleh dicap terlewat: user mungkin sedang membukanya.
	day := localDay(2026, time.October, 9)
	got := shouldMarkMissed(missedContext{
		local:    day,
		params:   dailyParams(),
		medStart: localDay(2026, time.October, 1),
		weekday:  day.Weekday(),
		timeMask: domain.EveryDay,
		now:      time.Date(2026, time.October, 9, 8, 30, 0, 0, testLoc),
		hour:     8, minute: 0, grace: 60,
	})
	if got {
		t.Fatal("masih dalam tenggang, belum boleh dicap terlewat")
	}
}

func TestShouldMarkMissed_BeforeTherapyStartNotMarked(t *testing.T) {
	// Hari sebelum terapi dimulai bukan terlewat: memang belum ada jadwal.
	day := localDay(2026, time.September, 28)
	got := shouldMarkMissed(missedContext{
		local:    day,
		params:   dailyParams(),
		medStart: localDay(2026, time.October, 1),
		weekday:  day.Weekday(),
		timeMask: domain.EveryDay,
		now:      time.Date(2026, time.September, 28, 10, 0, 0, 0, testLoc),
		hour:     8, minute: 0, grace: 60,
	})
	if got {
		t.Fatal("sebelum terapi mulai tidak boleh dicap terlewat")
	}
}

func TestShouldMarkMissed_AfterTherapyEndNotMarked(t *testing.T) {
	// Hari setelah terapi berakhir bukan terlewat. Ini yang mencegah pasien
	// yang sudah menuntaskan antibiotiknya dicap tidak patuh selamanya.
	end := localDay(2026, time.October, 3)
	day := localDay(2026, time.October, 5)
	got := shouldMarkMissed(missedContext{
		local:    day,
		params:   dailyParams(),
		medStart: localDay(2026, time.October, 1),
		medEnd:   &end,
		weekday:  day.Weekday(),
		timeMask: domain.EveryDay,
		now:      time.Date(2026, time.October, 5, 10, 0, 0, 0, testLoc),
		hour:     8, minute: 0, grace: 60,
	})
	if got {
		t.Fatal("setelah terapi berakhir tidak boleh dicap terlewat")
	}
}

func TestShouldMarkMissed_PauseDayNotMarked(t *testing.T) {
	// Hari dalam masa jeda siklus bukan terlewat: memang tidak ada dosis
	// yang dijadwalkan. Tanpa penyaringan ini, pasien dengan terapi siklus
	// akan dicap tidak patuh padahal sedang masa jeda.
	intake, pause := 3, 2
	anchor := domain.AnchorFromStart
	params := &domain.FrequencyParams{
		Pattern:         domain.PatternMonthlyCycle,
		CycleIntakeDays: &intake,
		CyclePauseDays:  &pause,
		CycleAnchor:     &anchor,
	}

	// Siklus mulai 1 Okt: 1,2,3 minum; 4,5 jeda.
	pauseDay := localDay(2026, time.October, 4)
	got := shouldMarkMissed(missedContext{
		local:    pauseDay,
		params:   params,
		medStart: localDay(2026, time.October, 1),
		weekday:  pauseDay.Weekday(),
		timeMask: domain.EveryDay,
		now:      time.Date(2026, time.October, 4, 10, 0, 0, 0, testLoc),
		hour:     8, minute: 0, grace: 60,
	})
	if got {
		t.Fatal("hari jeda siklus bukan terlewat")
	}

	// Hari minum pada siklus yang sama HARUS terlewat.
	intakeDay := localDay(2026, time.October, 6)
	got = shouldMarkMissed(missedContext{
		local:    intakeDay,
		params:   params,
		medStart: localDay(2026, time.October, 1),
		weekday:  intakeDay.Weekday(),
		timeMask: domain.EveryDay,
		now:      time.Date(2026, time.October, 6, 10, 0, 0, 0, testLoc),
		hour:     8, minute: 0, grace: 60,
	})
	if !got {
		t.Fatal("hari minum pada siklus harus dianggap terlewat")
	}
}

func TestShouldMarkMissed_WeeklyOnlyOnSelectedDays(t *testing.T) {
	// Obat Senin dan Kamis. Selasa bukan terlewat, karena memang tidak
	// dijadwalkan pada hari itu.
	mask := 9 // Senin(1) + Kamis(8)
	params := &domain.FrequencyParams{Pattern: domain.PatternWeekly, DaysOfWeek: &mask}

	// 2026-10-05 Senin.
	monday := localDay(2026, time.October, 5)
	if !shouldMarkMissed(missedContext{
		local: monday, params: params,
		medStart: localDay(2026, time.October, 1),
		weekday:  monday.Weekday(), timeMask: domain.EveryDay,
		now:  time.Date(2026, time.October, 5, 10, 0, 0, 0, testLoc),
		hour: 8, minute: 0, grace: 60,
	}) {
		t.Fatal("Senin harus dicap terlewat")
	}

	// 2026-10-06 Selasa, tidak dijadwalkan.
	tuesday := localDay(2026, time.October, 6)
	if shouldMarkMissed(missedContext{
		local: tuesday, params: params,
		medStart: localDay(2026, time.October, 1),
		weekday:  tuesday.Weekday(), timeMask: domain.EveryDay,
		now:  time.Date(2026, time.October, 6, 10, 0, 0, 0, testLoc),
		hour: 8, minute: 0, grace: 60,
	}) {
		t.Fatal("Selasa tidak dijadwalkan, bukan terlewat")
	}
}

func TestShouldMarkMissed_TimeMaskFiltersWeekday(t *testing.T) {
	// Jam minum hanya berlaku Senin. Pada Selasa, jam itu tidak menghasilkan
	// dosis, jadi bukan terlewat.
	day := localDay(2026, time.October, 6) // Selasa
	got := shouldMarkMissed(missedContext{
		local:    day,
		params:   dailyParams(),
		medStart: localDay(2026, time.October, 1),
		weekday:  day.Weekday(),
		timeMask: 1, // hanya Senin
		now:      time.Date(2026, time.October, 6, 10, 0, 0, 0, testLoc),
		hour:     8, minute: 0, grace: 60,
	})
	if got {
		t.Fatal("jam yang hanya berlaku Senin tidak boleh dicap terlewat pada Selasa")
	}
}

func TestMissedPass_SkipsPRN(t *testing.T) {
	// Obat PRN tidak pernah terlewat. Obat "bila perlu" memang sering tidak
	// diminum, dan menandainya terlewat akan membuat SEMUA obat PRN selalu
	// tampak gagal.
	params := &domain.FrequencyParams{Pattern: domain.PatternPRN}
	day := localDay(2026, time.October, 9)
	got := shouldMarkMissed(missedContext{
		local:    day,
		params:   params,
		medStart: localDay(2026, time.October, 1),
		weekday:  day.Weekday(),
		timeMask: domain.EveryDay,
		now:      time.Date(2026, time.October, 9, 10, 0, 0, 0, testLoc),
		hour:     8, minute: 0, grace: 60,
	})
	if got {
		t.Fatal("PRN tidak boleh dianggap terlewat")
	}
}

func TestWithinTherapy(t *testing.T) {
	loc := testLoc
	start := time.Date(2026, time.October, 1, 0, 0, 0, 0, loc)
	end := time.Date(2026, time.October, 3, 0, 0, 0, 0, loc)

	m := &domain.Medication{StartDate: start, EndDate: &end}

	cases := []struct {
		day  int
		want bool
	}{
		{1, true},  // hari mulai
		{2, true},  // tengah terapi
		{3, true},  // hari terakhir
		{4, false}, // setelah selesai
	}
	for _, c := range cases {
		day := time.Date(2026, time.October, c.day, 12, 0, 0, 0, loc)
		if got := withinTherapy(m, day, loc); got != c.want {
			t.Errorf("tanggal %d Okt: dapat %v, mau %v", c.day, got, c.want)
		}
	}

	// Tanpa end_date: terapi berjalan terus.
	open := &domain.Medication{StartDate: start}
	future := time.Date(2027, time.October, 1, 12, 0, 0, 0, loc)
	if !withinTherapy(open, future, loc) {
		t.Error("terapi tanpa end_date harus berjalan terus")
	}
	before := time.Date(2026, time.September, 30, 12, 0, 0, 0, loc)
	if withinTherapy(open, before, loc) {
		t.Error("sebelum start_date bukan bagian terapi")
	}
}

func TestNextRunTime(t *testing.T) {
	// Sebelum jam target: jalan hari ini.
	now := time.Date(2026, time.October, 10, 22, 0, 0, 0, testLoc)
	next := nextRunTime(now, 0, 30)
	if next.Day() != 11 || next.Hour() != 0 || next.Minute() != 30 {
		t.Errorf("22:00 harus menjadwalkan besok 00:30, dapat %s",
			next.Format(time.RFC3339))
	}

	// Setelah jam target: jalan besok.
	now = time.Date(2026, time.October, 10, 1, 0, 0, 0, testLoc)
	next = nextRunTime(now, 0, 30)
	if next.Day() != 11 {
		t.Errorf("01:00 harus menjadwalkan besok, dapat %s",
			next.Format(time.RFC3339))
	}
}
