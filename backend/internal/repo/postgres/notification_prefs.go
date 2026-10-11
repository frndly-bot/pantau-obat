package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
)

// File ini mengurus preferensi notifikasi per kanal.
//
// Satu baris per user per kanal. Bentuknya begitu supaya pengaturan jam
// sunyi dan maju-lebih-awal bisa berbeda antar kanal: user mungkin tidak
// ingin notifikasi push tengah malam, tapi tidak masalah menerima email
// keesokan harinya.
//
// Baris preferensi TIDAK dibuat otomatis saat registrasi. Yang tidak ada
// barisnya dianggap memakai nilai bawaan, sehingga user yang tidak pernah
// membuka pengaturan tidak menumpuk baris kosong.

const prefsColumns = `
    id, user_id, channel, enabled, quiet_start::text, quiet_end::text,
    advance_minutes, notif_throttled, created_at, updated_at`

// NotificationPrefs adalah preferensi satu kanal.
type NotificationPrefs struct {
	ID      string
	UserID  string
	Channel domain.Channel
	Enabled bool

	// QuietStart dan QuietEnd membentuk jendela jam sunyi. Nil berarti
	// tidak ada jam sunyi untuk kanal ini.
	//
	// Jendela sunyi BOLEH melewati tengah malam: mis. 22:00 sampai 06:00.
	// Pemeriksaannya ada di domain, bukan di sini, karena aturan itu
	// murni logika waktu.
	QuietStart *string
	QuietEnd   *string

	// AdvanceMinutes memajukan pengiriman beberapa menit sebelum jam dosis.
	AdvanceMinutes int

	// NotifThrottled berarti sistem sedang menahan pengiriman ke kanal ini,
	// biasanya karena penyedia melaporkan pengiriman sebelumnya gagal.
	NotifThrottled bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

// PrefsForUser mengambil preferensi seluruh kanal milik user.
//
// Kanal yang belum punya baris TIDAK dikembalikan. Pemanggil yang mengambil
// keputusan harus memakai domain.DefaultPrefs untuk kanal yang tidak ada,
// sehingga nilai bawaannya hanya didefinisikan di satu tempat.
func (db *DB) PrefsForUser(ctx context.Context, q Querier, userID string) (map[domain.Channel]NotificationPrefs, error) {
	rows, err := q.Query(ctx, `
        SELECT `+prefsColumns+`
          FROM notification_prefs
         WHERE user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("membaca preferensi notifikasi: %w", err)
	}
	defer rows.Close()

	out := map[domain.Channel]NotificationPrefs{}
	for rows.Next() {
		var p NotificationPrefs
		if err := rows.Scan(&p.ID, &p.UserID, &p.Channel, &p.Enabled,
			&p.QuietStart, &p.QuietEnd, &p.AdvanceMinutes, &p.NotifThrottled,
			&p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("membaca preferensi notifikasi: %w", err)
		}
		out[p.Channel] = p
	}
	return out, rows.Err()
}

// UpsertPrefs membuat atau mengubah preferensi satu kanal.
//
// Memakai UPSERT karena satu user hanya boleh punya satu baris per kanal.
// Kolom quiet_start dan quiet_end ditangani terpisah dari COALESCE, karena
// "kosongkan jam sunyi" dan "jangan ubah" adalah dua hal berbeda.
func (db *DB) UpsertPrefs(ctx context.Context, q Querier, userID string, in PrefsInput) (*NotificationPrefs, error) {
	var p NotificationPrefs
	err := q.QueryRow(ctx, `
        INSERT INTO notification_prefs (
            user_id, channel, enabled, quiet_start, quiet_end, advance_minutes
        ) VALUES ($1,$2,$3,$4,$5,$6)
        ON CONFLICT (user_id, channel) DO UPDATE
           SET enabled         = $3,
               quiet_start     = CASE WHEN $7 THEN NULL ELSE $4 END,
               quiet_end       = CASE WHEN $7 THEN NULL ELSE $5 END,
               advance_minutes = $6
        RETURNING `+prefsColumns,
		userID, in.Channel, in.Enabled, in.QuietStart, in.QuietEnd,
		in.AdvanceMinutes, in.ClearQuiet,
	).Scan(&p.ID, &p.UserID, &p.Channel, &p.Enabled, &p.QuietStart,
		&p.QuietEnd, &p.AdvanceMinutes, &p.NotifThrottled, &p.CreatedAt,
		&p.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("menyimpan preferensi notifikasi: %w", err)
	}
	return &p, nil
}

// PrefsInput adalah perubahan preferensi satu kanal.
type PrefsInput struct {
	Channel        domain.Channel
	Enabled        bool
	QuietStart     *string
	QuietEnd       *string
	ClearQuiet     bool
	AdvanceMinutes int
}

// SetThrottled menandai kanal sedang ditahan atau dilepas.
//
// Dipakai dispatcher saat penyedia melaporkan pengiriman gagal berulang.
// Menahan kanal mencegah sistem terus membuang percobaan ke penyedia yang
// sedang bermasalah, dan itu melindungi reputasi pengirim.
func (db *DB) SetThrottled(ctx context.Context, q Querier, userID string, channel domain.Channel, throttled bool) error {
	_, err := q.Exec(ctx, `
        UPDATE notification_prefs SET notif_throttled = $3
         WHERE user_id = $1 AND channel = $2`, userID, channel, throttled)
	if err != nil {
		return fmt.Errorf("memperbarui status kanal: %w", err)
	}
	return nil
}

// DeletePrefs menghapus preferensi satu kanal, sehingga kembali ke bawaan.
func (db *DB) DeletePrefs(ctx context.Context, q Querier, userID string, channel domain.Channel) error {
	_, err := q.Exec(ctx, `
        DELETE FROM notification_prefs WHERE user_id = $1 AND channel = $2`,
		userID, channel)
	if err != nil {
		return fmt.Errorf("menghapus preferensi notifikasi: %w", err)
	}
	return nil
}

// PrefsByChannel mengambil preferensi satu kanal, atau ErrNotFound.
func (db *DB) PrefsByChannel(ctx context.Context, q Querier, userID string, channel domain.Channel) (*NotificationPrefs, error) {
	var p NotificationPrefs
	err := q.QueryRow(ctx, `
        SELECT `+prefsColumns+`
          FROM notification_prefs
         WHERE user_id = $1 AND channel = $2`, userID, channel,
	).Scan(&p.ID, &p.UserID, &p.Channel, &p.Enabled, &p.QuietStart,
		&p.QuietEnd, &p.AdvanceMinutes, &p.NotifThrottled, &p.CreatedAt,
		&p.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("membaca preferensi notifikasi: %w", err)
	}
	return &p, nil
}
