-- +goose Up
-- -----------------------------------------------------------------------------
-- Menambahkan aksi audit yang hilang.
--
-- MASALAH: alur caregiver yang baru dibangun mencatat `reject` ke
-- audit_log.action, dan `pending` pada alur yang sama. Keduanya TIDAK ada di
-- CHECK audit_log.action, sehingga setiap penolakan undangan gagal dengan
-- "violates check constraint" — dan seluruh transaksinya ikut batal.
--
-- Kenapa ini ketahuan sekarang dan bukan sebelumnya: sebelum ada endpoint
-- penolakan undangan, aksi ini memang belum pernah ditulis. Janji dokumen
-- (data-model-v2 bagian 9 menyebut aksi `accept` dan `revoke`) tidak
-- menyertakan `reject`, padahal alur undangan yang lengkap pasti punya
-- penolakan.
--
-- Cara menambah nilai CHECK: DROP constraint lama lalu CREATE yang baru.
-- ALTER TABLE ... ALTER CONSTRAINT tidak bisa mengubah daftar nilainya
-- untuk CHECK constraint biasa.
-- -----------------------------------------------------------------------------

-- +goose StatementBegin
DO $$
DECLARE
    nama_constraint TEXT;
BEGIN
    SELECT conname INTO nama_constraint
      FROM pg_constraint
     WHERE conrelid = 'audit_log'::regclass
       AND contype = 'c'
       AND pg_get_constraintdef(oid) LIKE '%action%';

    IF nama_constraint IS NOT NULL THEN
        EXECUTE format('ALTER TABLE audit_log DROP CONSTRAINT %I', nama_constraint);
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE audit_log ADD CONSTRAINT ck_audit_action CHECK (action IN (
    -- aksi user
    'create', 'read', 'update', 'delete',
    'login', 'logout',
    'invite', 'accept', 'reject', 'revoke',
    'export',
    'log_taken', 'log_skipped', 'log_snoozed',
    'restock', 'catalog_search',
    -- aksi admin
    'admin_grant', 'admin_revoke', 'admin_scope_change',
    'user_disable', 'user_enable', 'user_erase',
    'catalog_create', 'catalog_update', 'catalog_publish',
    'catalog_unpublish', 'catalog_archive', 'catalog_import',
    'catalog_merge',
    'notif_retry', 'notif_cancel',
    'retention_purge'
));

COMMENT ON CONSTRAINT ck_audit_action ON audit_log IS
    'Aksi yang boleh dicatat. Menambah nilai = migrasi baru, bukan edit manual.';

-- +goose Down
ALTER TABLE audit_log DROP CONSTRAINT IF EXISTS ck_audit_action;

ALTER TABLE audit_log ADD CONSTRAINT ck_audit_action CHECK (action IN (
    'create', 'read', 'update', 'delete', 'login', 'logout',
    'invite', 'accept', 'revoke', 'export',
    'log_taken', 'log_skipped', 'restock', 'catalog_search',
    'admin_grant', 'admin_revoke', 'admin_scope_change',
    'user_disable', 'user_enable', 'user_erase',
    'catalog_create', 'catalog_update', 'catalog_publish',
    'catalog_unpublish', 'catalog_archive', 'catalog_import',
    'catalog_merge',
    'notif_retry', 'notif_cancel', 'retention_purge'
));
