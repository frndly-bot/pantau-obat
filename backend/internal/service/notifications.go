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

// Notifications adalah layanan preferensi dan perangkat notifikasi.
type Notifications struct {
	db     *postgres.DB
	logger *slog.Logger
}

// NewNotifications membuat layanan notifikasi.
func NewNotifications(db *postgres.DB, logger *slog.Logger) *Notifications {
	return &Notifications{db: db, logger: logger}
}

// ChannelPrefs adalah preferensi satu kanal yang sudah lengkap nilainya.
type ChannelPrefs = domain.ChannelPrefs

// AllPrefs mengembalikan preferensi SELURUH kanal, termasuk yang belum
// pernah diatur user.
//
// Kanal yang tidak punya baris di database diisi nilai bawaan dari domain.
// Ini yang membuat perilaku kanal yang belum diatur konsisten: tanpa ini,
// kanal yang belum diatur akan dianggap nonaktif di satu tempat dan aktif
// di tempat lain, tergantung kode mana yang membacanya.
func (s *Notifications) AllPrefs(ctx context.Context, userID string) ([]domain.ChannelPrefs, error) {
	stored, err := s.db.PrefsForUser(ctx, s.db.Pool(), userID)
	if err != nil {
		return nil, err
	}

	out := make([]domain.ChannelPrefs, 0, len(domain.AllChannels))
	for _, ch := range domain.AllChannels {
		if row, ok := stored[ch]; ok {
			out = append(out, domain.ChannelPrefs{
				Channel:        ch,
				Enabled:        row.Enabled,
				QuietStart:     strOr(row.QuietStart),
				QuietEnd:       strOr(row.QuietEnd),
				AdvanceMinutes: row.AdvanceMinutes,
				Throttled:      row.NotifThrottled,
			})
			continue
		}
		out = append(out, domain.DefaultPrefs(ch))
	}
	return out, nil
}

// PrefsFor menghitung preferensi efektif SATU kanal.
//
// Dipakai dispatcher sebelum mengirim. Kanal yang tidak ada barisnya memakai
// bawaan, sehingga pengiriman tetap punya aturan meski user belum pernah
// membuka pengaturan.
func (s *Notifications) PrefsFor(ctx context.Context, userID string, channel domain.Channel) (domain.ChannelPrefs, error) {
	row, err := s.db.PrefsByChannel(ctx, s.db.Pool(), userID, channel)
	if err != nil {
		if isNotFound(err) {
			return domain.DefaultPrefs(channel), nil
		}
		return domain.ChannelPrefs{}, err
	}
	return domain.ChannelPrefs{
		Channel:        channel,
		Enabled:        row.Enabled,
		QuietStart:     strOr(row.QuietStart),
		QuietEnd:       strOr(row.QuietEnd),
		AdvanceMinutes: row.AdvanceMinutes,
		Throttled:      row.NotifThrottled,
	}, nil
}

// UpdatePrefsInput adalah perubahan preferensi satu kanal.
type UpdatePrefsInput struct {
	Channel        domain.Channel
	Enabled        bool
	QuietStart     *string
	QuietEnd       *string
	ClearQuiet     bool
	AdvanceMinutes int
}

// UpdatePrefs menyimpan preferensi satu kanal.
func (s *Notifications) UpdatePrefs(ctx context.Context, userID string, in UpdatePrefsInput, meta Meta) (*domain.ChannelPrefs, error) {
	if err := s.validatePrefs(&in); err != nil {
		return nil, err
	}

	var out *domain.ChannelPrefs
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		row, err := s.db.UpsertPrefs(ctx, tx, userID, postgres.PrefsInput{
			Channel:        in.Channel,
			Enabled:        in.Enabled,
			QuietStart:     normalizeClockPtr(in.QuietStart),
			QuietEnd:       normalizeClockPtr(in.QuietEnd),
			ClearQuiet:     in.ClearQuiet,
			AdvanceMinutes: in.AdvanceMinutes,
		})
		if err != nil {
			return err
		}

		out = &domain.ChannelPrefs{
			Channel:        row.Channel,
			Enabled:        row.Enabled,
			QuietStart:     strOr(row.QuietStart),
			QuietEnd:       strOr(row.QuietEnd),
			AdvanceMinutes: row.AdvanceMinutes,
			Throttled:      row.NotifThrottled,
		}

		return s.db.WriteAudit(ctx, tx, postgres.AuditEntry{
			ActorID:      &userID,
			OwnerID:      &userID,
			ActorRole:    string(domain.ActorUser),
			Action:       postgres.ActionUpdate,
			ResourceType: postgres.ResourceNotif,
			ResourceID:   &userID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			AfterState: map[string]any{
				"channel":         string(row.Channel),
				"enabled":         row.Enabled,
				"quiet_start":     strOr(row.QuietStart),
				"quiet_end":       strOr(row.QuietEnd),
				"advance_minutes": row.AdvanceMinutes,
			},
		})
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// validatePrefs memeriksa input preferensi.
//
// Jam sunyi hanya boleh diisi KEDUANYA atau tidak sama sekali. Mengisi satu
// sisi saja membuat jendelanya tidak bisa dihitung, dan domain akan
// memperlakukannya sebagai "tidak ada jam sunyi" — sehingga user mengira
// sudah mengaturnya padahal tidak.
func (s *Notifications) validatePrefs(in *UpdatePrefsInput) error {
	var problems []string

	found := false
	for _, ch := range domain.AllChannels {
		if ch == in.Channel {
			found = true
			break
		}
	}
	if !found {
		problems = append(problems, fmt.Sprintf("kanal %q tidak dikenal", in.Channel))
	}

	if in.AdvanceMinutes < 0 || in.AdvanceMinutes > 120 {
		problems = append(problems, "maju lebih awal harus antara 0 dan 120 menit")
	}

	if !in.ClearQuiet {
		startSet := in.QuietStart != nil && strings.TrimSpace(*in.QuietStart) != ""
		endSet := in.QuietEnd != nil && strings.TrimSpace(*in.QuietEnd) != ""
		if startSet != endSet {
			problems = append(problems, "jam sunyi harus diisi keduanya atau dikosongkan keduanya")
		}
		if startSet {
			if _, _, err := domain.ParseClock(*in.QuietStart); err != nil {
				problems = append(problems, "jam mulai sunyi harus berbentuk HH:MM")
			}
		}
		if endSet {
			if _, _, err := domain.ParseClock(*in.QuietEnd); err != nil {
				problems = append(problems, "jam selesai sunyi harus berbentuk HH:MM")
			}
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("%w: %s", domain.ErrValidation, strings.Join(problems, "; "))
	}
	return nil
}

// RegisterDeviceInput adalah permintaan pendaftaran perangkat.
type RegisterDeviceInput struct {
	Platform   domain.DevicePlatform
	Token      string
	AppVersion *string
}

// RegisterDevice mendaftarkan perangkat penerima push.
//
// Token dibatasi 512 karakter dan tidak boleh kosong. Tidak ada validasi
// bentuk lebih lanjut karena bentuk token berbeda antar platform (web push
// memakai JSON endpoint, FCM memakai string acak), dan menebak-nebak
// bentuknya hanya akan menolak token yang sah.
func (s *Notifications) RegisterDevice(ctx context.Context, userID string, in RegisterDeviceInput, meta Meta) (*postgres.DeviceToken, error) {
	var problems []string

	if !in.Platform.Valid() {
		problems = append(problems, fmt.Sprintf("platform %q tidak dikenal", in.Platform))
	}
	token := strings.TrimSpace(in.Token)
	if token == "" {
		problems = append(problems, "token perangkat wajib diisi")
	}
	if len(token) > 512 {
		problems = append(problems, "token perangkat maksimal 512 karakter")
	}
	if in.AppVersion != nil && len(*in.AppVersion) > 50 {
		problems = append(problems, "versi aplikasi maksimal 50 karakter")
	}

	if len(problems) > 0 {
		return nil, fmt.Errorf("%w: %s", domain.ErrValidation, strings.Join(problems, "; "))
	}

	var out *postgres.DeviceToken
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		t, err := s.db.RegisterDeviceToken(ctx, tx, userID, postgres.DeviceTokenInput{
			Platform:   in.Platform,
			Token:      token,
			AppVersion: in.AppVersion,
		})
		if err != nil {
			return err
		}
		out = t

		// Token perangkat TIDAK dicatat ke audit, hanya platformnya.
		// Token itu kredensial pengiriman: siapa pun yang memegangnya bisa
		// mengirim notifikasi ke perangkat itu. Menyimpannya di jejak audit
		// berarti menggandakan tempat bocornya.
		return s.db.WriteAudit(ctx, tx, postgres.AuditEntry{
			ActorID:      &userID,
			OwnerID:      &userID,
			ActorRole:    string(domain.ActorUser),
			Action:       postgres.ActionCreate,
			ResourceType: postgres.ResourceNotif,
			ResourceID:   &t.ID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			AfterState: map[string]any{
				"platform":     string(t.Platform),
				"token_suffix": tokenSuffix(token),
			},
		})
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// UnregisterDevice mencabut satu perangkat.
func (s *Notifications) UnregisterDevice(ctx context.Context, userID, token string, meta Meta) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("%w: token perangkat wajib diisi", domain.ErrValidation)
	}

	return s.db.WithTx(ctx, func(tx pgx.Tx) error {
		if err := s.db.RevokeDeviceToken(ctx, tx, userID, token); err != nil {
			return err
		}
		return s.db.WriteAudit(ctx, tx, postgres.AuditEntry{
			ActorID:      &userID,
			OwnerID:      &userID,
			ActorRole:    string(domain.ActorUser),
			Action:       postgres.ActionDelete,
			ResourceType: postgres.ResourceNotif,
			ResourceID:   &userID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
			// Hanya akhiran token yang dicatat, cukup untuk mencocokkan
			// perangkat mana yang dicabut tanpa menyimpan kredensialnya.
			AfterState: map[string]any{"token_suffix": tokenSuffix(token)},
		})
	})
}

// ListDevices mendaftar perangkat aktif milik user.
func (s *Notifications) ListDevices(ctx context.Context, userID string) ([]postgres.DeviceToken, error) {
	return s.db.ListDeviceTokens(ctx, s.db.Pool(), userID)
}

// Inbox adalah satu riwayat pengingat untuk ditampilkan ke user.
type Inbox struct {
	ID        int64
	Kind      string
	Status    string
	Channel   string
	Title     string
	CreatedAt time.Time
	SentAt    *time.Time
}

// Inbox mengambil riwayat pengingat milik user.
//
// Isi pesan DISUSUN ULANG dari jenisnya, bukan dibaca dari payload.
// Payload di notification_jobs disimpan untuk keperluan penelusuran teknis
// dan bisa memuat detail internal; yang ditampilkan ke user dibuat dari
// data yang sudah dipastikan aman.
func (s *Notifications) Inbox(ctx context.Context, userID string, limit int) ([]Inbox, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}

	rows, err := s.db.Pool().Query(ctx, `
        SELECT j.id, j.kind, j.status,
               coalesce(d.channel, 'in_app'),
               j.created_at, d.sent_at
          FROM notification_jobs j
          LEFT JOIN notification_deliveries d ON d.job_id = j.id
         WHERE j.owner_id = $1
         ORDER BY j.created_at DESC
         LIMIT $2`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("membaca kotak masuk notifikasi: %w", err)
	}
	defer rows.Close()

	var out []Inbox
	for rows.Next() {
		var it Inbox
		if err := rows.Scan(&it.ID, &it.Kind, &it.Status, &it.Channel,
			&it.CreatedAt, &it.SentAt); err != nil {
			return nil, fmt.Errorf("membaca kotak masuk notifikasi: %w", err)
		}
		it.Title = inboxTitle(it.Kind)
		out = append(out, it)
	}
	return out, rows.Err()
}

// inboxTitle menyusun judul yang aman ditampilkan dari jenis job.
//
// Sengaja TIDAK memuat nama obat: kotak masuk bisa dilihat siapa pun yang
// memegang ponsel yang tidak terkunci, dan riwayat "obat apa yang diminum"
// adalah data kesehatan.
func inboxTitle(kind string) string {
	switch domain.JobKind(kind) {
	case domain.JobDoseReminder:
		return "Pengingat minum obat"
	case domain.JobRefillReminder:
		return "Pengingat restock obat"
	case domain.JobMissedEscalation:
		return "Pemberitahuan dosis terlewat"
	case domain.JobCareDigest:
		return "Ringkasan harian"
	case domain.JobRetention:
		return "Pemeliharaan sistem"
	}
	return "Notifikasi"
}

// tokenSuffix mengambil beberapa karakter terakhir token untuk keperluan
// pencocokan di audit.
func tokenSuffix(token string) string {
	const n = 6
	if len(token) <= n {
		return "***"
	}
	return "..." + token[len(token)-n:]
}

// strOr mengubah pointer string menjadi string, kosong bila nil.
func strOr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// normalizeClockPtr merapikan nilai jam menjadi "HH:MM".
//
// PostgreSQL mengembalikan TIME sebagai "HH:MM:SS", sedangkan kontrak API
// memakai "HH:MM". Normalisasi dilakukan sebelum menyimpan supaya tidak ada
// dua bentuk nilai untuk jam yang sama.
func normalizeClockPtr(s *string) *string {
	if s == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*s)
	if trimmed == "" {
		return nil
	}
	if hh, mm, err := domain.ParseClock(trimmed); err == nil {
		out := domain.FormatClock(hh, mm)
		return &out
	}
	return &trimmed
}
