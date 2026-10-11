package domain

import (
	"fmt"
	"time"
)

// File ini memuat mesin aturan frekuensi.
//
// Ini inti dari keputusan v2: jadwal disimpan sebagai ATURAN, bukan baris per
// hari. Fungsi di sini yang menjawab pertanyaan "apakah ada dosis jatuh pada
// detik ini", tanpa pernah menulis satu baris pun ke database.
//
// Konsekuensi yang harus dijaga: kompleksitasnya harus O(jumlah jam per obat),
// BUKAN O(jumlah hari terapi). Kalau suatu saat ada loop yang menyusuri hari
// satu per satu, itu tanda ada yang salah rancang.

// FrequencyParams adalah parameter mentah dari database, dikumpulkan supaya
// fungsi aturan bisa diuji tanpa database.
type FrequencyParams struct {
	Pattern FrequencyPattern

	TimesPerDay      *int
	IntervalHours    *int
	DaysOfWeek       *int
	IntervalMinHours *int

	CycleIntakeDays *int
	CyclePauseDays  *int
	CycleAnchor     *CycleAnchor
	CycleAnchorDOM  *int

	PRNMaxPerDay *int

	// StartTime adalah jam acuan untuk pola interval. Interval dihitung
	// dari jam ini, bukan dari tengah malam.
	StartTime string
}

// Validate memeriksa parameter frekuensi terhadap pattern-nya.
//
// Ini mengulang CHECK ck_freq_params di database, dan itu DISENGAJA.
// Tujuannya bukan mengandalkan salah satu, melainkan memberi pesan yang jelas
// sebelum menyentuh database. Pelanggaran CHECK di PostgreSQL hanya berbunyi
// "violates check constraint ck_freq_params", yang tidak berguna bagi user
// maupun bagi orang yang menelusuri bug.
func (p *FrequencyParams) Validate() error {
	if !p.Pattern.Valid() {
		return fmt.Errorf("%w: pola frekuensi %q tidak dikenal", ErrValidation, p.Pattern)
	}

	switch p.Pattern {
	case PatternDaily:
		if p.TimesPerDay == nil {
			return fmt.Errorf("%w: pola daily wajib memuat times_per_day", ErrValidation)
		}
		if *p.TimesPerDay < 1 || *p.TimesPerDay > 24 {
			return fmt.Errorf("%w: times_per_day harus antara 1 dan 24", ErrValidation)
		}

	case PatternInterval:
		if p.IntervalHours == nil {
			return fmt.Errorf("%w: pola interval wajib memuat interval_hours", ErrValidation)
		}
		if *p.IntervalHours < 1 || *p.IntervalHours > 168 {
			return fmt.Errorf("%w: interval_hours harus antara 1 dan 168", ErrValidation)
		}
		if p.IntervalMinHours != nil && (*p.IntervalMinHours < 1 || *p.IntervalMinHours > 168) {
			return fmt.Errorf("%w: interval_min_hours harus antara 1 dan 168", ErrValidation)
		}

	case PatternWeekly:
		if p.DaysOfWeek == nil {
			return fmt.Errorf("%w: pola weekly wajib memuat days_of_week", ErrValidation)
		}
		if *p.DaysOfWeek < 1 || *p.DaysOfWeek > 127 {
			return fmt.Errorf("%w: days_of_week harus antara 1 dan 127 (minimal satu hari)", ErrValidation)
		}

	case PatternMonthlyCycle:
		if p.CycleIntakeDays == nil || p.CyclePauseDays == nil || p.CycleAnchor == nil {
			return fmt.Errorf("%w: pola monthly_cycle wajib memuat cycle_intake_days, cycle_pause_days, dan cycle_anchor", ErrValidation)
		}
		if *p.CycleIntakeDays < 1 || *p.CycleIntakeDays > 365 {
			return fmt.Errorf("%w: cycle_intake_days harus antara 1 dan 365", ErrValidation)
		}
		if *p.CyclePauseDays < 0 || *p.CyclePauseDays > 365 {
			return fmt.Errorf("%w: cycle_pause_days harus antara 0 dan 365", ErrValidation)
		}
		switch *p.CycleAnchor {
		case AnchorDom:
			if p.CycleAnchorDOM == nil {
				return fmt.Errorf("%w: cycle_anchor dom wajib memuat cycle_anchor_dom", ErrValidation)
			}
			if *p.CycleAnchorDOM < 1 || *p.CycleAnchorDOM > 31 {
				return fmt.Errorf("%w: cycle_anchor_dom harus antara 1 dan 31", ErrValidation)
			}
		case AnchorEndOfMonth, AnchorFromStart:
			// tidak butuh parameter tambahan
		default:
			return fmt.Errorf("%w: cycle_anchor %q tidak dikenal", ErrValidation, *p.CycleAnchor)
		}

	case PatternPRN:
		if p.PRNMaxPerDay != nil && (*p.PRNMaxPerDay < 1 || *p.PRNMaxPerDay > 24) {
			return fmt.Errorf("%w: prn_max_per_day harus antara 1 dan 24", ErrValidation)
		}
	}

	return nil
}

// MatchesOn melaporkan apakah ATURAN ini menghasilkan dosis pada hari
// tertentu.
//
// Parameternya adalah waktu LOKAL pemilik (sudah dikonversi ke
// schedule_timezone), bukan waktu UTC. Konversi dilakukan pemanggil, supaya
// fungsi ini murni dan bisa diuji tanpa urusan zona.
func (p *FrequencyParams) MatchesOn(local time.Time) bool {
	switch p.Pattern {
	case PatternDaily:
		return true

	case PatternInterval:
		if p.IntervalHours == nil {
			return false
		}
		return p.intervalMatches(local)

	case PatternWeekly:
		if p.DaysOfWeek == nil {
			return false
		}
		return p.DaysOfWeek != nil && bitForWeekday(local.Weekday())&*p.DaysOfWeek != 0

	case PatternMonthlyCycle:
		return p.cycleMatches(local)

	case PatternPRN:
		// PRN tidak dijadwalkan. Tidak ada dosis yang "jatuh tempo"
		// dengan sendirinya; hanya dicatat manual saat diminum.
		return false
	}
	return false
}

// IsPaused melaporkan apakah hari itu berada dalam masa jeda siklus.
//
// Dipakai untuk membedakan dua hal yang berbeda: "tidak ada dosis karena
// memang bukan saatnya" versus "tidak ada dosis karena terlewat". Hanya yang
// kedua yang dihitung sebagai terlewat dan memicu notifikasi ke caregiver.
func (p *FrequencyParams) IsPaused(on time.Time) bool {
	if p.Pattern != PatternMonthlyCycle {
		return false
	}
	return p.inPauseWindow(on)
}

// cycleMatches melaporkan apakah hari itu masuk dalam fase minum siklus.
//
// Siklus dihitung dari anchor: intake_days hari minum, lalu pause_days hari
// jeda, berulang. Parameter `from` untuk anchor from_start adalah start_date
// obat; untuk dom dan end_of_month, anchor dihitung per bulan.
func (p *FrequencyParams) cycleMatches(local time.Time) bool {
	intake, pause, anchor := p.cycleParams()
	if intake == nil || pause == nil || anchor == nil {
		return false
	}
	if p.inPauseWindow(local) {
		return false
	}
	return true
}

// inPauseWindow melaporkan apakah tanggal berada di fase jeda.
func (p *FrequencyParams) inPauseWindow(local time.Time) bool {
	intake, pause, anchor := p.cycleParams()
	if intake == nil || pause == nil || anchor == nil {
		return false
	}
	if *pause == 0 {
		return false
	}

	// Titik awal siklus untuk bulan berjalan, bergantung pada anchor.
	cycleStart := p.cycleStartForMonth(local)
	if cycleStart.IsZero() {
		return false
	}

	// Hari ke berapa dalam siklus, dihitung dari tanggal.
	// Selalu >= 0 karena cycleStart tidak pernah setelah local.
	dayInCycle := int(local.Sub(cycleStart).Hours() / 24)
	if dayInCycle < 0 {
		return false
	}

	cycleLen := *intake + *pause
	if cycleLen <= 0 {
		return false
	}
	pos := dayInCycle % cycleLen

	// Fase minum adalah [0, intake), fase jeda adalah [intake, cycleLen).
	// Hari ke-intake sudah masuk jeda, sesuai makna "minum N hari".
	return pos >= *intake
}

// cycleStartForMonth menghitung tanggal mulai siklus untuk bulan yang
// memuat `local`, sesuai anchor.
//
// Ini bagian yang paling mudah salah. Aturannya:
//
//	dom          : tanggal cycle_anchor_dom di bulan itu. Kalau bulannya
//	               tidak punya tanggal itu (mis. 31 di Februari), dipakai
//	               tanggal TERAKHIR bulan itu. Tidak dilewati, karena
//	               melewati berarti ada bulan tanpa dosis sama sekali,
//	               dan itu dosis yang hilang tanpa pemberitahuan.
//
//	end_of_month : selalu tanggal terakhir bulan itu, mengikuti tahun
//	               kabisat (Februari 28 atau 29).
//
//	from_start   : tanggal 1 bulan itu, karena siklus berjalan menerus
//	               sejak start_date dan perhitungannya memakai modulo dari
//	               awal terapi, bukan dari awal bulan.
func (p *FrequencyParams) cycleStartForMonth(local time.Time) time.Time {
	if p.CycleAnchor == nil {
		return time.Time{}
	}
	year, month := local.Year(), local.Month()

	switch *p.CycleAnchor {
	case AnchorDom:
		if p.CycleAnchorDOM == nil {
			return time.Time{}
		}
		lastDay := daysInMonth(year, month)
		day := *p.CycleAnchorDOM
		if day > lastDay {
			day = lastDay
		}
		return time.Date(year, month, day, 0, 0, 0, 0, local.Location())

	case AnchorEndOfMonth:
		return time.Date(year, month, daysInMonth(year, month), 0, 0, 0, 0, local.Location())

	case AnchorFromStart:
		return time.Date(year, month, 1, 0, 0, 0, 0, local.Location())

	default:
		return time.Time{}
	}
}

// intervalMatches melaporkan apakah jam saat ini jatuh pada kelipatan interval
// dari jam acuan.
//
// Pola interval TIDAK memakai tanggal; yang menentukan hanya selisih jam dari
// jam mulai. Karena itu pola ini cocok untuk obat yang harus tepat tiap 8 jam,
// termasuk melewati pergantian hari.
func (p *FrequencyParams) intervalMatches(local time.Time) bool {
	if p.IntervalHours == nil || p.StartTime == "" {
		return false
	}
	startHour, startMin, err := ParseClock(p.StartTime)
	if err != nil {
		return false
	}

	start := time.Date(local.Year(), local.Month(), local.Day(),
		startHour, startMin, 0, 0, local.Location())

	// Kalau jam acuan hari ini masih di depan, acuannya adalah kemarin:
	// interval tetap berjalan menerus lintas hari.
	if start.After(local) {
		start = start.AddDate(0, 0, -1)
	}

	diff := local.Sub(start)
	interval := time.Duration(*p.IntervalHours) * time.Hour
	if interval <= 0 {
		return false
	}

	// Toleransi satu menit dipakai karena waktu datang dengan detik, dan
	// perbandingan modulo yang ketat akan gagal pada detik ke-0..59 dari
	// menit yang seharusnya cocok.
	remainder := diff % interval
	return remainder < time.Minute || interval-remainder < time.Minute
}

func (p *FrequencyParams) cycleParams() (*int, *int, *CycleAnchor) {
	return p.CycleIntakeDays, p.CyclePauseDays, p.CycleAnchor
}

// bitForWeekday mengubah time.Weekday menjadi bitmask.
// Senin=1, Selasa=2, Rabu=4, Kamis=8, Jumat=16, Sabtu=32, Minggu=64.
func bitForWeekday(w time.Weekday) int {
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

// daysInMonth mengembalikan jumlah hari dalam bulan, mengikuti tahun kabisat.
//
// Dihitung dengan trik "tanggal 0 bulan berikutnya": time.Date menormalkan
// tanggal 0 menjadi hari terakhir bulan sebelumnya. Cara ini lebih sulit
// salah dibanding tabel panjang bulan yang harus mengingat kabisat.
func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// ParseClock mengurai "HH:MM" menjadi jam dan menit.
func ParseClock(s string) (hour, minute int, err error) {
	if len(s) < 5 {
		return 0, 0, fmt.Errorf("jam %q harus berbentuk HH:MM", s)
	}
	// PostgreSQL mengembalikan TIME sebagai "HH:MM:SS"; ambil dua bagian
	// pertama saja supaya bentuk apa pun diterima.
	var h, m int
	if _, err := fmt.Sscanf(s, "%d:%d", &h, &m); err != nil {
		return 0, 0, fmt.Errorf("jam %q tidak bisa diurai: %w", s, err)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("jam %q di luar rentang", s)
	}
	return h, m, nil
}

// FormatClock menyusun "HH:MM" dari jam dan menit.
func FormatClock(hour, minute int) string {
	return fmt.Sprintf("%02d:%02d", hour, minute)
}
