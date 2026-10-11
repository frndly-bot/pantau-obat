-- +goose Up
CREATE TABLE medication_stock (
    id                      UUID PRIMARY KEY DEFAULT uuidv7(),
    medication_id           UUID NOT NULL UNIQUE REFERENCES medications(id) ON DELETE RESTRICT,
    owner_id                UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    unit_code               TEXT NOT NULL REFERENCES ref_unit(code) ON DELETE RESTRICT,
    initial_qty             NUMERIC(12,3) NOT NULL CHECK (initial_qty >= 0),
    current_qty             NUMERIC(12,3) NOT NULL CHECK (current_qty >= 0),
    qty_per_dose            NUMERIC(12,3) NOT NULL DEFAULT 1 CHECK (qty_per_dose > 0),
    refill_threshold_qty    NUMERIC(12,3) NULL CHECK (refill_threshold_qty IS NULL OR refill_threshold_qty >= 0),
    refill_reminder         BOOLEAN NOT NULL DEFAULT false,
    last_restocked_at       TIMESTAMPTZ NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at              TIMESTAMPTZ NULL
);
CREATE INDEX ix_stock_low
    ON medication_stock (owner_id)
    WHERE deleted_at IS NULL AND refill_reminder = true;
CREATE TRIGGER trg_stock_updated BEFORE UPDATE ON medication_stock
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE stock_transactions (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    stock_id        UUID NOT NULL REFERENCES medication_stock(id) ON DELETE RESTRICT,
    medication_id   UUID NOT NULL REFERENCES medications(id) ON DELETE RESTRICT,
    owner_id        UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    kind            TEXT NOT NULL CHECK (kind IN ('initial','restock','consume','adjust','expire')),
    qty_delta       NUMERIC(12,3) NOT NULL,
    balance_after   NUMERIC(12,3) NOT NULL CHECK (balance_after >= 0),
    log_id          UUID NULL REFERENCES medication_logs(id) ON DELETE RESTRICT,
    note            TEXT NULL CHECK (note IS NULL OR length(note) <= 500),
    recorded_by     UUID NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_stock_txn_stock ON stock_transactions (stock_id, created_at DESC);
CREATE INDEX ix_stock_txn_owner ON stock_transactions (owner_id, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS ix_stock_txn_owner;
DROP INDEX IF EXISTS ix_stock_txn_stock;
DROP TABLE IF EXISTS stock_transactions CASCADE;
DROP TRIGGER IF EXISTS trg_stock_updated ON medication_stock;
DROP INDEX IF EXISTS ix_stock_low;
DROP TABLE IF EXISTS medication_stock CASCADE;
