-- +goose Up
CREATE TABLE medication_templates (
    id                      UUID PRIMARY KEY DEFAULT uuidv7(),
    owner_id                UUID NULL,                       -- NULL = template sistem
    name                    TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    catalog_id              UUID NULL REFERENCES drug_catalog(id) ON DELETE RESTRICT,
    dosage_form_code        TEXT NULL REFERENCES ref_dosage_form(code) ON DELETE RESTRICT,
    dose_value              NUMERIC(12,3) NULL,
    dose_unit_code          TEXT NULL REFERENCES ref_unit(code) ON DELETE RESTRICT,
    route_code              TEXT NULL REFERENCES ref_route(code) ON DELETE RESTRICT,
    meal_relation           TEXT NULL
                            CHECK (meal_relation IS NULL OR meal_relation IN
                                   ('before_meal','after_meal','with_meal','not_related')),
    as_needed               BOOLEAN NOT NULL DEFAULT false,
    must_finish             BOOLEAN NOT NULL DEFAULT false,
    default_frequency       JSONB NOT NULL DEFAULT '{}',
    default_stock_qty       NUMERIC(12,3) NULL,
    default_refill_threshold NUMERIC(12,3) NULL,
    notes_reminder          TEXT NULL CHECK (notes_reminder IS NULL OR length(notes_reminder) <= 1000),
    is_system               BOOLEAN NOT NULL DEFAULT false,
    usage_count             INTEGER NOT NULL DEFAULT 0,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at              TIMESTAMPTZ NULL,
    CONSTRAINT ck_tpl_system_owner CHECK (
        (is_system AND owner_id IS NULL) OR (NOT is_system AND owner_id IS NOT NULL)
    )
);
CREATE UNIQUE INDEX uq_tpl_owner_name
    ON medication_templates (owner_id, lower(name))
    WHERE deleted_at IS NULL AND owner_id IS NOT NULL;
CREATE UNIQUE INDEX uq_tpl_system_name
    ON medication_templates (lower(name))
    WHERE deleted_at IS NULL AND is_system;
CREATE TRIGGER trg_tpl_updated BEFORE UPDATE ON medication_templates
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
COMMENT ON TABLE medication_templates IS 'Template input cepat. Hanya mengisi form, tidak menyimpan obat.';

-- +goose Down
DROP TRIGGER IF EXISTS trg_tpl_updated ON medication_templates;
DROP INDEX IF EXISTS uq_tpl_system_name;
DROP INDEX IF EXISTS uq_tpl_owner_name;
DROP TABLE IF EXISTS medication_templates CASCADE;
