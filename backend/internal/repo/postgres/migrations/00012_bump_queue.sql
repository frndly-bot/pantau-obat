-- +goose Up
CREATE TABLE bump_queue (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind            TEXT NOT NULL CHECK (kind IN ('dose','refill','digest')),
    owner_id        UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    medication_id   UUID NULL REFERENCES medications(id) ON DELETE RESTRICT,
    schedule_id     UUID NULL REFERENCES medication_times(id) ON DELETE RESTRICT,
    scheduled_at    TIMESTAMPTZ NOT NULL,
    slot_date       DATE NOT NULL,
    dedup_key       TEXT NOT NULL,
    attempts        SMALLINT NOT NULL DEFAULT 0,
    claimed_at      TIMESTAMPTZ NULL,
    done_at         TIMESTAMPTZ NULL,
    skip_tag        TEXT NULL,                               -- 'snooze'|'quiet'|NULL
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_bump_dedup ON bump_queue (dedup_key) WHERE done_at IS NULL;
CREATE INDEX ix_bump_due   ON bump_queue (scheduled_at) WHERE done_at IS NULL;
CREATE INDEX ix_bump_owner ON bump_queue (owner_id, scheduled_at DESC);

-- +goose Down
DROP INDEX IF EXISTS ix_bump_owner;
DROP INDEX IF EXISTS ix_bump_due;
DROP INDEX IF EXISTS uq_bump_dedup;
DROP TABLE IF EXISTS bump_queue CASCADE;
