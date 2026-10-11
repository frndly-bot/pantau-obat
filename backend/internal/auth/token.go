package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Token akses memakai JWT HS256 yang ditandatangani sendiri.
//
// Kenapa tidak memakai pustaka JWT: kebutuhan kita sangat sempit — tiga
// klaim, satu algoritma, tanpa JWKS, tanpa rotasi kunci. Menulisnya sendiri
// menghindari menarik dependensi besar, dan lebih penting: mencegah bug
// klasik "alg=none" atau "alg=RS256 diganti HS256" yang sering muncul
// justru dari pustaka yang menerima algoritma dari token. Di sini
// algoritma dipatok saat verifikasi, bukan dibaca dari header.

// ErrTokenInvalid dikembalikan untuk SEMUA kegagalan token.
//
// Sengaja tidak dibedakan (kedaluwarsa vs tanda tangan salah vs format
// rusak). Membedakannya di response hanya membantu penyerang memetakan
// perilaku server; penyebab sebenarnya dicatat di log.
var ErrTokenInvalid = errors.New("token tidak valid")

// AccessClaims adalah isi access token.
type AccessClaims struct {
	Subject   string // sub: user id
	SessionID string // sid: refresh_sessions.id, untuk mencabut per sesi
	IssuedAt  int64  // iat
	ExpiresAt int64  // exp
}

// TokenIssuer membuat dan memverifikasi access token.
type TokenIssuer struct {
	secret []byte
	ttl    time.Duration
}

// NewTokenIssuer membuat penerbit token. Secret wajib minimal 32 byte;
// sudah divalidasi di config, tapi diperiksa lagi di sini supaya paket ini
// tidak bisa dipakai dengan konfigurasi yang tidak aman.
func NewTokenIssuer(secret string, ttl time.Duration) (*TokenIssuer, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("secret token minimal 32 byte, dapat %d", len(secret))
	}
	if ttl <= 0 {
		return nil, errors.New("ttl token harus positif")
	}
	return &TokenIssuer{secret: []byte(secret), ttl: ttl}, nil
}

// TTL mengembalikan umur token, dipakai untuk mengisi field expires_in.
func (t *TokenIssuer) TTL() time.Duration { return t.ttl }

// Issue membuat access token baru untuk satu sesi.
func (t *TokenIssuer) Issue(userID, sessionID string, now time.Time) (string, error) {
	if userID == "" || sessionID == "" {
		return "", errors.New("user id dan session id wajib diisi")
	}

	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	claims := map[string]any{
		"sub": userID,
		"sid": sessionID,
		"iat": now.Unix(),
		"exp": now.Add(t.ttl).Unix(),
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("menyusun header: %w", err)
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("menyusun klaim: %w", err)
	}

	signingInput := base64.RawURLEncoding.EncodeToString(headerJSON) + "." +
		base64.RawURLEncoding.EncodeToString(claimsJSON)

	sig := t.sign(signingInput)
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// Verify memeriksa token dan mengembalikan klaimnya.
func (t *TokenIssuer) Verify(token string, now time.Time) (AccessClaims, error) {
	var claims AccessClaims

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return claims, ErrTokenInvalid
	}

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return claims, ErrTokenInvalid
	}
	var header struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return claims, ErrTokenInvalid
	}
	// Algoritma DIPATOK. Token yang mengaku "none" atau algoritma lain
	// ditolak di sini, sebelum tanda tangan diperiksa sama sekali.
	if header.Alg != "HS256" {
		return claims, ErrTokenInvalid
	}

	// Verifikasi tanda tangan SEBELUM membaca klaim.
	//
	// Urutan ini penting: membaca dan memercayai isi token sebelum
	// memastikan keasliannya adalah kesalahan klasik. Di sini payload
	// mentah dipakai untuk menghitung ulang tanda tangan lebih dulu.
	wantSig := t.sign(parts[0] + "." + parts[1])
	gotSig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return claims, ErrTokenInvalid
	}
	if !hmac.Equal(wantSig, gotSig) {
		return claims, ErrTokenInvalid
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, ErrTokenInvalid
	}
	var raw struct {
		Sub string `json:"sub"`
		Sid string `json:"sid"`
		Iat int64  `json:"iat"`
		Exp int64  `json:"exp"`
	}
	if err := json.Unmarshal(claimsJSON, &raw); err != nil {
		return claims, ErrTokenInvalid
	}
	if raw.Sub == "" || raw.Sid == "" {
		return claims, ErrTokenInvalid
	}
	if now.Unix() >= raw.Exp {
		return claims, ErrTokenInvalid
	}

	claims = AccessClaims{
		Subject:   raw.Sub,
		SessionID: raw.Sid,
		IssuedAt:  raw.Iat,
		ExpiresAt: raw.Exp,
	}
	return claims, nil
}

func (t *TokenIssuer) sign(input string) []byte {
	mac := hmac.New(sha256.New, t.secret)
	mac.Write([]byte(input))
	return mac.Sum(nil)
}

// NewRefreshToken membuat refresh token opaque 32 byte acak.
//
// "Opaque" berarti token ini tidak membawa data apa pun; maknanya hanya ada
// di database. Keuntungannya dibanding JWT: pencabutan berlaku seketika,
// karena setiap pemakaian harus dicek ke database.
func NewRefreshToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("membuat refresh token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashRefreshToken menghitung SHA-256 dari token.
//
// Yang disimpan di refresh_sessions adalah hash ini, bukan token aslinya.
// SHA-256 (bukan Argon2) dipilih dengan sengaja: token sudah punya 256 bit
// entropi acak, jadi tidak rentan tebak-menebak, dan hashing cepat justru
// dibutuhkan karena terjadi di setiap refresh.
func HashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// SessionCookieName adalah nama cookie refresh.
const SessionCookieName = "po_refresh"

// SessionCookiePath membatasi cookie hanya terkirim ke endpoint auth.
//
// Efeknya: permintaan ke /api/v1/medications tidak ikut membawa refresh
// token, sehingga permukaan penyadapan menyempit.
const SessionCookiePath = "/api/v1/auth"
