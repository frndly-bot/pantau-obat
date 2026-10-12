package service

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	"github.com/frndly-bot/pantau-obat/backend/internal/repo/postgres"
)

// File ini menangani ekspor riwayat pemakaian (TRD H2).
//
// Tujuan ekspor adalah DIBAWA KE DOKTER. Karena itu:
//   - Format CSV, bukan JSON — dibuka di spreadsheet tanpa alat tambahan.
//   - Nama obat dan dosis ditulis lengkap — inti laporannya.
//   - Baris disusun streaming: riwayat seumur hidup bisa jutaan baris, dan
//     memuat semuanya ke memori hanya untuk dijadikan CSV adalah pemborosan
//     yang bisa menghantam batas memori container.
//
// Akses: HANYA milik sendiri (TRD H2 AC kedua). Caregiver tidak punya jalur
// ekspor — catatan kesehatan keluar dari sistem hanya atas keputusan pasien.

// csvHeader adalah baris pertama file CSV.
//
// Nama kolom bahasa Indonesia, karena pembacanya dokter di Indonesia. Nilai
// enum di dalam sel tetap Inggris (taken, skipped) — menerjemahkan nilai data
// membuat pemrosesan ulang file jadi menebak-nebak.
var csvHeader = []string{
	"tanggal", "jam_jadwal", "obat", "dosis", "status",
	"jam_diminum", "sumber", "catatan",
}

// ExportCSV menulis riwayat pemakaian milik ownerID dalam rentang
// [from, to] (inklusif, tanggal kalender) ke w sebagai CSV.
//
// Rentang WAJIB dan dibatasi maksimal 366 hari: ekspor tanpa batas adalah
// jalan pintas yang bisa menyeret seluruh tabel ke satu response, dan laporan
// ke dokter praktisnya tidak pernah lebih dari setahun.
//
// Error yang dikembalikan setelah sebagian body tertulis TIDAK bisa
// diperbaiki — header sudah terkirim. Pemanggil (handler) karena itu
// memvalidasi dulu, lalu memanggil fungsi ini sebagai operasi yang dianggap
// tidak gagal di tengah jalan kecuali database benar-benar mati.
func (s *Logs) ExportCSV(ctx context.Context, w io.Writer, ownerID string, from, to time.Time) error {
	if to.Sub(from) > 366*24*time.Hour {
		return fmt.Errorf("%w: rentang ekspor maksimal 366 hari", domain.ErrValidation)
	}
	if to.Before(from) {
		return fmt.Errorf("%w: tanggal akhir tidak boleh sebelum tanggal awal", domain.ErrValidation)
	}

	// BOM UTF-8 supaya Excel membaca file sebagai UTF-8 tanpa dialog impor.
	if _, err := w.Write([]byte("\xEF\xBB\xBF")); err != nil {
		return fmt.Errorf("menulis bom: %w", err)
	}
	writeCSVRow(w, csvHeader)

	return s.db.ExportLogs(ctx, ownerID, from, to, func(r postgres.ExportLogRow) error {
		writeCSVRow(w, []string{
			r.SlotDate.Format(time.DateOnly),
			schedClock(r.ScheduledHour, r.ScheduledMin),
			csvCell(r.MedicationName),
			doseText(r.DoseValue, r.DoseUnit),
			r.Status,
			takenClock(r.TakenAt),
			r.Source,
			noteText(r.Note),
		})
		return nil
	})
}

// writeCSVRow menulis satu baris CSV dengan pemisah koma dan pemisah baris
// CRLF (RFC 4180 — yang dibaca Excel).
func writeCSVRow(w io.Writer, cells []string) {
	for i, c := range cells {
		if i > 0 {
			_, _ = io.WriteString(w, ",")
		}
		_, _ = io.WriteString(w, c)
	}
	_, _ = io.WriteString(w, "\r\n")
}

// csvCell memberi kutip ganda pada sel yang memuat koma, kutip, atau baris
// baru — sesuai RFC 4180. Kutip di dalam sel digandakan.
func csvCell(s string) string {
	needsQuote := false
	for _, c := range s {
		if c == ',' || c == '"' || c == '\n' || c == '\r' {
			needsQuote = true
			break
		}
	}
	if !needsQuote {
		return s
	}
	quoted := ""
	for _, c := range s {
		if c == '"' {
			quoted += `""`
			continue
		}
		quoted += string(c)
	}
	return `"` + quoted + `"`
}

// schedClock menyusun "HH:MM" jadwal, atau kosong untuk PRN.
func schedClock(hour, minute *int16) string {
	if hour == nil || minute == nil {
		return ""
	}
	return fmt.Sprintf("%02d:%02d", *hour, *minute)
}

// takenClock menyusun waktu diminum dalam zona tanggal slotnya, atau kosong.
//
// Timestamp disimpan UTC; menampilkannya UTC membuat "jam diminum" bisa
// bergeser sehari untuk user WIB yang minum pukul 23:30. Format tanggal
// kalender + jam (bukan RFC3339) menghindari kebingungan itu.
func takenClock(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02 15:04")
}

// doseText menyusun "1 tablet" — nilai dan satuannya dipisah titik.
func doseText(value float64, unit string) string {
	v := strconv.FormatFloat(value, 'f', -1, 64)
	if unit == "" {
		return v
	}
	return v + " " + unit
}

// noteText mengosongkan catatan kosong dan memberi kutip pada sisanya
// (catatan bebas, paling mungkin memuat koma).
func noteText(n *string) string {
	if n == nil || *n == "" {
		return ""
	}
	return csvCell(*n)
}
