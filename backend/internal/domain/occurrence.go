package domain

import (
	"fmt"
	"time"
)

// File ini memegang SATU-SATUNYA fungsi pembentuk occurrence key.
//
// Kenapa harus satu fungsi: kunci ini dipakai di tiga tempat berbeda —
// medication_logs (lewat partial unique index uq_log_occurrence),
// medication_dose_events (lewat PRIMARY KEY event_key), dan bump_queue
// (lewat dedup_key). Kalau formatnya beda di salah satu tempat, anti-ganda
// GAGAL: dosis yang sama bisa tercatat dua kali, stok bisa berkurang dua kali.
//
// Jangan pernah menyusun string kunci ini di tempat lain. Selalu panggil
// OccurrenceKey / ParseOccurrenceKey.

// OccurrenceKey menyusun identitas satu dosis terjadwal.
//
// Format: "<schedule_id>|<YYYY-MM-DD>|<HH>:<MM>"
// Contoh: "019d1...|2026-10-10|07:00"
//
// Parameter date adalah tanggal LOKAL pemilik (slot_date), bukan UTC.
// Jam dan menit diambil dari jam lokal jadwal.
func OccurrenceKey(scheduleID string, date time.Time, hour, minute int) string {
	return fmt.Sprintf("%s|%s|%02d:%02d", scheduleID, date.Format(time.DateOnly), hour, minute)
}

// OccurrenceKeyFromTime adalah bentuk praktis bila jam sudah tersedia
// sebagai time.Time di zona pemilik.
func OccurrenceKeyFromTime(scheduleID string, local time.Time) string {
	return OccurrenceKey(scheduleID, local, local.Hour(), local.Minute())
}

// OccurrenceParts adalah hasil penguraian occurrence key.
type OccurrenceParts struct {
	ScheduleID string
	Date       time.Time // UTC midnight; hanya tanggalnya yang bermakna
	Hour       int
	Minute     int
}

// ParseOccurrenceKey menguraikan kembali kunci menjadi bagiannya.
// Dipakai untuk debug dan pengujian, bukan untuk jalur panas.
func ParseOccurrenceKey(key string) (OccurrenceParts, error) {
	var p OccurrenceParts
	var dateStr, clock string

	n, err := fmt.Sscanf(key, "%36s", &p.ScheduleID)
	if err != nil || n != 1 {
		return p, fmt.Errorf("occurrence key tidak bisa diurai: %q", key)
	}

	// Sscanf dengan %s berhenti di spasi, bukan di '|', jadi uraikan manual.
	parts := splitN(key, '|', 3)
	if len(parts) != 3 {
		return p, fmt.Errorf("occurrence key harus punya 3 bagian, dapat %d: %q", len(parts), key)
	}
	p.ScheduleID, dateStr, clock = parts[0], parts[1], parts[2]

	d, err := time.Parse(time.DateOnly, dateStr)
	if err != nil {
		return p, fmt.Errorf("tanggal pada occurrence key tidak valid: %q", dateStr)
	}
	p.Date = d

	var h, m int
	if _, err := fmt.Sscanf(clock, "%d:%d", &h, &m); err != nil {
		return p, fmt.Errorf("jam pada occurrence key tidak valid: %q", clock)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return p, fmt.Errorf("jam di luar rentang pada occurrence key: %q", clock)
	}
	p.Hour, p.Minute = h, m
	return p, nil
}

func splitN(s string, sep rune, n int) []string {
	var out []string
	start := 0
	for i, r := range s {
		if r == sep && len(out) < n-1 {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}
