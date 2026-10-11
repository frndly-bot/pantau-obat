package domain

import (
	"testing"
	"time"
)

func clockAt(h, m int) time.Time {
	return time.Date(2026, time.October, 10, h, m, 0, 0, testLoc)
}

func TestInQuietHours_NormalWindow(t *testing.T) {
	// Jendela biasa dalam satu hari: 13:00-15:00.
	cases := []struct {
		h, m int
		want bool
	}{
		{12, 59, false},
		{13, 0, true}, // batas awal inklusif
		{14, 0, true},
		{14, 59, true},
		{15, 0, false}, // batas akhir eksklusif
		{16, 0, false},
	}
	for _, c := range cases {
		got := InQuietHours(clockAt(c.h, c.m), "13:00", "15:00")
		if got != c.want {
			t.Errorf("%02d:%02d harus %v, dapat %v", c.h, c.m, c.want, got)
		}
	}
}

func TestInQuietHours_CrossesMidnight(t *testing.T) {
	// Kasus yang paling lazim dan paling mudah salah: 22:00-06:00.
	// Perbandingan sederhana "start <= now < end" akan SELALU false di sini,
	// karena tidak ada waktu yang memenuhi keduanya.
	cases := []struct {
		h, m int
		want bool
	}{
		{21, 59, false},
		{22, 0, true}, // mulai
		{23, 30, true},
		{0, 0, true}, // tengah malam
		{3, 0, true},
		{5, 59, true},
		{6, 0, false}, // berakhir
		{7, 0, false},
		{12, 0, false},
	}
	for _, c := range cases {
		got := InQuietHours(clockAt(c.h, c.m), "22:00", "06:00")
		if got != c.want {
			t.Errorf("%02d:%02d harus %v, dapat %v", c.h, c.m, c.want, got)
		}
	}
}

func TestInQuietHours_SameStartEndMeansNoQuiet(t *testing.T) {
	// start == end berarti TIDAK ada jam sunyi, bukan 24 jam.
	// User yang mengisi keduanya sama jelas tidak berniat menahan
	// notifikasi sepanjang hari.
	for h := 0; h < 24; h += 3 {
		if InQuietHours(clockAt(h, 0), "22:00", "22:00") {
			t.Fatalf("%02d:00 tidak boleh dalam jam sunyi", h)
		}
	}
}

func TestInQuietHours_EmptyMeansNoQuiet(t *testing.T) {
	if InQuietHours(clockAt(23, 0), "", "") {
		t.Error("jam sunyi kosong berarti tidak ada jam sunyi")
	}
	if InQuietHours(clockAt(23, 0), "22:00", "") {
		t.Error("hanya satu sisi terisi berarti tidak ada jam sunyi")
	}
}

func TestQuietEndsAt_CrossesMidnightGoesToTomorrow(t *testing.T) {
	// Pukul 23:00 dalam jam sunyi 22:00-06:00 berakhir BESOK pukul 06:00.
	end := QuietEndsAt(clockAt(23, 0), "22:00", "06:00")
	if end.Day() != 11 || end.Hour() != 6 || end.Minute() != 0 {
		t.Errorf("harus 11 Okt 06:00, dapat %s", end.Format(time.RFC3339))
	}
}

func TestQuietEndsAt_SameDay(t *testing.T) {
	// Pukul 13:30 dalam jam sunyi 13:00-15:00 berakhir HARI INI 15:00.
	end := QuietEndsAt(clockAt(13, 30), "13:00", "15:00")
	if end.Day() != 10 || end.Hour() != 15 {
		t.Errorf("harus 10 Okt 15:00, dapat %s", end.Format(time.RFC3339))
	}
}

func TestQuietEndsAt_OutsideQuietReturnsNow(t *testing.T) {
	now := clockAt(12, 0)
	end := QuietEndsAt(now, "22:00", "06:00")
	if !end.Equal(now) {
		t.Errorf("di luar jam sunyi harus mengembalikan waktu yang sama")
	}
}

func TestEffectiveSendTime_AdvanceThenQuiet(t *testing.T) {
	// Urutan penyesuaian: maju lebih awal DULU, baru jam sunyi.
	// Dosis 07:00 dengan advance 10 menit -> 06:50, di luar jam sunyi
	// yang berakhir 06:00. Jadi tetap 06:50.
	prefs := ChannelPrefs{
		Channel: ChannelPush, Enabled: true,
		QuietStart: "22:00", QuietEnd: "06:00", AdvanceMinutes: 10,
	}
	got := EffectiveSendTime(clockAt(7, 0), prefs)
	if got.Hour() != 6 || got.Minute() != 50 {
		t.Errorf("harus 06:50, dapat %s", got.Format("15:04"))
	}
}

func TestEffectiveSendTime_QuietPushesToEnd(t *testing.T) {
	// Dosis 23:00 masuk jam sunyi, jadi ditahan sampai 06:00 besok.
	prefs := ChannelPrefs{
		Channel: ChannelPush, Enabled: true,
		QuietStart: "22:00", QuietEnd: "06:00", AdvanceMinutes: 0,
	}
	got := EffectiveSendTime(clockAt(23, 0), prefs)
	if got.Day() != 11 || got.Hour() != 6 {
		t.Errorf("harus 11 Okt 06:00, dapat %s", got.Format(time.RFC3339))
	}
}

func TestEffectiveSendTime_AdvanceIntoQuietGetsHeld(t *testing.T) {
	// Dosis 06:05 dengan advance 10 menit -> 05:55, MASUK jam sunyi yang
	// berakhir 06:00. Jadi ditahan sampai 06:00.
	prefs := ChannelPrefs{
		Channel: ChannelPush, Enabled: true,
		QuietStart: "22:00", QuietEnd: "06:00", AdvanceMinutes: 10,
	}
	got := EffectiveSendTime(clockAt(6, 5), prefs)
	if got.Hour() != 6 || got.Minute() != 0 {
		t.Errorf("harus ditahan sampai 06:00, dapat %s", got.Format("15:04"))
	}
}

func TestDefaultPrefs_PushEnabledOthersNot(t *testing.T) {
	// Push aktif bawaan: aplikasi pengingat obat yang tidak mengirim
	// pengingat tidak ada gunanya. Kanal lain butuh langkah tambahan user.
	push := DefaultPrefs(ChannelPush)
	if !push.Enabled {
		t.Error("push harus aktif secara bawaan")
	}
	for _, ch := range []Channel{ChannelEmail, ChannelTelegram} {
		if DefaultPrefs(ch).Enabled {
			t.Errorf("kanal %s tidak boleh aktif secara bawaan", ch)
		}
	}
	// Jam sunyi bawaan harus terisi, supaya tidak ada yang dibangunkan
	// tengah malam pada hari pertama pakai.
	if push.QuietStart == "" || push.QuietEnd == "" {
		t.Error("jam sunyi bawaan harus terisi")
	}
}

func TestChannelEnabled_ThrottledIsDisabled(t *testing.T) {
	p := ChannelPrefs{Channel: ChannelPush, Enabled: true, Throttled: false}
	if !ChannelEnabled(p) {
		t.Error("kanal aktif dan tidak ditahan harus dianggap aktif")
	}
	p.Throttled = true
	if ChannelEnabled(p) {
		t.Error("kanal yang ditahan harus dianggap tidak aktif")
	}
	p.Throttled = false
	p.Enabled = false
	if ChannelEnabled(p) {
		t.Error("kanal yang dimatikan user harus dianggap tidak aktif")
	}
}

func TestIsValidSnooze(t *testing.T) {
	for _, m := range []int{5, 10, 30} {
		if !IsValidSnooze(m) {
			t.Errorf("%d menit harus diizinkan", m)
		}
	}
	for _, m := range []int{0, 1, 7, 15, 60, -5} {
		if IsValidSnooze(m) {
			t.Errorf("%d menit tidak boleh diizinkan", m)
		}
	}
}
