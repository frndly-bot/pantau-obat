-- +goose Up
-- -----------------------------------------------------------------------------
-- Token untuk alur LUPA SANDI.
--
-- KENAPA TABEL TERSENDIRI
--
-- Alur ini berbeda dari login: pemegangnya adalah seseorang yang BELUM
-- membuktikan identitas — ia hanya punya akses ke email. Karena itu tokennya
-- butuh perlakuan berbeda dari refresh token:
--
--   - UMURNYA PENDEK (15 menit): cukup untuk membuka email dan mengeklik.
--   - SEKALI PAKAI: dipakai langsung ditandai. Token yang bisa dipakai
--     berkali-kali berarti kebocoran satu token = akses permanen.
--   - TIDAK TERSIMPAN MENTAH: yang disimpan hanya hash-nya. Bila database
--     bocor, token yang belum dipakai tetap tidak bisa dipakai penyerang.
--   - MENCATAT KAPAN DIMINTA: dipakai untuk menahan permintaan beruntun
--     pada alamat yang sama.
--
-- Token reset TIDAK membatalkan sesi yang sudah ada. Kalau seseorang
-- meminta reset dengan alamat yang bukan miliknya, pemilik akun yang sedang
-- memakai aplikasi tidak boleh tiba-tiba dikeluarkan.
-- -----------------------------------------------------------------------------


CREATE TABLE password_reset_tokens (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    token_hash      TEXT NOT NULL,
    expires_at      TIMESTAMPTZ NOT NULL,
    used_at         TIMESTAMPTZ NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Token yang sudah dipakai tidak boleh lebih awal dari dibuat.
    CONSTRAINT ck_reset_used_after_created
        CHECK (used_at IS NULL OR used_at >= created_at)
);

-- Jeda antar permintaan per user ditegakkan di aplikasi memakai indeks ini.
CREATE INDEX ix_reset_user_created
    ON password_reset_tokens (user_id, created_at DESC);

-- Pencarian token saat dipakai. Unik karena hash yang sama hanya lahir dari
-- token mentah yang sama.
CREATE UNIQUE INDEX ix_reset_hash ON password_reset_tokens (token_hash);

COMMENT ON TABLE password_reset_tokens IS
    'Token sekali-pakai untuk alur lupa sandi. Yang disimpan hanya hash tokennya; token mentah hanya ada di tautan email.';


-- +goose Down
DROP TABLE IF EXISTS password_reset_tokens;
