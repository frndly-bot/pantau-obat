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

// File ini mengurus jam minum sebagai operasi tersendiri.
//
// Dipisah dari medications.go karena mengubah jam punya konsekuensi berbeda
// dari mengubah data obat: dosis yang sudah tercatat merujuk jam tertentu,
// sehingga perubahan jam hanya boleh berlaku untuk dosis KE DEPAN, dan
// perubahannya dicatat sebagai dose_shift.

// UpdateTimeInput adalah perubahan pada satu jam minum.
type UpdateTimeInput struct {
	TimeLocal    *string
	Label        *domain.TimeLabel
	Weekdays     *int
	DoseOverride *string
	IsActive     *bool
}

// CreateTime menambah satu jam minum pada obat.
func (s *Medications) CreateTime(ctx context.Context, ownerID, medicationID string, in TimeInput, meta Meta) (*domain.MedicationTime, error) {
	if err := s.validateTimeInput(in); err != nil {
		return nil, err
	}

	var created *domain.MedicationTime
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		med, err := s.db.MedicationByID(ctx, tx, medicationID, ownerID)
		if err != nil {
			return err
		}
		// PRN tidak dijadwalkan, jadi jam minum untuknya tidak bermakna.
		// Membiarkannya ditambahkan akan menyesatkan: seolah obat ini punya
		// jadwal, padahal diminum hanya bila perlu.
		if med.AsNeeded {
			return fmt.Errorf("%w: obat PRN tidak memakai jam minum terjadwal", domain.ErrValidation)
		}

		freq, err := s.db.FrequencyByMedication(ctx, tx, medicationID)
		if err != nil && !isNotFound(err) {
			return err
		}
		if freq != nil && freq.Pattern == domain.PatternPRN {
			return fmt.Errorf("%w: obat PRN tidak memakai jam minum terjadwal", domain.ErrValidation)
		}

		var freqID *string
		if freq != nil {
			freqID = &freq.ID
		}

		weekdays := domain.EveryDay
		if in.Weekdays != nil {
			weekdays = *in.Weekdays
		}

		// sort_order diletakkan di akhir supaya jam baru muncul paling
		// bawah di daftar, sesuai urutan penambahannya.
		t, err := s.db.CreateMedicationTime(ctx, tx, postgres.CreateMedicationTimeInput{
			MedicationID: medicationID,
			FrequencyID:  freqID,
			OwnerID:      ownerID,
			TimeLocal:    in.TimeLocal,
			Label:        in.Label,
			Weekdays:     weekdays,
			DoseOverride: in.DoseOverride,
			SortOrder:    len(med.Times),
		})
		if err != nil {
			return err
		}

		if err := s.writeScheduleAudit(ctx, tx, ownerID, medicationID, "time_added", meta, map[string]any{
			"time_local": t.TimeLocal,
			"weekdays":   t.Weekdays,
		}); err != nil {
			s.logger.WarnContext(ctx, "gagal menulis audit penambahan jam",
				slog.String("medication_id", medicationID))
		}

		created = t
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// UpdateTime mengubah satu jam minum.
//
// Mengubah time_local dicatat sebagai dose_shift di medication_dose_events.
// Alasannya ada di data-model-v2 bagian 10 butir 4: dosis yang sudah tercatat
// memakai occurrence key berisi jam yang berlaku saat itu, jadi mengubah jam
// menciptakan dosis "baru" di jam berbeda untuk hari yang sama. Tanpa catatan
// dose_shift, perubahan itu tidak bisa dijelaskan saat menelusuri kenapa
// sebuah dosis hilang atau muncul.
func (s *Medications) UpdateTime(ctx context.Context, ownerID, medicationID, timeID string, in UpdateTimeInput, meta Meta) (*domain.MedicationTime, error) {
	if in.IsEmpty() {
		return nil, fmt.Errorf("%w: tidak ada field yang diubah", domain.ErrValidation)
	}
	if in.TimeLocal != nil {
		hh, mm, err := domain.ParseClock(*in.TimeLocal)
		if err != nil {
			return nil, fmt.Errorf("%w: jam harus berbentuk HH:MM", domain.ErrValidation)
		}
		normalized := domain.FormatClock(hh, mm)
		in.TimeLocal = &normalized
	}
	if in.Weekdays != nil && (*in.Weekdays < 1 || *in.Weekdays > 127) {
		return nil, fmt.Errorf("%w: weekdays harus antara 1 dan 127", domain.ErrValidation)
	}
	if in.Label != nil && !in.Label.Valid() {
		return nil, fmt.Errorf("%w: label jam tidak dikenal", domain.ErrValidation)
	}

	var updated *domain.MedicationTime
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		// Obat diperiksa lebih dulu supaya user lain tidak bisa mengubah jam
		// obat milik orang lain lewat id yang ditebak.
		if _, err := s.db.MedicationByID(ctx, tx, medicationID, ownerID); err != nil {
			return err
		}

		before, err := s.db.MedicationTimeByID(ctx, tx, timeID, ownerID)
		if err != nil {
			return err
		}
		if before.MedicationID != medicationID {
			// Jam ini milik obat lain. Ditolak sebagai "tidak ditemukan",
			// bukan "tidak berhak", supaya tidak membocorkan bahwa jam itu
			// memang ada.
			return domain.ErrNotFound
		}

		after, err := s.db.UpdateMedicationTime(ctx, tx, timeID, ownerID, postgres.UpdateMedicationTimeInput{
			TimeLocal:    in.TimeLocal,
			Label:        in.Label,
			Weekdays:     in.Weekdays,
			DoseOverride: in.DoseOverride,
			IsActive:     in.IsActive,
		})
		if err != nil {
			return err
		}

		// Catat penyimpangan hanya bila jamnya benar-benar berubah.
		if before.TimeLocal != after.TimeLocal {
			if err := s.recordDoseShift(ctx, tx, ownerID, medicationID, before, after); err != nil {
				s.logger.WarnContext(ctx, "gagal mencatat pergeseran jam",
					slog.String("medication_id", medicationID),
					slog.String("error", err.Error()))
			}
		}

		if err := s.writeScheduleAudit(ctx, tx, ownerID, medicationID, "time_updated", meta, map[string]any{
			"before_time": before.TimeLocal,
			"after_time":  after.TimeLocal,
		}); err != nil {
			s.logger.WarnContext(ctx, "gagal menulis audit perubahan jam",
				slog.String("medication_id", medicationID))
		}

		updated = after
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// DeleteTime menghapus satu jam minum.
//
// Soft-delete, bukan DELETE fisik: log dosis lama merujuk schedule_id ini
// lewat FK RESTRICT, jadi penghapusan fisiknya akan ditolak database.
// Menandainya terhapus membuat pengingat berhenti sementara riwayat tetap
// bisa ditelusuri.
func (s *Medications) DeleteTime(ctx context.Context, ownerID, medicationID, timeID string, meta Meta) error {
	return s.db.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := s.db.MedicationByID(ctx, tx, medicationID, ownerID); err != nil {
			return err
		}

		before, err := s.db.MedicationTimeByID(ctx, tx, timeID, ownerID)
		if err != nil {
			return err
		}
		if before.MedicationID != medicationID {
			return domain.ErrNotFound
		}

		if err := s.db.SoftDeleteMedicationTime(ctx, tx, timeID, ownerID); err != nil {
			return err
		}

		if err := s.writeScheduleAudit(ctx, tx, ownerID, medicationID, "time_deleted", meta, map[string]any{
			"time_local": before.TimeLocal,
		}); err != nil {
			s.logger.WarnContext(ctx, "gagal menulis audit penghapusan jam",
				slog.String("medication_id", medicationID))
		}
		return nil
	})
}

// recordDoseShift mencatat perubahan jam sebagai dose_shift.
//
// event_key memakai occurrence key yang sama dengan medication_logs dan
// bump_queue, sehingga satu dosis hanya bisa punya satu catatan penyimpangan.
// Tanggal yang dipakai adalah HARI INI di zona blok jadwal, karena perubahan
// jam mulai berlaku sejak sekarang, bukan surut.
func (s *Medications) recordDoseShift(
	ctx context.Context, tx pgx.Tx, ownerID, medicationID string,
	before, after *domain.MedicationTime,
) error {
	tz := domain.DefaultTimezone
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)

	hh, mm, err := domain.ParseClock(before.TimeLocal)
	if err != nil {
		return nil // jam lama tidak terbaca; tidak bisa menyusun kunci
	}

	return s.db.InsertDoseEvent(ctx, tx, postgres.DoseEventInput{
		EventKey:        domain.OccurrenceKey(before.ID, now, hh, mm),
		OwnerID:         ownerID,
		MedicationID:    medicationID,
		ScheduleID:      &before.ID,
		EventDate:       now,
		ScheduledHour:   hh,
		ScheduledMinute: mm,
		ScheduledTZ:     tz,
		Kind:            domain.EventDoseShift,
		Reason:          fmt.Sprintf("jam diubah dari %s ke %s", before.TimeLocal, after.TimeLocal),
	})
}

// writeScheduleAudit menulis jejak audit untuk perubahan jadwal.
func (s *Medications) writeScheduleAudit(
	ctx context.Context, tx pgx.Tx, ownerID, medicationID, action string,
	meta Meta, detail map[string]any,
) error {
	return s.db.WriteAudit(ctx, tx, postgres.AuditEntry{
		ActorID:      &ownerID,
		OwnerID:      &ownerID,
		ActorRole:    string(domain.ActorUser),
		Action:       postgres.ActionUpdate,
		ResourceType: postgres.ResourceSched,
		ResourceID:   &medicationID,
		IP:           meta.IP,
		UserAgent:    meta.UserAgent,
		Meta:         mergeMaps(map[string]any{"action": action}, detail),
	})
}

// validateTimeInput memeriksa input jam minum.
func (s *Medications) validateTimeInput(in TimeInput) error {
	var problems []string

	hh, mm, err := domain.ParseClock(in.TimeLocal)
	if err != nil {
		problems = append(problems, "jam harus berbentuk HH:MM")
	} else {
		_ = hh
		_ = mm
	}
	if in.Weekdays != nil && (*in.Weekdays < 1 || *in.Weekdays > 127) {
		problems = append(problems, "weekdays harus antara 1 dan 127")
	}
	if in.Label != nil && !in.Label.Valid() {
		problems = append(problems, "label jam tidak dikenal")
	}

	if len(problems) > 0 {
		return fmt.Errorf("%w: %s", domain.ErrValidation, strings.Join(problems, "; "))
	}
	return nil
}

// IsEmpty melaporkan apakah tidak ada field yang diubah.
func (u *UpdateTimeInput) IsEmpty() bool {
	return u.TimeLocal == nil && u.Label == nil && u.Weekdays == nil &&
		u.DoseOverride == nil && u.IsActive == nil
}

// mergeMaps menggabungkan dua map tanpa mengubah keduanya.
func mergeMaps(a, b map[string]any) map[string]any {
	out := make(map[string]any, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}
