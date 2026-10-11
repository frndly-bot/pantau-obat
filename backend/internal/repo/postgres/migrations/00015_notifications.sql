-- +goose Up
CREATE TABLE notification_prefs (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    channel         TEXT NOT NULL CHECK (channel IN ('push','email','telegram','in_app')),
    enabled         BOOLEAN NOT NULL DEFAULT true,
    quiet_start     TIME NULL,
    quiet_end       TIME NULL,
    advance_minutes SMALLINT NOT NULL DEFAULT 10 CHECK (advance_minutes BETWEEN 0 AND 120),
    notif_throttled BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_pref_user_channel UNIQUE (user_id, channel)
);
CREATE INDEX ix_pref_channel_enabled ON notification_prefs (channel) WHERE enabled = true;
CREATE TRIGGER trg_pref_updated BEFORE UPDATE ON notification_prefs
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE device_tokens (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    platform        TEXT NOT NULL CHECK (platform IN ('android','ios','web')),
    token           TEXT NOT NULL UNIQUE,
    app_version     TEXT NULL,
    last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at      TIMESTAMPTZ NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_devtok_user ON device_tokens (user_id) WHERE revoked_at IS NULL;

CREATE TABLE user_integrations (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    provider        TEXT NOT NULL CHECK (provider IN ('telegram','wa')),
    external_id     TEXT NOT NULL,
    verified_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at      TIMESTAMPTZ NULL,
    CONSTRAINT uq_integration_provider_ext UNIQUE (provider, external_id)
);
CREATE INDEX ix_integration_user ON user_integrations (user_id) WHERE revoked_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS ix_integration_user;
DROP TABLE IF EXISTS user_integrations CASCADE;
DROP INDEX IF EXISTS ix_devtok_user;
DROP TABLE IF EXISTS device_tokens CASCADE;
DROP TRIGGER IF EXISTS trg_pref_updated ON notification_prefs;
DROP INDEX IF EXISTS ix_pref_channel_enabled;
DROP TABLE IF EXISTS notification_prefs CASCADE;
