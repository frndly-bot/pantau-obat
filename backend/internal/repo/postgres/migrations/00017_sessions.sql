-- +goose Up
CREATE TABLE refresh_sessions (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    token_hash      TEXT NOT NULL UNIQUE,
    expires_at      TIMESTAMPTZ NOT NULL,
    revoked_at      TIMESTAMPTZ NULL,
    rotated_from    UUID NULL REFERENCES refresh_sessions(id) ON DELETE RESTRICT,
    created_ip      INET NULL,
    user_agent      TEXT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_session_user_active ON refresh_sessions (user_id, expires_at) WHERE revoked_at IS NULL;
CREATE INDEX ix_session_expiry      ON refresh_sessions (expires_at) WHERE revoked_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS ix_session_expiry;
DROP INDEX IF EXISTS ix_session_user_active;
DROP TABLE IF EXISTS refresh_sessions CASCADE;
