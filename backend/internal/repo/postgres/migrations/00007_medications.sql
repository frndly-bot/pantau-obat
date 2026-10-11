-- +goose Up
CREATE TABLE medications (
    id                      UUID PRIMARY KEY DEFAULT uuidv7(),
    owner_id                UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    catalog_id              UUID NULL REFERENCES drug_catalog(id) ON DELETE RESTRICT,
    name                    TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    dose_value              NUMERIC(12,3) NOT NULL CHECK (dose_value > 0),
    dose_unit_code          TEXT NOT NULL REFERENCES ref_unit(code) ON DELETE RESTRICT,
    strength_note           TEXT NULL,                       -- "120mg/5mL", "100IU/mL"
    route_code              TEXT NOT NULL REFERENCES ref_route(code) ON DELETE RESTRICT,
    dosage_form_code        TEXT NULL REFERENCES ref_dosage_form(code) ON DELETE RESTRICT,
    meal_relation           TEXT NOT NULL DEFAULT 'not_related'
                            CHECK (meal_relation IN
                                   ('before_meal','after_meal','with_meal','not_related')),
    as_needed               BOOLEAN NOT NULL DEFAULT false,
    must_finish             BOOLEAN NOT NULL DEFAULT false,
    start_date              DATE NOT NULL,
    start_time              TIME NOT NULL DEFAULT '00:00',
    end_date                DATE NULL CHECK (end_date IS NULL OR end_date >= start_date),
    notes                   TEXT NULL CHECK (notes IS NULL OR length(notes) <= 2000),
    notes_reminder          TEXT NULL CHECK (notes_reminder IS NULL OR length(notes_reminder) <= 1000),
    prescription_photo_url  TEXT NULL CHECK (prescription_photo_url IS NULL OR prescription_photo_url LIKE 'https://%'),
    color_tag               TEXT NULL CHECK (color_tag IS NULL OR color_tag ~ '^#[0-9a-fA-F]{6}$'),
    is_active               BOOLEAN NOT NULL DEFAULT true,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at              TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX uq_meds_owner_name
    ON medications (owner_id, lower(name), dose_value, dose_unit_code)
    WHERE deleted_at IS NULL;
CREATE INDEX ix_meds_owner ON medications (owner_id) WHERE deleted_at IS NULL;
CREATE INDEX ix_meds_owner_active
    ON medications (owner_id) WHERE deleted_at IS NULL AND is_active = true;
CREATE INDEX ix_meds_catalog ON medications (catalog_id) WHERE catalog_id IS NOT NULL;
CREATE TRIGGER trg_meds_updated BEFORE UPDATE ON medications
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TRIGGER IF EXISTS trg_meds_updated ON medications;
DROP INDEX IF EXISTS ix_meds_catalog;
DROP INDEX IF EXISTS ix_meds_owner_active;
DROP INDEX IF EXISTS ix_meds_owner;
DROP INDEX IF EXISTS uq_meds_owner_name;
DROP TABLE IF EXISTS medications CASCADE;
