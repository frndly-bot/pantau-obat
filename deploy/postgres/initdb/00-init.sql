-- =============================================================================
-- initdb — dijalankan otomatis HANYA saat volume pgdata masih kosong.
-- Tujuan: memasang ekstensi dasar. Migrasi goose juga memasangnya
-- (lihat backend/migrations/00001_extensions.sql), jadi dua jalur ini aman.
--
-- Kalau file ini diubah, perubahan TIDAK berlaku pada volume yang sudah ada.
-- Untuk volume yang sudah terisi, jalankan manual:
--   docker compose exec postgres psql -U pantau_obat -d pantau_obat \
--     -c "CREATE EXTENSION IF NOT EXISTS pgcrypto; CREATE EXTENSION IF NOT EXISTS citext;"
-- =============================================================================

CREATE EXTENSION IF NOT EXISTS pgcrypto;  -- digest(), gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS citext;    -- email case-insensitive

-- Skema `public` tetap jadi tempat objek aplikasi.
COMMENT ON SCHEMA public IS 'Skema aplikasi Pantau Obat.';
