-- +goose Up
CREATE TABLE medication_dose_events (
    event_key       TEXT PRIMARY KEY,
    owner_id        UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    medication_id   UUID NOT NULL REFERENCES medications(id) ON DELETE RESTRICT,
    schedule_id     UUID NULL REFERENCES medication_times(id) ON DELETE RESTRICT,
    event_date      DATE NOT NULL,
    scheduled_hour  SMALLINT NOT NULL CHECK (scheduled_hour BETWEEN 0 AND 23),
    scheduled_minute SMALLINT NOT NULL CHECK (scheduled_minute BETWEEN 0 AND 59),
    scheduled_tz    TEXT NOT NULL,
    kind            TEXT NOT NULL CHECK (kind IN ('missed','dose_shift','paused')),
    reason          TEXT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_dose_ev_owner_date ON medication_dose_events (owner_id, event_date DESC);
CREATE INDEX ix_dose_ev_med_date   ON medication_dose_events (medication_id, event_date DESC);

-- +goose Down
DROP INDEX IF EXISTS ix_dose_ev_med_date;
DROP INDEX IF EXISTS ix_dose_ev_owner_date;
DROP TABLE IF EXISTS medication_dose_events CASCADE;
