-- +goose Up
CREATE TABLE medication_frequency (
    id                  UUID PRIMARY KEY DEFAULT uuidv7(),
    medication_id       UUID NOT NULL UNIQUE REFERENCES medications(id) ON DELETE RESTRICT,
    owner_id            UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    pattern             TEXT NOT NULL CHECK (pattern IN
                            ('daily','interval','weekly','monthly_cycle','prn')),
    times_per_day       SMALLINT NULL CHECK (times_per_day IS NULL OR times_per_day BETWEEN 1 AND 24),
    interval_hours      SMALLINT NULL CHECK (interval_hours IS NULL OR interval_hours BETWEEN 1 AND 168),
    days_of_week        SMALLINT NULL CHECK (days_of_week IS NULL OR days_of_week BETWEEN 1 AND 127),
    interval_min_hours  SMALLINT NULL CHECK (interval_min_hours IS NULL OR interval_min_hours BETWEEN 1 AND 168),
    cycle_intake_days   SMALLINT NULL CHECK (cycle_intake_days IS NULL OR cycle_intake_days BETWEEN 1 AND 365),
    cycle_pause_days    SMALLINT NULL CHECK (cycle_pause_days IS NULL OR cycle_pause_days BETWEEN 0 AND 365),
    cycle_anchor        TEXT NULL CHECK (cycle_anchor IS NULL OR cycle_anchor IN
                            ('dom','end_of_month','from_start')),
    cycle_anchor_dom    SMALLINT NULL CHECK (cycle_anchor_dom IS NULL OR cycle_anchor_dom BETWEEN 1 AND 31),
    prn_max_per_day     SMALLINT NULL CHECK (prn_max_per_day IS NULL OR prn_max_per_day BETWEEN 1 AND 24),
    tolerance_minutes   SMALLINT NOT NULL DEFAULT 0 CHECK (tolerance_minutes BETWEEN 0 AND 120),
    start_time          TIME NOT NULL DEFAULT '08:00',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at          TIMESTAMPTZ NULL,
    -- pola wajib punya parameternya
    CONSTRAINT ck_freq_params CHECK (
        (pattern = 'daily'         AND times_per_day IS NOT NULL) OR
        (pattern = 'interval'      AND interval_hours IS NOT NULL) OR
        (pattern = 'weekly'        AND days_of_week IS NOT NULL) OR
        (pattern = 'monthly_cycle' AND cycle_intake_days IS NOT NULL
                                   AND cycle_pause_days IS NOT NULL
                                   AND cycle_anchor IS NOT NULL) OR
        (pattern = 'prn')
    )
);
CREATE INDEX ix_freq_owner ON medication_frequency (owner_id) WHERE deleted_at IS NULL;
CREATE TRIGGER trg_freq_updated BEFORE UPDATE ON medication_frequency
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TRIGGER IF EXISTS trg_freq_updated ON medication_frequency;
DROP INDEX IF EXISTS ix_freq_owner;
DROP TABLE IF EXISTS medication_frequency CASCADE;
