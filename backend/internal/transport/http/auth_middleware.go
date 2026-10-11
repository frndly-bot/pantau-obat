package http

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/frndly-bot/pantau-obat/backend/internal/auth"
)

// TokenVerifier memverifikasi access token dan mengembalikan klaimnya.
//
// Interface, bukan tipe konkret, supaya transport tidak mengimpor service
// secara langsung dan supaya pengujian bisa memakai verifier palsu.
type TokenVerifier interface {
	VerifyAccessToken(token string) (auth.AccessClaims, error)
}

// IdentityResolver menyelesaikan identitas pemilik token DAN memeriksa
// apakah token itu masih berlaku.
//
// Dua tanggung jawab disatukan di sini dengan sengaja. Memeriksa pencabutan
// butuh klaim `iat` dari token, sedangkan pemetaan identitas butuh id user.
// Kalau dipisah jadi dua pemanggilan, keduanya harus menerima klaim yang
// sama, dan mudah ada pemanggil yang lupa memanggil pemeriksaan kedua.
// Disatukan, lupa tidak mungkin.
type IdentityResolver interface {
	ResolveIdentity(ctx context.Context, userID string, issuedAt time.Time) (*Identity, error)
}

// Auth membaca bearer token, memeriksa masa berlaku DAN pencabutan, lalu
// mengisi identitas di context.
//
// Urutan pemeriksaan dan alasannya:
//
//  1. Tanda tangan JWT diverifikasi. Ini murah dan menolak token palsu
//     sebelum menyentuh database.
//  2. Token diperiksa terhadap garis potong pencabutan user. Ini yang
//     membuat "cabut semua akses" berlaku seketika, walau tokennya masih
//     jauh dari kedaluwarsa.
//  3. Identitas dibaca ulang dari database. Ini yang membuat perubahan
//     peran dan penonaktifan akun langsung berlaku.
//
// Token yang gagal TIDAK menggagalkan request di sini; middleware ini hanya
// mengisi apa yang ada. Penegakan dilakukan RequireAuth. Pemisahan itu
// membuat endpoint publik tetap bisa diakses walau token di browser basi.
func Auth(verifier TokenVerifier, resolver IdentityResolver) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := bearerToken(r)
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}

			claims, err := verifier.VerifyAccessToken(token)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			// Klaim `iat` diperlakukan sebagai waktu penerbitan token.
			// Nilai ini yang dibandingkan dengan garis potong pencabutan.
			issuedAt := time.Unix(claims.IssuedAt, 0)

			id, err := resolver.ResolveIdentity(r.Context(), claims.Subject, issuedAt)
			if err != nil || id == nil {
				next.ServeHTTP(w, r)
				return
			}
			id.SessionID = claims.SessionID

			ctx := WithIdentity(r.Context(), id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// bearerToken mengambil token dari header Authorization.
//
// Hanya skema "Bearer" yang diterima. Skema lain ditolak, supaya tidak ada
// jalur autentikasi alternatif yang luput dari pemeriksaan.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}
