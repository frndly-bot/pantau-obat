package postgres

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"sync"

	"github.com/frndly-bot/pantau-obat/backend/internal/config"
	_ "github.com/jackc/pgx/v5/stdlib" // registrasi driver "pgx" untuk database/sql
	"github.com/pressly/goose/v3"
)

// Migrasi di-embed ke dalam binary.
//
// Alasannya: satu image = satu artefak yang bisa dijalankan di mana saja
// tanpa perlu menyalin folder .sql ke sampingnya. Ini juga yang membuat
// MIGRATE_ON_START mungkin.
//
// Catatan: folder ini ada di dalam internal/repo/postgres/, bukan di
// backend/migrations/. Itu disengaja, karena //go:embed hanya bisa membaca
// file DI DALAM direktori paketnya. backend/migrations tetap ada sebagai
// symlink, supaya perintah `make db-migrate` dari root repo tetap bekerja.
//
//go:embed all:migrations/*.sql
var migrationsFS embed.FS

// goose membutuhkan *sql.DB, bukan *pgxpool.Pool. Kita buka koneksi terpisah
// lewat standard library adapter pgx, hanya untuk migrasi.
//
// Kenapa terpisah: pgxpool sudah dipakai untuk jalur request. Menyalurkan
// migrasi lewat pool yang sama berarti pengaturan pool ikut memengaruhi
// goose, dan sebaliknya. Memisahkannya membuat batas tanggung jawab jelas:
// satu koneksi khusus migrasi, ditutup begitu selesai.
var (
	migrateOnce sync.Once
	migrateErr  error
)

// openMigrationDB membuka koneksi khusus untuk menjalankan migrasi.
func openMigrationDB(cfg config.DBConfig) (*sql.DB, error) {
	db, err := sql.Open("pgx", cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("membuka koneksi migrasi: %w", err)
	}
	// Migrasi berjalan berurutan; satu koneksi cukup dan menghindari
	// deadlock antar koneksi saat mengunci tabel versi.
	db.SetMaxOpenConns(1)
	return db, nil
}

// Migrate menjalankan seluruh migrasi yang belum naik.
//
// Idempoten: kalau semua sudah naik, tidak ada yang dilakukan. Aman
// dipanggil setiap kali proses start.
func Migrate(ctx context.Context, cfg config.DBConfig, logger *slog.Logger) error {
	migrateOnce.Do(func() {
		migrateErr = runMigrate(ctx, cfg, logger)
	})
	return migrateErr
}

func runMigrate(ctx context.Context, cfg config.DBConfig, logger *slog.Logger) error {
	sqlDB, err := openMigrationDB(cfg)
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping koneksi migrasi: %w", err)
	}

	goose.SetBaseFS(migrationsFS)
	goose.SetLogger(gooseLogger{logger})
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("menyetel dialek goose: %w", err)
	}

	before, _ := goose.GetDBVersionContext(ctx, sqlDB)

	if err := goose.UpContext(ctx, sqlDB, "migrations"); err != nil {
		return fmt.Errorf("menjalankan migrasi: %w", err)
	}

	after, err := goose.GetDBVersionContext(ctx, sqlDB)
	if err != nil {
		return fmt.Errorf("membaca versi migrasi: %w", err)
	}
	if after == before {
		logger.Info("migrasi: skema sudah terbaru", slog.Int64("versi", after))
	} else {
		logger.Info("migrasi selesai",
			slog.Int64("dari", before), slog.Int64("ke", after))
	}
	return nil
}

// MigrateStatus membaca versi migrasi yang sedang aktif tanpa mengubah apa pun.
func MigrateStatus(ctx context.Context, cfg config.DBConfig) (int64, error) {
	sqlDB, err := openMigrationDB(cfg)
	if err != nil {
		return 0, err
	}
	defer sqlDB.Close()

	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return 0, fmt.Errorf("menyetel dialek goose: %w", err)
	}
	return goose.GetDBVersionContext(ctx, sqlDB)
}

// gooseLogger menjembatani log goose ke slog, supaya output migrasi punya
// format JSON yang sama dengan sisa aplikasi.
type gooseLogger struct{ l *slog.Logger }

func (g gooseLogger) Printf(format string, v ...any) {
	g.l.Info(fmt.Sprintf(format, v...))
}

func (g gooseLogger) Fatalf(format string, v ...any) {
	// goose memanggil Fatalf pada kegagalan. Jangan benar-benar os.Exit:
	// error tetap dikembalikan ke pemanggil supaya bisa ditangani.
	g.l.Error(fmt.Sprintf(format, v...))
}
