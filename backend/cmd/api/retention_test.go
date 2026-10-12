package main

import (
	"testing"
	"time"
)

// TestNextRetentionRun menguji perhitungan waktu jalan berikutnya.
//
// Ini bagian yang paling mudah salah: pembersihan harus berjalan SEKALI
// sehari, dan kesalahan di sini tidak terlihat sampai berhari-hari kemudian —
// entah pembersihan berjalan dua kali, atau tidak pernah berjalan.
func TestNextRetentionRun(t *testing.T) {
	cases := []struct {
		name     string
		now      time.Time
		hour     int
		want     time.Time
		wantNote string
	}{
		{
			name: "jam belum lewat hari ini → hari ini",
			now:  time.Date(2026, 10, 12, 1, 30, 0, 0, time.UTC),
			hour: 3,
			want: time.Date(2026, 10, 12, 3, 0, 0, 0, time.UTC),
		},
		{
			name: "jam sudah lewat → besok",
			now:  time.Date(2026, 10, 12, 4, 0, 0, 0, time.UTC),
			hour: 3,
			want: time.Date(2026, 10, 13, 3, 0, 0, 0, time.UTC),
		},
		{
			name: "tepat pada jamnya → besok, bukan sekarang",
			// Penting: bila "sekarang" dianggap sah, loop akan menjalankan
			// pembersihan dua kali berturut-turut (sekarang dan besok pada
			// jam yang sama), atau berputar tanpa tidur.
			now:  time.Date(2026, 10, 12, 3, 0, 0, 0, time.UTC),
			hour: 3,
			want: time.Date(2026, 10, 13, 3, 0, 0, 0, time.UTC),
		},
		{
			name: "melewati batas bulan",
			now:  time.Date(2026, 10, 31, 23, 0, 0, 0, time.UTC),
			hour: 3,
			want: time.Date(2026, 11, 1, 3, 0, 0, 0, time.UTC),
		},
		{
			name: "melewati batas tahun",
			now:  time.Date(2026, 12, 31, 23, 30, 0, 0, time.UTC),
			hour: 3,
			want: time.Date(2027, 1, 1, 3, 0, 0, 0, time.UTC),
		},
		{
			name: "tahun kabisat: 28 Feb → 29 Feb",
			now:  time.Date(2028, 2, 28, 23, 0, 0, 0, time.UTC),
			hour: 3,
			want: time.Date(2028, 2, 29, 3, 0, 0, 0, time.UTC),
		},
		{
			name: "waktu lokal dikonversi ke UTC",
			// Pemanggil memberi waktu lokal WIB (UTC+7). Pukul 01:00 WIB
			// adalah 18:00 UTC hari SEBELUMNYA, jadi jam 3 UTC masih di
			// depan dan jadwalnya hari itu juga dalam UTC.
			now:  time.Date(2026, 10, 12, 1, 0, 0, 0, time.FixedZone("WIB", 7*3600)),
			hour: 3,
			want: time.Date(2026, 10, 12, 3, 0, 0, 0, time.UTC),
		},
		{
			name: "jam 0 (tengah malam UTC)",
			now:  time.Date(2026, 10, 12, 0, 30, 0, 0, time.UTC),
			hour: 0,
			want: time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := nextRetentionRun(tc.now, tc.hour)
			if !got.Equal(tc.want) {
				t.Errorf("nextRetentionRun(%s, %d) = %s, mau %s",
					tc.now.Format(time.RFC3339), tc.hour,
					got.Format(time.RFC3339), tc.want.Format(time.RFC3339))
			}
			// Sifat yang harus selalu berlaku: hasilnya di MASA DEPAN.
			// Kalau tidak, loop akan berputar tanpa tidur.
			if !got.After(tc.now.UTC()) {
				t.Errorf("hasil %s tidak di masa depan (sekarang %s) — loop akan berputar tanpa jeda",
					got.Format(time.RFC3339), tc.now.UTC().Format(time.RFC3339))
			}
		})
	}
}

// TestRetentionHour memastikan nilai lingkungan yang tidak sah tidak membuat
// layanan pemeliharaan gagal start.
func TestRetentionHour(t *testing.T) {
	cases := []struct {
		env  string
		want int
	}{
		{"", 3},
		{"0", 0},
		{"23", 23},
		{"12", 12},
		{"24", 3},  // di luar rentang → bawaan
		{"-1", 3},  // negatif → bawaan
		{"abc", 3}, // bukan angka → bawaan
		{"3.5", 3}, // pecahan → bawaan
	}

	for _, tc := range cases {
		t.Run("RETENTION_HOUR="+tc.env, func(t *testing.T) {
			t.Setenv("RETENTION_HOUR", tc.env)
			if got := retentionHour(); got != tc.want {
				t.Errorf("retentionHour() dengan %q = %d, mau %d", tc.env, got, tc.want)
			}
		})
	}
}
