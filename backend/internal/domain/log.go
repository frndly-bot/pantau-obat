package domain

import "time"

// MedicationLog adalah satu catatan dosis.
//
// Dua bentuk baris hidup berdampingan di tabel yang sama, dan bedanya
// penting:
//
//   - Dosis TERJADWAL: schedule_id terisi, dan occurrence key-nya
//     "<schedule_id>|<tanggal>|<HH>:<MM>" menjamin satu dosis hanya punya
//     satu log hidup. Inilah yang mencegah dosis tercatat dua kali.
//
//   - Dosis PRN/spontan: schedule_id NULL. Tidak ada jadwalnya, jadi tidak
//     ada yang bisa dibandingkan, dan memang boleh berkali-kali sehari.
//     CHECK ck_log_occurrence menegakkan bahwa jam juga NULL bila
//     schedule_id NULL, supaya tidak ada baris setengah terjadwal.
type MedicationLog struct {
	ID           string
	MedicationID string
	ScheduleID   *string
	OwnerID      string
	RecordedBy   string

	SlotDate        time.Time
	ScheduledAt     *time.Time
	ScheduledHour   *int
	ScheduledMinute *int
	ScheduledTZ     *string

	Status  LogStatus
	TakenAt *time.Time
	Note    *string
	Source  LogSource

	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsScheduled melaporkan apakah catatan ini untuk dosis terjadwal.
func (l *MedicationLog) IsScheduled() bool { return l.ScheduleID != nil }

// OccurrenceKey menyusun kunci dosis untuk catatan ini.
//
// Memakai SATU fungsi yang sama dengan bump_queue dan medication_dose_events.
// Kalau formatnya berbeda di salah satu tempat, anti-ganda gagal: dosis yang
// sama bisa tercatat dua kali, dan stok bisa terpotong dua kali.
//
// Mengembalikan string kosong untuk dosis PRN, karena dosis seperti itu
// memang tidak punya identitas yang perlu dijaga keunikannya.
func (l *MedicationLog) OccurrenceKey() string {
	if !l.IsScheduled() || l.ScheduledHour == nil || l.ScheduledMinute == nil {
		return ""
	}
	return OccurrenceKey(*l.ScheduleID, l.SlotDate, *l.ScheduledHour, *l.ScheduledMinute)
}

// ConsumesStock melaporkan apakah catatan ini memotong saldo stok.
//
// HANYA taken dan late yang memotong. skipped TIDAK, karena obatnya memang
// tidak masuk ke tubuh; kalau stoknya tetap dipotong, sisa obat akan
// dilaporkan lebih sedikit daripada kenyataan, dan peringatan restock muncul
// terlalu cepat.
//
// snoozed juga tidak memotong: menunda bukan meminum.
func (l *MedicationLog) ConsumesStock() bool {
	return l.Status == StatusTaken || l.Status == StatusLate
}

// ComputeStatus menentukan status dari selisih waktu.
//
// taken bila selisih terhadap jadwal <= tolerance_minutes, late bila lebih.
// Arah selisihnya tidak dipedulikan: mencatat 10 menit lebih awal tetap
// dihitung "taken", karena yang penting obatnya diminum di sekitar jam itu.
//
// tolerated menit 0 berarti memakai nilai bawaan 60, sesuai TRD bagian D1.
func ComputeStatus(scheduled time.Time, takenAt time.Time, toleranceMinutes int) LogStatus {
	if toleranceMinutes <= 0 {
		toleranceMinutes = 60
	}
	diff := takenAt.Sub(scheduled)
	if diff < 0 {
		diff = -diff
	}
	if diff <= time.Duration(toleranceMinutes)*time.Minute {
		return StatusTaken
	}
	return StatusLate
}

// LogInput adalah permintaan pencatatan dosis terjadwal.
type LogInput struct {
	ScheduleID      string
	SlotDate        time.Time
	ScheduledHour   int
	ScheduledMinute int
	ScheduledTZ     string
	ScheduledAt     *time.Time

	Status  LogStatus
	TakenAt *time.Time
	Note    *string
}

// PRNLogInput adalah permintaan pencatatan dosis PRN.
//
// Tidak ada schedule_id dan tidak ada jam jadwal, karena dosis ini memang
// tidak dijadwalkan.
type PRNLogInput struct {
	SlotDate time.Time
	Status   LogStatus
	TakenAt  *time.Time
	Note     *string
}
