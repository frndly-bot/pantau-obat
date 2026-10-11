package service

import (
	"time"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// UpdateMedicationInput adalah perubahan data obat yang diminta klien.
//
// Semua pointer supaya hanya field yang dikirim yang berubah. Tanpa ini,
// PATCH yang hanya ingin mengubah warna akan ikut mengosongkan catatan.
//
// Ditempatkan di paket service, bukan diambil langsung dari repo, karena
// handler transport tidak boleh mengimpor repo. Tipe ini yang menjadi
// perantara, sehingga ketergantungan tetap satu arah:
// transport -> service -> repo.
type UpdateMedicationInput struct {
	Name           *string
	DoseValue      *float64
	DoseUnitCode   *string
	StrengthNote   *string
	RouteCode      *string
	DosageFormCode *string
	MealRelation   *domain.MealRelation
	AsNeeded       *bool
	MustFinish     *bool

	StartDate *time.Time
	StartTime *string
	EndDate   *time.Time

	// ClearEndDate membedakan "kosongkan tanggal selesai" (terapi jadi
	// tanpa batas) dari "jangan ubah". COALESCE tidak bisa membedakan
	// keduanya, jadi perlu penanda tersendiri.
	ClearEndDate bool

	Notes         *string
	NotesReminder *string
	ColorTag      *string
	IsActive      *bool
}

// IsEmpty melaporkan apakah tidak ada field yang diubah.
func (u *UpdateMedicationInput) IsEmpty() bool {
	return u.Name == nil && u.DoseValue == nil && u.DoseUnitCode == nil &&
		u.StrengthNote == nil && u.RouteCode == nil && u.DosageFormCode == nil &&
		u.MealRelation == nil && u.AsNeeded == nil && u.MustFinish == nil &&
		u.StartDate == nil && u.StartTime == nil && u.EndDate == nil &&
		!u.ClearEndDate && u.Notes == nil && u.NotesReminder == nil &&
		u.ColorTag == nil && u.IsActive == nil
}
