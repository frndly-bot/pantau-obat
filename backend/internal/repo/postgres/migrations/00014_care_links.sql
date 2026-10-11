-- +goose Up
CREATE TABLE care_links (
    id                  UUID PRIMARY KEY DEFAULT uuidv7(),
    owner_id            UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    caregiver_id        UUID NULL REFERENCES users(id) ON DELETE RESTRICT,
    invite_email        CITEXT NULL,
    relation            TEXT NOT NULL CHECK (relation IN
                            ('family','child','parent','spouse','friend','nurse','other')),
    role                TEXT NOT NULL DEFAULT 'viewer' CHECK (role IN ('viewer','helper')),
    status              TEXT NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending','accepted','rejected','revoked','expired')),
    invited_by          UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    invite_token_hash   TEXT NULL,
    invite_expires_at   TIMESTAMPTZ NULL,
    decided_at          TIMESTAMPTZ NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at          TIMESTAMPTZ NULL,
    CONSTRAINT ck_care_not_self CHECK (caregiver_id IS NULL OR owner_id <> caregiver_id)
);
CREATE UNIQUE INDEX uq_care_active_pair
    ON care_links (owner_id, caregiver_id)
    WHERE deleted_at IS NULL AND caregiver_id IS NOT NULL AND status IN ('pending','accepted');
CREATE UNIQUE INDEX uq_care_pending_invite
    ON care_links (owner_id, invite_email)
    WHERE deleted_at IS NULL AND status = 'pending' AND invite_email IS NOT NULL;
CREATE UNIQUE INDEX uq_care_token
    ON care_links (invite_token_hash)
    WHERE invite_token_hash IS NOT NULL AND status = 'pending';
CREATE INDEX ix_care_owner   ON care_links (owner_id) WHERE status = 'accepted' AND deleted_at IS NULL;
CREATE INDEX ix_care_caregiver ON care_links (caregiver_id) WHERE status = 'accepted' AND deleted_at IS NULL;
CREATE TRIGGER trg_care_updated BEFORE UPDATE ON care_links
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TRIGGER IF EXISTS trg_care_updated ON care_links;
DROP INDEX IF EXISTS ix_care_caregiver;
DROP INDEX IF EXISTS ix_care_owner;
DROP INDEX IF EXISTS uq_care_token;
DROP INDEX IF EXISTS uq_care_pending_invite;
DROP INDEX IF EXISTS uq_care_active_pair;
DROP TABLE IF EXISTS care_links CASCADE;
