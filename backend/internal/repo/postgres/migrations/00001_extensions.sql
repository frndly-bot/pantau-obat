-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;  -- digest(), gen_random_uuid() fallback
CREATE EXTENSION IF NOT EXISTS citext;    -- email case-insensitive

-- +goose Down
DROP EXTENSION IF EXISTS citext;
DROP EXTENSION IF EXISTS pgcrypto;
