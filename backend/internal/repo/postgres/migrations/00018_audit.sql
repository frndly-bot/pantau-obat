-- +goose Up
CREATE TABLE audit_log (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_id        UUID NULL REFERENCES users(id) ON DELETE RESTRICT,
    owner_id        UUID NULL REFERENCES users(id) ON DELETE RESTRICT,
    actor_role      TEXT NOT NULL DEFAULT 'user' CHECK (actor_role IN ('user','admin','system')),
    action          TEXT NOT NULL CHECK (action IN
                        ('create','read','update','delete','login','logout','invite','accept','revoke',
                         'export','log_taken','log_skipped','restock','catalog_search',
                         -- aksi admin
                         'admin_grant','admin_revoke','admin_scope_change',
                         'user_disable','user_enable','user_erase',
                         'catalog_create','catalog_update','catalog_publish','catalog_unpublish',
                         'catalog_archive','catalog_import','catalog_merge',
                         'notif_retry','notif_cancel','retention_purge')),
    resource_type   TEXT NOT NULL CHECK (resource_type IN
                        ('medication','schedule','log','care_link','user','auth','stock','catalog',
                         'admin','notification','retention')),
    resource_id     UUID NULL,
    ip              INET NULL,
    user_agent      TEXT NULL,
    -- jejak sebelum/sesudah untuk perubahan sensitif (peran, status, publish)
    before_state    JSONB NULL,
    after_state     JSONB NULL,
    meta            JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_audit_owner     ON audit_log (owner_id, created_at DESC);
CREATE INDEX ix_audit_actor     ON audit_log (actor_id, created_at DESC);
CREATE INDEX ix_audit_resource  ON audit_log (resource_type, resource_id, created_at DESC);
CREATE INDEX ix_audit_meta_gin  ON audit_log USING GIN (meta);
-- Query "apa saja yang dilakukan admin" tanpa scan seluruh tabel.
CREATE INDEX ix_audit_admin     ON audit_log (actor_id, created_at DESC) WHERE actor_role = 'admin';
CREATE INDEX ix_audit_action    ON audit_log (action, created_at DESC);
-- append-only: tidak ada updated_at / deleted_at.

COMMENT ON COLUMN audit_log.actor_role IS 'Peran pelaku saat aksi terjadi, disalin agar jejak tidak berubah saat peran user berubah.';
COMMENT ON COLUMN audit_log.before_state IS 'Nilai sensitif sebelum perubahan (mis. role, status publikasi). NULL bila tidak relevan.';

-- -----------------------------------------------------------------------------

-- +goose Down
DROP INDEX IF EXISTS ix_audit_action;
DROP INDEX IF EXISTS ix_audit_admin;
DROP INDEX IF EXISTS ix_audit_meta_gin;
DROP INDEX IF EXISTS ix_audit_resource;
DROP INDEX IF EXISTS ix_audit_actor;
DROP INDEX IF EXISTS ix_audit_owner;
DROP TABLE IF EXISTS audit_log CASCADE;
