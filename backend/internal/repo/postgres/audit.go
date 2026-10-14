package postgres

import (
	"context"
	"encoding/json"
	"fmt"
)

// AuditEntry adalah satu baris jejak audit.
//
// actor_role DISALIN saat aksi terjadi, bukan di-join saat dibaca. Alasannya
// di data-model-v2 bagian 9: kalau peran user berubah di kemudian hari,
// jejak lama harus tetap menunjukkan peran pada saat aksi itu dilakukan.
type AuditEntry struct {
	ActorID      *string
	OwnerID      *string
	ActorRole    string
	Action       string
	ResourceType string
	ResourceID   *string
	IP           *string
	UserAgent    *string
	BeforeState  any
	AfterState   any
	Meta         any
}

// WriteAudit menyimpan satu baris audit.
//
// Fungsi ini TIDAK boleh menggagalkan aksi utama. Kalau penulisan audit
// gagal, request tetap dianggap sukses: menolak pencatatan dosis hanya
// karena jejak audit tidak tersimpan justru merugikan user. Kegagalannya
// dilaporkan ke pemanggil supaya bisa dicatat di log.
func (db *DB) WriteAudit(ctx context.Context, q Querier, e AuditEntry) error {
	before, err := toJSONB(e.BeforeState)
	if err != nil {
		return fmt.Errorf("menyusun before_state: %w", err)
	}
	after, err := toJSONB(e.AfterState)
	if err != nil {
		return fmt.Errorf("menyusun after_state: %w", err)
	}
	meta, err := toJSONBObject(e.Meta)
	if err != nil {
		return fmt.Errorf("menyusun meta: %w", err)
	}

	_, err = q.Exec(ctx, `
        INSERT INTO audit_log
            (actor_id, owner_id, actor_role, action, resource_type, resource_id,
             ip, user_agent, before_state, after_state, meta)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		e.ActorID, e.OwnerID, e.ActorRole, e.Action, e.ResourceType, e.ResourceID,
		e.IP, e.UserAgent, before, after, meta)
	if err != nil {
		return fmt.Errorf("menulis audit: %w", err)
	}
	return nil
}

// toJSONB mengubah nilai apa pun menjadi JSON yang bisa dikirim ke kolom
// JSONB.
//
// Nilai nil dikembalikan sebagai nil supaya kolom yang boleh NULL tetap
// menerima NULL. Untuk kolom yang NOT NULL DEFAULT '{}' — terutama
// audit_log.meta — pemanggil harus memakai toJSONBObject, karena mengirim
// NULL ke kolom NOT NULL adalah pelanggaran constraint, bukan nilai default.
func toJSONB(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	// Objek JSON kosong tidak berguna; perlakukan seperti kosong.
	if string(b) == "{}" || string(b) == "null" {
		return nil, nil
	}
	return b, nil
}

// toJSONBObject mengubah nilai menjadi JSON objek, dengan {} sebagai default
// bila kosong.
//
// Dipakai untuk kolom yang NOT NULL, seperti audit_log.meta. Mengirim NULL
// ke kolom itu akan ditolak PostgreSQL, jadi nilai kosong harus diwakili
// objek kosong, bukan NULL.
func toJSONBObject(v any) ([]byte, error) {
	b, err := toJSONB(v)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return []byte(`{}`), nil
	}
	return b, nil
}

// Konstanta aksi audit untuk alur autentikasi.
//
// Nilainya harus ada di CHECK audit_log.action. Dikumpulkan di sini supaya
// salah tulis ketahuan saat kompilasi, bukan saat runtime.
const (
	ActionCreate           = "create"
	ActionRead             = "read"
	ActionUpdate           = "update"
	ActionDelete           = "delete"
	ActionLogin            = "login"
	ActionLogout           = "logout"
	ActionInvite           = "invite"
	ActionAccept           = "accept"
	ActionReject           = "reject"
	ActionRevoke           = "revoke"
	ActionExport           = "export"
	ActionLogTaken         = "log_taken"
	ActionLogSkipped       = "log_skipped"
	ActionLogSnoozed       = "log_snoozed"
	ActionRestock          = "restock"
	ActionCatalogSearch    = "catalog_search"
	ActionUserDisable      = "user_disable"
	ActionUserEnable       = "user_enable"
	ActionUserErase        = "user_erase"
	ActionAdminGrant       = "admin_grant"
	ActionAdminRevoke      = "admin_revoke"
	ActionAdminScopeChange = "admin_scope_change"
	ActionNotifRetry       = "notif_retry"
	ActionCatalogCreate    = "catalog_create"
	ActionCatalogPublish   = "catalog_publish"
	ActionCatalogUpdate    = "catalog_update"
	ActionCatalogUnpublish = "catalog_unpublish"
	ActionCatalogArchive   = "catalog_archive"
	ActionCatalogImport    = "catalog_import"
	ActionCatalogMerge     = "catalog_merge"
	ActionNotifCancel      = "notif_cancel"
	ActionRetentionPurge   = "retention_purge"
)

// Konstanta resource_type audit.
const (
	ResourceUser   = "user"
	ResourceAuth   = "auth"
	ResourceMed    = "medication"
	ResourceSched  = "schedule"
	ResourceLog    = "log"
	ResourceStock  = "stock"
	ResourceCare   = "care_link"
	ResourceCat    = "catalog"
	ResourceNotif  = "notification"
	ResourceAdmin  = "admin"
	ResourceRetain = "retention"
)

// CountSnoozesForOccurrence menghitung berapa kali satu jam jadwal sudah
// ditunda pada hari ini.
//
// KENAPA DARI JEJAK AUDIT, BUKAN KOLOM TERSENDIRI
//
// Snooze sengaja TIDAK membuat catatan dosis: yang digeser adalah waktu
// pengingat, bukan keadaan dosisnya. Karena itu tidak ada baris di
// `medication_logs` yang bisa dihitung. Jejak audit adalah satu-satunya
// tempat penundaan tercatat, dan setiap snooze memang menulis satu baris ke
// sana — termasuk penolakan karena batas tercapai.
//
// Batas harinya memakai tanggal WIB sebagai acuan karena `slot_date` dosis
// disimpan dalam zona pemilik, dan hari itulah yang dimaksud pengguna saat
// ia menunda "dosis pagi ini".
func (db *DB) CountSnoozesForOccurrence(ctx context.Context, q Querier, ownerID, scheduleID string) (int, error) {
	var n int
	err := q.QueryRow(ctx, `
        SELECT count(*)
          FROM audit_log
         WHERE owner_id = $1
           AND resource_type = $2
           AND resource_id = $3
           AND meta ? 'snooze_minutes'
           AND (created_at AT TIME ZONE 'Asia/Jakarta')::date
               = (now() AT TIME ZONE 'Asia/Jakarta')::date`,
		ownerID, ResourceLog, scheduleID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("menghitung penundaan dosis: %w", err)
	}
	return n, nil
}
