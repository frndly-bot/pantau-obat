package postgres

import (
	"context"
	"fmt"
)

// CaregiverRecipient adalah satu caregiver yang menerima eskalasi.
type CaregiverRecipient struct {
	CaregiverID string
	Role        string
	Relation    string
}

// AcceptedCaregivers mengembalikan caregiver yang sudah menerima undangan
// untuk satu pasien.
//
// HANYA status `accepted`. Undangan yang masih `pending` berarti orangnya
// belum menyetujui apa pun — mengirim data kesehatan pasien ke alamat yang
// belum tentu bersedia, dan belum tentu benar orangnya, adalah kebocoran.
// `rejected` dan yang sudah dicabut (`deleted_at`) juga tidak termasuk.
func (db *DB) AcceptedCaregivers(ctx context.Context, q Querier, ownerID string) ([]CaregiverRecipient, error) {
	rows, err := q.Query(ctx, `
        SELECT caregiver_id, role, relation
          FROM care_links
         WHERE owner_id = $1
           AND status = 'accepted'
           AND deleted_at IS NULL
           AND caregiver_id IS NOT NULL
         ORDER BY created_at`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("mencari caregiver pasien: %w", err)
	}
	defer rows.Close()

	var out []CaregiverRecipient
	for rows.Next() {
		var c CaregiverRecipient
		if err := rows.Scan(&c.CaregiverID, &c.Role, &c.Relation); err != nil {
			return nil, fmt.Errorf("membaca caregiver: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// MissedStreak menghitung berapa dosis terlewat BERTURUT-TURUT pada hari-hari
// terakhir untuk satu pasien.
//
// `today` adalah tanggal lokal pasien sebagai "YYYY-MM-DD".
//
// KENAPA STRING, BUKAN time.Time
//
// Ini pernah menjadi bug nyata: tanggal dibangun di zona pasien
// (`2026-10-15T00:00+07:00`), dikirim sebagai `timestamptz`, lalu di-cast
// `::date` — dan cast itu memakai zona sesi Postgres (UTC di server ini),
// sehingga hasilnya bergeser ke `2026-10-14`. Akibatnya streak dihitung
// untuk hari yang salah, dan eskalasi tidak pernah terpicu.
//
// Mengirim "YYYY-MM-DD" membuat Postgres mem-parse tanggal murni tanpa
// konversi zona sama sekali. Tanggal adalah konsep kalender, bukan titik
// waktu — jadi memang tidak seharusnya melewati konversi zona.
//
// KENAPA PER HARI, BUKAN PER DOSIS
//
// Aturan yang dijanjikan notification-v2 berbunyi "streak terlewat
// berturut-turut". Yang bermakna bagi caregiver bukan "pasien melewatkan
// tiga dosis pagi" — itu bisa terjadi karena satu pagi yang sibuk. Yang
// bermakna adalah "sudah tiga hari pasien tidak minum sama sekali": itu pola
// yang perlu ditindak.
//
// KENAPA HARI TANPA JADWAL TIDAK MEMUTUS STREAK
//
// Pasien yang minum obat Senin dan Kamis, lalu melewatkan Senin dan Kamis,
// tetap punya masalah walau Selasa-Rabu memang tidak ada jadwal. Karena itu
// yang dihitung hanya hari yang MEMILIKI dosis terjadwal; hari kosong
// dilewati, bukan dianggap patuh.
//
// Mengembalikan 0 bila hari terakhir yang dinilai tidak terlewat.
func (db *DB) MissedStreak(ctx context.Context, q Querier, ownerID, today string) (int, error) {
	var streak int
	err := q.QueryRow(ctx, `
        WITH dinilai AS (
            -- Hanya hari yang MEMILIKI catatan dosis. Hari tanpa jadwal
            -- tidak muncul di sini sama sekali, sehingga ia tidak bisa
            -- memutus rangkaian — sesuai catatan di atas.
            SELECT e.event_date,
                   bool_and(e.kind = 'missed') AS semua_terlewat
              FROM medication_dose_events e
              JOIN medications m ON m.id = e.medication_id
             WHERE e.owner_id = $1
               AND m.deleted_at IS NULL
               AND e.event_date < $2::date
               AND e.event_date >= $2::date - INTERVAL '30 days'
             GROUP BY e.event_date
        ),
        berurut AS (
            SELECT semua_terlewat,
                   row_number() OVER (ORDER BY event_date DESC) AS urutan
              FROM dinilai
        ),
        -- Batas rangkaian: urutan hari pertama yang TIDAK terlewat penuh.
        -- Bila semua hari terlewat, batasnya jauh di luar jangkauan.
        batas AS (
            SELECT coalesce(min(urutan), 1000000) AS n
              FROM berurut WHERE NOT semua_terlewat
        )
        SELECT count(*)
          FROM berurut, batas
         WHERE berurut.semua_terlewat
           AND berurut.urutan < batas.n`,
		ownerID, today).Scan(&streak)
	if err != nil {
		return 0, fmt.Errorf("menghitung streak terlewat: %w", err)
	}
	return streak, nil
}

// EscalationAlreadySentToday melaporkan apakah pasien ini sudah pernah
// dieskalasikan hari ini.
//
// notification-v2 bagian 12 menetapkan batas "maksimal 1 kali per hari per
// pasien". Tanpa batas itu, pasien yang sedang bermasalah akan membuat
// caregiver menerima pesan berulang setiap kali pass berjalan — dan
// notifikasi yang datang terus-menerus justru berhenti dibaca.
//
// Dihitung dari bump_queue, bukan dari tabel terpisah: eskalasi memang
// hidup di sana, jadi di sanalah batasnya bisa ditegakkan tanpa risiko
// terlewat saat ada dua proses berjalan bersamaan.
//
// `day` adalah tanggal lokal pasien sebagai "YYYY-MM-DD", dengan alasan yang
// sama seperti MissedStreak: tanggal kalender tidak boleh melewati konversi
// zona waktu.
func (db *DB) EscalationAlreadySentToday(ctx context.Context, q Querier, subjectOwnerID, day string) (bool, error) {
	var ada bool
	err := q.QueryRow(ctx, `
        SELECT EXISTS (
            SELECT 1 FROM bump_queue
             WHERE kind = 'escalation'
               AND subject_owner_id = $1
               AND slot_date = $2::date
        )`, subjectOwnerID, day).Scan(&ada)
	if err != nil {
		return false, fmt.Errorf("memeriksa eskalasi hari ini: %w", err)
	}
	return ada, nil
}
