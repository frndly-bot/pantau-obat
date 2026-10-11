package domain

import "time"

// User adalah akun pemilik data. Password hash TIDAK pernah masuk struct ini
// supaya tidak ikut ter-serialize ke JSON secara tidak sengaja. Hash hanya
// dipakai di dalam service/auth, lewat tipe Credentials di bawah.
type User struct {
	ID       string
	Email    string
	FullName string
	Phone    *string
	Timezone string
	IsActive bool

	Role       UserRole
	AdminScope []AdminScope // nil = admin dengan akses penuh

	AdminGrantedAt *time.Time
	AdminGrantedBy *string
	LastLoginAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// IsAdmin melaporkan apakah user berperan admin dan akunnya masih aktif.
// Akun nonaktif tidak dianggap admin walau kolom role-nya masih 'admin':
// menonaktifkan akun harus langsung mencabut kemampuannya, tanpa menunggu
// peran diubah manual.
func (u *User) IsAdmin() bool {
	return u != nil && u.IsActive && u.Role == RoleAdmin
}

// HasScope melaporkan apakah user boleh menyentuh bidang tertentu.
func (u *User) HasScope(want AdminScope) bool {
	if !u.IsAdmin() {
		return false
	}
	return HasScope(u.Role, u.AdminScope, want)
}

// Credentials adalah satu-satunya tempat password hash dibawa keluar dari
// repo. Dipisah dari User supaya jelas di tanda tangan fungsi mana hash
// boleh muncul, dan tidak pernah ikut ke response.
type Credentials struct {
	UserID       string
	PasswordHash string
	IsActive     bool
}

// RefreshSession adalah satu sesi refresh token.
//
// Yang disimpan hanya SHA-256 dari token. Token aslinya tidak pernah
// disimpan, jadi bocornya isi tabel ini tidak langsung memberi penyerang
// token yang bisa dipakai.
type RefreshSession struct {
	ID          string
	UserID      string
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	RotatedFrom *string
	CreatedAt   time.Time
}

// Active melaporkan apakah sesi masih bisa dipakai.
func (s *RefreshSession) Active(now time.Time) bool {
	return s != nil && s.RevokedAt == nil && now.Before(s.ExpiresAt)
}

// WeakPasswordError menjelaskan kenapa password ditolak, dalam bahasa
// yang bisa langsung ditampilkan ke user.
type WeakPasswordError struct {
	Reason string
}

func (e *WeakPasswordError) Error() string { return e.Reason }
