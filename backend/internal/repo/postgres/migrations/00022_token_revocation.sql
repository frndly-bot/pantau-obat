-- +goose Up
-- -----------------------------------------------------------------------------
-- Pencabutan akses seketika untuk access token berumur panjang.
--
-- MASALAH: access token adalah JWT yang tidak bisa ditarik kembali sebelum
-- kedaluwarsa. Dengan umur 24 jam, aksi admin seperti "nonaktifkan akun"
-- tidak langsung berlaku: token yang sudah beredar masih lolos sampai habis
-- waktunya. Memperpendek umur token menyempitkan jendela itu, tapi memaksa
-- user refresh jauh lebih sering.
--
-- SOLUSI: satu garis potong waktu per user. Token yang diterbitkan SEBELUM
-- garis itu dianggap mati, walau tanda tangannya sah dan belum kedaluwarsa.
-- Ini bekerja karena klaim `iat` sudah ada di setiap token, jadi tidak perlu
-- menyimpan daftar token sama sekali.
--
-- Kenapa tabel terpisah, bukan kolom di `users`: pencabutan adalah peristiwa
-- yang punya alasan dan pelaku. Menyimpannya sebagai baris membuat jejaknya
-- bisa diaudit, dan memungkinkan pencabutan lama dibersihkan tanpa menyentuh
-- data user.
-- -----------------------------------------------------------------------------
CREATE TABLE user_token_revocations (
    user_id     UUID PRIMARY KEY REFERENCES users(id) ON DELETE RESTRICT,
    revoked_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    reason      TEXT NOT NULL CHECK (reason IN (
                    'password_changed',
                    'logout_all_devices',
                    'token_reuse_detected',
                    'admin_disable',
                    'admin_erase',
                    'admin_revoke_role',
                    'user_request')),
    revoked_by  UUID NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE user_token_revocations IS
    'Garis potong waktu: access token dengan iat < revoked_at ditolak. Satu baris per user.';

COMMENT ON COLUMN user_token_revocations.revoked_at IS
    'Token yang diterbitkan sebelum waktu ini tidak berlaku lagi, walau JWT-nya masih valid.';

CREATE INDEX ix_revocation_time ON user_token_revocations (revoked_at);

CREATE TRIGGER trg_revocation_updated BEFORE UPDATE ON user_token_revocations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TRIGGER IF EXISTS trg_revocation_updated ON user_token_revocations;
DROP INDEX IF EXISTS ix_revocation_time;
DROP TABLE IF EXISTS user_token_revocations CASCADE;
