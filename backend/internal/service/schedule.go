package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	"github.com/frndly-bot/pantau-obat/backend/internal/repo/postgres"
)

// Schedule adalah layanan jadwal dosis harian.
//
// Kenapa layanan ini ada: aplikasi user harus bisa menampilkan dosis yang
// BELUM dicatat ("jam 14:00 — belum diminum"), bukan hanya yang sudah
// dicatat. Tanpa itu layar Hari Ini kosong sampai user mencatat sesuatu,
// dan justru saat itulah pengingat paling dibutuhkan.
//
// Seluruh perhitungan aturan frekuensi memakai `domain.ScheduledSlotsInRange`
// — fungsi yang SAMA yang dipakai laporan kepatuhan. Jadi angka di layar Hari
// Ini dan angka kepatuhan tidak mungkin berbeda.
type Schedule struct {
	db     *postgres.DB
	logger *slog.Logger
}

// NewSchedule membuat layanan jadwal.
func NewSchedule(db *postgres.DB, logger *slog.Logger) *Schedule {
	return &Schedule{db: db, logger: logger}
}

// SlotStatus adalah keadaan satu dosis terjadwal.
type SlotStatus string

const (
	// SlotPending: jamnya belum lewat, belum ada catatan.
	SlotPending SlotStatus = "pending"
	// SlotDue: jamnya sudah lewat, belum ada catatan — inilah yang perlu
	// ditindak user.
	SlotDue SlotStatus = "due"
	// SlotTaken / SlotLate / SlotSkipped / SlotSnoozed: sudah dicatat.
	SlotTaken   SlotStatus = "taken"
	SlotLate    SlotStatus = "late"
	SlotSkipped SlotStatus = "skipped"
	SlotSnoozed SlotStatus = "snoozed"
)

// DaySlot adalah satu dosis pada satu tanggal, siap ditampilkan.
type DaySlot struct {
	MedicationID   string
	MedicationName string
	DoseValue      float64
	DoseUnitCode   string
	MealRelation   domain.MealRelation
	ColorTag       *string

	// ScheduleID adalah id baris medication_times. Nilai inilah yang dikirim
	// sebagai `schedule_id` saat mencatat dosis.
	ScheduleID   string
	TimeLocal    string
	TimeLabel    *domain.TimeLabel
	DoseOverride *string

	SlotDate    time.Time
	DueAt       time.Time
	ScheduledTZ string

	Status SlotStatus

	// Terisi bila sudah ada catatan.
	LogID   *string
	TakenAt *time.Time
	Note    *string

	// Sisa stok saat ini, bila obat punya stok. Dipakai layar untuk
	// memperingatkan sebelum user kehabisan.
	StockRemaining *float64
	StockUnitCode  *string
}

// DayInput adalah permintaan jadwal satu hari.
type DayInput struct {
	OwnerID string
	// Date adalah tanggal lokal pemilik. Bila zero, dipakai hari ini.
	Date time.Time
	// Timezone adalah zona waktu pemilik ("Asia/Jakarta").
	Timezone string
	// Now bisa diisi untuk pengujian; bila zero dipakai waktu sekarang.
	Now time.Time
}

// Day menyusun jadwal satu hari untuk seorang pemilik.
//
// Urutan kerja:
//  1. Baca zona waktu pemilik (bukan UTC — pukul 07:00 WIB adalah tanggal
//     yang berbeda di UTC).
//  2. Ambil seluruh obat beserta aturan frekuensi dan jamnya dalam SATU query.
//  3. Minta domain menghitung slot yang jatuh pada tanggal itu.
//  4. Ambil catatan yang sudah ada pada tanggal itu, lalu tempelkan ke slot
//     lewat occurrence key (schedule_id + tanggal + jam).
//
// Langkah 4 memakai kunci yang sama dengan indeks unik anti-ganda di database,
// jadi slot tidak mungkin tertukar dengan catatan dosis lain.
func (s *Schedule) Day(ctx context.Context, in DayInput) ([]DaySlot, error) {
	tz := in.Timezone
	if tz == "" {
		tz = domain.DefaultTimezone
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		// Zona tidak dikenal: pakai WIB, sama seperti layanan lain. Lebih baik
		// menampilkan jam yang sedikit meleset daripada gagal total.
		loc = time.FixedZone("WIB", 7*3600)
	}

	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	nowLocal := now.In(loc)

	day := in.Date
	if day.IsZero() {
		day = nowLocal
	}
	dayStart := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
	dayEnd := dayStart

	var meds []postgres.MedicationWithSchedule
	var logs []*domain.MedicationLog
	var stocks map[string]*domain.MedicationStock

	err = s.db.WithTx(ctx, func(tx pgx.Tx) error {
		all, err := s.db.ListMedicationsWithSchedule(ctx, tx, in.OwnerID)
		if err != nil {
			return err
		}
		meds = all

		existing, err := s.db.ListLogs(ctx, tx, postgres.ListLogsFilter{
			OwnerID: in.OwnerID,
			From:    &dayStart,
			To:      &dayEnd,
			Limit:   200,
		})
		if err != nil {
			return err
		}
		logs = existing

		// Stok diambil sekaligus untuk semua obat, bukan per obat: satu kali
		// buka layar Hari Ini tidak boleh berarti N perjalanan ke database.
		allStocks, err := s.db.StocksByOwner(ctx, tx, in.OwnerID)
		if err != nil {
			return err
		}
		stocks = allStocks
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("menyusun jadwal harian: %w", err)
	}

	// Peta catatan berdasarkan occurrence key. Satu dosis hanya boleh punya
	// satu catatan hidup (dijamin indeks unik di database), jadi peta ini
	// tidak pernah kehilangan data.
	byOccurrence := make(map[string]*domain.MedicationLog, len(logs))
	for _, l := range logs {
		if key := l.OccurrenceKey(); key != "" {
			byOccurrence[key] = l
		}
	}

	var out []DaySlot
	for i := range meds {
		m := &meds[i]
		if !m.IsActive {
			continue
		}

		params := toFrequencyParams(m.Frequency)
		slots := domain.ScheduledSlotsInRange(
			&m.Medication, params, m.Times, dayStart, dayEnd, loc, now, true)

		for si := range slots {
			slot := &slots[si]

			ds := DaySlot{
				MedicationID:   m.ID,
				MedicationName: m.Name,
				DoseValue:      m.DoseValue,
				DoseUnitCode:   m.DoseUnitCode,
				MealRelation:   m.MealRelation,
				ColorTag:       m.ColorTag,
				ScheduleID:     slot.ScheduleID,
				TimeLocal:      slot.TimeLocal,
				TimeLabel:      slot.TimeLabel,
				DoseOverride:   slot.DoseOverride,
				SlotDate:       slot.Date,
				DueAt:          slot.DueAt,
				ScheduledTZ:    tz,
			}

			// Status bawaan ditentukan waktu: belum lewat = menunggu,
			// sudah lewat = perlu ditindak.
			if slot.IsPending {
				ds.Status = SlotPending
			} else {
				ds.Status = SlotDue
			}

			// Catatan yang sudah ada menimpa status waktu.
			key := domain.OccurrenceKey(slot.ScheduleID, slot.Date,
				slot.DueAt.Hour(), slot.DueAt.Minute())
			if log, ok := byOccurrence[key]; ok {
				ds.Status = SlotStatus(log.Status)
				id := log.ID
				ds.LogID = &id
				ds.TakenAt = log.TakenAt
				ds.Note = log.Note
			}

			if stock := stocks[m.ID]; stock != nil {
				remaining := stock.CurrentQty
				unit := stock.UnitCode
				ds.StockRemaining = &remaining
				ds.StockUnitCode = &unit
			}

			out = append(out, ds)
		}
	}

	return out, nil
}
