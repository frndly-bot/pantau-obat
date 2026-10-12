package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// File ini mengurus stok.
//
// ATURAN KERAS YANG DITEGAKKAN DI DATABASE, BUKAN DI SINI:
//
//   - current_qty hanya berubah lewat stock_transactions. Trigger
//     apply_stock_txn yang menghitung saldo baru dan menolak saldo negatif.
//   - balance_after diisi trigger, bukan aplikasi. Kolom itu NOT NULL,
//     jadi aplikasi tidak bisa mengisinya sendiri tanpa melawan trigger.
//   - log_id unik (uq_stock_txn_log), sehingga satu dosis hanya bisa
//     memotong saldo sekali walau request-nya dikirim berulang.
//
// Karena itu repo ini TIDAK melakukan UPDATE pada current_qty sama sekali.
// Yang ditulis hanya baris transaksi; sisanya urusan database.

const stockColumns = `
    id, medication_id, owner_id, unit_code, initial_qty, current_qty,
    qty_per_dose, refill_threshold_qty, refill_reminder, last_restocked_at,
    created_at, updated_at`

// scanStock membaca satu baris medication_stock.
func scanStock(row pgx.Row) (*domain.MedicationStock, error) {
	var s domain.MedicationStock
	err := row.Scan(
		&s.ID, &s.MedicationID, &s.OwnerID, &s.UnitCode, &s.InitialQty,
		&s.CurrentQty, &s.QtyPerDose, &s.RefillThresholdQty, &s.RefillReminder,
		&s.LastRestockedAt, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("membaca stok: %w", err)
	}
	return &s, nil
}

// CreateStockInput adalah data pembuatan baris stok.
type CreateStockInput struct {
	MedicationID       string
	OwnerID            string
	UnitCode           string
	InitialQty         float64
	QtyPerDose         float64
	RefillThresholdQty *float64
	RefillReminder     bool
}

// CreateStock membuat baris stok untuk sebuah obat.
//
// current_qty diisi 0, BUKAN initial_qty. Alasannya penting: saldo harus
// berasal dari transaksi, bukan dari pengisian langsung. Setelah baris ini
// dibuat, pemanggil wajib menulis transaksi `initial`, dan trigger yang akan
// menaikkan saldonya. Kalau current_qty diisi langsung di sini, kolom
// initial_qty dan saldo tidak akan punya jejak yang bisa ditelusuri.
func (db *DB) CreateStock(ctx context.Context, q Querier, in CreateStockInput) (*domain.MedicationStock, error) {
	row := q.QueryRow(ctx, `
        INSERT INTO medication_stock (
            medication_id, owner_id, unit_code, initial_qty, current_qty,
            qty_per_dose, refill_threshold_qty, refill_reminder
        ) VALUES ($1,$2,$3,$4,0,$5,$6,$7)
        RETURNING `+stockColumns,
		in.MedicationID, in.OwnerID, in.UnitCode, in.InitialQty,
		in.QtyPerDose, in.RefillThresholdQty, in.RefillReminder,
	)
	return scanStock(row)
}

// StockByMedication mengambil baris stok sebuah obat.
func (db *DB) StockByMedication(ctx context.Context, q Querier, medicationID string) (*domain.MedicationStock, error) {
	row := q.QueryRow(ctx, `
        SELECT `+stockColumns+`
          FROM medication_stock
         WHERE medication_id = $1 AND deleted_at IS NULL`, medicationID)
	return scanStock(row)
}

// StocksByOwner mengambil SELURUH baris stok milik satu pemilik sekaligus.
//
// Kenapa satu query untuk semua, bukan satu per obat: layar Hari Ini
// menampilkan sisa stok untuk setiap obat yang jatuh tempo hari ini. Dengan
// 20 obat, memanggil StockByMedication 20 kali berarti 20 perjalanan ke
// database untuk satu kali buka halaman. Query ini mengembalikan semuanya
// dalam satu perjalanan, dan pemanggil menyusunnya menjadi peta.
func (db *DB) StocksByOwner(ctx context.Context, q Querier, ownerID string) (map[string]*domain.MedicationStock, error) {
	rows, err := q.Query(ctx, `
        SELECT `+stockColumns+`
          FROM medication_stock
         WHERE owner_id = $1 AND deleted_at IS NULL`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("membaca stok pemilik: %w", err)
	}
	defer rows.Close()

	out := map[string]*domain.MedicationStock{}
	for rows.Next() {
		s, err := scanStock(rows)
		if err != nil {
			return nil, err
		}
		out[s.MedicationID] = s
	}
	return out, rows.Err()
}

// StockByID mengambil baris stok berdasarkan id.
func (db *DB) StockByID(ctx context.Context, q Querier, id, ownerID string) (*domain.MedicationStock, error) {
	row := q.QueryRow(ctx, `
        SELECT `+stockColumns+`
          FROM medication_stock
         WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL`, id, ownerID)
	return scanStock(row)
}

// UpdateStockSettings mengubah pengaturan stok.
//
// Yang boleh diubah hanya pengaturan, BUKAN saldo. Mengubah current_qty
// langsung akan memutus hubungan antara saldo dan riwayat transaksi, dan
// selisihnya tidak akan pernah bisa dijelaskan lagi. Saldo berubah hanya
// lewat transaksi.
func (db *DB) UpdateStockSettings(ctx context.Context, q Querier, medicationID, ownerID string, unitCode *string, qtyPerDose *float64, threshold *float64, clearThreshold bool, reminder *bool) (*domain.MedicationStock, error) {
	row := q.QueryRow(ctx, `
        UPDATE medication_stock SET
            unit_code            = COALESCE($3, unit_code),
            qty_per_dose         = COALESCE($4, qty_per_dose),
            refill_threshold_qty = CASE WHEN $5 THEN NULL
                                        ELSE COALESCE($6, refill_threshold_qty) END,
            refill_reminder      = COALESCE($7, refill_reminder)
         WHERE medication_id = $1 AND owner_id = $2 AND deleted_at IS NULL
        RETURNING `+stockColumns,
		medicationID, ownerID, unitCode, qtyPerDose, clearThreshold, threshold, reminder,
	)
	return scanStock(row)
}

// StockTxnInput adalah data satu transaksi stok.
type StockTxnInput struct {
	StockID      string
	MedicationID string
	OwnerID      string
	Kind         domain.StockTxnKind
	QtyDelta     float64
	LogID        *string
	Note         *string
	RecordedBy   *string
}

// InsertStockTransaction menulis satu baris transaksi stok.
//
// balance_after sengaja TIDAK ada di daftar kolom INSERT. Kolom itu NOT NULL
// tanpa default, tapi trigger BEFORE INSERT mengisinya sebelum constraint
// diperiksa. Mengirim nilai dari aplikasi akan sia-sia karena trigger
// menimpanya, dan menyesatkan karena terlihat seperti aplikasi yang
// menentukan saldo.
func (db *DB) InsertStockTransaction(ctx context.Context, q Querier, in StockTxnInput) error {
	_, err := q.Exec(ctx, `
        INSERT INTO stock_transactions (
            stock_id, medication_id, owner_id, kind, qty_delta,
            log_id, note, recorded_by
        ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		in.StockID, in.MedicationID, in.OwnerID, in.Kind, in.QtyDelta,
		in.LogID, in.Note, in.RecordedBy,
	)
	if err != nil {
		// Saldo negatif ditolak trigger, dan itu dilaporkan sebagai
		// pelanggaran CHECK. Dipetakan ke sentinel domain supaya handler
		// bisa membalas 409 dengan pesan yang berguna, bukan 500.
		if IsCheckViolation(err) || isRaiseException(err) {
			return domain.ErrStockInsufficient
		}
		// log_id unik berarti dosis ini sudah pernah memotong saldo.
		if IsUniqueViolation(err) && ConstraintName(err) == "uq_stock_txn_log" {
			return domain.ErrLogOccurrenceTaken
		}
		return fmt.Errorf("menulis transaksi stok: %w", err)
	}
	return nil
}

// StockTransaction adalah satu baris riwayat stok.
type StockTransaction struct {
	ID           string
	Kind         string
	QtyDelta     float64
	BalanceAfter float64
	LogID        *string
	Note         *string
	RecordedBy   *string
	CreatedAt    time.Time
}

// ListStockTransactions mengambil riwayat transaksi stok.
func (db *DB) ListStockTransactions(ctx context.Context, q Querier, medicationID, ownerID string, limit int) ([]StockTransaction, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := q.Query(ctx, `
        SELECT id, kind, qty_delta, balance_after, log_id, note, recorded_by, created_at
          FROM stock_transactions
         WHERE medication_id = $1 AND owner_id = $2
         ORDER BY created_at DESC
         LIMIT $3`, medicationID, ownerID, limit)
	if err != nil {
		return nil, fmt.Errorf("membaca riwayat stok: %w", err)
	}
	defer rows.Close()

	var out []StockTransaction
	for rows.Next() {
		var t StockTransaction
		if err := rows.Scan(&t.ID, &t.Kind, &t.QtyDelta, &t.BalanceAfter,
			&t.LogID, &t.Note, &t.RecordedBy, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("membaca riwayat stok: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// LowStockRow adalah satu obat yang saldonya sudah melewati ambang.
type LowStockRow struct {
	MedicationID       string
	MedicationName     string
	UnitCode           string
	CurrentQty         float64
	QtyPerDose         float64
	RefillThresholdQty *float64
	DosesRemaining     float64
}

// ListLowStock mengambil obat yang perlu restock.
//
// Memakai view v_medication_stock_status supaya perhitungan
// doses_remaining dan needs_refill tidak ditulis dua kali. View itu juga
// yang dipakai halaman stok, sehingga keduanya tidak bisa berbeda.
func (db *DB) ListLowStock(ctx context.Context, q Querier, ownerID string) ([]LowStockRow, error) {
	rows, err := q.Query(ctx, `
        SELECT medication_id, medication_name, unit_code, current_qty,
               qty_per_dose, refill_threshold_qty, doses_remaining
          FROM v_medication_stock_status
         WHERE owner_id = $1 AND needs_refill = true
         ORDER BY current_qty ASC`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("membaca stok menipis: %w", err)
	}
	defer rows.Close()

	var out []LowStockRow
	for rows.Next() {
		var r LowStockRow
		if err := rows.Scan(&r.MedicationID, &r.MedicationName, &r.UnitCode,
			&r.CurrentQty, &r.QtyPerDose, &r.RefillThresholdQty, &r.DosesRemaining); err != nil {
			return nil, fmt.Errorf("membaca stok menipis: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// isRaiseException melaporkan apakah error berasal dari RAISE EXCEPTION
// di dalam trigger.
//
// Kode 23514 adalah pelanggaran CHECK, sedangkan RAISE EXCEPTION tanpa
// ERRCODE eksplisit menghasilkan P0001. Keduanya perlu dikenali karena
// trigger stok bisa memakai salah satunya.
func isRaiseException(err error) bool {
	return sqlState(err) == "P0001"
}

// TouchLastRestocked mengisi waktu restock terakhir.
func (db *DB) TouchLastRestocked(ctx context.Context, q Querier, medicationID string) error {
	_, err := q.Exec(ctx, `
        UPDATE medication_stock SET last_restocked_at = now()
         WHERE medication_id = $1 AND deleted_at IS NULL`, medicationID)
	if err != nil {
		return fmt.Errorf("memperbarui waktu restock: %w", err)
	}
	return nil
}

// ConsumptionRate menghitung laju pemakaian stok per hari dari riwayat
// transaksi, beserta waktu konsumsi terakhir.
//
// Yang dihitung HANYA kind='consume', bukan semua transaksi. Bedanya
// penting: restock menambah saldo dan adjust bisa ke mana saja, sehingga
// memasukkannya akan menghasilkan "laju pemakaian" yang aneh — misalnya
// negatif setelah restock besar, atau positif hanya karena koreksi manual.
// Yang mencerminkan pemakaian nyata hanya consume.
//
// Laju dihitung sebagai total qty yang dikonsumsi dibagi jumlah hari dalam
// rentang, BUKAN dibagi jumlah hari yang ada transaksinya. Kalau dibagi
// jumlah hari bertransaksi, satu hari dengan satu dosis akan menghasilkan
// laju "1 per hari" walau user sebenarnya hanya minum sekali seminggu — dan
// perkiraan tanggal habisnya akan terlalu cepat.
func (db *DB) ConsumptionRate(
	ctx context.Context, q Querier, medicationID, ownerID string, days int,
) (ratePerDay float64, lastConsumedAt *time.Time, err error) {
	if days <= 0 {
		days = 30
	}

	var totalConsumed float64
	var lastAt *time.Time
	err = q.QueryRow(ctx, `
        SELECT
            coalesce(sum(-qty_delta) FILTER (WHERE kind = 'consume'), 0),
            max(created_at) FILTER (WHERE kind = 'consume')
          FROM stock_transactions
         WHERE medication_id = $1
           AND owner_id = $2
           AND created_at >= now() - $3::interval`,
		medicationID, ownerID, fmt.Sprintf("%d days", days),
	).Scan(&totalConsumed, &lastAt)
	if err != nil {
		return 0, nil, fmt.Errorf("menghitung laju pemakaian stok: %w", err)
	}

	if totalConsumed <= 0 {
		// Belum ada pemakaian sama sekali. Laju nol, dan pemanggil yang
		// memutuskan untuk tidak menampilkan perkiraan sama sekali —
		// mengarang angka dari nol akan menyesatkan.
		return 0, lastAt, nil
	}

	return totalConsumed / float64(days), lastAt, nil
}
