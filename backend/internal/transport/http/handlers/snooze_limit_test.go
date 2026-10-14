package handlers

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	pohttp "github.com/frndly-bot/pantau-obat/backend/internal/transport/http"
)

// TestSnoozeLimit_MemakaiKonstantaDomain menjaga agar batas penundaan
// benar-benar berasal dari satu sumber.
//
// Latar belakang: `MaxConsecutiveSnoozes = 3` dideklarasikan di
// `domain/notification.go` dan komentar di `service/logs.go` menyebut
// "batas tiga snooze beruntun", TAPI tidak ada yang memeriksanya. Pengingat
// bisa ditunda tanpa batas, dan banner di aplikasi sempat menjanjikan aturan
// yang tidak berjalan.
//
// Uji ini mengunci dua hal:
//  1. Konstanta domain bernilai 3 (TRD bagian D2).
//  2. Pesan galatnya bisa dibedakan klien dari kesalahan validasi biasa,
//     karena akibatnya berbeda bagi pengguna.
func TestSnoozeLimit_MemakaiKonstantaDomain(t *testing.T) {
	if domain.MaxConsecutiveSnoozes != 3 {
		t.Fatalf("MaxConsecutiveSnoozes = %d, TRD D2 menetapkan 3",
			domain.MaxConsecutiveSnoozes)
	}
}

// TestSnoozeLimit_TerpisahDariValidasi memastikan penolakan karena batas
// tidak dilebur menjadi kesalahan validasi biasa.
//
// Bila keduanya sama, klien akan menyuruh pengguna "perbaiki isian" padahal
// isiannya sudah benar — yang terjadi adalah dosisnya sudah ditunda tiga kali
// dan kini dianggap terlewat. Dua hal itu butuh pesan yang berbeda.
func TestSnoozeLimit_TerpisahDariValidasi(t *testing.T) {
	if errors.Is(domain.ErrSnoozeLimit, domain.ErrValidation) {
		t.Fatal("ErrSnoozeLimit tidak boleh sama dengan ErrValidation")
	}

	status, code, _ := pohttp.MapDomainError(domain.ErrSnoozeLimit)
	if status != http.StatusTooManyRequests {
		t.Fatalf("status = %d, ingin 429 (batas percobaan, bukan input salah)", status)
	}
	if code != "SNOOZE_LIMIT" {
		t.Fatalf("code = %q, ingin SNOOZE_LIMIT", code)
	}
}

// TestSnoozeLimit_PesanMenjelaskanAkibat memastikan pesannya menyebut APA
// YANG TERJADI, bukan sekadar "terlalu banyak permintaan".
//
// Pengguna yang membaca "terlalu banyak permintaan" akan mencoba lagi nanti,
// padahal mencoba lagi tidak akan berhasil sampai besok. Yang perlu ia tahu:
// dosisnya kini dianggap terlewat.
func TestSnoozeLimit_PesanMenjelaskanAkibat(t *testing.T) {
	_, _, msg := pohttp.MapDomainError(domain.ErrSnoozeLimit)
	if !strings.Contains(msg, "terlewat") {
		t.Fatalf("pesan %q tidak menyebut akibatnya bagi pengguna", msg)
	}
}
