package http

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Alasan verifikasi webhook ditaruh di paket transport, bukan di handler:
// ini memeriksa PERMINTAAN MASUK, bukan pekerjaan latar. Handler hanya perlu
// tahu "terima atau tolak", dan aturan kriptografinya diuji terpisah tanpa
// perlu menjalankan HTTP server.

// Kesalahan verifikasi webhook. Dipisah supaya handler bisa memilih status
// yang tepat dan log bisa menyebut alasan sebenarnya — tanpa membocorkan
// detail itu ke klien.
var (
	// ErrWebhookSecretEmpty = tidak ada secret yang dikonfigurasi. Ini
	// KEADAAN SERVER, bukan kesalahan klien: kalau dibalas 401, operator
	// akan mengira kredensialnya salah padahal webhook memang belum
	// diaktifkan.
	ErrWebhookSecretEmpty = errors.New("secret webhook belum dikonfigurasi")

	// ErrWebhookSignatureMissing = header tanda tangan tidak ada.
	ErrWebhookSignatureMissing = errors.New("header tanda tangan tidak ada")

	// ErrWebhookSignatureMalformed = format header tidak dikenali.
	ErrWebhookSignatureMalformed = errors.New("format tanda tangan tidak dikenali")

	// ErrWebhookSignatureMismatch = tanda tangan tidak cocok dengan isi.
	ErrWebhookSignatureMismatch = errors.New("tanda tangan tidak cocok")

	// ErrWebhookTimestampStale = cap waktu di luar toleransi.
	ErrWebhookTimestampStale = errors.New("cap waktu di luar toleransi")

	// ErrWebhookTimestampMalformed = cap waktu tidak bisa diurai.
	ErrWebhookTimestampMalformed = errors.New("format cap waktu tidak dikenali")
)

// SignaturePrefix adalah awalan skema tanda tangan yang diharapkan.
//
// Format lengkapnya: `sha256=<hex>`. Awalan ini ditulis eksplisit karena
// tanpa itu, sebuah implementasi bisa tergoda menerima hex telanjang, dan
// pemanggil mengira skema lain (mis. sha1) juga diterima.
const SignaturePrefix = "sha256="

// VerifyWebhookHMAC memeriksa tanda tangan HMAC-SHA256 atas raw body.
//
// Parameter:
//   - secret: secret bersama dengan penyedia. Kosong = webhook nonaktif.
//   - rawBody: byte body PERSIS seperti diterima. Body yang sudah di-parse
//     ulang lalu di-marshal kembali bisa berbeda urutan kuncinya, dan tanda
//     tangannya tidak akan pernah cocok.
//   - signature: isi header X-Signature.
//   - timestamp: isi header X-Timestamp (RFC3339 atau detik Unix).
//   - now: waktu saat ini, disuntikkan supaya bisa diuji.
//   - tolerance: rentang waktu yang masih diterima, dua arah.
//
// Perbandingan tanda tangan memakai perbandingan waktu-tetap. Perbandingan
// biasa (`==`) berhenti di byte pertama yang berbeda, dan selisih waktu itu
// cukup untuk merekonstruksi tanda tangan yang benar byte demi byte.
func VerifyWebhookHMAC(secret string, rawBody []byte, signature, timestamp string, now time.Time, tolerance time.Duration) error {
	if strings.TrimSpace(secret) == "" {
		return ErrWebhookSecretEmpty
	}

	signature = strings.TrimSpace(signature)
	if signature == "" {
		return ErrWebhookSignatureMissing
	}

	// Hanya skema sha256 yang diterima. Skema lain ditolak sebagai format
	// yang tidak dikenali, bukan dicoba satu per satu.
	if !strings.HasPrefix(signature, SignaturePrefix) {
		return ErrWebhookSignatureMalformed
	}
	providedHex := strings.TrimPrefix(signature, SignaturePrefix)

	provided, err := hex.DecodeString(providedHex)
	if err != nil {
		return ErrWebhookSignatureMalformed
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(rawBody)
	expected := mac.Sum(nil)

	if !hmac.Equal(provided, expected) {
		return ErrWebhookSignatureMismatch
	}

	// Cap waktu diperiksa SETELAH tanda tangan sah.
	//
	// Urutannya disengaja: pesan error yang berbeda untuk "cap waktu basi"
	// dan "tanda tangan salah" hanya berguna kalau pengirimnya memang tahu
	// secret-nya. Kalau diperiksa lebih dulu, penyerang tanpa secret bisa
	// memakai perbedaan pesan itu untuk menebak isi header yang benar.
	if tolerance > 0 {
		ts, err := parseWebhookTimestamp(timestamp)
		if err != nil {
			return ErrWebhookTimestampMalformed
		}
		diff := now.Sub(ts)
		if diff < 0 {
			diff = -diff
		}
		if diff > tolerance {
			return ErrWebhookTimestampStale
		}
	}

	return nil
}

// parseWebhookTimestamp menerima dua bentuk yang lazim dipakai penyedia:
// RFC3339 dan detik Unix. Menerima keduanya disengaja — memaksa satu bentuk
// membuat integrasi gagal hanya karena perbedaan gaya, bukan karena
// keamanan.
func parseWebhookTimestamp(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, ErrWebhookTimestampMalformed
	}

	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}

	if secs, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return time.Unix(secs, 0), nil
	}

	return time.Time{}, ErrWebhookTimestampMalformed
}

// SignWebhookHMAC menyusun header tanda tangan untuk sebuah body.
//
// Dipakai pengujian dan alat pengembangan untuk MEMBUAT permintaan yang sah.
// Ia sengaja ada di kode produksi, bukan hanya di _test.go, supaya uji
// end-to-end dan skrip operasional memakai perhitungan yang sama persis
// dengan yang diverifikasi — kalau rumusnya ditulis dua kali, cepat atau
// lambat keduanya berbeda.
func SignWebhookHMAC(secret string, rawBody []byte, at time.Time) (signature, timestamp string) {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(rawBody)
	return SignaturePrefix + hex.EncodeToString(mac.Sum(nil)), strconv.FormatInt(at.Unix(), 10)
}

// SecureTokenEqual membandingkan dua token rahasia dengan waktu tetap.
//
// Dipakai webhook Telegram, yang memakai header token statis, bukan HMAC.
// Panjangnya lebih dulu diperiksa ke dalam perbandingan supaya pemanggil
// tidak perlu memikirkannya; subtle memerlukan panjang yang sama.
func SecureTokenEqual(provided, expected string) bool {
	if expected == "" || provided == "" {
		return false
	}
	if len(provided) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

// WebhookErrorStatus memetakan kesalahan verifikasi ke status HTTP.
//
// 401 untuk masalah kredensial, 400 untuk format yang salah, dan 500 untuk
// secret yang belum diatur — itu kesalahan konfigurasi server, dan
// menyalahkan klien karenanya akan menyesatkan operator.
func WebhookErrorStatus(err error) (int, string) {
	switch {
	case errors.Is(err, ErrWebhookSecretEmpty):
		return 500, "webhook belum diaktifkan"
	case errors.Is(err, ErrWebhookSignatureMissing),
		errors.Is(err, ErrWebhookSignatureMismatch),
		errors.Is(err, ErrWebhookTimestampStale):
		return 401, "tanda tangan webhook tidak sah"
	case errors.Is(err, ErrWebhookSignatureMalformed),
		errors.Is(err, ErrWebhookTimestampMalformed):
		return 400, "tanda tangan webhook tidak dikenali"
	default:
		return 400, "permintaan webhook tidak sah"
	}
}

// FormatWebhookReason menghasilkan ringkasan singkat untuk log.
func FormatWebhookReason(err error) string {
	if err == nil {
		return "ok"
	}
	return fmt.Sprintf("%v", err)
}
