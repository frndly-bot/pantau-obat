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

// Logs adalah layanan pencatatan dosis.
//
// Ini inti aplikasi: di sinilah aturan jadwal bertemu kenyataan, dan di
// sinilah saldo stok berubah. Karena itu seluruh jalur tulisnya berjalan
// dalam SATU transaksi: catatan dosis, potong stok, pembatalan pengingat,
// dan jejak audit. Setengah jadi di sini berarti saldo dan riwayat berbeda.
type Logs struct {
	db     *postgres.DB
	logger *slog.Logger
}

// NewLogs membuat layanan pencatatan dosis.
func NewLogs(db *postgres.DB, logger *slog.Logger) *Logs {
	return &Logs{db: db, logger: logger}
}

// RecordInput adalah permintaan pencatatan dosis.
type RecordInput struct {
	ScheduleID      string
	SlotDate        time.Time
	ScheduledHour   int
	ScheduledMinute int
	ScheduledTZ     string

	Status  domain.LogStatus
	TakenAt *time.Time
	Note    *string
}

// Record mencatat dosis TERJADWAL.
//
// Urutan langkahnya penting dan tidak boleh ditukar:
//
//  1. Obat diverifikasi milik user, dan jamnya benar-benar milik obat itu.
//     Tanpa ini, id yang ditebak bisa mencatat dosis pada obat orang lain.
//  2. Catatan disimpan. Di sini uq_log_occurrence bekerja: kalau dosis itu
//     sudah tercatat, insert-nya gagal dan seluruh transaksi batal. Itu
//     yang membuat anti-ganda tidak bergantung pada pemeriksaan di aplikasi.
//  3. Stok dipotong, TAPI hanya bila statusnya taken atau late. skipped
//     tidak memotong, karena obatnya tidak masuk ke tubuh.
//  4. Pengingat yang menggantung dibatalkan, supaya user tidak menerima
//     notifikasi untuk sesuatu yang baru saja ia kerjakan.
func (s *Logs) Record(ctx context.Context, ownerID, medicationID string, in RecordInput, meta Meta) (*RecordResult, error) {
	if err := validateRecordInput(&in); err != nil {
		return nil, err
	}

	var result *RecordResult
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		med, err := s.db.MedicationByID(ctx, tx, medicationID, ownerID)
		if err != nil {
			return err
		}
		if med.AsNeeded {
			return fmt.Errorf("%w: obat PRN dicatat lewat endpoint tanpa jam jadwal", domain.ErrValidation)
		}

		// Jam harus benar-benar milik obat ini. Kalau tidak diperiksa,
		// schedule_id dari obat lain bisa dipakai untuk mencatat dosis
		// di bawah obat yang salah.
		sch, err := s.db.MedicationTimeByID(ctx, tx, in.ScheduleID, ownerID)
		if err != nil {
			return err
		}
		if sch.MedicationID != medicationID {
			// Ditolak sebagai "tidak ditemukan", bukan "tidak berhak",
			// supaya tidak membocorkan bahwa jam itu memang ada.
			return domain.ErrNotFound
		}

		takenAt, status, err := s.resolveStatus(ctx, tx, med, in)
		if err != nil {
			return err
		}

		hour, minute := in.ScheduledHour, in.ScheduledMinute
		tz := in.ScheduledTZ
		if tz == "" {
			tz = domain.DefaultTimezone
		}

		log, err := s.db.CreateLog(ctx, tx, postgres.CreateLogInput{
			MedicationID:    medicationID,
			ScheduleID:      &in.ScheduleID,
			OwnerID:         ownerID,
			RecordedBy:      ownerID,
			SlotDate:        in.SlotDate,
			ScheduledHour:   &hour,
			ScheduledMinute: &minute,
			ScheduledTZ:     &tz,
			Status:          status,
			TakenAt:         takenAt,
			Note:            in.Note,
			Source:          domain.SourceSelf,
		})
		if err != nil {
			return err
		}

		stockResult, err := s.applyStock(ctx, tx, log, ownerID)
		if err != nil {
			return err
		}

		if err := s.db.ClearReminderQueue(ctx, tx, medicationID, in.ScheduleID, in.SlotDate, hour, minute); err != nil {
			// Pengingat yang gagal dibatalkan tidak boleh menggagalkan
			// pencatatan dosis. Yang paling penting adalah catatannya
			// tersimpan dan stoknya benar.
			s.logger.WarnContext(ctx, "gagal membatalkan pengingat",
				slog.String("medication_id", medicationID))
		}

		if err := s.writeLogAudit(ctx, tx, ownerID, log, meta); err != nil {
			s.logger.WarnContext(ctx, "gagal menulis audit pencatatan dosis",
				slog.String("log_id", log.ID))
		}

		result = &RecordResult{Log: log, Stock: stockResult}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// RecordPRN mencatat dosis PRN (tanpa jadwal).
//
// Dosis PRN boleh dicatat berkali-kali sehari, jadi tidak ada occurrence key
// yang dijaga. Yang membatasi hanya prn_max_per_day, dan itu pun hanya
// peringatan, bukan penolakan: menolak pencatatan obat yang sudah telanjur
// diminum akan membuat riwayatnya tidak jujur.
func (s *Logs) RecordPRN(ctx context.Context, ownerID, medicationID string, in PRNInput, meta Meta) (*RecordResult, error) {
	if in.Status == "" {
		in.Status = domain.StatusTaken
	}
	if !in.Status.Valid() {
		return nil, fmt.Errorf("%w: status %q tidak dikenal", domain.ErrValidation, in.Status)
	}
	if in.SlotDate.IsZero() {
		return nil, fmt.Errorf("%w: tanggal dosis wajib diisi", domain.ErrValidation)
	}

	var result *RecordResult
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		// Keberadaan obat diverifikasi, tapi hasilnya tidak dipakai lebih
		// lanjut. Pemeriksaan ini yang mencegah user mencatat dosis pada obat
		// milik orang lain lewat id yang ditebak; kalau obatnya bukan
		// miliknya, MedicationByID mengembalikan ErrNotFound.
		if _, err := s.db.MedicationByID(ctx, tx, medicationID, ownerID); err != nil {
			return err
		}
		// Sengaja TIDAK menolak obat non-PRN di sini. Obat terjadwal juga
		// boleh dicatat di luar jadwalnya (misalnya dosis tambahan yang
		// diresepkan), dan menolaknya justru membuat catatan tidak lengkap.
		// Yang dibedakan hanya bentuk barisnya: tanpa schedule_id.

		takenAt := s.normalizeTakenAt(in.Status, in.TakenAt)

		log, err := s.db.CreateLog(ctx, tx, postgres.CreateLogInput{
			MedicationID: medicationID,
			ScheduleID:   nil,
			OwnerID:      ownerID,
			RecordedBy:   ownerID,
			SlotDate:     in.SlotDate,
			Status:       in.Status,
			TakenAt:      takenAt,
			Note:         in.Note,
			Source:       domain.SourceSelf,
		})
		if err != nil {
			return err
		}

		stockResult, err := s.applyStock(ctx, tx, log, ownerID)
		if err != nil {
			return err
		}

		if err := s.writeLogAudit(ctx, tx, ownerID, log, meta); err != nil {
			s.logger.WarnContext(ctx, "gagal menulis audit pencatatan dosis PRN",
				slog.String("log_id", log.ID))
		}

		result = &RecordResult{Log: log, Stock: stockResult}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// PRNInput adalah permintaan pencatatan dosis PRN.
type PRNInput struct {
	SlotDate time.Time
	Status   domain.LogStatus
	TakenAt  *time.Time
	Note     *string
}

// RecordResult adalah hasil pencatatan dosis.
type RecordResult struct {
	Log   *domain.MedicationLog
	Stock *StockChange
}

// StockChange melaporkan perubahan saldo akibat pencatatan dosis.
type StockChange struct {
	Consumed     bool
	QtyDelta     float64
	BalanceAfter float64
	UnitCode     string
	NeedsRefill  bool
}

// resolveStatus menentukan taken_at dan status final.
//
// taken_at diambil dari waktu SERVER bila klien tidak mengirimnya. Itu
// disengaja: waktu klien bisa dipalsukan atau salah zona, sedangkan yang
// dicatat adalah kapan server menerima konfirmasinya.
//
// Status dihitung dari selisih terhadap jadwal, memakai tolerance dari
// aturan frekuensi. Status `skipped` dilewatkan begitu saja: user yang
// menyatakan tidak minum tidak perlu dinilai telat atau tepat.
func (s *Logs) resolveStatus(ctx context.Context, q pgx.Tx, med *domain.Medication, in RecordInput) (*time.Time, domain.LogStatus, error) {
	if in.Status == domain.StatusSkipped {
		return nil, domain.StatusSkipped, nil
	}
	if in.Status == domain.StatusSnoozed {
		return nil, domain.StatusSnoozed, nil
	}

	now := time.Now().UTC()
	takenAt := now
	if in.TakenAt != nil {
		takenAt = in.TakenAt.UTC()
	}

	tolerance := 60
	if freq, err := s.db.FrequencyByMedication(ctx, q, med.ID); err == nil && freq != nil {
		tolerance = freq.ToleranceMinutes
	} else if err != nil && !isNotFound(err) {
		return nil, "", err
	}

	tz := in.ScheduledTZ
	if tz == "" {
		tz = domain.DefaultTimezone
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}

	// Jadwal dibandingkan dalam waktu LOKAL pemilik, bukan UTC. Membandingkan
	// dalam UTC akan menghitung selisih 7 jam untuk pengguna WIB hanya karena
	// zonanya berbeda, dan semua dosis akan berstatus late.
	scheduledLocal := time.Date(
		in.SlotDate.Year(), in.SlotDate.Month(), in.SlotDate.Day(),
		in.ScheduledHour, in.ScheduledMinute, 0, 0, loc)

	status := domain.ComputeStatus(scheduledLocal, takenAt.In(loc), tolerance)
	return &takenAt, status, nil
}

// normalizeTakenAt memastikan taken_at terisi sesuai statusnya.
//
// CHECK ck_log_taken_at di database mewajibkan taken_at ada untuk status
// taken dan late, dan kosong untuk status lainnya. Diisi di sini supaya
// pelanggaran constraint tidak pernah tercapai.
func (s *Logs) normalizeTakenAt(status domain.LogStatus, takenAt *time.Time) *time.Time {
	if status != domain.StatusTaken && status != domain.StatusLate {
		return nil
	}
	if takenAt != nil {
		t := takenAt.UTC()
		return &t
	}
	now := time.Now().UTC()
	return &now
}

// applyStock memotong saldo stok bila statusnya memang memotong.
//
// ATURAN: hanya taken dan late yang memotong. skipped tidak, karena obatnya
// tidak masuk ke tubuh; memotongnya akan melaporkan sisa obat lebih sedikit
// daripada kenyataan dan memunculkan peringatan restock terlalu cepat.
// snoozed juga tidak: menunda bukan meminum.
//
// Obat tanpa baris stok dilewatkan tanpa error. Stok bersifat opsional; user
// boleh melacak obat tanpa menghitung sisa tabletnya.
//
// PENTING: saat saldo TIDAK berubah, saldo terkini tetap dibaca dan
// dilaporkan. Mengembalikan struct kosong akan mengirim balance_after nol,
// dan klien yang menampilkannya akan menunjukkan stok 0 padahal saldonya
// masih utuh. Angka nol di sini bukan "tidak ada data", tapi "saldonya nol",
// dan keduanya sangat berbeda bagi user.
func (s *Logs) applyStock(ctx context.Context, tx pgx.Tx, log *domain.MedicationLog, ownerID string) (*StockChange, error) {
	stock, err := s.db.StockForConsume(ctx, tx, log.MedicationID)
	if err != nil {
		if isNotFound(err) {
			// Tidak ada baris stok: bukan kesalahan. Dosis tetap tercatat,
			// dan tidak ada saldo yang bisa dilaporkan (nil, bukan nol).
			return nil, nil
		}
		return nil, err
	}

	// Saldo tidak berubah untuk skipped dan snoozed. Tetap laporkan saldo
	// terkini supaya klien tidak perlu GET ulang, dan supaya tidak ada
	// risiko angka nol ditafsirkan sebagai saldo kosong.
	if !log.ConsumesStock() {
		return &StockChange{
			Consumed:     false,
			QtyDelta:     0,
			BalanceAfter: stock.CurrentQty,
			UnitCode:     stock.UnitCode,
			NeedsRefill:  stock.NeedsRefill(),
		}, nil
	}

	in := postgres.StockTxnInput{
		StockID:      stock.ID,
		MedicationID: log.MedicationID,
		OwnerID:      ownerID,
		Kind:         domain.StockConsume,
		QtyDelta:     -stock.QtyPerDose,
		LogID:        &log.ID,
		RecordedBy:   &ownerID,
	}
	if err := s.db.InsertStockTransaction(ctx, tx, in); err != nil {
		return nil, err
	}

	// Baca ulang supaya saldo akhir memuat hasil perhitungan trigger.
	after, err := s.db.StockByMedication(ctx, tx, log.MedicationID)
	if err != nil {
		return nil, err
	}

	return &StockChange{
		Consumed:     true,
		QtyDelta:     -stock.QtyPerDose,
		BalanceAfter: after.CurrentQty,
		UnitCode:     after.UnitCode,
		NeedsRefill:  after.NeedsRefill(),
	}, nil
}

// Correct mengoreksi catatan dosis.
//
// Koreksi = soft-delete baris lama + insert baris baru, dalam satu transaksi.
// Bukan UPDATE, karena riwayat bahwa dosis pernah dicatat dengan status lain
// harus tetap bisa ditelusuri.
//
// Stok ikut dikoreksi. Kalau status lama memotong stok dan status baru tidak,
// potongannya dikembalikan. Sebaliknya, kalau status lama tidak memotong dan
// status baru memotong, potongan baru dibuat. Tanpa ini, koreksi akan membuat
// saldo tidak lagi cocok dengan riwayat dosis.
func (s *Logs) Correct(ctx context.Context, ownerID, logID string, in CorrectInput, meta Meta) (*RecordResult, error) {
	if in.Status == "" {
		return nil, fmt.Errorf("%w: status wajib diisi", domain.ErrValidation)
	}
	if !in.Status.Valid() {
		return nil, fmt.Errorf("%w: status %q tidak dikenal", domain.ErrValidation, in.Status)
	}

	var result *RecordResult
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		old, err := s.db.LogByID(ctx, tx, logID, ownerID)
		if err != nil {
			return err
		}

		oldConsumed := old.ConsumesStock()
		newConsumes := in.Status == domain.StatusTaken || in.Status == domain.StatusLate

		// Perubahan saldo dihitung dari SELISIH, bukan dari status baru saja.
		// Kalau keduanya sama-sama memotong, tidak ada penyesuaian yang perlu
		// dibuat.
		var delta float64
		if oldConsumed != newConsumes {
			stock, err := s.db.StockForConsume(ctx, tx, old.MedicationID)
			if err != nil && !isNotFound(err) {
				return err
			}
			if stock != nil {
				if newConsumes {
					delta = -stock.QtyPerDose // dulu tidak potong, sekarang potong
				} else {
					delta = stock.QtyPerDose // dulu potong, sekarang tidak
				}
			}
		}

		if err := s.db.SoftDeleteLog(ctx, tx, logID, ownerID); err != nil {
			return err
		}

		takenAt := s.normalizeTakenAt(in.Status, in.TakenAt)
		newLog, err := s.db.CreateLog(ctx, tx, postgres.CreateLogInput{
			MedicationID:    old.MedicationID,
			ScheduleID:      old.ScheduleID,
			OwnerID:         ownerID,
			RecordedBy:      ownerID,
			SlotDate:        old.SlotDate,
			ScheduledAt:     old.ScheduledAt,
			ScheduledHour:   old.ScheduledHour,
			ScheduledMinute: old.ScheduledMinute,
			ScheduledTZ:     old.ScheduledTZ,
			Status:          in.Status,
			TakenAt:         takenAt,
			Note:            in.Note,
			Source:          old.Source,
		})
		if err != nil {
			return err
		}

		var change *StockChange
		if delta != 0 {
			stock, err := s.db.StockForConsume(ctx, tx, old.MedicationID)
			if err != nil {
				return err
			}
			if err := s.db.InsertStockTransaction(ctx, tx, postgres.StockTxnInput{
				StockID:      stock.ID,
				MedicationID: old.MedicationID,
				OwnerID:      ownerID,
				Kind:         domain.StockAdjust,
				QtyDelta:     delta,
				LogID:        &newLog.ID,
				Note:         strPtr("koreksi catatan dosis"),
				RecordedBy:   &ownerID,
			}); err != nil {
				return err
			}
			after, err := s.db.StockByMedication(ctx, tx, old.MedicationID)
			if err != nil {
				return err
			}
			change = &StockChange{
				Consumed:     newConsumes,
				QtyDelta:     delta,
				BalanceAfter: after.CurrentQty,
				UnitCode:     after.UnitCode,
				NeedsRefill:  after.NeedsRefill(),
			}
		} else {
			change = &StockChange{Consumed: newConsumes}
		}

		if err := s.db.WriteAudit(ctx, tx, postgres.AuditEntry{
			ActorID:      &ownerID,
			OwnerID:      &ownerID,
			ActorRole:    string(domain.ActorUser),
			Action:       postgres.ActionUpdate,
			ResourceType: postgres.ResourceLog,
			ResourceID:   &newLog.ID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			BeforeState: map[string]any{
				"id":     old.ID,
				"status": string(old.Status),
			},
			AfterState: map[string]any{
				"id":     newLog.ID,
				"status": string(newLog.Status),
			},
			Meta: map[string]any{"correction": true, "stock_delta": delta},
		}); err != nil {
			s.logger.WarnContext(ctx, "gagal menulis audit koreksi dosis",
				slog.String("log_id", newLog.ID))
		}

		result = &RecordResult{Log: newLog, Stock: change}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// CorrectInput adalah permintaan koreksi catatan dosis.
type CorrectInput struct {
	Status  domain.LogStatus
	TakenAt *time.Time
	Note    *string
}

// Snooze menunda pengingat satu dosis.
//
// Snooze TIDAK membuat catatan dosis. Ia menggeser waktu pengingat, dan
// riwayat pengiriman tidak boleh ikut kotor karena itu. Yang diperbarui
// adalah baris di bump_queue, sesuai notification-v2 bagian 7.
func (s *Logs) Snooze(ctx context.Context, ownerID, medicationID, scheduleID string, minutes int, meta Meta) error {
	if minutes != 5 && minutes != 10 && minutes != 30 {
		return fmt.Errorf("%w: snooze hanya boleh 5, 10, atau 30 menit", domain.ErrValidation)
	}

	return s.db.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := s.db.MedicationByID(ctx, tx, medicationID, ownerID); err != nil {
			return err
		}
		sch, err := s.db.MedicationTimeByID(ctx, tx, scheduleID, ownerID)
		if err != nil {
			return err
		}
		if sch.MedicationID != medicationID {
			return domain.ErrNotFound
		}

		// Batas tiga snooze beruntun per dosis, sesuai TRD bagian D2.
		//
		// Sebelumnya konstanta `MaxConsecutiveSnoozes` hanya DIDEFINISIKAN
		// dan tidak pernah diperiksa, sehingga pengingat bisa ditunda tanpa
		// batas. Pada aplikasi kesehatan, aturan yang diumumkan tapi tidak
		// dijalankan lebih merusak daripada fitur yang tidak pernah
		// dijanjikan: pengguna mengandalkannya untuk mengambil keputusan.
		//
		// Dihitung dari jejak audit, bukan dari kolom baru: snooze memang
		// tidak punya baris tersendiri (ia tidak membuat catatan dosis),
		// jadi audit adalah satu-satunya tempat jejaknya tersimpan.
		dipakai, err := s.db.CountSnoozesForOccurrence(ctx, tx, ownerID, scheduleID)
		if err != nil {
			return err
		}
		if dipakai >= domain.MaxConsecutiveSnoozes {
			// Dicatat juga supaya penolakan terlihat di jejak — pengguna
			// yang bertanya "kenapa tidak bisa ditunda lagi" punya
			// jawaban yang bisa diperiksa, bukan hanya pesan sesaat.
			_ = s.db.WriteAudit(ctx, tx, postgres.AuditEntry{
				ActorID:      &ownerID,
				OwnerID:      &ownerID,
				ActorRole:    string(domain.ActorUser),
				Action:       postgres.ActionUpdate,
				ResourceType: postgres.ResourceLog,
				ResourceID:   &scheduleID,
				IP:           meta.IP,
				UserAgent:    meta.UserAgent,
				Meta: map[string]any{
					"snooze_rejected": "limit_reached",
					"limit":           domain.MaxConsecutiveSnoozes,
					"medication_id":   medicationID,
				},
			})
			return domain.ErrSnoozeLimit
		}

		return s.db.WriteAudit(ctx, tx, postgres.AuditEntry{
			ActorID:      &ownerID,
			OwnerID:      &ownerID,
			ActorRole:    string(domain.ActorUser),
			Action:       postgres.ActionUpdate,
			ResourceType: postgres.ResourceLog,
			ResourceID:   &scheduleID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			Meta: map[string]any{
				"snooze_minutes": minutes,
				"snooze_number":  dipakai + 1,
				"medication_id":  medicationID,
			},
		})
	})
}

// List mengambil daftar catatan dosis.
func (s *Logs) List(ctx context.Context, ownerID string, f LogListFilter) ([]*domain.MedicationLog, error) {
	return s.db.ListLogs(ctx, s.db.Pool(), postgres.ListLogsFilter{
		OwnerID:      ownerID,
		MedicationID: f.MedicationID,
		From:         f.From,
		To:           f.To,
		Status:       f.Status,
		Source:       f.Source,
		Limit:        f.Limit,
	})
}

// LogListFilter adalah syarat pencarian daftar catatan dosis.
type LogListFilter struct {
	MedicationID string
	From         *time.Time
	To           *time.Time
	Status       string
	// Source menyaring asal catatan: self, caregiver, atau system.
	Source string
	Limit  int
}

// Today mengambil ringkasan catatan dosis hari ini.
//
// "Hari ini" dihitung memakai zona waktu user, bukan UTC. Tanpa itu, pukul
// 07:00 WIB masih dianggap tanggal kemarin karena UTC-nya pukul 00:00.
func (s *Logs) Today(ctx context.Context, ownerID, timezone string) ([]*domain.MedicationLog, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.FixedZone("WIB", 7*3600)
	}
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	return s.db.ListLogs(ctx, s.db.Pool(), postgres.ListLogsFilter{
		OwnerID: ownerID,
		From:    &today,
		To:      &today,
		Limit:   200,
	})
}

// writeLogAudit menulis jejak audit pencatatan dosis.
//
// action dipilih sesuai status, karena audit_log.action membedakan
// log_taken dan log_skipped. Membedakannya membuat riwayat bisa ditelusuri
// tanpa perlu membaca isi meta.
func (s *Logs) writeLogAudit(ctx context.Context, q pgx.Tx, ownerID string, log *domain.MedicationLog, meta Meta) error {
	action := postgres.ActionLogTaken
	if log.Status == domain.StatusSkipped {
		action = postgres.ActionLogSkipped
	}

	// NAMA OBAT TIDAK MASUK AUDIT MAUPUN LOG. Ini batas privasi, bukan
	// preferensi: yang disimpan hanya id. Lihat backend/README.md butir 4.
	detail := map[string]any{
		"medication_id": log.MedicationID,
		"status":        string(log.Status),
		"slot_date":     log.SlotDate.Format(time.DateOnly),
		"source":        string(log.Source),
	}
	if log.ScheduledHour != nil && log.ScheduledMinute != nil {
		detail["scheduled_time"] = domain.FormatClock(*log.ScheduledHour, *log.ScheduledMinute)
	}

	return s.db.WriteAudit(ctx, q, postgres.AuditEntry{
		ActorID:      &ownerID,
		OwnerID:      &ownerID,
		ActorRole:    string(domain.ActorUser),
		Action:       action,
		ResourceType: postgres.ResourceLog,
		ResourceID:   &log.ID,
		IP:           meta.IP,
		UserAgent:    meta.UserAgent,
		AfterState:   detail,
	})
}

// validateRecordInput memeriksa input pencatatan dosis terjadwal.
func validateRecordInput(in *RecordInput) error {
	var problems []string

	if strings.TrimSpace(in.ScheduleID) == "" {
		problems = append(problems, "schedule_id wajib diisi untuk dosis terjadwal")
	}
	if in.SlotDate.IsZero() {
		problems = append(problems, "slot_date wajib diisi")
	}
	if in.ScheduledHour < 0 || in.ScheduledHour > 23 {
		problems = append(problems, "jam jadwal harus antara 0 dan 23")
	}
	if in.ScheduledMinute < 0 || in.ScheduledMinute > 59 {
		problems = append(problems, "menit jadwal harus antara 0 dan 59")
	}
	if in.Status != "" && !in.Status.Valid() {
		problems = append(problems, fmt.Sprintf("status %q tidak dikenal", in.Status))
	}
	if in.Note != nil && len(*in.Note) > 500 {
		problems = append(problems, "catatan maksimal 500 karakter")
	}

	if len(problems) > 0 {
		return fmt.Errorf("%w: %s", domain.ErrValidation, strings.Join(problems, "; "))
	}
	return nil
}

// RecordFor mencatat dosis ATAS NAMA pemilik data oleh pihak lain.
//
// Dipakai jalur caregiver helper. Bedanya dengan Record hanya pada
// recorded_by dan source:
//
//   - RecordedBy diisi actorID (caregiver), bukan pemilik.
//   - Source diisi 'caregiver', bukan 'self'.
//
// Perbedaan itu bukan hiasan: saat menelusuri kenapa sebuah dosis tercatat
// padahal pasien tidak membuka aplikasinya, satu-satunya yang menjawab
// adalah kolom ini. Nilainya ditetapkan DI SINI, bukan diterima dari klien —
// kalau klien yang menentukan, catatan atas nama pasien bisa menyamar
// sebagai catatan pasien sendiri.
//
// Pemeriksaan hak akses dilakukan PEMANGGIL, sebelum masuk ke sini. Fungsi
// ini tidak memeriksa relasi caregiver, karena paket service tidak boleh
// tahu tentang alur otorisasi yang spesifik.
func (s *Logs) RecordFor(ctx context.Context, ownerID, actorID, medicationID string, in RecordInput, meta Meta) (*RecordResult, error) {
	if err := validateRecordInput(&in); err != nil {
		return nil, err
	}

	var result *RecordResult
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		med, err := s.db.MedicationByID(ctx, tx, medicationID, ownerID)
		if err != nil {
			return err
		}
		if med.AsNeeded {
			return fmt.Errorf("%w: obat PRN dicatat lewat endpoint tanpa jam jadwal", domain.ErrValidation)
		}

		sch, err := s.db.MedicationTimeByID(ctx, tx, in.ScheduleID, ownerID)
		if err != nil {
			return err
		}
		if sch.MedicationID != medicationID {
			return domain.ErrNotFound
		}

		takenAt, status, err := s.resolveStatus(ctx, tx, med, in)
		if err != nil {
			return err
		}

		hour, minute := in.ScheduledHour, in.ScheduledMinute
		tz := in.ScheduledTZ
		if tz == "" {
			tz = domain.DefaultTimezone
		}

		log, err := s.db.CreateLog(ctx, tx, postgres.CreateLogInput{
			MedicationID:    medicationID,
			ScheduleID:      &in.ScheduleID,
			OwnerID:         ownerID,
			RecordedBy:      actorID,
			SlotDate:        in.SlotDate,
			ScheduledHour:   &hour,
			ScheduledMinute: &minute,
			ScheduledTZ:     &tz,
			Status:          status,
			TakenAt:         takenAt,
			Note:            in.Note,
			Source:          domain.SourceCaregiver,
		})
		if err != nil {
			return err
		}

		stockResult, err := s.applyStock(ctx, tx, log, ownerID)
		if err != nil {
			return err
		}

		if err := s.db.ClearReminderQueue(ctx, tx, medicationID, in.ScheduleID, in.SlotDate, hour, minute); err != nil {
			s.logger.WarnContext(ctx, "gagal membatalkan pengingat",
				slog.String("medication_id", medicationID))
		}

		// Audit ditulis dengan owner_id = pemilik data, bukan caregiver.
		// Dengan begitu pasien bisa melihat bahwa ada orang lain yang
		// mencatat dosis atas namanya — dan itu memang perlu dia ketahui.
		action := postgres.ActionLogTaken
		if log.Status == domain.StatusSkipped {
			action = postgres.ActionLogSkipped
		}
		if err := s.db.WriteAudit(ctx, tx, postgres.AuditEntry{
			ActorID:      &actorID,
			OwnerID:      &ownerID,
			ActorRole:    string(domain.ActorUser),
			Action:       action,
			ResourceType: postgres.ResourceLog,
			ResourceID:   &log.ID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			AfterState: map[string]any{
				"medication_id": medicationID,
				"status":        string(log.Status),
				"slot_date":     log.SlotDate.Format(time.DateOnly),
				"source":        string(domain.SourceCaregiver),
				"by_caregiver":  true,
			},
		}); err != nil {
			s.logger.WarnContext(ctx, "gagal menulis audit pencatatan oleh caregiver",
				slog.String("log_id", log.ID))
		}

		result = &RecordResult{Log: log, Stock: stockResult}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// WriteExportAudit mencatat jejak ekspor CSV ke audit_log.
//
// WAJIB untuk ekspor atas nama pasien (jalur caregiver): data kesehatan
// keluar dari sistem atas nama orang lain, dan itu harus berjejak dengan
// alasan yang bisa dibaca manusia. Ekspor milik sendiri TIDAK lewat sini —
// tidak ada yang perlu diaudit dari orang membaca catatannya sendiri.
func (s *Logs) WriteExportAudit(ctx context.Context, actorID, ownerID string, from, to time.Time, reason string) error {
	return s.db.WriteAudit(ctx, s.db.Pool(), postgres.AuditEntry{
		ActorID:      &actorID,
		OwnerID:      &ownerID,
		ActorRole:    string(domain.ActorUser),
		Action:       postgres.ActionExport,
		ResourceType: postgres.ResourceLog,
		Meta: map[string]any{
			"from":   from.Format(time.DateOnly),
			"to":     to.Format(time.DateOnly),
			"reason": reason,
		},
	})
}
