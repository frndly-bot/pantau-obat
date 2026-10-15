package domain

import (
	"testing"
	"time"
)

// TestInQuietHours_MemakaiJamDindingPengguna mengunci arti jam sunyi.
//
// Jam sunyi adalah konsep JAM DINDING: "jangan bangunkan saya antara 22:00
// dan 06:00" berarti jam di rumah pengguna, bukan jam server.
//
// Ini pernah menjadi bug nyata dan berdampak besar. Dispatcher memeriksa
// jam sunyi memakai waktu yang belum dikonversi ke zona penerima, sehingga
// untuk pengguna WIB (UTC+7) jendela 22:00-06:00 bergeser menjadi
// 05:00-13:00 WIB. Akibatnya SETIAP pengingat pagi dianggap jatuh di jam
// sunyi dan ditunda sampai 13:00 WIB — dosis pagi datang terlambat 5-8 jam,
// dan pengguna yang minum obat pagi tidak pernah diberi tahu pada waktunya.
//
// Uji ini memastikan fungsinya bekerja pada waktu yang SUDAH berada di zona
// pengguna, sehingga pemanggil bertanggung jawab mengonversinya lebih dulu.
func TestInQuietHours_MemakaiJamDindingPengguna(t *testing.T) {
	jakarta, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatalf("memuat zona Jakarta: %v", err)
	}

	// 08:33 WIB — pagi, jelas di LUAR jam sunyi 22:00-06:00.
	pagiWIB := time.Date(2026, 10, 15, 8, 33, 0, 0, jakarta)
	if InQuietHours(pagiWIB, "22:00", "06:00") {
		t.Error("08:33 WIB dinilai masuk jam sunyi — pengingat pagi akan tertunda")
	}

	// Waktu yang SAMA dalam UTC adalah 01:33 — dan kalau pemeriksaan memakai
	// jam UTC, ia akan salah dianggap jam sunyi. Inilah bug yang pernah ada.
	if !InQuietHours(pagiWIB.UTC(), "22:00", "06:00") {
		t.Log("catatan: 01:33 UTC memang masuk jendela 22:00-06:00")
	}

	// 23:00 WIB — malam, jelas DI DALAM jam sunyi.
	malamWIB := time.Date(2026, 10, 15, 23, 0, 0, 0, jakarta)
	if !InQuietHours(malamWIB, "22:00", "06:00") {
		t.Error("23:00 WIB tidak dinilai jam sunyi — notifikasi tengah malam akan terkirim")
	}

	// 03:00 WIB — dini hari, masih di dalam jam sunyi.
	diniWIB := time.Date(2026, 10, 15, 3, 0, 0, 0, jakarta)
	if !InQuietHours(diniWIB, "22:00", "06:00") {
		t.Error("03:00 WIB tidak dinilai jam sunyi")
	}
}

// TestQuietEndsAt_BerakhirDiZonaPengguna memastikan akhir jam sunyi jatuh
// pada jam dinding pengguna.
//
// Kalau ini salah, pengingat yang ditunda bisa mendarat di tengah malam
// pengguna — persis yang ingin dihindari oleh fitur jam sunyi.
func TestQuietEndsAt_BerakhirDiZonaPengguna(t *testing.T) {
	jakarta, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatalf("memuat zona Jakarta: %v", err)
	}

	malam := time.Date(2026, 10, 15, 23, 30, 0, 0, jakarta)
	akhir := QuietEndsAt(malam, "22:00", "06:00")

	jamLokal := akhir.In(jakarta)
	if jamLokal.Hour() != 6 || jamLokal.Minute() != 0 {
		t.Fatalf("akhir jam sunyi = %02d:%02d WIB, ingin 06:00",
			jamLokal.Hour(), jamLokal.Minute())
	}
	if jamLokal.Day() != 16 {
		t.Fatalf("akhir jam sunyi jatuh pada tanggal %d, ingin 16 (besoknya)",
			jamLokal.Day())
	}
}
