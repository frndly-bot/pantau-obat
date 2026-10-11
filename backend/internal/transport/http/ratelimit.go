package http

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimit adalah pembatas jumlah permintaan berbasis jendela tetap.
//
// DISIPLIN KUNCI: kunci pembatas TIDAK BOLEH memuat email atau nama obat.
// Email adalah data pribadi, dan menyimpannya di kunci berarti menyimpannya
// di memori dan berpotensi di log. Untuk login, kuncinya adalah IP,
// sedangkan pembatasan per email ditangani di service memakai hash.
//
// Implementasi ini in-memory. Untuk satu proses (arsitektur v0.1), itu
// cukup. Kalau worker nanti dipisah jadi binary kedua, pembatas ini harus
// pindah ke penyimpanan bersama, karena tiap proses akan punya hitungannya
// sendiri. Batasan itu dicatat di sini supaya tidak terlupa.
type RateLimit struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	limit   int
	window  time.Duration
}

type bucket struct {
	count   int
	resetAt time.Time
}

// NewRateLimit membuat pembatas dengan sejumlah permintaan per jendela.
func NewRateLimit(limit int, window time.Duration) *RateLimit {
	rl := &RateLimit{
		buckets: make(map[string]*bucket),
		limit:   limit,
		window:  window,
	}
	go rl.cleanupLoop()
	return rl
}

// Allow melaporkan apakah permintaan dengan kunci tertentu boleh lanjut,
// beserta sisa kuota dan waktu tunggu bila ditolak.
func (rl *RateLimit) Allow(key string, now time.Time) (ok bool, remaining int, retryAfter time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, exists := rl.buckets[key]
	if !exists || now.After(b.resetAt) {
		rl.buckets[key] = &bucket{count: 1, resetAt: now.Add(rl.window)}
		return true, rl.limit - 1, 0
	}

	if b.count >= rl.limit {
		return false, 0, time.Until(b.resetAt)
	}
	b.count++
	return true, rl.limit - b.count, 0
}

// cleanupLoop membuang jendela yang sudah lewat.
//
// Tanpa ini, map tumbuh tanpa batas: setiap IP yang pernah mengakses akan
// meninggalkan satu entri selamanya. Untuk server yang dilihat banyak IP,
// itu kebocoran memori yang berjalan lambat.
func (rl *RateLimit) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		rl.mu.Lock()
		for k, b := range rl.buckets {
			if now.After(b.resetAt) {
				delete(rl.buckets, k)
			}
		}
		rl.mu.Unlock()
	}
}

// RateLimitByIP membatasi permintaan per alamat IP.
//
// Dipakai untuk endpoint publik seperti login dan register. IP asli dibaca
// dari X-Forwarded-For oleh middleware RealIP, jadi yang terhitung adalah
// IP klien, bukan IP Caddy.
func RateLimitByIP(rl *RateLimit) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := "ip:" + clientIPString(r.RemoteAddr)
			ok, remaining, retryAfter := rl.Allow(key, time.Now())

			w.Header().Set("X-RateLimit-Limit", itoa(rl.limit))
			w.Header().Set("X-RateLimit-Remaining", itoa(max0(remaining)))

			if !ok {
				seconds := int(retryAfter.Seconds())
				if seconds < 1 {
					seconds = 1
				}
				// Retry-After memberi tahu klien kapan boleh mencoba lagi,
				// supaya ia tidak menebak-nebak dan memperparah keadaan.
				w.Header().Set("Retry-After", itoa(seconds))
				WriteError(w, r, http.StatusTooManyRequests, CodeRateLimited,
					"terlalu banyak permintaan, coba lagi sebentar lagi", nil)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimitByUser membatasi permintaan per user yang sudah login.
func RateLimitByUser(rl *RateLimit) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := IdentityFrom(r.Context())
			if id == nil {
				// Belum login: pembatasan per user tidak berlaku.
				next.ServeHTTP(w, r)
				return
			}
			key := "u:" + id.UserID
			ok, remaining, retryAfter := rl.Allow(key, time.Now())

			w.Header().Set("X-RateLimit-Limit", itoa(rl.limit))
			w.Header().Set("X-RateLimit-Remaining", itoa(max0(remaining)))

			if !ok {
				seconds := int(retryAfter.Seconds())
				if seconds < 1 {
					seconds = 1
				}
				w.Header().Set("Retry-After", itoa(seconds))
				WriteError(w, r, http.StatusTooManyRequests, CodeRateLimited,
					"terlalu banyak permintaan, coba lagi sebentar lagi", nil)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ClientIP mengembalikan alamat IP klien siap pakai untuk kolom inet,
// atau nil bila tidak bisa ditentukan.
//
// Dipakai bersama oleh pembatas laju dan penulis audit, supaya keduanya
// tidak pernah berbeda pendapat soal apa itu "IP klien".
func ClientIP(r *http.Request) *string {
	// Caddy mengisi X-Real-IP dengan alamat klien sebenarnya.
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return &ip
	}
	if ip := clientIPString(r.RemoteAddr); ip != "" {
		return &ip
	}
	return nil
}

// clientIPKey menyusun kunci pembatas dari alamat remote.
func clientIPKey(remoteAddr string) string {
	if ip := clientIPString(remoteAddr); ip != "" {
		return ip
	}
	return remoteAddr
}

// clientIPString mengubah RemoteAddr menjadi alamat IP yang bisa diterima
// tipe inet PostgreSQL.
//
// RemoteAddr berbentuk "host:port", dan untuk IPv6 host-nya ditulis dalam
// tanda kurung siku, misalnya "[::1]:54321". Tanda kurung itu bagian dari
// notasi alamat host:port, BUKAN bagian alamat IP. Kalau ikut dikirim ke
// PostgreSQL, tipe inet menolaknya dengan "invalid input syntax for type
// inet". Jadi kurungnya harus dibuang.
func clientIPString(remoteAddr string) string {
	addr := remoteAddr
	if addr == "" {
		return ""
	}

	// Bentuk IPv6: "[::1]:54321"
	if addr[0] == '[' {
		if end := strings.IndexByte(addr, ']'); end > 0 {
			return addr[1:end]
		}
		return addr
	}

	// Bentuk IPv4 atau hostname: "127.0.0.1:54321".
	// Perhatikan: IPv6 tanpa kurung sulit dibedakan dari host:port, jadi
	// pemisah terakhir dipakai. Untuk IPv6 tanpa kurung, Go tidak
	// menghasilkannya di RemoteAddr, jadi kasus ini tidak muncul.
	if i := strings.LastIndexByte(addr, ':'); i > 0 {
		return addr[:i]
	}
	return addr
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

// itoa mengubah int menjadi string tanpa memanggil strconv, supaya
// pemakaian di header tetap murah.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
