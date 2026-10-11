package domain

import (
	"testing"
	"time"
)

// TestIsRevoked_PresisiDetik mengunci bug yang benar-benar terjadi:
// klaim `iat` JWT hanya presisi detik, sedangkan `revoked_at` PostgreSQL
// presisi mikrodetik. Perbandingan apa adanya membuat token yang terbit di
// detik yang sama dengan pencabutan selalu dianggap mati — dan itu membuat
// "ganti password lalu login ulang" mustahil.
func TestIsRevoked_PresisiDetik(t *testing.T) {
	// Skenario nyata: pencabutan tercatat 12:24:06.334811, lalu login ulang
	// pada detik yang sama menghasilkan token dengan iat 12:24:06.000000.
	revokedAt := time.Date(2026, 10, 11, 12, 24, 6, 334811000, time.UTC)
	issuedAt := time.Unix(revokedAt.Unix(), 0) // 12:24:06.000000

	rev := &Revocation{UserID: "u1", RevokedAt: revokedAt, Reason: RevokeReasonPasswordChange}

	if rev.IsRevoked(issuedAt) {
		t.Fatalf("token yang terbit di detik yang sama dengan pencabutan "+
			"harus DITERIMA (iat=%s revoked_at=%s); kalau ditolak, login ulang "+
			"setelah ganti password selalu gagal 401",
			issuedAt.Format(time.RFC3339Nano), revokedAt.Format(time.RFC3339Nano))
	}
}

// TestIsRevoked_TokenLamaTetapMati memastikan perbaikan presisi TIDAK
// melemahkan pencabutan: token yang terbit sebelum detik pencabutan tetap
// harus ditolak.
func TestIsRevoked_TokenLamaTetapMati(t *testing.T) {
	revokedAt := time.Date(2026, 10, 11, 12, 24, 6, 334811000, time.UTC)
	rev := &Revocation{UserID: "u1", RevokedAt: revokedAt, Reason: RevokeReasonPasswordChange}

	cases := []struct {
		name     string
		issuedAt time.Time
		want     bool
	}{
		{"satu detik sebelum", revokedAt.Add(-time.Second), true},
		{"satu menit sebelum", revokedAt.Add(-time.Minute), true},
		{"satu hari sebelum", revokedAt.Add(-24 * time.Hour), true},
		{"satu detik sesudah", revokedAt.Add(time.Second), false},
		{"satu menit sesudah", revokedAt.Add(time.Minute), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rev.IsRevoked(tc.issuedAt); got != tc.want {
				t.Fatalf("IsRevoked(%s) = %v, ingin %v",
					tc.issuedAt.Format(time.RFC3339Nano), got, tc.want)
			}
		})
	}
}

// TestIsRevoked_TanpaPencabutan memastikan user yang belum pernah dicabut
// tidak terpengaruh (garis potong nil).
func TestIsRevoked_TanpaPencabutan(t *testing.T) {
	var rev *Revocation
	if rev.IsRevoked(time.Now()) {
		t.Fatal("tanpa baris pencabutan, tidak ada token yang boleh ditolak")
	}
}
