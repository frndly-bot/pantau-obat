package domain

import (
	"errors"
	"time"
)

// EmailError menjelaskan kenapa email ditolak.
type EmailError struct{ Reason string }

func (e *EmailError) Error() string { return e.Reason }

// NameError menjelaskan kenapa nama ditolak.
type NameError struct{ Reason string }

func (e *NameError) Error() string { return e.Reason }

// TimezoneError menjelaskan kenapa zona waktu ditolak.
type TimezoneError struct{ Reason string }

func (e *TimezoneError) Error() string { return e.Reason }

// Input registrasi dan login sengaja diletakkan di domain, bukan di handler
// transport. Alasannya: paket `auth` boleh mengimpor `domain` tapi tidak
// boleh mengimpor `transport`. Kalau input didefinisikan di transport,
// paket auth tidak akan bisa memvalidasinya tanpa melanggar aturan impor.

// RegisterInput adalah data pendaftaran akun baru.
type RegisterInput struct {
	Email    string
	Password string
	FullName string
	Timezone string
	Phone    string
}

// Validate memeriksa input registrasi.
//
// Memeriksa SEMUA masalah, bukan berhenti di yang pertama, supaya user
// tidak memperbaiki satu per satu.
func (in *RegisterInput) Validate() error {
	normalized, err := NormalizeEmail(in.Email)
	if err != nil {
		return err
	}
	in.Email = normalized

	if err := ValidateFullName(in.FullName); err != nil {
		return err
	}
	if err := ValidateTimezone(in.Timezone); err != nil {
		return err
	}
	if in.Phone != "" {
		if err := ValidatePhone(in.Phone); err != nil {
			return err
		}
	}
	return nil
}

// LoginInput adalah data masuk.
type LoginInput struct {
	Email    string
	Password string
}

// ChangePasswordInput adalah data ganti password.
type ChangePasswordInput struct {
	CurrentPassword string
	NewPassword     string
}

// LogoutInput menentukan apakah hanya sesi ini atau semua perangkat.
type LogoutInput struct {
	AllDevices bool
}

// AuthResult adalah hasil login/registrasi yang sukses.
type AuthResult struct {
	User         *User
	AccessToken  string
	RefreshToken string
	ExpiresIn    int // detik
}

// SessionInfo adalah metadata sesi untuk daftar perangkat.
type SessionInfo struct {
	ID        string
	CreatedAt time.Time
	ExpiresAt time.Time
	IP        *string
	UserAgent *string
}

// ErrSessionNotFound menandai sesi refresh tidak ada atau sudah dicabut.
// Dipisah dari ErrTokenRevoked supaya service bisa membedakan "token tidak
// pernah ada" dari "token sudah dipakai lalu dicabut" saat mendeteksi reuse.
var ErrSessionNotFound = errors.New("sesi tidak ditemukan")

// ErrAccountInactive menandai akun yang dinonaktifkan admin.
var ErrAccountInactive = errors.New("akun tidak aktif")
