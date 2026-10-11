-- +goose Up
CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    email           CITEXT NOT NULL UNIQUE,
    phone           TEXT NULL CHECK (phone IS NULL OR length(phone) BETWEEN 6 AND 20),
    password_hash   TEXT NOT NULL,
    full_name       TEXT NOT NULL CHECK (length(full_name) BETWEEN 1 AND 100),
    timezone        TEXT NOT NULL DEFAULT 'Asia/Jakarta',
    is_active       BOOLEAN NOT NULL DEFAULT true,
    -- Peran: 'user' = pengguna aplikasi, 'admin' = pengelola aplikasi.
    -- Dipisah dari is_active supaya blokir akun tidak mengubah peran.
    -- Admin hanya ditetapkan lewat operasi DB/seed, tidak ada endpoint
    -- registrasi yang boleh menerima role dari klien.
    role            TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user','admin')),
    admin_scope     TEXT[] NULL,                             -- NULL = akses penuh
    admin_granted_at TIMESTAMPTZ NULL,
    admin_granted_by UUID NULL,                              -- diisi lewat ALTER setelah tabel ada
    last_login_at   TIMESTAMPTZ NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ NULL
);
CREATE INDEX ix_users_inactive ON users (id) WHERE is_active = false;
CREATE INDEX ix_users_admins   ON users (id) WHERE role = 'admin' AND deleted_at IS NULL;
CREATE TRIGGER trg_users_updated BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- FK drug_catalog.owner_id baru bisa ditambah setelah tabel users ada
-- (drug_catalog didefinisikan lebih dahulu karena medications butuh drug_catalog).
ALTER TABLE drug_catalog
    ADD CONSTRAINT fk_catalog_owner
    FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE RESTRICT;

-- Self-FK: siapa yang memberi peran admin (null = bootstrap/seed).
ALTER TABLE users
    ADD CONSTRAINT fk_users_admin_granted_by
    FOREIGN KEY (admin_granted_by) REFERENCES users(id) ON DELETE RESTRICT;

-- Admin scope yang valid. NULL berarti akses penuh (super admin).
ALTER TABLE users
    ADD CONSTRAINT ck_admin_scope CHECK (
        admin_scope IS NULL
        OR admin_scope <@ ARRAY['users','catalog','notifications','reports']::TEXT[]
    );

-- Hanya akun role='admin' yang boleh punya admin_scope.
ALTER TABLE users
    ADD CONSTRAINT ck_admin_scope_only_admin CHECK (
        role = 'admin' OR admin_scope IS NULL
    );

--     (definisi penuh audit_log ada di bagian 21)
COMMENT ON COLUMN users.role IS 'user = pengguna aplikasi; admin = pengelola. Ditetapkan lewat DB, bukan API.';
COMMENT ON COLUMN users.admin_scope IS 'Bidang admin: users, catalog, notifications, reports. NULL = penuh.';

-- +goose Down
ALTER TABLE users DROP CONSTRAINT IF EXISTS ck_admin_scope_only_admin;
ALTER TABLE users DROP CONSTRAINT IF EXISTS ck_admin_scope;
ALTER TABLE users DROP CONSTRAINT IF EXISTS fk_users_admin_granted_by;
ALTER TABLE drug_catalog DROP CONSTRAINT IF EXISTS fk_catalog_owner;
DROP TRIGGER IF EXISTS trg_users_updated ON users;
DROP INDEX IF EXISTS ix_users_admins;
DROP INDEX IF EXISTS ix_users_inactive;
DROP TABLE IF EXISTS users CASCADE;
