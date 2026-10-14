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
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
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
	// Subcommand untuk lingkungan container.
	//
	// Image runtime memakai scratch: TIDAK ada shell, curl, maupun wget.
	// Healthcheck dan pekerjaan pemeliharaan karena itu dibawa oleh binary
	// aplikasi sendiri — satu-satunya alat yang dijamin ada.
	//
	//   healthz    LIVENESS untuk HEALTHCHECK Docker. Sengaja BUKAN readiness:
	//              database yang sedang bermasalah tidak boleh me-restart
	//              container — restart tidak menyembuhkan database, dan justru
	//              mematikan worker yang sedang berjalan. Kesehatan database
	//              diperiksa /readyz oleh reverse proxy, bukan oleh Docker.
	//   migrate    Naikkan migrasi goose lalu keluar. Dipisah dari start server
	//              supaya rollback aplikasi tidak ikut menurunkan skema, dan
	//              dua instance yang start bersamaan tidak balapan (lihat
	//              deploy/docker-compose.yml service `migrate`).
	//   retention  Bersihkan tabel append-only yang usang (bump_queue,
	//              notification_jobs, user_token_revocations) lalu keluar.
	//              Dipanggil berkala oleh service `retention`.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "healthz":
			os.Exit(healthz())
		case "migrate":
			os.Exit(migrateCmd())
		case "retention":
			os.Exit(retentionCmd())
		}
		// Subcommand yang tidak dikenal dibiarkan jatuh ke jalur server:
		// salah ketik tidak boleh berubah menjadi server yang "jalan" dengan
		// perintah yang diabaikan diam-diam.
		slog.Default().Error("subcommand tidak dikenal", slog.String("subcommand", os.Args[1]))
		os.Exit(2)
	}

	if err := run(); err != nil {
		slog.Default().Error("startup gagal", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

// healthz memeriksa apakah server melayani /healthz pada APP_PORT.
//
// Return value dipakai langsung sebagai exit code: 0 sehat, 1 tidak.
func healthz() int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "konfigurasi tidak valid:", err)
		return 1
	}

	client := &http.Client{Timeout: 5 * time.Second}
	port := cfg.Port
	if port == "" {
		port = "8080"
	}
	resp, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthz tidak terjangkau:", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthz membalas %s\n", resp.Status)
		return 1
	}
	return 0
}

// migrateCmd menjalankan migrasi goose lalu keluar.
//
// Taxa exit: 0 berhasil (atau memang sudah di versi terakhir), 1 gagal.
// Kegagalan TIDAK boleh dianggap halus: container yang menjalankannya harus
// mati supaya orkestrator tahu migrasinya gagal dan api tidak boleh naik.
func migrateCmd() int {
	logger := slog.Default()
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "konfigurasi tidak valid:", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := postgres.Migrate(ctx, cfg.DB, logger); err != nil {
		logger.Error("migrasi gagal", slog.String("error", err.Error()))
		return 1
	}

	ver, err := postgres.MigrateStatus(ctx, cfg.DB)
	if err != nil {
		// Migrasinya sudah sukses; gagal membaca versi hanya melemahkan
		// laporannya, bukan hasilnya.
		logger.Warn("migrasi selesai, versi tidak terbaca", slog.String("error", err.Error()))
		return 0
	}
	logger.Info("migrasi selesai", slog.Int64("versi", ver))
	return 0
}

// retentionCmd menjalankan pembersihan data kedaluwarsa.
//
// Dua mode:
//   - sekali jalan (bawaan): untuk dijalankan cron host atau `docker compose run`
//   - `--loop`: container menunggu sampai jam yang ditentukan, menjalankan
//     pembersihan, lalu tidur lagi
//
// Mode `--loop` ada karena `restart: unless-stopped` pada perintah SEKALI JALAN
// menghasilkan restart loop: Docker menganggap container yang keluar sebagai
// "harus dijalankan lagi", sehingga pembersihan berjalan tiap kali backoff
// habis — bukan harian. Terukur: 854 restart. Pekerjaan yang seharusnya sekali
// sehari menjadi beban tetap, dan itu memboroskan resource sekaligus membuat
// log penuh.
func retentionCmd() int {
	loop := false
	for _, arg := range os.Args[2:] {
		switch arg {
		case "--loop":
			loop = true
		case "-h", "--help":
			fmt.Fprintln(os.Stdout, "pemakaian: api retention [--loop]")
			return 0
		default:
			fmt.Fprintf(os.Stderr, "argumen tidak dikenal: %s\n", arg)
			return 2
		}
	}

	logger := slog.Default()
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "konfigurasi tidak valid:", err)
		return 1
	}

	if !loop {
		// Sekali jalan: timeout total wajar supaya tidak menggantung.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := runRetentionOnce(ctx, cfg, logger); err != nil {
			logger.Error("retensi gagal", slog.String("error", err.Error()))
			return 1
		}
		return 0
	}

	// Mode loop. Sinyal ditangani supaya `docker stop` menghentikannya dengan
	// rapi, bukan menunggu sampai Docker menembak SIGKILL.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	hour := retentionHour()
	logger.Info("retensi berjalan dalam mode loop",
		slog.Int("jam_utc", hour),
		slog.String("catatan", "pembersihan dijalankan sekali sehari pada jam tersebut"))

	for {
		next := nextRetentionRun(time.Now(), hour)
		logger.Info("retensi menunggu jadwal berikutnya",
			slog.String("jalan_pada", next.Format(time.RFC3339)))

		select {
		case <-ctx.Done():
			logger.Info("retensi berhenti")
			return 0
		case <-time.After(time.Until(next)):
		}

		// Timeout per putaran, bukan untuk seluruh masa hidup proses.
		runCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		err := runRetentionOnce(runCtx, cfg, logger)
		cancel()

		if err != nil {
			// Kegagalan satu putaran TIDAK menghentikan loop: pembersihan
			// yang tertunda sehari masih bisa menyusul besok. Yang penting
			// kegagalannya terlihat di log.
			logger.Error("retensi gagal pada putaran ini", slog.String("error", err.Error()))
		}
	}
}

// retentionHour membaca jam jalan (UTC) dari RETENTION_HOUR.
//
// Nilai yang tidak masuk akal diabaikan dan diganti bawaan, bukan membuat
// proses gagal: jadwal pembersihan bukan alasan yang cukup untuk mematikan
// layanan pemeliharaan.
func retentionHour() int {
	const fallback = 3
	raw := os.Getenv("RETENTION_HOUR")
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 || n > 23 {
		slog.Default().Warn("RETENTION_HOUR tidak sah, memakai bawaan",
			slog.String("nilai", raw), slog.Int("dipakai", fallback))
		return fallback
	}
	return n
}

// nextRetentionRun menghitung waktu jalan berikutnya pada jam tertentu (UTC).
//
// Bila jam itu belum lewat hari ini, dipakai hari ini; bila sudah lewat,
// besok. Hasilnya selalu di masa depan, sehingga pemanggil tidak perlu
// menangani kasus "sudah lewat".
func nextRetentionRun(now time.Time, hour int) time.Time {
	nowUTC := now.UTC()
	next := time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), hour, 0, 0, 0, time.UTC)
	if !next.After(nowUTC) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// runRetentionOnce menjalankan satu putaran pembersihan.
//
// Aturan umur data ada di sini — SATU tempat di kode — supaya tidak ada SQL
// berserakan yang nilainya berbeda antar tempat.
func runRetentionOnce(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	db, err := postgres.NewDB(ctx, cfg.DB)
	if err != nil {
		return fmt.Errorf("database tidak terjangkau: %w", err)
	}
	defer db.Close()

	queue := worker.NewDBQueue(db)
	total := 0

	// bump_queue: 7 hari. Antrean pengingat; umurnya pendek. Riwayatnya tidak
	// punya pembaca — hasil kirim tercatat di notification_jobs.
	if n, err := queue.PurgeDoneBumps(ctx, db.Pool(), 7*24*time.Hour); err != nil {
		return fmt.Errorf("membersihkan bump_queue: %w", err)
	} else if n > 0 {
		logger.Info("bump_queue dibersihkan", slog.Int64("jumlah", n))
		total += int(n)
	}

	// notification_jobs: 30 hari. Riwayat pengiriman; jawaban "kenapa tidak
	// sampai" harus masih bisa dicari selama itu. Hanya job selesai yang
	// dihapus — job failed tetap ada untuk diperiksa operator.
	if n, err := db.PurgeDoneJobs(ctx, db.Pool(), 30*24*time.Hour); err != nil {
		return fmt.Errorf("membersihkan notification_jobs: %w", err)
	} else if n > 0 {
		logger.Info("notification_jobs dibersihkan", slog.Int64("jumlah", n))
		total += int(n)
	}

	// user_token_revocations: 35 hari. Garis potong access token 24 jam.
	// Setelah 35 hari tidak ada lagi token lama yang bisa dipotong, sehingga
	// barisnya aman dihapus — termasuk ketika user yang bersangkutan sudah
	// tidak ada (DELETE di sini tidak memicu FK apa pun).
	if n, err := db.PurgeOldRevocations(ctx, 35*24*time.Hour); err != nil {
		return fmt.Errorf("membersihkan user_token_revocations: %w", err)
	} else if n > 0 {
		logger.Info("user_token_revocations dibersihkan", slog.Int64("jumlah", n))
		total += int(n)
	}

	// refresh_sessions kedaluwarsa: sesi yang lewat 30 hari dari exp-nya.
	// Sesi aktif TIDAK disentuh — yang dihapus hanya jejak sesi yang sudah
	// mati sendiri dan tidak mungkin dipakai lagi.
	if n, err := db.PurgeExpiredSessions(ctx, 30*24*time.Hour); err != nil {
		return fmt.Errorf("membersihkan refresh_sessions: %w", err)
	} else if n > 0 {
		logger.Info("refresh_sessions dibersihkan", slog.Int64("jumlah", n))
		total += int(n)
	}

	// device_tokens yang dicabut: 30 hari. Token aktif tidak disentuh.
	if n, err := db.PurgeRevokedTokens(ctx, 30*24*time.Hour); err != nil {
		return fmt.Errorf("membersihkan device_tokens: %w", err)
	} else if n > 0 {
		logger.Info("device_tokens dibersihkan", slog.Int64("jumlah", n))
		total += int(n)
	}

	logger.Info("retensi selesai", slog.Int("total_baris", total))
	return nil
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
	scheduleSvc := service.NewSchedule(db, logger)
	refsSvc := service.NewRefs(db, logger)
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
	// Keputusan v0.1: kanal yang AKTIF hanya in-app. Push, email, dan
	// Telegram tetap dirancang (kodenya tidak dihapus) tapi aktivasi
	// ditunda — otomatis tidak aktif karena kredensialnya kosong.
	//
	// LogSender di sini bukan "pengganti sementara": untuk kanal in-app,
	// "mengirim" berarti mencatat job `sent` yang dibaca inbox aplikasi
	// sendiri (GET /notifications). Karena itu Enabled=true — kalau false,
	// dispatcher menjatuhkan semua job ke `skipped` NO_CHANNEL dan inbox
	// user menampilkan pengingat yang sebenarnya tidak pernah terkirim.
	var senders []worker.Sender
	if cfg.Push.Enabled() {
		// Langganan push dipasang supaya sender benar-benar punya daftar
		// perangkat. Tanpa ini, sender akan mengembalikan error "belum
		// dipasang" untuk setiap pengiriman.
		// DITUNDA v0.2: aktivasi butuh kredensial VAPID dari operator.
		senders = append(senders, worker.NewPushSender(cfg.Push, logger).
			WithSubscriptions(workerBridge))
	}
	// Pengirim email dibuat SEKALI di sini, terlepas dari kanal notifikasi.
	//
	// Alasannya: sejak alur LUPA SANDI ada, email bukan lagi sekadar kanal
	// pengingat yang boleh ditunda. Tanpa email, pengguna yang lupa sandinya
	// tidak punya jalan masuk kembali. Karena itu sender-nya selalu
	// disiapkan; yang menentukan dipakai atau tidak adalah konfigurasi SMTP
	// di dalamnya.
	emailSender := worker.NewEmailSender(cfg.Email, logger)

	if cfg.Email.Enabled() {
		senders = append(senders, emailSender)
	}
	if len(senders) == 0 {
		logger.Info("kanal in-app aktif; kanal lain menunggu konfigurasi")
		senders = append(senders, worker.NewLogSender(logger, true))
	}
	if !cfg.Email.Enabled() {
		logger.Warn("SMTP belum dikonfigurasi — alur lupa sandi tidak akan bisa mengirim tautan")
	} else {
		logger.Info("SMTP aktif",
			slog.String("host", cfg.Email.Host),
			slog.Int("port", cfg.Email.Port))
	}

	queue := worker.NewDBQueue(db)
	scheduler := worker.NewScheduler(db, queue, logger)
	dispatcher := worker.NewDispatcher(db, queue, logger, senders...).
		WithPrefs(workerBridge)
	missedPass := worker.NewMissedPass(db, logger)

	// Pemulihan sandi memakai URL publik dari konfigurasi — BUKAN dari
	// header permintaan. Tautan yang disusun dari header Host bisa diarahkan
	// penyerang ke domainnya sendiri, dan pengguna yang mengekliknya akan
	// menyerahkan token pemulihannya.
	resetSvc := service.NewPasswordResetter(db, worker.NewMailerAdapter(emailSender), cfg.PublicURL, logger)

	authHandler := handlers.NewAuth(authSvc, resetSvc, logger, cfg.IsProduction())
	usersHandler := handlers.NewUsers(usersSvc, logger)
	medsHandler := handlers.NewMedications(medsSvc, logger)
	logsHandler := handlers.NewLogs(logsSvc, usersSvc, logger)
	stockHandler := handlers.NewStock(stockSvc, logger)
	adherenceHandler := handlers.NewAdherence(adherenceSvc, usersSvc, logger)
	scheduleHandler := handlers.NewSchedule(scheduleSvc, usersSvc, logger)
	refsHandler := handlers.NewRefs(refsSvc, logger)
	notifHandler := handlers.NewNotifications(notifSvc, logger)
	careHandler := handlers.NewCareLinks(careSvc, logsSvc, medsSvc, stockSvc,
		adherenceSvc, usersSvc, scheduleSvc, logger)
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
		ScheduleHandler:     scheduleHandler,
		RefsHandler:         refsHandler,
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
