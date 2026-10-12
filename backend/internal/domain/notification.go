package domain

import "time"

// File ini memuat aturan preferensi notifikasi.
//
// Aturannya ada di sini, bukan di repo maupun di handler, karena ini logika
// waktu murni: jam sunyi bisa melewati tengah malam, dan memajukan
// pengiriman bisa memindahkan dosis ke hari sebelumnya. Keduanya gampang
// salah kalau ditulis sambil lalu.

// ChannelPrefs adalah preferensi satu kanal yang sudah lengkap nilainya.
//
// Berbeda dari baris database yang boleh belum ada: di sini semua kanal
// selalu punya nilai, memakai bawaan bila user belum pernah mengaturnya.
type ChannelPrefs struct {
	Channel        Channel
	Enabled        bool
	QuietStart     string // "HH:MM", kosong berarti tidak ada jam sunyi
	QuietEnd       string
	AdvanceMinutes int
	Throttled      bool
}

// DefaultPrefs mengembalikan preferensi bawaan untuk satu kanal.
//
// Dipakai untuk kanal yang belum punya baris di database. Nilai bawaannya
// sengaja ditaruh di SATU tempat, supaya perilaku kanal yang belum diatur
// tidak berbeda-beda tergantung di mana kodenya membaca.
func DefaultPrefs(ch Channel) ChannelPrefs {
	return ChannelPrefs{
		Channel: ch,
		// Keputusan v0.1: kanal AKTIF hanya in-app. Ia selalu bisa
		// "mengirim" karena isinya dibaca dari inbox aplikasi sendiri —
		// tidak bergantung pada kredensial operator apa pun. Push aktif
		// secara bawaan di versi sebelumnya; aktivasi ditunda ke v0.2
		// bersama email dan Telegram (butuh VAPID/SMTP/token bot).
		Enabled: ch == ChannelInApp,

		// Jam sunyi bawaan 22:00-06:00. Ini keputusan yang perlu disadari:
		// menahan notifikasi tengah malam lebih baik daripada membuat user
		// mematikan izin notifikasi sepenuhnya setelah dibangunkan sekali.
		QuietStart: "22:00",
		QuietEnd:   "06:00",

		// Tanpa maju-lebih-awal. Mengirim pengingat sebelum waktunya bisa
		// membuat obat diminum terlalu awal.
		AdvanceMinutes: 0,
	}
}

// AllChannels adalah daftar kanal yang didukung.
var AllChannels = []Channel{ChannelPush, ChannelEmail, ChannelTelegram, ChannelInApp}

// InQuietHours melaporkan apakah waktu lokal berada dalam jam sunyi.
//
// MENANGANI JENDELA YANG MELEWATI TENGAH MALAM. Kasus 22:00-06:00 adalah
// yang paling lazim, dan perbandingan sederhana "start <= now && now < end"
// akan selalu salah untuk kasus itu: tidak ada waktu yang memenuhi keduanya.
//
// Aturannya:
//
//	start < end   -> jendela biasa, mis. 13:00-15:00
//	start > end   -> jendela melewati tengah malam, mis. 22:00-06:00
//	start == end  -> TIDAK ada jam sunyi, bukan 24 jam
//
// Kasus start == end sengaja berarti "tidak ada jam sunyi", karena user yang
// mengisi jam mulai dan selesai sama jelas tidak berniat menahan notifikasi
// sepanjang hari.
func InQuietHours(now time.Time, quietStart, quietEnd string) bool {
	if quietStart == "" || quietEnd == "" {
		return false
	}
	if quietStart == quietEnd {
		return false
	}

	startH, startM, err := ParseClock(quietStart)
	if err != nil {
		return false
	}
	endH, endM, err := ParseClock(quietEnd)
	if err != nil {
		return false
	}

	nowMinutes := now.Hour()*60 + now.Minute()
	startMinutes := startH*60 + startM
	endMinutes := endH*60 + endM

	if startMinutes < endMinutes {
		// Jendela biasa dalam satu hari.
		return nowMinutes >= startMinutes && nowMinutes < endMinutes
	}

	// Jendela melewati tengah malam: dua rentang, sebelum dan sesudah tidur.
	return nowMinutes >= startMinutes || nowMinutes < endMinutes
}

// QuietEndsAt menghitung kapan jam sunyi berakhir, dihitung dari `now`.
//
// Dipakai untuk menunda pengiriman: notifikasi yang jatuh di jam sunyi
// disimpan sampai jendelanya lewat, bukan dibuang. Pengingat minum obat
// yang hilang lebih buruk daripada pengingat yang datang terlambat.
func QuietEndsAt(now time.Time, quietStart, quietEnd string) time.Time {
	if !InQuietHours(now, quietStart, quietEnd) {
		return now
	}

	endH, endM, err := ParseClock(quietEnd)
	if err != nil {
		return now
	}

	end := time.Date(now.Year(), now.Month(), now.Day(), endH, endM, 0, 0, now.Location())
	if !end.After(now) {
		// Jam sunyi melewati tengah malam, jadi berakhir besok.
		end = end.AddDate(0, 0, 1)
	}
	return end
}

// EffectiveSendTime menghitung kapan sebuah dosis sebaiknya dikirim.
//
// Dua penyesuaian diterapkan berurutan, dan urutannya penting:
//
//  1. Maju lebih awal (advance_minutes) memindahkan waktu mundur.
//  2. Jam sunyi menahannya sampai jendela sunyi lewat.
//
// Kalau urutannya dibalik, dosis pukul 07:00 dengan advance 10 menit akan
// dimajukan ke 06:50 SETELAH diperiksa terhadap jam sunyi yang berakhir
// 06:00, sehingga lolos — padahal kalau diperiksa sesudahnya, ia memang
// sudah di luar jendela. Urutan ini yang membuat hasilnya dapat diprediksi.
func EffectiveSendTime(dueAt time.Time, prefs ChannelPrefs) time.Time {
	send := dueAt
	if prefs.AdvanceMinutes > 0 {
		send = send.Add(-time.Duration(prefs.AdvanceMinutes) * time.Minute)
	}
	if InQuietHours(send, prefs.QuietStart, prefs.QuietEnd) {
		send = QuietEndsAt(send, prefs.QuietStart, prefs.QuietEnd)
	}
	return send
}

// ChannelEnabled melaporkan apakah kanal boleh dipakai untuk user ini.
//
// Kanal yang sedang ditahan (throttled) dianggap tidak aktif: pengiriman ke
// penyedia yang sedang bermasalah hanya membuang percobaan dan merusak
// reputasi pengirim.
func ChannelEnabled(p ChannelPrefs) bool {
	return p.Enabled && !p.Throttled
}

// SnoozeOptions adalah pilihan tunda yang diizinkan, sesuai TRD bagian D2.
var SnoozeOptions = []int{5, 10, 30}

// IsValidSnooze melaporkan apakah durasi tunda diizinkan.
func IsValidSnooze(minutes int) bool {
	for _, m := range SnoozeOptions {
		if m == minutes {
			return true
		}
	}
	return false
}

// MaxConsecutiveSnoozes adalah batas tunda beruntun per dosis.
//
// Setelah batas ini, dosis dianggap terlewat. Tanpa batas, pengingatnya
// berputar tanpa akhir dan justru melatih user untuk mengabaikannya.
const MaxConsecutiveSnoozes = 3

// MissedEscalationThreshold adalah berapa dosis terlewat berturut-turut
// sebelum caregiver diberi tahu, sesuai TRD bagian F4.
const MissedEscalationThreshold = 3
