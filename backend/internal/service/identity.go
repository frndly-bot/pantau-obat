package service

import (
	"context"
	"time"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	pohttp "github.com/frndly-bot/pantau-obat/backend/internal/transport/http"
)

// Ini SATU-SATUNYA tempat paket service mengimpor transport, dan itu
// disengaja.
//
// Alasannya: middleware Auth membutuhkan hasil berupa pohttp.Identity,
// sedangkan data user berasal dari repo. Kalau pemetaan ditaruh di paket
// transport, transport harus mengimpor repo, dan itu melanggar aturan
// "transport -> service, domain" saja.
//
// Alternatif yang lebih bersih: memindahkan tipe Identity ke paket netral
// (mis. internal/identity) yang diimpor kedua belah pihak. Itu dikerjakan
// kalau jumlah tipe yang dipetakan bertambah. Untuk satu tipe, kompromi ini
// lebih sederhana dan sudah dicatat di sini supaya bukan kelalaian.

// IdentityResolver memenuhi http.IdentityResolver.
//
// Dipisah dari Auth supaya tanggung jawabnya jelas: Auth mengurus
// kredensial, sedangkan ini menjawab "siapa pemilik token ini sekarang, dan
// apakah tokennya masih berlaku".
type IdentityResolver struct {
	auth *Auth
}

// NewIdentityResolver membuat penyelesai identitas untuk middleware Auth.
func NewIdentityResolver(a *Auth) *IdentityResolver {
	return &IdentityResolver{auth: a}
}

// ResolveIdentity membaca user terkini dan memeriksa pencabutan token.
//
// Dua pemeriksaan terjadi di sini, dan keduanya wajib:
//
//  1. Pencabutan token. Token yang diterbitkan sebelum garis potong
//     dicabut, walaupun JWT-nya masih sah dan belum kedaluwarsa. Ini yang
//     membuat "cabut semua akses" berlaku seketika, bukan menunggu sampai
//     24 jam token habis.
//
//  2. Status akun terkini. Akun yang dinonaktifkan atau dihapus langsung
//     kehilangan akses, tanpa menunggu token kedaluwarsa.
//
// Mengembalikan domain.ErrTokenRevoked bila token sudah dicabut, dan
// domain.ErrAccountInactive bila akunnya tidak aktif. Pemanggil (middleware)
// memperlakukan keduanya sama: tidak mengisi identitas, sehingga RequireAuth
// yang menolak dengan 401.
func (r *IdentityResolver) ResolveIdentity(
	ctx context.Context, userID string, issuedAt time.Time,
) (*pohttp.Identity, error) {
	// Satu pembacaan untuk kedua pemeriksaan. Memisahkannya berarti dua
	// query database di setiap request terautentikasi, dan itu pemborosan
	// yang tidak perlu untuk data yang berasal dari baris yang sama.
	user, err := r.auth.Me(ctx, userID)
	if err != nil {
		return nil, err
	}

	revoked, err := r.auth.IsTokenRevoked(ctx, userID, issuedAt)
	if err != nil {
		// Gagal membaca status pencabutan diperlakukan sebagai GAGAL
		// TERTUTUP: identitas tidak diisi, sehingga request ditolak.
		//
		// Ini pilihan yang disengaja. Kalau kegagalan diperlakukan sebagai
		// "tidak dicabut", gangguan database sesaat akan membuat token yang
		// sudah dicabut berlaku kembali. Menolak request terautentikasi saat
		// database bermasalah lebih aman: seluruh API memang tidak bisa
		// melayani apa pun tanpa database.
		return nil, err
	}
	if revoked {
		return nil, domain.ErrTokenRevoked
	}

	return IdentityFromUser(user), nil
}

// IdentityFromUser memetakan domain.User ke identitas transport.
func IdentityFromUser(u *domain.User) *pohttp.Identity {
	var scope []string
	if u.AdminScope != nil {
		scope = make([]string, len(u.AdminScope))
		for i, s := range u.AdminScope {
			scope[i] = string(s)
		}
	}
	// scope nil = akses penuh, konsisten dengan kolom admin_scope NULL.
	return &pohttp.Identity{
		UserID:     u.ID,
		Role:       string(u.Role),
		AdminScope: scope,
	}
}
