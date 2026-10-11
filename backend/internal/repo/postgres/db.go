// Package postgres berisi implementasi repository di atas PostgreSQL 18
// memakai pgx v5.
//
// ATURAN IMPORT: paket ini hanya boleh mengimpor `domain` dan paket pihak
// ketiga. Tidak boleh mengimpor `service` atau `transport`.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/frndly-bot/pantau-obat/backend/internal/config"
)

// DB membungkus pool koneksi pgx.
type DB struct {
	pool *pgxpool.Pool
}

// NewDB membuka pool koneksi dan langsung memverifikasinya dengan ping.
// Gagal cepat lebih baik daripada melayani request lalu error satu per satu.
func NewDB(ctx context.Context, cfg config.DBConfig) (*DB, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("DB_URL tidak bisa diurai: %w", err)
	}
	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	// Batas umur koneksi supaya koneksi basi tidak menumpuk di sisi PG.
	poolCfg.MaxConnLifetime = time.Hour
	poolCfg.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("membuka pool koneksi: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database gagal: %w", err)
	}

	return &DB{pool: pool}, nil
}

// Pool mengembalikan pool mentah. Dipakai worker yang butuh kendali
// transaksi sendiri; sisanya sebaiknya lewat method terbungkus.
func (db *DB) Pool() *pgxpool.Pool { return db.pool }

// Ping memeriksa koneksi database. Dipakai endpoint /readyz.
func (db *DB) Ping(ctx context.Context) error { return db.pool.Ping(ctx) }

// Close menutup seluruh koneksi.
func (db *DB) Close() { db.pool.Close() }

// Health melaporkan statistik pool, untuk endpoint diagnostik internal.
func (db *DB) Health() map[string]int32 {
	s := db.pool.Stat()
	return map[string]int32{
		"total":        s.TotalConns(),
		"idle":         s.IdleConns(),
		"acquired":     s.AcquiredConns(),
		"max":          s.MaxConns(),
		"constructing": s.ConstructingConns(),
	}
}

// Querier adalah irisan yang dipenuhi pgxpool.Pool maupun pgx.Tx.
//
// Dengan ini, satu fungsi repo bisa dipakai baik di dalam maupun di luar
// transaksi tanpa duplikasi kode. Ini yang membuat jalur tulis multi-tabel
// (koreksi log + potong stok) bisa atomik tanpa menulis dua versi query.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// WithTx menjalankan fn di dalam satu transaksi.
//
// Rollback otomatis bila fn mengembalikan error ATAU panic. Karena koreksi
// log berpasangan dengan potong stok harus atomik, jalur tulis yang
// menyentuh lebih dari satu tabel WAJIB lewat fungsi ini.
//
// BATASAN YANG PERLU DISADARI: error dari fn MEMBATALKAN seluruh transaksi,
// termasuk perubahan yang sudah ditulis di dalamnya. Itu perilaku yang benar
// untuk hampir semua kasus, tapi SALAH untuk kasus di mana perubahan itu
// justru harus bertahan walaupun hasilnya dilaporkan sebagai error.
//
// Contoh nyata: deteksi penggunaan ulang refresh token. Saat itu terjadi,
// pencabutan seluruh akses harus TETAP tersimpan, sementara request-nya
// dibalas 401. Kalau pencabutan ditulis di dalam transaksi yang kemudian
// di-rollback karena error, pencabutannya hilang tanpa jejak. Untuk kasus
// itu, pakai WithTxCommitOnError.
func (db *DB) WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return db.runTx(ctx, false, fn)
}

// WithTxCommitOnError menjalankan fn dalam satu transaksi, dan TETAP
// meng-commit perubahan walaupun fn mengembalikan error.
//
// Dipakai HANYA untuk kasus di mana efek sampingnya sengaja harus bertahan
// meski operasinya dilaporkan gagal. Contoh: penolakan refresh token yang
// dipakai ulang harus sekaligus mematikan seluruh akses user.
//
// Panic tetap menyebabkan rollback. Itu disengaja: panic menandakan keadaan
// yang tidak terduga, dan menyimpan perubahan dari jalur yang tidak terduga
// justru berbahaya.
func (db *DB) WithTxCommitOnError(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return db.runTx(ctx, true, fn)
}

func (db *DB) runTx(ctx context.Context, commitOnError bool, fn func(tx pgx.Tx) error) error {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("memulai transaksi: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			// Rollback setelah Commit yang sukses adalah no-op yang aman.
			// Error diabaikan karena yang dilaporkan ke pemanggil adalah
			// err utama dari fn.
			_ = tx.Rollback(ctx)
		}
	}()

	fnErr := fn(tx)

	// Satu-satunya perbedaan antara kedua mode ada di sini.
	if fnErr != nil && !commitOnError {
		return fnErr
	}

	if err := tx.Commit(ctx); err != nil {
		// Kalau commit gagal padahal fn juga error, yang dilaporkan adalah
		// error dari fn: itu yang bermakna bagi pemanggil. Kegagalan commit
		// tetap dicatat lewat pembungkusan, supaya tidak hilang begitu saja.
		if fnErr != nil {
			return fmt.Errorf("%w (commit juga gagal: %v)", fnErr, err)
		}
		return fmt.Errorf("commit transaksi: %w", err)
	}
	committed = true

	return fnErr
}

// Kode SQLSTATE PostgreSQL yang perlu dibedakan di layer repo.
// Pemetaan ke sentinel domain dilakukan di sini, supaya service tidak perlu
// tahu apa pun tentang pgx maupun SQLSTATE.
const (
	codeUniqueViolation     = "23505"
	codeForeignKeyViolation = "23503"
	codeCheckViolation      = "23514"
	codeNotNullViolation    = "23502"
)

// IsUniqueViolation melaporkan apakah error berasal dari pelanggaran unique.
func IsUniqueViolation(err error) bool { return sqlState(err) == codeUniqueViolation }

// IsForeignKeyViolation melaporkan apakah error berasal dari pelanggaran FK.
func IsForeignKeyViolation(err error) bool { return sqlState(err) == codeForeignKeyViolation }

// IsCheckViolation melaporkan apakah error berasal dari pelanggaran CHECK.
func IsCheckViolation(err error) bool { return sqlState(err) == codeCheckViolation }

// IsNotNullViolation melaporkan apakah error berasal dari kolom NOT NULL
// yang kosong.
func IsNotNullViolation(err error) bool { return sqlState(err) == codeNotNullViolation }

// ConstraintName mengembalikan nama constraint yang dilanggar, bila ada.
//
// Ini yang membedakan duplikat obat dari duplikat jam: keduanya sama-sama
// 23505, yang membedakan hanya nama index-nya (uq_meds_owner_name vs
// uq_times_med_time).
func ConstraintName(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	return ""
}

func sqlState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}
