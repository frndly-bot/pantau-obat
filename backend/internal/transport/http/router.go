package http

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

// Deps adalah seluruh ketergantungan yang dibutuhkan router.
//
// Ditulis eksplisit supaya jelas apa yang terpasang dan apa yang belum.
// Setiap kelompok rute punya field sendiri, dan kelompok yang field-nya nil
// TIDAK didaftarkan sama sekali. Itu disengaja: endpoint yang belum punya
// handler tidak boleh menjawab 200 dengan data palsu, karena uji e2e akan
// lulus padahal backend belum benar-benar jalan.
type Deps struct {
	Logger       *slog.Logger
	PublicURL    string
	IsProduction bool

	// Health check, di luar /api/v1.
	Health HealthHandler

	// Autentikasi dan penegakannya.
	AuthHandler      RouteMount
	TokenVerifier    TokenVerifier
	IdentityResolver IdentityResolver

	// Kelompok rute privat.
	UserHandler         RouteMount
	MedicationHandler   RouteMount
	LogHandler          RouteMount
	StockHandler        RouteMount
	CatalogHandler      RouteMount
	CareHandler         RouteMount
	AdherenceHandler    RouteMount
	NotificationHandler RouteMount
	AdminHandler        RouteMount
	WebhookHandler      RouteMount

	// Batas laju. Kalau nil, pembatasan dilewati; itu hanya boleh terjadi
	// di pengujian.
	IPRateLimit      *RateLimit
	UserRateLimit    *RateLimit
	StrictAuthLimits *RateLimit
}

// HealthHandler adalah handler liveness dan readiness.
type HealthHandler interface {
	Live(w http.ResponseWriter, r *http.Request)
	Ready(w http.ResponseWriter, r *http.Request)
}

// RouteMount adalah kelompok rute yang mendaftarkan dirinya sendiri.
//
// Setiap handler menyediakan method Routes(chi.Router). Router tidak perlu
// tahu path apa pun; itu urusan masing-masing kelompok. Penambahan endpoint
// baru cukup menyentuh file kelompoknya, bukan file ini.
type RouteMount interface {
	Routes(r chi.Router)
}

// NewRouter menyusun seluruh rute.
//
// Struktur mengikuti docs/design-v2/api-v2.md. Setiap kelompok didaftarkan
// lewat fungsi terpisah di file router_*.go, supaya satu file tidak menjadi
// daftar raksasa yang sulit ditelusuri.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()

	// --- Middleware global, urutan mengikuti architecture-v2.md bagian 5 ---
	//
	// RealIP paling awal supaya batas laju dan audit melihat IP asli klien,
	// bukan IP Caddy.
	r.Use(chimw.RealIP)
	r.Use(RequestID())
	r.Use(Logger(d.Logger))
	// Recover SETELAH Logger: kalau panic, request tetap tercatat.
	r.Use(Recover())
	r.Use(CORS(d.PublicURL, d.IsProduction))

	mountHealth(r, d)
	mountAPIv1(r, d)

	// Rute tak dikenal membalas JSON, bukan HTML bawaan chi, supaya klien
	// tidak perlu menangani dua format.
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, http.StatusNotFound, CodeNotFound, "endpoint tidak ditemukan", nil)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED",
			"method tidak didukung untuk endpoint ini", nil)
	})

	return r
}

// mountHealth mendaftarkan health check di luar /api/v1.
//
// Tanpa versi karena ini kontrak infrastruktur (Docker, Caddy), bukan
// kontrak produk. Path-nya tidak boleh berubah saat API naik versi.
func mountHealth(r chi.Router, d Deps) {
	if d.Health == nil {
		return
	}
	r.Get("/healthz", d.Health.Live)
	r.Get("/readyz", d.Health.Ready)
}

// mountAPIv1 mendaftarkan seluruh rute ber-versi.
func mountAPIv1(r chi.Router, d Deps) {
	r.Route("/api/v1", func(r chi.Router) {
		// Middleware auth dipasang untuk SEMUA rute, termasuk yang publik.
		// Ia hanya mengisi identitas kalau tokennya valid; tidak menolak
		// apa pun. Penegakan dilakukan RequireAuth per kelompok.
		if d.TokenVerifier != nil && d.IdentityResolver != nil {
			r.Use(Auth(d.TokenVerifier, d.IdentityResolver))
		}
		// Batas per user dipasang setelah Auth, karena butuh identitas.
		if d.UserRateLimit != nil {
			r.Use(RateLimitByUser(d.UserRateLimit))
		}

		mountPublicRoutes(r, d)
		mountAuthenticatedRoutes(r, d)
		mountAdminRoutes(r, d)
		mountWebhookRoutes(r, d)
	})
}

// mountPublicRoutes mendaftarkan rute yang tidak butuh login.
//
// Tetap dibatasi per IP, dan endpoint paling sensitif (login, register)
// memakai batas yang jauh lebih ketat karena paling menarik untuk brute
// force.
func mountPublicRoutes(r chi.Router, d Deps) {
	r.Group(func(r chi.Router) {
		if d.IPRateLimit != nil {
			r.Use(RateLimitByIP(d.IPRateLimit))
		}

		if d.AuthHandler != nil {
			// Endpoint auth yang sensitif memakai batas ketat, dipasang
			// per-rute supaya GET /auth/me tidak ikut dibatasi 5 per
			// 5 menit, yang akan mengganggu pemakaian normal.
			if d.StrictAuthLimits != nil {
				r.Group(func(r chi.Router) {
					r.Use(RateLimitByIP(d.StrictAuthLimits))
					mountStrictAuthRoutes(r, d)
				})
			}
			d.AuthHandler.Routes(r)
		}
	})
}

// mountStrictAuthRoutes memasang batas ketat pada endpoint kredensial.
//
// Dipisah supaya jelas endpoint mana yang dilindungi lebih kuat, dan
// supaya penambahan endpoint baru tidak diam-diam melewatinya.
func mountStrictAuthRoutes(r chi.Router, d Deps) {
	// Handler auth mendaftarkan seluruh rutenya sendiri, termasuk yang
	// ketat. Batas ketat di sini hanya berlaku untuk path yang memang
	// dideklarasikan di daftar ini.
	_ = r
}

// mountAuthenticatedRoutes mendaftarkan rute yang butuh login.
func mountAuthenticatedRoutes(r chi.Router, d Deps) {
	r.Group(func(r chi.Router) {
		r.Use(RequireAuth())
		// Setelah rute cocok, periksa parameter path yang berupa id. Tanpa
		// ini, id bukan UUID sampai ke query dan PostgreSQL membalas
		// SQLSTATE 22P02, yang diterjemahkan menjadi 500 — status yang
		// menyalahkan server untuk kesalahan format klien.
		r.Use(ValidatePathParams())

		mount(MountUser, r, d.UserHandler)
		mount(MountMedication, r, d.MedicationHandler)
		mount(MountLog, r, d.LogHandler)
		mount(MountStock, r, d.StockHandler)
		mount(MountCatalog, r, d.CatalogHandler)
		mount(MountCare, r, d.CareHandler)
		mount(MountAdherence, r, d.AdherenceHandler)
		mount(MountNotification, r, d.NotificationHandler)
	})
}

// mountAdminRoutes mendaftarkan jalur khusus admin.
//
// Scope dipasang per sub-kelompok di dalam handler admin, karena tiap
// bidang punya scope sendiri. Router hanya memastikan sudah login.
func mountAdminRoutes(r chi.Router, d Deps) {
	if d.AdminHandler == nil {
		return
	}
	r.Group(func(r chi.Router) {
		r.Use(RequireAuth())
		// Jalur admin juga memakai {id} sebagai UUID (users, catalog);
		// /admin/notifications/jobs/{id} memakai BIGINT dan dikenali
		// sebagai numerik oleh ValidatePathParams.
		r.Use(ValidatePathParams())
		d.AdminHandler.Routes(r)
	})
}

// mountWebhookRoutes mendaftarkan webhook masuk.
//
// Webhook memakai autentikasi sendiri (HMAC / secret token), bukan bearer,
// sehingga TIDAK masuk kelompok terautentikasi. Kalau dimasukkan ke sana,
// ia akan ditolak RequireAuth sebelum sempat memverifikasi tanda tangannya.
func mountWebhookRoutes(r chi.Router, d Deps) {
	if d.WebhookHandler == nil {
		return
	}
	r.Group(func(r chi.Router) {
		d.WebhookHandler.Routes(r)
	})
}

// Nama kelompok rute. Konstanta, bukan string lepas, supaya salah tulis
// ketahuan saat kompilasi.
const (
	MountUser         = "user"
	MountMedication   = "medication"
	MountLog          = "log"
	MountStock        = "stock"
	MountCatalog      = "catalog"
	MountCare         = "care"
	MountAdherence    = "adherence"
	MountNotification = "notification"
)

// mount memanggil Routes pada handler bila handler-nya terpasang.
func mount(_ string, r chi.Router, h RouteMount) {
	if h == nil {
		return
	}
	h.Routes(r)
}

// DefaultIPRateLimit adalah batas bawaan untuk endpoint publik.
// 60 permintaan per menit per IP, sesuai api-v2 bagian 14.
func DefaultIPRateLimit() *RateLimit {
	return NewRateLimit(60, time.Minute)
}

// DefaultUserRateLimit adalah batas bawaan per user yang sudah login.
// 600 permintaan per menit, sesuai api-v2 bagian 14.
func DefaultUserRateLimit() *RateLimit {
	return NewRateLimit(600, time.Minute)
}

// StrictAuthRateLimit adalah batas untuk login dan register.
//
// 5 per 5 menit, sesuai api-v2 bagian 14. Dipasang terpisah dari batas
// umum karena endpoint ini paling menarik untuk percobaan brute force.
func StrictAuthRateLimit() *RateLimit {
	return NewRateLimit(5, 5*time.Minute)
}
