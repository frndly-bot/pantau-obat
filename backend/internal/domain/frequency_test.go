package domain

import (
	"testing"
	"time"
)

// Location uji memakai zona yang punya offset tetap supaya hasilnya tidak
// bergantung pada DST saat test dijalankan.
var testLoc = time.FixedZone("WIB", 7*3600)

func ptrInt(v int) *int { return &v }

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, testLoc)
}

func at(y int, m time.Month, d, hh, mm int) time.Time {
	return time.Date(y, m, d, hh, mm, 0, 0, testLoc)
}

// -----------------------------------------------------------------------------
// Harness
// -----------------------------------------------------------------------------

func TestValidate_RejectsMissingParams(t *testing.T) {
	cases := []struct {
		name string
		p    FrequencyParams
	}{
		{"daily tanpa times_per_day", FrequencyParams{Pattern: PatternDaily}},
		{"interval tanpa interval_hours", FrequencyParams{Pattern: PatternInterval}},
		{"weekly tanpa days_of_week", FrequencyParams{Pattern: PatternWeekly}},
		{"monthly_cycle tanpa intake", FrequencyParams{Pattern: PatternMonthlyCycle}},
		{"pola tak dikenal", FrequencyParams{Pattern: "tiap_saat"}},
		{"monthly_cycle dom tanpa anchor_dom", FrequencyParams{
			Pattern:         PatternMonthlyCycle,
			CycleIntakeDays: ptrInt(21),
			CyclePauseDays:  ptrInt(7),
			CycleAnchor:     anchorPtr(AnchorDom),
		}},
		{"times_per_day di luar rentang", FrequencyParams{
			Pattern: PatternDaily, TimesPerDay: ptrInt(25),
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.p.Validate(); err == nil {
				t.Fatal("harusnya ditolak, tapi lolos validasi")
			}
		})
	}
}

func TestValidate_AcceptsValidParams(t *testing.T) {
	cases := []struct {
		name string
		p    FrequencyParams
	}{
		{"daily", FrequencyParams{Pattern: PatternDaily, TimesPerDay: ptrInt(2)}},
		{"interval", FrequencyParams{Pattern: PatternInterval, IntervalHours: ptrInt(8)}},
		{"weekly", FrequencyParams{Pattern: PatternWeekly, DaysOfWeek: ptrInt(10)}},
		{"monthly_cycle dom", FrequencyParams{
			Pattern: PatternMonthlyCycle, CycleIntakeDays: ptrInt(21),
			CyclePauseDays: ptrInt(7), CycleAnchor: anchorPtr(AnchorDom),
			CycleAnchorDOM: ptrInt(5),
		}},
		{"monthly_cycle end_of_month", FrequencyParams{
			Pattern: PatternMonthlyCycle, CycleIntakeDays: ptrInt(3),
			CyclePauseDays: ptrInt(0), CycleAnchor: anchorPtr(AnchorEndOfMonth),
		}},
		{"prn tanpa parameter", FrequencyParams{Pattern: PatternPRN}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.p.Validate(); err != nil {
				t.Fatalf("harusnya lolos, tapi ditolak: %v", err)
			}
		})
	}
}

func anchorPtr(a CycleAnchor) *CycleAnchor { return &a }

// -----------------------------------------------------------------------------
// Pola harian dan mingguan
// -----------------------------------------------------------------------------

func TestDaily_MatchesEveryDay(t *testing.T) {
	p := FrequencyParams{Pattern: PatternDaily, TimesPerDay: ptrInt(1)}
	for d := 1; d <= 7; d++ {
		if !p.MatchesOn(date(2026, time.October, d)) {
			t.Fatalf("daily harus cocok pada 2026-10-%02d", d)
		}
	}
	if p.IsPaused(date(2026, time.October, 3)) {
		t.Fatal("daily tidak boleh punya masa jeda")
	}
}

func TestWeekly_MatchesOnlySelectedDays(t *testing.T) {
	// Senin(1) + Kamis(8) = 9
	p := FrequencyParams{Pattern: PatternWeekly, DaysOfWeek: ptrInt(9)}

	// 2026-10-05 adalah Senin, 2026-10-08 Kamis, 2026-10-06 Selasa.
	if !p.MatchesOn(date(2026, time.October, 5)) {
		t.Fatal("Senin harus cocok")
	}
	if !p.MatchesOn(date(2026, time.October, 8)) {
		t.Fatal("Kamis harus cocok")
	}
	if p.MatchesOn(date(2026, time.October, 6)) {
		t.Fatal("Selasa tidak boleh cocok")
	}
	// Minggu = 64, tidak ada di bitmask 9.
	if p.MatchesOn(date(2026, time.October, 11)) {
		t.Fatal("Minggu tidak boleh cocok")
	}
}

// -----------------------------------------------------------------------------
// Pola interval
// -----------------------------------------------------------------------------

func TestInterval_MatchesEveryEightHours(t *testing.T) {
	p := FrequencyParams{
		Pattern:       PatternInterval,
		IntervalHours: ptrInt(8),
		StartTime:     "06:00",
	}

	shouldMatch := []time.Time{
		at(2026, time.October, 10, 6, 0),
		at(2026, time.October, 10, 14, 0),
		at(2026, time.October, 10, 22, 0),
		at(2026, time.October, 11, 6, 0), // lintas hari
	}
	for _, ts := range shouldMatch {
		if !p.MatchesOn(ts) {
			t.Errorf("harusnya cocok pada %s", ts.Format(time.RFC3339))
		}
	}

	shouldNot := []time.Time{
		at(2026, time.October, 10, 7, 0),
		at(2026, time.October, 10, 13, 30),
		at(2026, time.October, 10, 5, 0),
	}
	for _, ts := range shouldNot {
		if p.MatchesOn(ts) {
			t.Errorf("tidak boleh cocok pada %s", ts.Format(time.RFC3339))
		}
	}
}

func TestInterval_BeforeStartTimeUsesPreviousDay(t *testing.T) {
	// Jam acuan 06:00, interval 12 jam. Pukul 18:00 hari sebelumnya adalah
	// kelipatan yang sah, dan harus tetap cocok saat diperiksa pukul 05:00.
	p := FrequencyParams{
		Pattern:       PatternInterval,
		IntervalHours: ptrInt(12),
		StartTime:     "06:00",
	}
	if !p.MatchesOn(at(2026, time.October, 10, 18, 0)) {
		t.Fatal("18:00 harus cocok (12 jam setelah 06:00)")
	}
}

// -----------------------------------------------------------------------------
// Siklus bulanan — bagian yang paling mudah salah
// -----------------------------------------------------------------------------

func TestMonthlyCycle_TwentyOneDaysOnSevenDaysOff(t *testing.T) {
	p := FrequencyParams{
		Pattern:         PatternMonthlyCycle,
		CycleIntakeDays: ptrInt(21),
		CyclePauseDays:  ptrInt(7),
		CycleAnchor:     anchorPtr(AnchorFromStart),
	}

	// Siklus mulai 1 Oktober: hari 1..21 minum, hari 22..28 jeda,
	// hari 29 mulai siklus baru.
	if !p.MatchesOn(date(2026, time.October, 1)) {
		t.Fatal("hari ke-1 harus minum")
	}
	if !p.MatchesOn(date(2026, time.October, 21)) {
		t.Fatal("hari ke-21 harus minum")
	}
	if p.MatchesOn(date(2026, time.October, 22)) {
		t.Fatal("hari ke-22 harus JEDA")
	}
	if !p.IsPaused(date(2026, time.October, 22)) {
		t.Fatal("hari ke-22 harus dikenali sebagai jeda")
	}
	if p.MatchesOn(date(2026, time.October, 28)) {
		t.Fatal("hari ke-28 harus JEDA")
	}
	if !p.MatchesOn(date(2026, time.October, 29)) {
		t.Fatal("hari ke-29 harus mulai siklus baru (minum)")
	}
}

func TestMonthlyCycle_EndOfMonthFollowsLeapYear(t *testing.T) {
	p := FrequencyParams{
		Pattern:         PatternMonthlyCycle,
		CycleIntakeDays: ptrInt(1),
		CyclePauseDays:  ptrInt(0),
		CycleAnchor:     anchorPtr(AnchorEndOfMonth),
	}

	// 2026 bukan kabisat: Februari 28 hari.
	got := p.cycleStartForMonth(date(2026, time.February, 15))
	if got.Day() != 28 || got.Month() != time.February {
		t.Fatalf("2026-02 harus berakhir tanggal 28, dapat %s", got.Format(time.DateOnly))
	}

	// 2028 kabisat: Februari 29 hari.
	got = p.cycleStartForMonth(date(2028, time.February, 15))
	if got.Day() != 29 {
		t.Fatalf("2028-02 harus berakhir tanggal 29 (kabisat), dapat %d", got.Day())
	}

	// April selalu 30.
	got = p.cycleStartForMonth(date(2026, time.April, 10))
	if got.Day() != 30 {
		t.Fatalf("April harus berakhir tanggal 30, dapat %d", got.Day())
	}
}

func TestMonthlyCycle_Dom31FallsBackToLastDay(t *testing.T) {
	// Ini keputusan yang diambil sadar: dom=31 pada bulan yang tidak punya
	// tanggal 31 memakai tanggal TERAKHIR bulan itu, bukan dilewati.
	//
	// Melewati berarti ada bulan tanpa dosis sama sekali, dan itu dosis yang
	// hilang tanpa pemberitahuan ke user maupun caregiver.
	p := FrequencyParams{
		Pattern:         PatternMonthlyCycle,
		CycleIntakeDays: ptrInt(1),
		CyclePauseDays:  ptrInt(0),
		CycleAnchor:     anchorPtr(AnchorDom),
		CycleAnchorDOM:  ptrInt(31),
	}

	cases := []struct {
		month time.Month
		year  int
		want  int
	}{
		{time.January, 2026, 31},
		{time.February, 2026, 28}, // bukan kabisat
		{time.February, 2028, 29}, // kabisat
		{time.April, 2026, 30},
		{time.June, 2026, 30},
		{time.December, 2026, 31},
	}
	for _, c := range cases {
		got := p.cycleStartForMonth(date(c.year, c.month, 15))
		if got.Day() != c.want {
			t.Errorf("dom=31 di %s %d: harus %d, dapat %d",
				c.month, c.year, c.want, got.Day())
		}
	}
}

func TestMonthlyCycle_Dom5StaysAtFive(t *testing.T) {
	p := FrequencyParams{
		Pattern:         PatternMonthlyCycle,
		CycleIntakeDays: ptrInt(3),
		CyclePauseDays:  ptrInt(5),
		CycleAnchor:     anchorPtr(AnchorDom),
		CycleAnchorDOM:  ptrInt(5),
	}

	// Mulai 5 Oktober: 5,6,7 minum; 8..12 jeda; 13 mulai lagi.
	if !p.MatchesOn(date(2026, time.October, 5)) {
		t.Fatal("5 Okt harus minum")
	}
	if !p.MatchesOn(date(2026, time.October, 7)) {
		t.Fatal("7 Okt harus minum")
	}
	if p.MatchesOn(date(2026, time.October, 8)) {
		t.Fatal("8 Okt harus jeda")
	}
	if !p.MatchesOn(date(2026, time.October, 13)) {
		t.Fatal("13 Okt harus mulai siklus baru")
	}
}

func TestMonthlyCycle_ZeroPauseNeverPauses(t *testing.T) {
	p := FrequencyParams{
		Pattern:         PatternMonthlyCycle,
		CycleIntakeDays: ptrInt(1),
		CyclePauseDays:  ptrInt(0),
		CycleAnchor:     anchorPtr(AnchorEndOfMonth),
	}
	for d := 1; d <= 28; d++ {
		ts := date(2026, time.February, d)
		if p.IsPaused(ts) {
			t.Fatalf("pause=0 tidak boleh ada jeda, tapi %s dianggap jeda", ts.Format(time.DateOnly))
		}
	}
}

// -----------------------------------------------------------------------------
// PRN
// -----------------------------------------------------------------------------

func TestPRN_NeverScheduled(t *testing.T) {
	p := FrequencyParams{Pattern: PatternPRN}
	// PRN tidak pernah "jatuh tempo" dengan sendirinya. Kalau ini berubah,
	// sistem akan mulai mengirim pengingat untuk obat yang justru diminum
	// hanya bila perlu — persis yang tidak diinginkan.
	for d := 1; d <= 5; d++ {
		if p.MatchesOn(date(2026, time.October, d)) {
			t.Fatal("PRN tidak boleh dijadwalkan otomatis")
		}
	}
}

func TestPRN_AcceptsOptionalMaxPerDay(t *testing.T) {
	p := FrequencyParams{Pattern: PatternPRN, PRNMaxPerDay: ptrInt(4)}
	if err := p.Validate(); err != nil {
		t.Fatalf("prn dengan max_per_day harus lolos: %v", err)
	}
	p.PRNMaxPerDay = ptrInt(30)
	if err := p.Validate(); err == nil {
		t.Fatal("prn_max_per_day 30 harus ditolak (maks 24)")
	}
}

// -----------------------------------------------------------------------------
// Util
// -----------------------------------------------------------------------------

func TestDaysInMonth(t *testing.T) {
	cases := []struct {
		y    int
		m    time.Month
		want int
	}{
		{2026, time.January, 31},
		{2026, time.February, 28},
		{2028, time.February, 29},
		{2000, time.February, 29}, // kabisat abad
		{1900, time.February, 28}, // bukan kabisat abad
		{2026, time.April, 30},
		{2026, time.December, 31},
	}
	for _, c := range cases {
		if got := daysInMonth(c.y, c.m); got != c.want {
			t.Errorf("daysInMonth(%d, %s) = %d, mau %d", c.y, c.m, got, c.want)
		}
	}
}

func TestParseClock(t *testing.T) {
	cases := []struct {
		in         string
		h, m       int
		shouldFail bool
	}{
		{"07:00", 7, 0, false},
		{"07:00:00", 7, 0, false}, // bentuk keluaran PostgreSQL
		{"23:59", 23, 59, false},
		{"00:00", 0, 0, false},
		{"24:00", 0, 0, true},
		{"12:60", 0, 0, true},
		{"abc", 0, 0, true},
		{"", 0, 0, true},
	}
	for _, c := range cases {
		h, m, err := ParseClock(c.in)
		if c.shouldFail {
			if err == nil {
				t.Errorf("ParseClock(%q) harusnya gagal", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseClock(%q) error: %v", c.in, err)
			continue
		}
		if h != c.h || m != c.m {
			t.Errorf("ParseClock(%q) = %02d:%02d, mau %02d:%02d", c.in, h, m, c.h, c.m)
		}
	}
}
