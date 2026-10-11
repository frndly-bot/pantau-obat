-- +goose Up
CREATE TABLE medication_times (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    medication_id   UUID NOT NULL REFERENCES medications(id) ON DELETE RESTRICT,
    frequency_id    UUID NULL REFERENCES medication_frequency(id) ON DELETE RESTRICT,
    owner_id        UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    time_local      TIME NOT NULL,
    label           TEXT NULL CHECK (label IS NULL OR label IN ('morning','afternoon','evening','dawn','other')),
    weekdays        SMALLINT NOT NULL DEFAULT 127 CHECK (weekdays BETWEEN 1 AND 127),
    dose_override   TEXT NULL,
    sort_order      SMALLINT NOT NULL DEFAULT 0,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX uq_times_med_time
    ON medication_times (medication_id, time_local, weekdays)
    WHERE deleted_at IS NULL;
CREATE INDEX ix_times_med    ON medication_times (medication_id) WHERE deleted_at IS NULL;
CREATE INDEX ix_times_owner  ON medication_times (owner_id, is_active) WHERE deleted_at IS NULL;
CREATE INDEX ix_times_clock  ON medication_times (time_local) WHERE deleted_at IS NULL AND is_active;
CREATE TRIGGER trg_times_updated BEFORE UPDATE ON medication_times
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Keputusan: jadwal disimpan sebagai ATURAN (medication_frequency +
-- medication_times). Tidak ada baris yang di-generate per tanggal, jadi
-- resep "selamanya" tidak menambah satu baris pun.
-- Cara kerja:
--   * Waktu minum (dengan timezone user) dihitung di kode Go dari aturan,
--     dengan bantuan *occurrence key* + `medication_timezone` bila zona user
--     berubah di tengah terapi.
--   * Yang disimpan tetap: medication_logs (dosis benar-benar diminum /
--     terlewat / dilewati). Ini yang membuat resep "selamanya" tetap murah:
--     biaya tumbuh mengikuti jumlah dosis yang BENAR-BENAR diambil, bukan
--     jumlah hari di kalender.
--   * Yang benar-benar perlu baris: pekerjaan yang akan datang, mis. snooze
--     dan job notifikasi. Itu disimpan di bump_queue, bukan di jadwal.
-- Konsekuensi yang harus dijaga worker:
--   * Denda "terlewat" dihitung dari jadwal hidup + medication_dose_events,
--     bukan dari status baris.

-- Zona waktu per blok jadwal (wajib, karena user bisa pindah zona).
ALTER TABLE medication_frequency
    ADD COLUMN IF NOT EXISTS schedule_timezone TEXT NOT NULL DEFAULT 'Asia/Jakarta';

-- +goose Down
ALTER TABLE medication_frequency DROP COLUMN IF EXISTS schedule_timezone;
DROP TRIGGER IF EXISTS trg_times_updated ON medication_times;
DROP INDEX IF EXISTS ix_times_clock;
DROP INDEX IF EXISTS ix_times_owner;
DROP INDEX IF EXISTS ix_times_med;
DROP INDEX IF EXISTS uq_times_med_time;
DROP TABLE IF EXISTS medication_times CASCADE;
