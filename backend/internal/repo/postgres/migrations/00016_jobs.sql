-- +goose Up
CREATE TABLE notification_jobs (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind            TEXT NOT NULL CHECK (kind IN
                        ('dose_reminder','missed_escalation','care_digest','retention','refill_reminder')),
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending','claimed','done','failed','cancelled')),
    dedup_key       TEXT NOT NULL UNIQUE,
    owner_id        UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    payload         JSONB NOT NULL DEFAULT '{}',
    run_at          TIMESTAMPTZ NOT NULL,
    attempts        SMALLINT NOT NULL DEFAULT 0,
    max_attempts    SMALLINT NOT NULL DEFAULT 5,
    last_error      TEXT NULL,
    claimed_at      TIMESTAMPTZ NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_jobs_claimable ON notification_jobs (run_at) WHERE status = 'pending';
CREATE INDEX ix_jobs_owner_time ON notification_jobs (owner_id, created_at DESC);
CREATE TRIGGER trg_jobs_updated BEFORE UPDATE ON notification_jobs
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE notification_deliveries (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    job_id          BIGINT NOT NULL REFERENCES notification_jobs(id) ON DELETE CASCADE,
    channel         TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending','sent','failed','skipped')),
    provider_id     TEXT NULL,
    error_code      TEXT NULL,
    error_msg       TEXT NULL,
    attempts        SMALLINT NOT NULL DEFAULT 0,
    sent_at         TIMESTAMPTZ NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_delivery_job_channel UNIQUE (job_id, channel)
);
CREATE INDEX ix_delivery_status ON notification_deliveries (status) WHERE status IN ('pending','failed');

-- +goose Down
DROP INDEX IF EXISTS ix_delivery_status;
DROP TABLE IF EXISTS notification_deliveries CASCADE;
DROP TRIGGER IF EXISTS trg_jobs_updated ON notification_jobs;
DROP INDEX IF EXISTS ix_jobs_owner_time;
DROP INDEX IF EXISTS ix_jobs_claimable;
DROP TABLE IF EXISTS notification_jobs CASCADE;
