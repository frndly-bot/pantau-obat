-- +goose Up
-- -----------------------------------------------------------------------------
-- Memperluas CHECK pada `medications.prescription_photo_url` agar menerima
-- KUNCI OBJEK, bukan hanya URL https://.
--
-- LATAR
--
-- Kolom ini dibuat saat desain awal, ketika foto resep dibayangkan sebagai
-- URL ke penyimpanan luar. Constraint-nya karena itu menuntut awalan
-- `https://`, dan itu memang masuk akal pada waktu itu: kolom yang disajikan
-- sebagai `src` gambar tidak boleh diisi `javascript:` atau `data:`.
--
-- Sejak foto resep disimpan di object storage (pgsty/silo) dengan bucket
-- PRIVAT, yang disimpan justru kunci objeknya — bukan URL. Alasannya:
-- tautan ke bucket privat adalah tautan bertanda tangan yang berlaku
-- terbatas (10 menit di sini). Menyimpan tautan seperti itu ke database
-- berarti kolomnya berisi tautan mati dalam hitungan menit, dan foto lama
-- tidak akan pernah bisa dibuka lagi.
--
-- Akibatnya: penulisan kunci objek DITOLAK constraint lama dengan galat
-- check-constraint, yang muncul ke pengguna sebagai 500 tanpa penjelasan.
--
-- YANG TIDAK BERUBAH
--
-- Perlindungan aslinya tetap dipertahankan, dan justru dipersempit: nilai
-- yang diterima hanya (a) NULL, (b) URL https, atau (c) kunci objek dengan
-- bentuk yang PERSIS seperti yang ditulis aplikasi —
-- `photos/<uuid>/<uuid>/<angka>.<ext>`. `javascript:`, `data:`, path
-- relatif, dan `..` tetap ditolak. Bentuknya diverifikasi dengan regex,
-- bukan sekadar awalan `photos/`, supaya kolom ini tidak berubah menjadi
-- tempat menyimpan teks bebas.
-- -----------------------------------------------------------------------------

ALTER TABLE medications
    DROP CONSTRAINT medications_prescription_photo_url_check;

ALTER TABLE medications
    ADD CONSTRAINT medications_prescription_photo_url_check
    CHECK (
        prescription_photo_url IS NULL
        OR prescription_photo_url LIKE 'https://%'
        OR prescription_photo_url ~ '^photos/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/[0-9]+\.(jpg|jpeg|png|webp|heic)$'
    );

COMMENT ON COLUMN medications.prescription_photo_url IS
    'Kunci objek foto resep di object storage (bucket privat), atau URL https untuk data lama. Kunci dipakai — bukan URL — karena tautan ke bucket privat berlaku terbatas dan akan kedaluwarsa bila disimpan.';


-- +goose Down
ALTER TABLE medications
    DROP CONSTRAINT medications_prescription_photo_url_check;

ALTER TABLE medications
    ADD CONSTRAINT medications_prescription_photo_url_check
    CHECK (
        prescription_photo_url IS NULL
        OR prescription_photo_url LIKE 'https://%'
    );
