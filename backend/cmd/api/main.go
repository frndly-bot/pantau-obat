// Command api adalah entrypoint tunggal backend Pantau Obat.
//
// Satu binary berisi HTTP server DAN worker (scheduler, dispatcher,
// catch-up pass). Alasan memilih satu proses ada di
// docs/design-v2/architecture-v2.md bagian 2 (Opsi A).
//
// Batas modul tetap dijaga ketat: worker tidak boleh mengimpor transport,
// sehingga memindahkannya ke binary kedua nanti hanya perlu file main.go baru.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/frndly-bot/pantau-obat/backend/internal/config"
	"github.com/frndly-bot/pantau-obat/backend/internal/repo/postgres"
	"github.com/frndly-bot/pantau-obat/backend/internal/service"
	pohttp "github.com/frndly-bot/pantau-obat/backend/internal/transport/http"
	"github.com/frndly-bot/pantau-obat/backend/internal/transport/http/handlers"
	"github.com/frndly-bot/pantau-obat/backend/internal/worker"
)

// Versi diisi saat build:
//
//	go build -ldflags "-X main.version=$(git describe --tags)"
var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Default().Error("startup gagal", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	// --- Konfigurasi ---
	// Dimuat paling awal. Variabel wajib yang kosong = berhenti sekarang,
	// bukan nanti saat request pertama datang.
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := newLogger(cfg.Runtime.LogLevel)
	slog.SetDefault(logger)
	logger.Info("memulai api",
		slog.String("versi", version),
		slog.String("env", string(cfg.Env)),
		slog.String("port", cfg.Port),
	)

	// Context yang dibatalkan saat SIGTERM/SIGINT.
	// Semua yang berumur panjang (server, worker) memakai ini.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// --- Database ---
	db, err := postgres.NewDB(ctx, cfg.DB)
	if err != nil {
		return err
	}
	defer db.Close()
	logger.Info("database terhubung")

	// --- Migrasi ---
	// Di produksi, migrasi dijalankan sebagai job terpisah supaya rollback
	// aplikasi tidak otomatis menurunkan skema. Di development, praktis
	// dijalankan saat start.
	if cfg.Runtime.MigrateOnStart {
		if err := postgres.Migrate(ctx, cfg.DB, logger); err != nil {
			return err
		}
	} else {
		ver, err := postgres.MigrateStatus(ctx, cfg.DB)
		if err != nil {
			logger.Warn("tidak bisa membaca versi migrasi", slog.String("error", err.Error()))
		} else {
			logger.Info("migrasi dilewati", slog.Int64("versi", ver))
		}
	}

	// --- Router ---
	health := handlers.NewHealth(db, logger)

	// --- Autentikasi ---
	// Penerbit token dan layanan auth dibuat di sini, lalu diteruskan ke
	// router. Transport tidak membangunnya sendiri, supaya konfigurasi
	// keamanan (secret, TTL) hanya dibaca di satu tempat.
	authSvc, err := service.NewAuth(db, service.AuthConfig{
		JWTSecret:  cfg.Auth.JWTSecret,
		JWTTTL:     cfg.Auth.JWTTTL,
		RefreshTTL: cfg.Auth.RefreshTTL,
		PublicURL:  cfg.PublicURL,
	}, logger)
	if err != nil {
		return err
	}
	usersSvc := service.NewUsers(db, logger)
	medsSvc := service.NewMedications(db, logger)
	logsSvc := service.NewLogs(db, logger)
	stockSvc := service.NewStock(db, logger)
	adherenceSvc := service.NewAdherence(db, logger)
	notifSvc := service.NewNotifications(db, logger)
	careSvc := service.NewCareLinks(db, logger)
	catalogSvc := service.NewCatalog(db, logger)
	adminSvc := service.NewAdmin(db, logger)
	workerBridge := service.NewWorkerBridge(notifSvc, db)

	// --- Worker ---
	//
	// Scheduler, dispatcher, dan missed_pass hidup di proses yang SAMA
	// dengan HTTP server, sesuai architecture-v2 bagian 2 (Opsi A).
	//
	// Kanal notifikasi yang tersedia ditentukan di sini. Kalau VAPID belum
	// dikonfigurasi, LogSender dipakai dengan Enabled=false, sehingga
	// dispatcher tahu tidak ada kanal yang perlu dicoba — dan antrean tidak
	// menumpuk sebagai kegagalan.
	var senders []worker.Sender
	if cfg.Push.Enabled() {
		// Langganan push dipasang supaya sender benar-benar punya daftar
		// perangkat. Tanpa ini, sender akan mengembalikan error "belum
		// dipasang" untuk setiap pengiriman.
		senders = append(senders, worker.NewPushSender(cfg.Push, logger).
			WithSubscriptions(workerBridge))
	}
	if cfg.Email.Enabled() {
		senders = append(senders, worker.NewEmailSender(cfg.Email, logger))
	}
	if len(senders) == 0 {
		logger.Warn("tidak ada kanal notifikasi yang dikonfigurasi; " +
			"pengingat hanya akan dicatat, tidak dikirim")
		senders = append(senders, worker.NewLogSender(logger, false))
	}

	queue := worker.NewDBQueue(db)
	scheduler := worker.NewScheduler(db, queue, logger)
	dispatcher := worker.NewDispatcher(db, queue, logger, senders...).
		WithPrefs(workerBridge)
	missedPass := worker.NewMissedPass(db, logger)

	authHandler := handlers.NewAuth(authSvc, logger, cfg.IsProduction())
	usersHandler := handlers.NewUsers(usersSvc, logger)
	medsHandler := handlers.NewMedications(medsSvc, logger)
	logsHandler := handlers.NewLogs(logsSvc, usersSvc, logger)
	stockHandler := handlers.NewStock(stockSvc, logger)
	adherenceHandler := handlers.NewAdherence(adherenceSvc, usersSvc, logger)
	notifHandler := handlers.NewNotifications(notifSvc, logger)
	careHandler := handlers.NewCareLinks(careSvc, logsSvc, medsSvc, adherenceSvc, usersSvc, logger)
	// PublicURL diisi dari konfigurasi, BUKAN dari header permintaan.
	// Tautan undangan yang disusun dari header Host bisa diarahkan penyerang
	// ke domainnya sendiri, dan caregiver yang mengkliknya akan menyerahkan
	// akses ke data kesehatan pasien.
	careHandler.PublicURL = cfg.PublicURL
	catalogHandler := handlers.NewCatalog(catalogSvc, logger)
	adminHandler := handlers.NewAdmin(adminSvc, logger)
	webhookProcessor := service.NewWebhookProcessor(db, logger)
	webhookHandler := handlers.NewWebhook(*cfg, webhookProcessor, logger)

	router := pohttp.NewRouter(pohttp.Deps{
		Logger:       logger,
		PublicURL:    cfg.PublicURL,
		IsProduction: cfg.IsProduction(),
		Health:       health,

		AuthHandler:         authHandler,
		UserHandler:         usersHandler,
		MedicationHandler:   medsHandler,
		LogHandler:          logsHandler,
		StockHandler:        stockHandler,
		AdherenceHandler:    adherenceHandler,
		NotificationHandler: notifHandler,
		CareHandler:         careHandler,
		CatalogHandler:      catalogHandler,
		AdminHandler:        adminHandler,
		WebhookHandler:      webhookHandler,
		TokenVerifier:       authSvc,
		IdentityResolver:    service.NewIdentityResolver(authSvc),

		IPRateLimit:   pohttp.DefaultIPRateLimit(),
		UserRateLimit: pohttp.DefaultUserRateLimit(),
	})

	srv := &http.Server{
		Addr:    cfg.Addr(),
		Handler: router,
		// Timeout eksplisit: tanpa ini, koneksi lambat bisa menahan
		// goroutine tanpa batas.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// --- Jalankan server ---
	// Server berjalan di goroutine sendiri supaya main bisa menunggu sinyal.
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("http server siap", slog.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// --- Worker ---
	//
	// Ketiga loop dijalankan di goroutine sendiri, memakai ctx yang sama
	// dengan HTTP server sehingga ikut berhenti saat SIGTERM. WaitGroup
	// dipakai supaya proses tidak keluar sebelum loop benar-benar berhenti —
	// kalau tidak, pengiriman yang sedang berjalan bisa terpotong di tengah.
	var workerWG sync.WaitGroup
	runWorker := func(name string, fn func(context.Context) error) {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			if err := fn(ctx); err != nil && !errors.Is(err, worker.ErrWorkerStopped) {
				logger.Error("worker berhenti karena kesalahan",
					slog.String("worker", name),
					slog.String("error", err.Error()))
			}
		}()
	}
	runWorker("scheduler", scheduler.Run)
	runWorker("dispatcher", dispatcher.Run)
	runWorker("missed_pass", missedPass.Run)
	logger.Info("worker berjalan", slog.Int("jumlah", 3))

	// --- Tunggu ---
	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		logger.Info("sinyal berhenti diterima, menutup dengan rapi")
	}

	// --- Graceful shutdown ---
	// Berhenti menerima koneksi baru, tunggu request berjalan selesai
	// (maks 30 detik), baru tutup pool. Urutan ini penting: menutup pool
	// lebih dulu akan membuat request yang sedang jalan gagal.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown tidak rapi", slog.String("error", err.Error()))
		return err
	}

	// Tunggu worker benar-benar berhenti sebelum pool ditutup. Tanpa ini,
	// worker bisa masih memakai koneksi yang sudah ditutup.
	workerWG.Wait()
	logger.Info("berhenti dengan rapi")
	return nil
}

// newLogger membuat logger JSON terstruktur ke stdout.
//
// JSON, bukan teks: Docker menangkapnya, dan format terstruktur membuat
// field seperti request_id bisa dicari tanpa regex rapuh.
func newLogger(level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
}
