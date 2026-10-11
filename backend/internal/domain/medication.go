package domain

import "time"

// Medication adalah obat milik seorang user.
//
// Obat TIDAK menyimpan waktu dosis per hari. Jadwal disimpan sebagai ATURAN
// di MedicationFrequency dan jam konkret di MedicationTime. Itu yang membuat
// terapi "selamanya" 2x sehari hanya memakai 3 baris, bukan 730 baris setahun.
type Medication struct {
	ID        string
	OwnerID   string
	CatalogID *string

	Name           string
	DoseValue      float64
	DoseUnitCode   string
	StrengthNote   *string
	RouteCode      string
	DosageFormCode *string
	MealRelation   MealRelation

	AsNeeded   bool
	MustFinish bool

	StartDate time.Time
	StartTime string // "HH:MM", disimpan sebagai teks jam lokal
	EndDate   *time.Time

	Notes                *string
	NotesReminder        *string
	PrescriptionPhotoURL *string
	ColorTag             *string

	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time

	// Frekuensi, jam, dan stok ikut dibawa supaya GET detail bisa
	// mengembalikan obat lengkap dalam satu query.
	Frequency *MedicationFrequency
	Times     []MedicationTime
	Stock     *MedicationStock
}

// Ongoing melaporkan apakah terapi masih berjalan pada tanggal tertentu.
// EndDate nil berarti terapi tanpa batas.
func (m *Medication) Ongoing(on time.Time) bool {
	if m.EndDate == nil {
		return true
	}
	return !on.After(*m.EndDate)
}

// MedicationFrequency adalah ATURAN frekuensi, bukan daftar hari.
//
// Kolom bertipe pointer karena parameternya bergantung pada pattern:
// pattern 'daily' memakai TimesPerDay, 'interval' memakai IntervalHours, dan
// seterusnya. Hanya yang relevan yang terisi, dan CHECK ck_freq_params di
// database menegakkan itu.
type MedicationFrequency struct {
	ID           string
	MedicationID string
	OwnerID      string

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

	ToleranceMinutes int
	StartTime        string

	// ScheduleTimezone adalah zona saat blok jadwal ini dibuat.
	//
	// Disimpan per blok, BUKAN dibaca dari users.timezone saat menghitung.
	// Alasannya: kalau pasien pindah zona atau mengubah preferensinya,
	// tanggal dosis yang SUDAH terjadi tidak boleh berubah. Zona ini yang
	// menjaga blok jadwal tetap konsisten dengan saat ia dibuat.
	ScheduleTimezone string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// MedicationTime adalah satu jam konkret dalam sehari.
type MedicationTime struct {
	ID           string
	MedicationID string
	FrequencyID  *string
	OwnerID      string

	TimeLocal    string // "HH:MM"
	Label        *TimeLabel
	Weekdays     int
	DoseOverride *string
	SortOrder    int
	IsActive     bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

// MatchesWeekday melaporkan apakah jam ini berlaku pada hari tertentu.
//
// Bitmask mengikuti konvensi: Senin=1, Selasa=2, Rabu=4, Kamis=8,
// Jumat=16, Sabtu=32, Minggu=64. Nilai 127 berarti setiap hari.
func (t *MedicationTime) MatchesWeekday(weekday time.Weekday) bool {
	// time.Weekday: Minggu=0, Senin=1, ... Sabtu=6
	var bit int
	switch weekday {
	case time.Sunday:
		bit = 64
	case time.Monday:
		bit = 1
	case time.Tuesday:
		bit = 2
	case time.Wednesday:
		bit = 4
	case time.Thursday:
		bit = 8
	case time.Friday:
		bit = 16
	case time.Saturday:
		bit = 32
	}
	return t.Weekdays&bit != 0
}

// EveryDay adalah bitmask untuk semua hari. Dipakai sebagai default.
const EveryDay = 127

// MedicationStock adalah saldo stok obat.
type MedicationStock struct {
	ID           string
	MedicationID string
	OwnerID      string

	UnitCode           string
	InitialQty         float64
	CurrentQty         float64
	QtyPerDose         float64
	RefillThresholdQty *float64
	RefillReminder     bool
	LastRestockedAt    *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

// DosesRemaining memperkirakan berapa dosis yang masih tersedia.
func (s *MedicationStock) DosesRemaining() float64 {
	if s.QtyPerDose <= 0 {
		return 0
	}
	return s.CurrentQty / s.QtyPerDose
}

// NeedsRefill melaporkan apakah saldo sudah melewati ambang peringatan.
func (s *MedicationStock) NeedsRefill() bool {
	if s.RefillThresholdQty == nil || !s.RefillReminder {
		return false
	}
	return s.CurrentQty <= *s.RefillThresholdQty
}
