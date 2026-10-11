package domain

import "time"

// File ini menghitung kepatuhan.
//
// Ini bagian yang paling bertanggung jawab secara moral: angkanya dipakai
// pasien untuk memahami dirinya sendiri, dan dipakai caregiver untuk menilai
// orang lain. Angka yang terlalu tinggi membuat masalah terlewat; angka yang
// terlalu rendah membuat user kehilangan kepercayaan pada aplikasinya.
//
// Karena itu ada satu aturan yang tidak boleh dilanggar di seluruh file ini:
// PENYEBUT dihitung dari ATURAN jadwal, bukan dari catatan yang ada. Kalau
// penyebutnya berasal dari log, pasien yang tidak pernah membuka aplikasi akan
// selalu tampak patuh 100%, karena tidak ada catatan yang bisa dihitung.

// AdherenceSummary adalah ringkasan kepatuhan pada satu rentang.
type AdherenceSummary struct {
	From time.Time
	To   time.Time

	// ScheduledDoses adalah jumlah dosis yang SEHARUSNYA ada pada rentang.
	//
	// Dihitung dari aturan frekuensi, dan INI penyebut yang benar. Dosis
	// dalam masa jeda siklus dan di luar rentang terapi tidak dihitung.
	ScheduledDoses int

	// TakenDoses mencakup status taken dan late. Late tetap dihitung patuh,
	// karena obatnya memang diminum; hanya waktunya yang bergeser.
	TakenDoses int

	// LateDoses adalah bagian dari TakenDoses, disajikan terpisah karena
	// "diminum tapi telat" berbeda artinya bagi pasien.
	LateDoses int

	// SkippedDoses adalah dosis yang sengaja dilewati user.
	SkippedDoses int

	// MissedDoses adalah dosis yang lewat jam tanpa catatan apa pun.
	MissedDoses int

	// PendingDoses adalah dosis yang jamnya belum lewat dan belum dicatat.
	//
	// Dosis ini TIDAK dihitung sebagai terlewat, dan juga tidak dihitung
	// patuh. Memasukkannya ke penyebut akan menghukum user atas dosis yang
	// belum tiba waktunya.
	PendingDoses int

	// Ratio adalah TakenDoses dibagi (ScheduledDoses - PendingDoses).
	//
	// Nil bila tidak ada dosis yang keadaannya sudah pasti. Nil, bukan nol:
	// nol berarti "semua terlewat", dan itu tuduhan yang salah untuk user
	// baru yang belum punya jadwal berjalan.
	Ratio *float64
}

// AdherenceByMedication memecah ringkasan per obat.
type AdherenceByMedication struct {
	MedicationID   string
	MedicationName string
	ScheduledDoses int
	TakenDoses     int
	LateDoses      int
	SkippedDoses   int
	MissedDoses    int
	PendingDoses   int
	Ratio          *float64
	ColorTag       *string
}

// DosesPerDay memperkirakan berapa dosis per hari untuk pola tertentu.
//
// Dipakai untuk menghitung penyebut. Hasilnya perkiraan, karena pola
// monthly_cycle bergantung pada panjang bulan dan weekly bergantung pada
// sebaran hari. Yang penting di sini: perkiraannya HARUS dari aturan, bukan
// dari catatan.
//
// Mengembalikan 0 untuk PRN, karena obat PRN tidak dijadwalkan dan tidak
// masuk hitungan kepatuhan. Menghitungnya akan selalu menghasilkan angka
// rendah yang tidak adil: obat "bila perlu" memang sering tidak diminum.
func DosesPerDay(p *FrequencyParams) float64 {
	if p == nil {
		return 0
	}

	switch p.Pattern {
	case PatternDaily:
		if p.TimesPerDay == nil {
			return 0
		}
		return float64(*p.TimesPerDay)

	case PatternInterval:
		if p.IntervalHours == nil || *p.IntervalHours <= 0 {
			return 0
		}
		// 24 jam dibagi interval. Tiap 8 jam = 3 dosis sehari.
		return 24.0 / float64(*p.IntervalHours)

	case PatternWeekly:
		if p.DaysOfWeek == nil {
			return 0
		}
		// Jumlah hari aktif dibagi 7. Dua hari seminggu = 2/7 per hari.
		return float64(countBits(*p.DaysOfWeek)) / 7.0

	case PatternMonthlyCycle:
		if p.CycleIntakeDays == nil || p.CyclePauseDays == nil {
			return 0
		}
		cycleLen := *p.CycleIntakeDays + *p.CyclePauseDays
		if cycleLen <= 0 {
			return 0
		}
		// Fraksi hari minum dalam satu siklus.
		return float64(*p.CycleIntakeDays) / float64(cycleLen)

	case PatternPRN:
		// Tidak dijadwalkan. Tidak dihitung.
		return 0
	}
	return 0
}

// countBits menghitung jumlah bit aktif pada bitmask hari.
func countBits(mask int) int {
	n := 0
	for mask != 0 {
		n += mask & 1
		mask >>= 1
	}
	return n
}

// ScheduledDosesInRange menghitung berapa dosis yang seharusnya ada pada
// rentang tanggal, memakai aturan frekuensi dan jam aktif.
//
// Menyusuri hari satu per satu. Ini memang O(jumlah hari) dan bukan
// O(jumlah jam), yang bertentangan dengan catatan performa di TRD bagian 7.1.
// Alasannya: laporan kepatuhan selalu terbatas rentang (7, 30, atau 90 hari),
// dan untuk rentang sependek itu penyusuran hari jauh lebih sederhana dan
// lebih sulit salah dibanding menghitung siklus secara analitis. Yang dijaga
// agar tetap O(jumlah jam) adalah scheduler, karena ia berjalan tiap menit.
func ScheduledDosesInRange(
	med *Medication, freq *FrequencyParams, times []MedicationTime,
	from, to time.Time, loc *time.Location,
	now time.Time, includeFuture bool,
) (scheduled, pending int) {
	if med == nil || freq == nil || len(times) == 0 || loc == nil {
		return 0, 0
	}
	// Obat PRN tidak punya jadwal, jadi tidak ada dosis yang "seharusnya".
	if freq.Pattern == PatternPRN {
		return 0, 0
	}

	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		local := time.Date(d.Year(), d.Month(), d.Day(), 12, 0, 0, 0, loc)

		// Di luar rentang terapi obat.
		if local.Before(med.StartDate.In(loc)) {
			continue
		}
		if med.EndDate != nil && local.After(med.EndDate.In(loc).AddDate(0, 0, 1)) {
			continue
		}

		if !freq.MatchesOn(local) {
			continue
		}

		for i := range times {
			t := &times[i]
			if !t.IsActive {
				continue
			}
			if !t.MatchesWeekday(local.Weekday()) {
				continue
			}

			scheduled++

			// Dosis yang jamnya belum lewat dianggap masih menunggu,
			// bukan terlewat. Dihitung terpisah supaya tidak masuk penyebut.
			if !includeFuture {
				hh, mm, err := ParseClock(t.TimeLocal)
				if err == nil {
					dueAt := time.Date(local.Year(), local.Month(), local.Day(),
						hh, mm, 0, 0, loc)
					if dueAt.After(now.In(loc)) {
						pending++
					}
				}
			}
		}
	}
	return scheduled, pending
}

// ComputeRatio menghitung rasio kepatuhan.
//
// Penyebutnya adalah dosis yang keadaannya SUDAH PASTI, yaitu terjadwal
// dikurangi yang masih menunggu. Memasukkan dosis yang belum tiba waktunya
// akan membuat pasien yang baru minum pagi ini terlihat patuh 50%, padahal
// dosis malamnya belum saatnya.
func ComputeRatio(taken, scheduled, pending int) *float64 {
	denominator := scheduled - pending
	if denominator <= 0 {
		return nil
	}
	r := float64(taken) / float64(denominator)
	if r > 1 {
		// Bisa terjadi bila ada dosis PRN yang dihitung taken tapi tidak
		// masuk penyebut. Dibatasai di 1 supaya tidak ada angka di atas 100%.
		r = 1
	}
	return &r
}
