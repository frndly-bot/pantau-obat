package http

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"
)

// konteks kunci untuk nilai yang disimpan di request context.
type ctxKey int

const (
	ctxKeyRequestID ctxKey = iota
	ctxKeyUser
	ctxKeyLogger
)

// Identity adalah identitas pemanggil yang diisi middleware Auth.
// Ditempatkan di transport, bukan domain, karena bentuknya bergantung
// pada cara autentikasi (JWT), bukan pada aturan bisnis.
type Identity struct {
	UserID     string
	Role       string
	AdminScope []string // nil = akses penuh
	SessionID  string
	System     bool // true untuk jalur internal/webhook, bukan user
}

// IsAdmin melaporkan apakah pemanggil berperan admin.
func (id *Identity) IsAdmin() bool { return id != nil && id.Role == "admin" }

// HasScope melaporkan apakah admin ini boleh menyentuh bidang tertentu.
func (id *Identity) HasScope(want string) bool {
	if !id.IsAdmin() {
		return false
	}
	if id.AdminScope == nil {
		return true // NULL = akses penuh
	}
	for _, s := range id.AdminScope {
		if s == want {
			return true
		}
	}
	return false
}

// RequestIDFrom mengambil id request dari context.
func RequestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyRequestID).(string); ok {
		return v
	}
	return ""
}

// IdentityFrom mengambil identitas pemanggil dari context.
func IdentityFrom(ctx context.Context) *Identity {
	if v, ok := ctx.Value(ctxKeyUser).(*Identity); ok {
		return v
	}
	return nil
}

// LoggerFrom mengambil logger ber-request-id dari context.
func LoggerFrom(ctx context.Context) *slog.Logger {
	if v, ok := ctx.Value(ctxKeyLogger).(*slog.Logger); ok {
		return v
	}
	return slog.Default()
}

// WithIdentity menyisipkan identitas ke context. Dipakai middleware Auth
// dan pengujian.
func WithIdentity(ctx context.Context, id *Identity) context.Context {
	return context.WithValue(ctx, ctxKeyUser, id)
}

// Middleware adalah bentuk middleware standar Go.
type Middleware func(http.Handler) http.Handler

// Chain menyusun middleware supaya yang pertama di daftar menjadi lapisan
// TERLUAR. Urutannya penting dan mengikuti docs/design-v2/architecture-v2.md
// bagian 5.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// RequestID membuat id unik per request dan menyimpannya di context.
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get("X-Request-Id")
			if id == "" {
				id = uuid.NewString()
			}
			ctx := context.WithValue(r.Context(), ctxKeyRequestID, id)
			w.Header().Set("X-Request-Id", id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Logger mencatat setiap request setelah selesai.
//
// ATURAN PRIVASI: NAMA OBAT TIDAK BOLEH MASUK LOG. Yang dicatat hanya
// method, path, status, dan durasi. Path bisa memuat id obat, dan itu
// tidak masalah karena id bukan data kesehatan. Jangan pernah menambahkan
// body request atau query yang memuat nama ke logger ini.
func Logger(base *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			reqID := RequestIDFrom(r.Context())

			l := base.With(
				slog.String("request_id", reqID),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
			)
			ctx := context.WithValue(r.Context(), ctxKeyLogger, l)

			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r.WithContext(ctx))

			level := slog.LevelInfo
			if rec.status >= 500 {
				level = slog.LevelError
			} else if rec.status >= 400 {
				level = slog.LevelWarn
			}
			l.LogAttrs(ctx, level, "permintaan selesai",
				slog.Int("status", rec.status),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			)
		})
	}
}

// Recover menangkap panic supaya satu handler yang rusak tidak menjatuhkan
// seluruh proses dan tidak membalas HTML stack trace ke klien.
//
// WAJIB dipasang SETELAH Logger, supaya panic tetap tercatat bersama
// request id-nya.
func Recover() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger := LoggerFrom(r.Context())
					logger.ErrorContext(r.Context(), "panic tertangkap",
						slog.Any("panic", rec),
						slog.String("stack", string(debug.Stack())),
					)
					// Balas generik, jangan bocorkan stack trace.
					WriteError(w, r, http.StatusInternalServerError,
						CodeInternal, "terjadi kesalahan pada server", nil)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// CORS membatasi origin yang boleh mengakses API.
//
// Di development, localhost diizinkan. Di produksi, HANYA origin di daftar
// yang lolos. Tidak ada wildcard: aplikasi ini membawa data kesehatan.
func CORS(publicURL string, isProduction bool) Middleware {
	allowed := map[string]bool{}
	if publicURL != "" {
		allowed[strings.TrimRight(publicURL, "/")] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				ok := allowed[strings.TrimRight(origin, "/")]
				if !ok && !isProduction {
					// Hanya di development: izinkan localhost port apa pun.
					ok = strings.HasPrefix(origin, "http://localhost:") ||
						strings.HasPrefix(origin, "http://127.0.0.1:")
				}
				if ok {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Vary", "Origin")
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers",
						"Authorization, Content-Type, Idempotency-Key, X-Request-Id")
					w.Header().Set("Access-Control-Max-Age", "600")
				}
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// statusRecorder mengingat status yang ditulis handler, karena
// http.ResponseWriter tidak menyimpannya.
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.written {
		return
	}
	s.status = code
	s.written = true
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.written {
		s.written = true
	}
	return s.ResponseWriter.Write(b)
}
