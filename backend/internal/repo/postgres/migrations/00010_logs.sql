-- +goose Up
CREATE TABLE medication_logs (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    medication_id   UUID NOT NULL REFERENCES medications(id) ON DELETE RESTRICT,
    schedule_id     UUID NULL REFERENCES medication_times(id) ON DELETE RESTRICT,
    owner_id        UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    recorded_by     UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    slot_date       DATE NOT NULL,                           -- tanggal lokal pemilik
    scheduled_at    TIMESTAMPTZ NULL,
    scheduled_hour  SMALLINT NULL CHECK (scheduled_hour IS NULL OR scheduled_hour BETWEEN 0 AND 23),
    scheduled_minute SMALLINT NULL CHECK (scheduled_minute IS NULL OR scheduled_minute BETWEEN 0 AND 59),
    scheduled_tz    TEXT NULL,                               -- zona saat dosis itu jatuh
    status          TEXT NOT NULL CHECK (status IN ('taken','late','skipped','snoozed')),
    taken_at        TIMESTAMPTZ NULL,
    note            TEXT NULL CHECK (note IS NULL OR length(note) <= 500),
    source          TEXT NOT NULL DEFAULT 'self' CHECK (source IN ('self','caregiver','system')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ NULL,
    -- occurrence key: membuat dosis terjadwal unik tanpa tabel slot
    CONSTRAINT ck_log_occurrence CHECK (
        (schedule_id IS NULL AND scheduled_hour IS NULL AND scheduled_minute IS NULL)
        OR (schedule_id IS NOT NULL AND scheduled_hour IS NOT NULL AND scheduled_minute IS NOT NULL)
    ),
    CONSTRAINT ck_log_taken_at CHECK (
        (status IN ('taken','late') AND taken_at IS NOT NULL) OR
        (status NOT IN ('taken','late'))
    )
);
-- Kunci anti-ganda utama: satu dosis per (obat, jam jadwal, tanggal, jam:menit).
-- Schedule_id NULL = dosis PRN/spontan, boleh berkali-kali, jadi tidak diikutkan.
CREATE UNIQUE INDEX uq_log_occurrence
    ON medication_logs (medication_id, schedule_id, slot_date, scheduled_hour, scheduled_minute)
    WHERE deleted_at IS NULL AND schedule_id IS NOT NULL;
CREATE INDEX ix_log_owner_date ON medication_logs (owner_id, slot_date DESC) WHERE deleted_at IS NULL;
CREATE INDEX ix_log_med_date   ON medication_logs (medication_id, slot_date DESC) WHERE deleted_at IS NULL;
CREATE INDEX ix_log_recorder   ON medication_logs (recorded_by, created_at DESC);
CREATE TRIGGER trg_logs_updated BEFORE UPDATE ON medication_logs
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- taken / skipped dicatat di medication_logs (baris hidup, bisa dikoreksi).
-- Yang lain ditulis di sini supaya jumlah baris tetap = jumlah kejadian nyata:
--   * missed    : slot lewat jam tanpa log
--   * dose_shift: frekuensi diubah (mis. tiap 8 jam -> 3x sehari)
--   * paused    : terapi dijeda, atau month/day tidak match slot PRN
-- event_key = "schedule_id|YYYY-MM-DD|HH:MM" (PRIMARY KEY -> anti dobel).

-- +goose Down
DROP TRIGGER IF EXISTS trg_logs_updated ON medication_logs;
DROP INDEX IF EXISTS ix_log_recorder;
DROP INDEX IF EXISTS ix_log_med_date;
DROP INDEX IF EXISTS ix_log_owner_date;
DROP INDEX IF EXISTS uq_log_occurrence;
DROP TABLE IF EXISTS medication_logs CASCADE;
