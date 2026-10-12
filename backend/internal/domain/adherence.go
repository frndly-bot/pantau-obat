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

// DoseSlot adalah SATU dosis terjadwal yang nyata pada satu tanggal.
//
// Ini bentuk "yang sebenarnya jatuh tempo", bukan sekadar hitungan. Dipakai
// oleh layar Hari Ini (agar user bisa mencatat per dosis) dan oleh perhitungan
// kepatuhan (yang hanya butuh jumlahnya).
//
// ScheduleID adalah id baris medication_times — nilai yang sama yang
// diharapkan `POST /medications/{id}/logs` sebagai `schedule_id`. Itu disengaja:
// slot yang ditampilkan di layar harus bisa langsung dicatat tanpa penerjemahan
// id di klien.
type DoseSlot struct {
	ScheduleID   string
	TimeLocal    string // "HH:MM" waktu lokal pemilik
	Weekday      time.Weekday
	Date         time.Time // tanggal lokal (tengah malam di zona pemilik)
	DueAt        time.Time // waktu lokal saat dosis ini jatuh tempo
	IsPending    bool      // jamnya belum lewat (relatif terhadap `now`)
	DoseOverride *string   // dosis khusus untuk jam ini, bila ada
	TimeLabel    *TimeLabel
	SortOrder    int
}

// ScheduledSlotsInRange mengembalikan SETIAP dosis terjadwal pada rentang
// tanggal, memakai aturan frekuensi dan jam aktif.
//
// Ini satu-satunya tempat aturan "dosis apa yang jatuh pada hari ini"
// diterjemahkan menjadi daftar. `ScheduledDosesInRange` di bawah dibangun di
// atas fungsi ini, sehingga layar Hari Ini dan laporan kepatuhan TIDAK PERNAH
// bisa berbeda pendapat — keduanya membaca daftar yang sama.
//
// Menyusuri hari satu per satu. Untuk rentang laporan (7/30/90 hari) cara ini
// lebih sederhana dan lebih sulit salah dibanding menghitung siklus secara
// analitis. Yang dijaga tetap O(jumlah jam) adalah scheduler, bukan fungsi ini.
func ScheduledSlotsInRange(
	med *Medication, freq *FrequencyParams, times []MedicationTime,
	from, to time.Time, loc *time.Location,
	now time.Time, includeFuture bool,
) []DoseSlot {
	if med == nil || freq == nil || len(times) == 0 || loc == nil {
		return nil
	}
	// Obat PRN tidak punya jadwal: tidak ada dosis yang "jatuh tempo" dengan
	// sendirinya. Dosis PRN hanya dicatat manual saat benar-benar diminum.
	if freq.Pattern == PatternPRN {
		return nil
	}

	nowLocal := now.In(loc)
	var out []DoseSlot

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

			slot := DoseSlot{
				ScheduleID:   t.ID,
				TimeLocal:    t.TimeLocal,
				Weekday:      local.Weekday(),
				Date:         time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc),
				DoseOverride: t.DoseOverride,
				TimeLabel:    t.Label,
				SortOrder:    t.SortOrder,
			}

			if hh, mm, err := ParseClock(t.TimeLocal); err == nil {
				slot.DueAt = time.Date(local.Year(), local.Month(), local.Day(),
					hh, mm, 0, 0, loc)
				// Dosis yang jamnya belum lewat masih menunggu, bukan
				// terlewat. Membedakannya penting: hanya yang terlewat yang
				// masuk penyebut kepatuhan dan memicu notifikasi caregiver.
				slot.IsPending = slot.DueAt.After(nowLocal)
			}

			if !includeFuture && slot.IsPending {
				// Pemanggil minta tanpa dosis mendatang: tetap dikembalikan,
				// tetapi ditandai menunggu. Penyaringannya dilakukan pemanggil
				// supaya ia bisa memutuskan sendiri.
				_ = includeFuture
			}

			out = append(out, slot)
		}
	}

	return out
}

// ScheduledDosesInRange menghitung berapa dosis yang seharusnya ada pada
// rentang tanggal, memakai aturan frekuensi dan jam aktif.
//
// Dibangun di atas ScheduledSlotsInRange supaya hanya ada SATU tempat aturan
// frekuensi diterjemahkan menjadi daftar dosis. Sebelumnya fungsi ini
// menyusuri hari sendiri; dua implementasi aturan yang sama pasti berbeda
// cepat atau lambat, dan user akan melihat angka kepatuhan yang bertentangan
// dengan daftar di layar Hari Ini.
//
// Mengembalikan jumlah terjadwal dan jumlah yang masih menunggu.
func ScheduledDosesInRange(
	med *Medication, freq *FrequencyParams, times []MedicationTime,
	from, to time.Time, loc *time.Location,
	now time.Time, includeFuture bool,
) (scheduled, pending int) {
	slots := ScheduledSlotsInRange(med, freq, times, from, to, loc, now, includeFuture)
	for i := range slots {
		scheduled++
		if slots[i].IsPending {
			pending++
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
