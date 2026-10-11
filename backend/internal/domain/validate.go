package domain

import (
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"
)

// Batas panjang yang dipakai berulang di validasi. Nilainya harus sama
// dengan CHECK constraint di migrasi.
const (
	FullNameMinLen = 1
	FullNameMaxLen = 100
	PhoneMinLen    = 6
	PhoneMaxLen    = 20
	EmailMaxLen    = 254 // batas praktis RFC 5321
)

// NormalizeEmail merapikan dan memeriksa format email.
//
// Huruf besar/kecil TIDAK diubah di sini. Kolomnya bertipe CITEXT, jadi
// perbandingan di database sudah tidak peduli huruf besar/kecil. Mengubahnya
// di Go akan membuat nilai yang disimpan berbeda dari yang diketik user,
// tanpa manfaat.
func NormalizeEmail(raw string) (string, error) {
	email := strings.TrimSpace(raw)
	if email == "" {
		return "", &EmailError{Reason: "email wajib diisi"}
	}
	if len(email) > EmailMaxLen {
		return "", &EmailError{Reason: "email terlalu panjang"}
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return "", &EmailError{Reason: "format email tidak valid"}
	}
	// mail.ParseAddress menerima "Nama <a@b.c>"; kita hanya mau alamatnya.
	if strings.ContainsAny(email, "<>") {
		return "", &EmailError{Reason: "email tidak boleh memuat nama tampilan"}
	}
	if !strings.Contains(email, "@") {
		return "", &EmailError{Reason: "format email tidak valid"}
	}
	return email, nil
}

// ValidateFullName memeriksa nama lengkap.
func ValidateFullName(raw string) error {
	name := strings.TrimSpace(raw)
	if name == "" {
		return &NameError{Reason: "nama lengkap wajib diisi"}
	}
	n := utf8.RuneCountInString(name)
	if n < FullNameMinLen {
		return &NameError{Reason: "nama lengkap wajib diisi"}
	}
	if n > FullNameMaxLen {
		return &NameError{Reason: "nama lengkap maksimal 100 karakter"}
	}
	return nil
}

// ValidatePhone memeriksa nomor telepon.
//
// Diterima: digit, spasi, tanda hubung, tanda kurung, dan awalan +.
// Tidak ada parsing negara; nomor disimpan apa adanya supaya tidak ada
// nomor yang berubah diam-diam.
func ValidatePhone(raw string) error {
	phone := strings.TrimSpace(raw)
	n := len(phone)
	if n < PhoneMinLen || n > PhoneMaxLen {
		return &PhoneError{Reason: "nomor telepon harus 6 sampai 20 karakter"}
	}
	digits := 0
	for i, r := range phone {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case r == ' ' || r == '-' || r == '(' || r == ')':
			// pemisah, boleh
		case r == '+' && i == 0:
			// awalan negara, hanya di posisi pertama
		default:
			return &PhoneError{Reason: "nomor telepon hanya boleh memuat angka, spasi, - ( ) dan + di awal"}
		}
	}
	if digits < 6 {
		return &PhoneError{Reason: "nomor telepon minimal memuat 6 angka"}
	}
	return nil
}

// PhoneError menjelaskan kenapa nomor telepon ditolak.
type PhoneError struct{ Reason string }

func (e *PhoneError) Error() string { return e.Reason }

// ValidateTimezone memastikan zona waktu dikenal IANA.
//
// Kenapa penting: slot_date dihitung backend dari zona ini. Zona yang tidak
// dikenal akan membuat perhitungan jadwal gagal atau, lebih buruk, memakai
// UTC diam-diam dan menggeser semua pengingat.
func ValidateTimezone(tz string) error {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		return &TimezoneError{Reason: "zona waktu wajib diisi"}
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return &TimezoneError{Reason: "zona waktu tidak dikenal"}
	}
	return nil
}

// DefaultTimezone dipakai bila user tidak memilih.
const DefaultTimezone = "Asia/Jakarta"
