// Package auth memuat primitif autentikasi: hash password, token akses, dan
// sesi refresh. Paket ini HANYA boleh mengimpor `domain` dan pustaka standar.
//
// Sengaja tidak menyentuh database. Pemanggilan ke repo dilakukan di service,
// supaya logika token bisa diuji tanpa Postgres.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// Parameter Argon2id. Nilai ini dari docs/trd-v2/TRD-v2.md bagian 7.2 dan
// data-model-v2. Jangan diturunkan tanpa alasan: menurunkan memory atau
// jumlah iterasi melemahkan ketahanan terhadap serangan GPU.
const (
	argon2Memory  uint32 = 64 * 1024 // 64 MiB
	argon2Time    uint32 = 3
	argon2Threads uint8  = 4
	argon2KeyLen  uint32 = 32
	argon2SaltLen        = 16
)

// ErrPasswordMismatch dikembalikan bila password tidak cocok dengan hash.
// Bukan sentinel domain karena ini murni urusan kriptografi.
var ErrPasswordMismatch = errors.New("password tidak cocok")

// HashPassword menghasilkan hash Argon2id berformat PHC.
//
// Format PHC dipilih karena menyertakan parameter di dalam string hash:
//
//	$argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>
//
// Konsekuensinya penting: kalau parameter dinaikkan nanti, password lama
// tetap bisa diverifikasi dengan parameter lamanya, tanpa memaksa semua
// user mengganti password. Verifikasi membaca parameter dari hash, bukan
// dari konstanta, jadi kompatibilitas ke belakang terjaga.
func HashPassword(password string) (string, error) {
	if err := ValidatePasswordStrength(password); err != nil {
		return "", err
	}

	salt := make([]byte, argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("membuat salt: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)

	// base64 tanpa padding, sesuai spesifikasi PHC.
	saltB64 := base64.RawStdEncoding.EncodeToString(salt)
	keyB64 := base64.RawStdEncoding.EncodeToString(key)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argon2Memory, argon2Time, argon2Threads, saltB64, keyB64), nil
}

// VerifyPassword mencocokkan password dengan hash PHC.
//
// Mengembalikan nil bila cocok, ErrPasswordMismatch bila tidak, dan error
// lain bila hash-nya rusak. Parameter dibaca DARI HASH, bukan dari konstanta,
// supaya hash lama dengan parameter berbeda tetap bisa diverifikasi.
func VerifyPassword(password, encodedHash string) error {
	memory, timeCost, threads, salt, wantKey, err := parsePHC(encodedHash)
	if err != nil {
		return err
	}

	gotKey := argon2.IDKey([]byte(password), salt, timeCost, memory, threads, uint32(len(wantKey)))

	// subtle.ConstantTimeCompare, BUKAN bytes.Equal.
	//
	// Perbandingan biasa berhenti di byte pertama yang berbeda, sehingga
	// waktu eksekusinya membocorkan berapa banyak byte awal yang sudah benar.
	// Perbandingan konstan-waktu menutup saluran bocor itu.
	if subtle.ConstantTimeCompare(gotKey, wantKey) != 1 {
		return ErrPasswordMismatch
	}
	return nil
}

// ValidatePasswordStrength menegakkan aturan password minimum.
//
// Aturan disimpan di sini, bukan di handler, supaya registrasi dan ganti
// password memakai batas yang sama persis.
func ValidatePasswordStrength(password string) error {
	const minLen = 8
	const maxLen = 128 // batas atas mencegah biaya hashing jadi vektor DoS

	if len(password) < minLen {
		return &domain.WeakPasswordError{
			Reason: fmt.Sprintf("password minimal %d karakter", minLen),
		}
	}
	if len(password) > maxLen {
		return &domain.WeakPasswordError{
			Reason: fmt.Sprintf("password maksimal %d karakter", maxLen),
		}
	}
	// Sengaja TIDAK mewajibkan simbol/angka. Aturan komposisi mendorong pola
	// seperti "Password1!" yang panjangnya tetap, dan lebih mudah ditebak
	// daripada frasa panjang. Yang ditegakkan hanya panjang minimum.
	return nil
}

// parsePHC mengurai string hash Argon2id berformat PHC.
func parsePHC(encoded string) (memory, timeCost uint32, threads uint8, salt, key []byte, err error) {
	parts := strings.Split(encoded, "$")
	// Format: ["", "argon2id", "v=19", "m=...,t=...,p=...", salt, hash]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return 0, 0, 0, nil, nil, errors.New("format hash bukan argon2id PHC")
	}

	var version int
	if _, err = fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return 0, 0, 0, nil, nil, fmt.Errorf("versi hash tidak terbaca: %w", err)
	}
	if version != argon2.Version {
		return 0, 0, 0, nil, nil, fmt.Errorf("versi argon2 tidak didukung: %d", version)
	}

	var p uint8
	if _, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &timeCost, &p); err != nil {
		return 0, 0, 0, nil, nil, fmt.Errorf("parameter hash tidak terbaca: %w", err)
	}
	threads = p

	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return 0, 0, 0, nil, nil, fmt.Errorf("salt tidak terbaca: %w", err)
	}
	key, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return 0, 0, 0, nil, nil, fmt.Errorf("hash tidak terbaca: %w", err)
	}
	if len(salt) == 0 || len(key) == 0 {
		return 0, 0, 0, nil, nil, errors.New("salt atau hash kosong")
	}
	return memory, timeCost, threads, salt, key, nil
}
