-- +goose Up
-- -----------------------------------------------------------------------------
-- Menambahkan role caregiver `manager`.
--
-- LATAR: api-v2 bagian 9 menjanjikan tiga tingkat caregiver — viewer (baca),
-- helper (catat log), manager (helper + kelola obat & stok pasien). Design
-- UI (design/user/caregiver.html) menyediakan "Tambah obat" dan "Ubah stok"
-- atas nama pasien. Backend hanya punya viewer dan helper, sehingga janji
-- kontrak itu tidak bisa dipenuhi dan design tidak bisa diimplementasi.
--
-- CHECK constraint lama tidak bisa diubah in-place; PostgreSQL tidak
-- mendukung ALTER CONSTRAINT untuk CHECK. Caranya: DROP lama, ADD baru.
-- Aman karena nilainya superset — baris 'viewer'/'helper' yang sudah ada
-- tetap valid di CHECK baru.
-- -----------------------------------------------------------------------------

ALTER TABLE care_links
    DROP CONSTRAINT care_links_role_check;

ALTER TABLE care_links
    ADD CONSTRAINT care_links_role_check
    CHECK (role = ANY (ARRAY['viewer'::text, 'helper'::text, 'manager'::text]));

-- +goose Down
-- -----------------------------------------------------------------------------
-- Kembali ke dua role. GAGAL otomatis (dengan error CHECK violation) kalau
-- masih ada baris 'manager' — dan itu benar: menurunkan skema tidak boleh
-- diam-diam menghilangkan data yang sudah ditulis.
-- -----------------------------------------------------------------------------

ALTER TABLE care_links
    DROP CONSTRAINT care_links_role_check;

ALTER TABLE care_links
    ADD CONSTRAINT care_links_role_check
    CHECK (role = ANY (ARRAY['viewer'::text, 'helper'::text]));
