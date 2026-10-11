package domain

import (
	"testing"
	"time"
)

func TestDosesPerDay_Daily(t *testing.T) {
	cases := []struct {
		times int
		want  float64
	}{
		{1, 1}, {2, 2}, {3, 3}, {4, 4},
	}
	for _, c := range cases {
		p := &FrequencyParams{Pattern: PatternDaily, TimesPerDay: ptrInt(c.times)}
		if got := DosesPerDay(p); got != c.want {
			t.Errorf("daily %dx: dapat %v, mau %v", c.times, got, c.want)
		}
	}
}

func TestDosesPerDay_Interval(t *testing.T) {
	// Tiap 8 jam = 3 dosis sehari.
	p := &FrequencyParams{Pattern: PatternInterval, IntervalHours: ptrInt(8)}
	if got := DosesPerDay(p); got != 3 {
		t.Errorf("interval 8 jam: dapat %v, mau 3", got)
	}
	// Tiap 12 jam = 2 dosis sehari.
	p = &FrequencyParams{Pattern: PatternInterval, IntervalHours: ptrInt(12)}
	if got := DosesPerDay(p); got != 2 {
		t.Errorf("interval 12 jam: dapat %v, mau 2", got)
	}
	// Tiap 24 jam = 1 dosis sehari.
	p = &FrequencyParams{Pattern: PatternInterval, IntervalHours: ptrInt(24)}
	if got := DosesPerDay(p); got != 1 {
		t.Errorf("interval 24 jam: dapat %v, mau 1", got)
	}
}

func TestDosesPerDay_Weekly(t *testing.T) {
	// Senin + Kamis (1 + 8) = 2 hari dari 7.
	p := &FrequencyParams{Pattern: PatternWeekly, DaysOfWeek: ptrInt(9)}
	want := 2.0 / 7.0
	if got := DosesPerDay(p); got != want {
		t.Errorf("weekly 2 hari: dapat %v, mau %v", got, want)
	}
	// Setiap hari = 7/7 = 1.
	p = &FrequencyParams{Pattern: PatternWeekly, DaysOfWeek: ptrInt(127)}
	if got := DosesPerDay(p); got != 1 {
		t.Errorf("weekly 7 hari: dapat %v, mau 1", got)
	}
}

func TestDosesPerDay_MonthlyCycle(t *testing.T) {
	// 21 hari minum, 7 hari jeda = 28 hari siklus -> 21/28 = 0.75
	p := &FrequencyParams{
		Pattern:         PatternMonthlyCycle,
		CycleIntakeDays: ptrInt(21),
		CyclePauseDays:  ptrInt(7),
	}
	if got := DosesPerDay(p); got != 0.75 {
		t.Errorf("siklus 21/7: dapat %v, mau 0.75", got)
	}
	// Tanpa jeda = 1 dosis sehari.
	p = &FrequencyParams{
		Pattern:         PatternMonthlyCycle,
		CycleIntakeDays: ptrInt(5),
		CyclePauseDays:  ptrInt(0),
	}
	if got := DosesPerDay(p); got != 1 {
		t.Errorf("siklus tanpa jeda: dapat %v, mau 1", got)
	}
}

func TestDosesPerDay_PRNIsZero(t *testing.T) {
	// PRN tidak dijadwalkan, jadi tidak boleh masuk hitungan kepatuhan.
	// Kalau ini bukan nol, obat "bila perlu" akan selalu tampak tidak patuh.
	p := &FrequencyParams{Pattern: PatternPRN}
	if got := DosesPerDay(p); got != 0 {
		t.Errorf("PRN harus 0, dapat %v", got)
	}
}

func TestScheduledDosesInRange_DailyTwoTimes(t *testing.T) {
	loc := testLoc
	med := &Medication{StartDate: date(2026, time.October, 1)}
	freq := &FrequencyParams{Pattern: PatternDaily, TimesPerDay: ptrInt(2)}
	times := []MedicationTime{
		{TimeLocal: "07:00", Weekdays: EveryDay, IsActive: true},
		{TimeLocal: "19:00", Weekdays: EveryDay, IsActive: true},
	}

	// 7 hari x 2 dosis = 14
	from := date(2026, time.October, 1)
	to := date(2026, time.October, 7)
	now := date(2026, time.October, 8)

	scheduled, pending := ScheduledDosesInRange(med, freq, times, from, to, loc, now, false)
	if scheduled != 14 {
		t.Errorf("7 hari x 2 = 14, dapat %d", scheduled)
	}
	if pending != 0 {
		t.Errorf("semua sudah lewat, pending harus 0, dapat %d", pending)
	}
}

func TestScheduledDosesInRange_RespectsTherapyRange(t *testing.T) {
	loc := testLoc
	end := date(2026, time.October, 3)
	med := &Medication{
		StartDate: date(2026, time.October, 1),
		EndDate:   &end,
	}
	freq := &FrequencyParams{Pattern: PatternDaily, TimesPerDay: ptrInt(1)}
	times := []MedicationTime{{TimeLocal: "07:00", Weekdays: EveryDay, IsActive: true}}

	// Rentang laporan lebih lebar dari terapi. Terapi hanya 1-3 Okt = 3 hari.
	scheduled, _ := ScheduledDosesInRange(med, freq, times,
		date(2026, time.October, 1), date(2026, time.October, 10), loc,
		date(2026, time.October, 11), false)

	if scheduled != 3 {
		t.Errorf("terapi 1-3 Okt harus 3 dosis, dapat %d", scheduled)
	}
}

func TestScheduledDosesInRange_SkipsPauseDays(t *testing.T) {
	loc := testLoc
	med := &Medication{StartDate: date(2026, time.October, 1)}
	freq := &FrequencyParams{
		Pattern:         PatternMonthlyCycle,
		CycleIntakeDays: ptrInt(3),
		CyclePauseDays:  ptrInt(2),
		CycleAnchor:     anchorPtr(AnchorFromStart),
	}
	times := []MedicationTime{{TimeLocal: "08:00", Weekdays: EveryDay, IsActive: true}}

	// Siklus: 1,2,3 minum | 4,5 jeda | 6,7,8 minum | 9,10 jeda ...
	// Rentang 1-10 Okt: minum pada 1,2,3,6,7,8 = 6 dosis.
	scheduled, _ := ScheduledDosesInRange(med, freq, times,
		date(2026, time.October, 1), date(2026, time.October, 10), loc,
		date(2026, time.October, 11), false)

	if scheduled != 6 {
		t.Errorf("siklus 3/2 dalam 10 hari harus 6 dosis, dapat %d", scheduled)
	}
}

func TestScheduledDosesInRange_PRNNeverScheduled(t *testing.T) {
	loc := testLoc
	med := &Medication{StartDate: date(2026, time.October, 1)}
	freq := &FrequencyParams{Pattern: PatternPRN}
	times := []MedicationTime{{TimeLocal: "08:00", Weekdays: EveryDay, IsActive: true}}

	scheduled, pending := ScheduledDosesInRange(med, freq, times,
		date(2026, time.October, 1), date(2026, time.October, 10), loc,
		date(2026, time.October, 11), false)

	if scheduled != 0 {
		t.Errorf("PRN tidak dijadwalkan, dapat %d", scheduled)
	}
	if pending != 0 {
		t.Errorf("PRN tidak punya pending, dapat %d", pending)
	}
}

func TestScheduledDosesInRange_PendingNotCountedAsPast(t *testing.T) {
	loc := testLoc
	med := &Medication{StartDate: date(2026, time.October, 1)}
	freq := &FrequencyParams{Pattern: PatternDaily, TimesPerDay: ptrInt(2)}
	times := []MedicationTime{
		{TimeLocal: "07:00", Weekdays: EveryDay, IsActive: true},
		{TimeLocal: "19:00", Weekdays: EveryDay, IsActive: true},
	}
	// Sekarang pukul 10:00 tanggal 10 Okt. Dosis 07:00 sudah lewat,
	// 19:00 belum. Jadi hari ini: 2 terjadwal, 1 pending.
	now := time.Date(2026, time.October, 10, 10, 0, 0, 0, loc)

	scheduled, pending := ScheduledDosesInRange(med, freq, times,
		date(2026, time.October, 10), date(2026, time.October, 10), loc, now, false)

	if scheduled != 2 {
		t.Errorf("harus 2 terjadwal, dapat %d", scheduled)
	}
	if pending != 1 {
		t.Errorf("harus 1 pending (19:00), dapat %d", pending)
	}
}

func TestScheduledDosesInRange_SkipsInactiveTimes(t *testing.T) {
	loc := testLoc
	med := &Medication{StartDate: date(2026, time.October, 1)}
	freq := &FrequencyParams{Pattern: PatternDaily, TimesPerDay: ptrInt(2)}
	times := []MedicationTime{
		{TimeLocal: "07:00", Weekdays: EveryDay, IsActive: true},
		{TimeLocal: "19:00", Weekdays: EveryDay, IsActive: false}, // dimatikan
	}

	scheduled, _ := ScheduledDosesInRange(med, freq, times,
		date(2026, time.October, 1), date(2026, time.October, 3), loc,
		date(2026, time.October, 4), false)

	if scheduled != 3 {
		t.Errorf("hanya jam aktif dihitung: 3 hari x 1 = 3, dapat %d", scheduled)
	}
}

func TestComputeRatio(t *testing.T) {
	cases := []struct {
		name      string
		taken     int
		scheduled int
		pending   int
		wantNil   bool
		want      float64
	}{
		{"semua diminum", 7, 7, 0, false, 1.0},
		{"separuh", 5, 10, 0, false, 0.5},
		{"tidak ada dosis", 0, 0, 0, true, 0},
		{"semua masih pending", 0, 2, 2, true, 0},
		{"satu sudah, satu pending", 1, 2, 1, false, 1.0},
		{"tidak ada yang diminum", 0, 4, 0, false, 0.0},
		{"dibatasi di 1", 5, 3, 0, false, 1.0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ComputeRatio(c.taken, c.scheduled, c.pending)
			if c.wantNil {
				if got != nil {
					t.Fatalf("harus nil, dapat %v", *got)
				}
				return
			}
			if got == nil {
				t.Fatal("harus ada nilai, dapat nil")
			}
			if *got != c.want {
				t.Errorf("dapat %v, mau %v", *got, c.want)
			}
		})
	}
}

func TestComputeRatio_PendingExcludedFromDenominator(t *testing.T) {
	// Ini inti keadilan perhitungan: dosis yang belum tiba waktunya tidak
	// boleh masuk penyebut. Kalau masuk, pasien yang baru minum pagi ini
	// akan terlihat patuh 50% padahal dosis malamnya belum saatnya.
	got := ComputeRatio(1, 2, 1)
	if got == nil {
		t.Fatal("harus ada nilai")
	}
	if *got != 1.0 {
		t.Errorf("1 dari 1 yang sudah pasti = 100%%, dapat %v", *got)
	}
}
