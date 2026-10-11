package domain

import "time"

// Revocation menyatakan bahwa seluruh akses seorang user harus dimatikan
// sejak satu titik waktu tertentu.
//
// # MASALAH YANG DISELESAIKAN
//
// Access token adalah JWT berumur panjang (24 jam). Selama masa itu, token
// yang sudah beredar tidak bisa ditarik kembali: server tidak menyimpan
// daftar token yang diterbitkan. Konsekuensinya, aksi admin seperti
// "nonaktifkan akun ini" tidak langsung berlaku, karena token yang sudah
// dipegang penyerang masih lolos verifikasi tanda tangan sampai kedaluwarsa.
//
// Memperpendek umur token menyempitkan jendela itu, tapi tidak menutupnya,
// dan memaksa user refresh lebih sering. Yang dipakai di sini adalah
// sebaliknya: token tetap panjang, tapi ada satu titik waktu yang menjadi
// garis potong. Token yang diterbitkan SEBELUM garis itu dianggap mati,
// walau tanda tangannya masih valid dan waktunya belum habis.
//
// Ini bekerja karena klaim `iat` (issued at) sudah ada di setiap access
// token. Jadi tidak perlu daftar token: cukup satu stempel waktu per user.
type Revocation struct {
	UserID string

	// RevokedAt adalah garis potong. Token dengan iat < RevokedAt ditolak.
	RevokedAt time.Time

	// Reason dicatat untuk audit, supaya jelas kenapa akses dimatikan.
	// Contoh: "password_changed", "admin_disable", "admin_revoke_admin".
	Reason string
}

// IsRevoked melaporkan apakah token yang diterbitkan pada issuedAt sudah
// tidak berlaku lagi.
//
// # PRESISI DIPOTONG KE DETIK — INI PENTING
//
// Klaim `iat` pada JWT hanya presisi DETIK (`now.Unix()`), sedangkan
// `revoked_at` dari PostgreSQL presisi mikrodetik. Membandingkan keduanya
// apa adanya membuat token yang terbit di DETIK YANG SAMA dengan pencabutan
// selalu dianggap mati, karena iat (12:24:06.000000) selalu "sebelum"
// revoked_at (12:24:06.334811).
//
// Akibatnya nyata dan pernah terjadi: ganti password mencabut semua akses,
// lalu user login ulang dengan password baru — dan token hasil login itu
// LANGSUNG ditolak 401 kalau login terjadi masih di detik yang sama. Alur
// yang seharusnya benar jadi mustahil. Terbukti pada uji apidoc:
// change-password (200) -> relogin (200) -> request berikutnya (401).
//
// Kedua sisi karena itu dipotong ke detik lebih dulu. Konsekuensinya yang
// perlu disadari: token yang terbit di detik yang sama dengan pencabutan
// TIDAK ikut mati. Jendelanya maksimal satu detik, dan sesi refresh-nya
// tetap dicabut, jadi perlindungan utamanya tidak hilang. Sebaliknya,
// menolak token sedetik itu berarti mengorbankan login ulang yang sah —
// dan itu kerugian yang jauh lebih sering terjadi.
func (r *Revocation) IsRevoked(issuedAt time.Time) bool {
	if r == nil {
		return false
	}
	// issuedAt dari JWT sudah tepat pada detik, tapi Truncate dipasang di
	// KEDUA sisi supaya aturannya tidak bergantung pada asumsi pemanggil.
	return issuedAt.Truncate(time.Second).Before(r.RevokedAt.Truncate(time.Second))
}

// Alasan pencabutan. Nilainya disimpan di kolom reason agar bisa ditelusuri.
const (
	RevokeReasonPasswordChange  = "password_changed"
	RevokeReasonLogoutAll       = "logout_all_devices"
	RevokeReasonTokenReuse      = "token_reuse_detected"
	RevokeReasonAdminDisable    = "admin_disable"
	RevokeReasonAdminErase      = "admin_erase"
	RevokeReasonAdminRevokeRole = "admin_revoke_role"
	RevokeReasonUserRequest     = "user_request"
)
