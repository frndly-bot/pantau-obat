-- +goose Up
-- -----------------------------------------------------------------------------
-- Eskalasi ke caregiver: jenis antrean baru + kolom "tentang siapa".
--
-- LATAR
--
-- notification-v2 bagian 9 menjanjikan: "kalau streak >= N (default 3):
-- buat job missed_escalation untuk caregiver". Janji itu belum pernah
-- dijalankan — `MissedPass` hanya menandai dosis terlewat, tidak pernah
-- memberi tahu siapa pun. Akibatnya caregiver baru tahu ada masalah kalau
-- ia membuka aplikasi sendiri, padahal justru merekalah yang bisa menindak.
--
-- KENAPA MENAMBAH KOLOM, BUKAN MEMBUAT TABEL ANTREAN BARU
--
-- `bump_queue` sudah menjawab "kapan sesuatu harus terjadi", dengan
-- penjadwalan, jam sunyi, dedup, retry, dan pencatatan ke notification_jobs.
-- Eskalasi butuh SEMUA itu. Tabel antrean kedua akan menggandakan seluruh
-- mesin itu, dan dua mesin yang mirip pasti akan menyimpang satu sama lain.
--
-- KENAPA BUTUH `subject_owner_id`
--
-- Sampai sekarang `owner_id` selalu berarti "pemilik data DAN penerima" —
-- dua peran yang kebetulan selalu orang yang sama. Pada eskalasi keduanya
-- BERBEDA: datanya milik pasien, tapi penerimanya caregiver.
--
-- Memaksakan satu kolom untuk dua makna akan memaksa salah satunya salah,
-- dan kesalahan itu berbentuk data kesehatan yang dikirim ke orang yang
-- salah. Karena itu perannya dipisah:
--
--   owner_id         = PENERIMA notifikasi (yang membuka kotak masuk)
--   subject_owner_id = PEMILIK data (pasien yang datanya dibicarakan)
--
-- Untuk semua jenis antrean yang sudah ada, keduanya bernilai sama.
-- -----------------------------------------------------------------------------

ALTER TABLE bump_queue
    DROP CONSTRAINT bump_queue_kind_check;

ALTER TABLE bump_queue
    ADD CONSTRAINT bump_queue_kind_check
    CHECK (kind IN ('dose','refill','digest','escalation'));

-- Kolom baru: pemilik data, bila berbeda dari penerima.
--
-- NULL untuk jenis antrean lama supaya tidak ada baris yang perlu ditulis
-- ulang: ketiadaannya berarti "sama dengan owner_id", dan itu memang benar.
ALTER TABLE bump_queue
    ADD COLUMN subject_owner_id UUID NULL REFERENCES users(id) ON DELETE RESTRICT;

COMMENT ON COLUMN bump_queue.owner_id IS
    'PENERIMA notifikasi — pemilik kotak masuk yang membacanya.';
COMMENT ON COLUMN bump_queue.subject_owner_id IS
    'PEMILIK data yang dibicarakan, bila berbeda dari penerima (mis. pasien pada eskalasi ke caregiver). NULL berarti sama dengan owner_id.';

-- Indeks untuk menelusuri eskalasi satu pasien: "siapa saja yang pernah
-- diberi tahu tentang pasien ini".
CREATE INDEX ix_bump_subject
    ON bump_queue (subject_owner_id, scheduled_at DESC)
    WHERE subject_owner_id IS NOT NULL;


-- +goose Down
DROP INDEX IF EXISTS ix_bump_subject;

ALTER TABLE bump_queue DROP COLUMN IF EXISTS subject_owner_id;

ALTER TABLE bump_queue
    DROP CONSTRAINT bump_queue_kind_check;

ALTER TABLE bump_queue
    ADD CONSTRAINT bump_queue_kind_check
    CHECK (kind IN ('dose','refill','digest'));
