package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	"github.com/frndly-bot/pantau-obat/backend/internal/repo/postgres"
)

// Stock adalah layanan pengelolaan stok.
//
// Satu aturan yang tidak boleh dilanggar di seluruh file ini:
// current_qty TIDAK PERNAH diubah langsung. Setiap perubahan saldo harus
// lewat stock_transactions, karena trigger apply_stock_txn yang menghitung
// saldo baru sekaligus menolak saldo negatif. Menulis current_qty langsung
// akan memutus hubungan antara saldo dan riwayatnya, dan selisihnya tidak
// akan pernah bisa dijelaskan lagi.
type Stock struct {
	db     *postgres.DB
	logger *slog.Logger
}

// NewStock membuat layanan stok.
func NewStock(db *postgres.DB, logger *slog.Logger) *Stock {
	return &Stock{db: db, logger: logger}
}

// Detail adalah ringkasan stok sebuah obat.
type Detail struct {
	Stock *domain.MedicationStock

	// EstimatedDaysLeft adalah perkiraan berapa hari lagi obat habis.
	//
	// Dihitung dari LAJU PEMAKAIAN NYATA dalam 30 hari terakhir, bukan dari
	// jumlah dosis terjadwal. Bedanya penting: pasien yang sering melewatkan
	// dosis akan kehabisan obat lebih lambat daripada jadwalnya, dan
	// perkiraan berbasis jadwal akan terus salah untuknya.
	//
	// Nil berarti belum cukup data untuk memperkirakan. Itu lebih jujur
	// daripada mengarang angka.
	EstimatedDaysLeft *float64

	// DosesRemaining adalah sisa dosis, dihitung dari saldo dibagi pemakaian
	// per dosis.
	DosesRemaining float64

	// NeedsRefill menandai saldo sudah melewati ambang peringatan.
	NeedsRefill bool

	// LastConsumedAt adalah kapan terakhir kali stok terpotong.
	LastConsumedAt *time.Time
}

// Get mengambil detail stok sebuah obat.
func (s *Stock) Get(ctx context.Context, ownerID, medicationID string) (*Detail, error) {
	var out *Detail
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := s.db.MedicationByID(ctx, tx, medicationID, ownerID); err != nil {
			return err
		}

		stock, err := s.db.StockByMedication(ctx, tx, medicationID)
		if err != nil {
			return err
		}

		detail := &Detail{
			Stock:          stock,
			DosesRemaining: stock.DosesRemaining(),
			NeedsRefill:    stock.NeedsRefill(),
		}

		// Baca laju pemakaian dari riwayat transaksi. Kegagalan di sini
		// tidak menggagalkan seluruh permintaan: detail stok tetap berguna
		// walau perkiraan harinya tidak tersedia.
		rate, lastAt, err := s.db.ConsumptionRate(ctx, tx, medicationID, ownerID, 30)
		if err != nil {
			s.logger.WarnContext(ctx, "gagal membaca laju pemakaian stok",
				slog.String("medication_id", medicationID),
				slog.String("error", err.Error()))
		} else {
			detail.LastConsumedAt = lastAt
			if rate > 0 {
				days := stock.CurrentQty / rate
				detail.EstimatedDaysLeft = &days
			}
		}

		out = detail
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SetSettings membuat atau mengubah pengaturan stok.
//
// Obat pertama-tama diverifikasi milik user. Setelah itu, dua keadaan
// ditangani berbeda:
//
//   - Belum ada baris stok: baris dibuat, dan bila qty awal > 0, transaksi
//     `initial` ikut ditulis supaya saldonya punya jejak.
//   - Sudah ada: hanya pengaturannya yang diubah. Saldo TIDAK disentuh,
//     karena mengubah saldo lewat jalur ini akan memutus riwayatnya.
func (s *Stock) SetSettings(ctx context.Context, ownerID, medicationID string, in SettingsInput, meta Meta) (*domain.MedicationStock, error) {
	if err := validateSettings(&in); err != nil {
		return nil, err
	}

	var out *domain.MedicationStock
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		med, err := s.db.MedicationByID(ctx, tx, medicationID, ownerID)
		if err != nil {
			return err
		}

		// Cek keberadaan baris stok. Hasilnya tidak dipakai selain untuk
		// menentukan jalur mana yang ditempuh, jadi tidak disimpan ke variabel.
		_, err = s.db.StockByMedication(ctx, tx, medicationID)
		if err == nil {
			updated, err := s.db.UpdateStockSettings(ctx, tx, medicationID, ownerID,
				&in.UnitCode, &in.QtyPerDose, in.RefillThresholdQty,
				in.ClearThreshold, &in.RefillReminder)
			if err != nil {
				return err
			}

			// Saldo awal hanya boleh disuntik saat baris stok BARU dibuat.
			// Kalau user mengirim initial_qty pada obat yang sudah punya
			// stok, itu tidak diabaikan diam-diam: ia diminta memakai
			// endpoint restock, supaya penambahan saldo punya alasan yang
			// tercatat.
			if in.InitialQty > 0 {
				return fmt.Errorf("%w: stok sudah ada; pakai endpoint restock untuk menambah saldo", domain.ErrValidation)
			}

			if err := s.writeStockAudit(ctx, tx, ownerID, medicationID, "stock_settings_updated", meta, map[string]any{
				"unit_code":    updated.UnitCode,
				"qty_per_dose": updated.QtyPerDose,
			}); err != nil {
				s.logger.WarnContext(ctx, "gagal menulis audit pengaturan stok",
					slog.String("medication_id", medicationID))
			}
			out = updated
			return nil
		}
		if !isNotFound(err) {
			return err
		}

		created, err := s.db.CreateStock(ctx, tx, postgres.CreateStockInput{
			MedicationID:       medicationID,
			OwnerID:            ownerID,
			UnitCode:           in.UnitCode,
			InitialQty:         in.InitialQty,
			QtyPerDose:         in.QtyPerDose,
			RefillThresholdQty: in.RefillThresholdQty,
			RefillReminder:     in.RefillReminder,
		})
		if err != nil {
			return err
		}

		if in.InitialQty > 0 {
			if err := s.db.InsertStockTransaction(ctx, tx, postgres.StockTxnInput{
				StockID:      created.ID,
				MedicationID: medicationID,
				OwnerID:      ownerID,
				Kind:         domain.StockInitial,
				QtyDelta:     in.InitialQty,
				Note:         strPtr("stok awal"),
				RecordedBy:   &ownerID,
			}); err != nil {
				return err
			}
			// Baca ulang supaya saldo memuat hasil perhitungan trigger.
			created, err = s.db.StockByMedication(ctx, tx, medicationID)
			if err != nil {
				return err
			}
		}

		if err := s.writeStockAudit(ctx, tx, ownerID, medicationID, "stock_created", meta, map[string]any{
			"unit_code":   created.UnitCode,
			"initial_qty": in.InitialQty,
			"medication":  med.ID,
		}); err != nil {
			s.logger.WarnContext(ctx, "gagal menulis audit pembuatan stok",
				slog.String("medication_id", medicationID))
		}

		out = created
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SettingsInput adalah permintaan pengaturan stok.
type SettingsInput struct {
	UnitCode           string
	InitialQty         float64
	QtyPerDose         float64
	RefillThresholdQty *float64
	ClearThreshold     bool
	RefillReminder     bool
}

// Restock menambah saldo stok.
//
// Menulis satu transaksi, tidak menyentuh saldo langsung. Trigger yang
// menghitung saldo baru dan mengisi balance_after.
func (s *Stock) Restock(ctx context.Context, ownerID, medicationID string, in RestockInput, meta Meta) (*StockChange, error) {
	if in.Qty <= 0 {
		return nil, fmt.Errorf("%w: jumlah restock harus lebih dari nol", domain.ErrValidation)
	}
	if in.Note != nil && len(*in.Note) > 500 {
		return nil, fmt.Errorf("%w: catatan maksimal 500 karakter", domain.ErrValidation)
	}

	var out *StockChange
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := s.db.MedicationByID(ctx, tx, medicationID, ownerID); err != nil {
			return err
		}

		stock, err := s.db.StockForConsume(ctx, tx, medicationID)
		if err != nil {
			if isNotFound(err) {
				return fmt.Errorf("%w: obat ini belum punya pengaturan stok", domain.ErrValidation)
			}
			return err
		}

		if err := s.db.InsertStockTransaction(ctx, tx, postgres.StockTxnInput{
			StockID:      stock.ID,
			MedicationID: medicationID,
			OwnerID:      ownerID,
			Kind:         domain.StockRestock,
			QtyDelta:     in.Qty,
			Note:         in.Note,
			RecordedBy:   &ownerID,
		}); err != nil {
			return err
		}

		if err := s.db.TouchLastRestocked(ctx, tx, medicationID); err != nil {
			s.logger.WarnContext(ctx, "gagal memperbarui waktu restock",
				slog.String("medication_id", medicationID))
		}

		after, err := s.db.StockByMedication(ctx, tx, medicationID)
		if err != nil {
			return err
		}

		if err := s.writeStockAudit(ctx, tx, ownerID, medicationID, "stock_restocked", meta, map[string]any{
			"qty":           in.Qty,
			"balance_after": after.CurrentQty,
			"note":          in.Note,
		}); err != nil {
			s.logger.WarnContext(ctx, "gagal menulis audit restock",
				slog.String("medication_id", medicationID))
		}

		out = &StockChange{
			QtyDelta:     in.Qty,
			BalanceAfter: after.CurrentQty,
			UnitCode:     after.UnitCode,
			NeedsRefill:  after.NeedsRefill(),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RestockInput adalah permintaan penambahan saldo.
type RestockInput struct {
	Qty  float64
	Note *string
}

// Adjust mengoreksi saldo secara manual.
//
// Delta boleh negatif maupun positif. Kalau negatif, trigger menolaknya bila
// saldo akan menjadi negatif, dan itu dilaporkan sebagai STOCK_INSUFFICIENT.
// Pengurangan lewat adjust TIDAK menaut log_id, karena koreksi memang tidak
// berasal dari satu dosis tertentu.
func (s *Stock) Adjust(ctx context.Context, ownerID, medicationID string, in AdjustInput, meta Meta) (*StockChange, error) {
	if in.QtyDelta == 0 {
		return nil, fmt.Errorf("%w: perubahan saldo tidak boleh nol", domain.ErrValidation)
	}
	if strings.TrimSpace(in.Note) == "" {
		// Alasan wajib untuk koreksi manual. Tanpa itu, saldo yang berubah
		// tidak bisa dijelaskan saat ditelusuri, dan itu persis yang membuat
		// data stok tidak bisa dipercaya.
		return nil, fmt.Errorf("%w: alasan koreksi wajib diisi", domain.ErrValidation)
	}
	if len(in.Note) > 500 {
		return nil, fmt.Errorf("%w: alasan maksimal 500 karakter", domain.ErrValidation)
	}
	if !domain.IsAdjustableStockKind(in.Kind) {
		return nil, fmt.Errorf("%w: jenis koreksi %q tidak dikenal", domain.ErrValidation, in.Kind)
	}

	var out *StockChange
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := s.db.MedicationByID(ctx, tx, medicationID, ownerID); err != nil {
			return err
		}

		stock, err := s.db.StockForConsume(ctx, tx, medicationID)
		if err != nil {
			if isNotFound(err) {
				return fmt.Errorf("%w: obat ini belum punya pengaturan stok", domain.ErrValidation)
			}
			return err
		}

		note := in.Note
		if err := s.db.InsertStockTransaction(ctx, tx, postgres.StockTxnInput{
			StockID:      stock.ID,
			MedicationID: medicationID,
			OwnerID:      ownerID,
			Kind:         in.Kind,
			QtyDelta:     in.QtyDelta,
			Note:         &note,
			RecordedBy:   &ownerID,
		}); err != nil {
			return err
		}

		after, err := s.db.StockByMedication(ctx, tx, medicationID)
		if err != nil {
			return err
		}

		if err := s.writeStockAudit(ctx, tx, ownerID, medicationID, "stock_adjusted", meta, map[string]any{
			"qty_delta":     in.QtyDelta,
			"kind":          string(in.Kind),
			"balance_after": after.CurrentQty,
			"note":          in.Note,
		}); err != nil {
			s.logger.WarnContext(ctx, "gagal menulis audit koreksi stok",
				slog.String("medication_id", medicationID))
		}

		out = &StockChange{
			QtyDelta:     in.QtyDelta,
			BalanceAfter: after.CurrentQty,
			UnitCode:     after.UnitCode,
			NeedsRefill:  after.NeedsRefill(),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// AdjustInput adalah permintaan koreksi saldo.
type AdjustInput struct {
	QtyDelta float64
	Kind     domain.StockTxnKind
	Note     string
}

// Transactions mengambil riwayat transaksi stok.
func (s *Stock) Transactions(ctx context.Context, ownerID, medicationID string, limit int) ([]postgres.StockTransaction, error) {
	var out []postgres.StockTransaction
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := s.db.MedicationByID(ctx, tx, medicationID, ownerID); err != nil {
			return err
		}
		var err error
		out, err = s.db.ListStockTransactions(ctx, tx, medicationID, ownerID, limit)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// LowStock mengambil daftar obat yang perlu restock.
func (s *Stock) LowStock(ctx context.Context, ownerID string) ([]postgres.LowStockRow, error) {
	return s.db.ListLowStock(ctx, s.db.Pool(), ownerID)
}

// writeStockAudit menulis jejak audit perubahan stok.
//
// Nama obat TIDAK masuk audit maupun log. Yang disimpan hanya id, sesuai
// batas privasi yang sudah ditetapkan.
func (s *Stock) writeStockAudit(ctx context.Context, q pgx.Tx, ownerID, medicationID, action string, meta Meta, detail map[string]any) error {
	detail["medication_id"] = medicationID
	return s.db.WriteAudit(ctx, q, postgres.AuditEntry{
		ActorID:      &ownerID,
		OwnerID:      &ownerID,
		ActorRole:    string(domain.ActorUser),
		Action:       postgres.ActionUpdate,
		ResourceType: postgres.ResourceStock,
		ResourceID:   &medicationID,
		IP:           meta.IP,
		UserAgent:    meta.UserAgent,
		Meta:         mergeMaps(map[string]any{"action": action}, detail),
	})
}

// validateSettings memeriksa pengaturan stok.
func validateSettings(in *SettingsInput) error {
	var problems []string

	in.UnitCode = strings.TrimSpace(in.UnitCode)
	if in.UnitCode == "" {
		problems = append(problems, "satuan stok wajib diisi")
	}
	if in.InitialQty < 0 {
		problems = append(problems, "stok awal tidak boleh negatif")
	}
	if in.QtyPerDose <= 0 {
		problems = append(problems, "pemakaian per dosis harus lebih dari nol")
	}
	if in.RefillThresholdQty != nil && *in.RefillThresholdQty < 0 {
		problems = append(problems, "ambang restock tidak boleh negatif")
	}

	if len(problems) > 0 {
		return fmt.Errorf("%w: %s", domain.ErrValidation, strings.Join(problems, "; "))
	}
	return nil
}
