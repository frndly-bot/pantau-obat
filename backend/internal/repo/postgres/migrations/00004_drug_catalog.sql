-- +goose Up
CREATE TABLE drug_catalog (
    -- identitas internal yang stabil, dipakai FK dari medications/templates.
    -- Kode penyedia (kfa_code, bpom_*) disimpan terpisah supaya sumber bisa
    -- diganti tanpa mengubah FK di seluruh skema.
    id                  UUID PRIMARY KEY DEFAULT uuidv7(),

    -- ---- kolom kanonik (untuk pencarian & tampilan cepat) ----
    display_name        TEXT NOT NULL,                       -- "Abacavir Sulfate 300 mg Tablet Salut Selaput"
    nama_dagang         TEXT NULL,
    generik_nama        TEXT NULL,                           -- nama generik/INN, mis. "abacavir"
    is_generik          BOOLEAN NULL,                        -- true = obat generik
    rxterm              TEXT NULL,                           -- istilah resep
    manufacturer        TEXT NULL,
    registrar           TEXT NULL,
    produksi_buatan     TEXT NULL CHECK (produksi_buatan IS NULL OR produksi_buatan IN ('local','imported','other')),
    country_of_origin   TEXT NULL,
    dosage_form_code    TEXT REFERENCES ref_dosage_form(code) ON DELETE RESTRICT,
    dosage_form_name    TEXT NULL,
    route_code          TEXT REFERENCES ref_route(code) ON DELETE RESTRICT,
    route_name          TEXT NULL,
    dose_value_text     TEXT NULL,                           -- "300 mg"
    ucum_code           TEXT NULL,                           -- "mg"
    uom_name            TEXT NULL,                           -- "Tablet"
    controlled_code     TEXT REFERENCES ref_controlled_drug(code) ON DELETE RESTRICT,
    controlled_name     TEXT NULL,
    atc_l1_code         TEXT NULL,
    atc_l1_name         TEXT NULL,
    atc_l2_code         TEXT NULL,
    atc_l2_name         TEXT NULL,
    atc_l3_code         TEXT NULL,
    atc_l3_name         TEXT NULL,
    atc_l4_code         TEXT NULL,
    atc_l4_name         TEXT NULL,
    atc_l5_code         TEXT NULL,                           -- ATC kode penuh
    atc_l5_name         TEXT NULL,
    atc_ddd             TEXT NULL,

    -- ---- label/peringatan (teks panjang, boleh HTML dari sumber) ----
    indication          TEXT NULL,
    warning             TEXT NULL,
    side_effect         TEXT NULL,
    description         TEXT NULL,
    dosage_usage        TEXT NULL,
    storage_note        TEXT NULL,                           -- cara penyimpanan
    is_discontinued     BOOLEAN NULL,

    -- ---- tanggal penting ----
    nie_start_date      DATE NULL,
    nie_end_date        DATE NULL,

    -- ---- metadata penyedia (semuanya boleh NULL) ----
    kfa_code            TEXT NULL,                           -- kode KFA
    bpom_nie            TEXT NULL,                           -- nomor registrasi BPOM
    barcode             TEXT NULL,
    kode_lkpp           TEXT NULL,
    kode_kbki           TEXT NULL,

    -- ---- tambahan spesifik (harga, kemasan, campuran) ----
    dose_per_unit       NUMERIC(12,3) NULL,
    pack_desc           TEXT NULL,
    pack_qty            NUMERIC(12,3) NULL,
    fix_price           NUMERIC(14,2) NULL,
    het_price           NUMERIC(14,2) NULL,
    is_fornas           BOOLEAN NULL,
    reference_url       TEXT NULL CHECK (reference_url IS NULL OR reference_url LIKE 'https://%'),

    -- ---- kolom longgar untuk data penyedia di masa depan ----
    active_ingredients  JSONB NOT NULL DEFAULT '[]',         -- [{zat_aktif, kekuatan}]
    packaging_options   JSONB NOT NULL DEFAULT '[]',         -- [{nama, qty, uom}]
    identifiers         JSONB NOT NULL DEFAULT '[]',         -- [{code, source_name, url}]
    tags                TEXT[] NOT NULL DEFAULT '{}',
    extra_meta         JSONB NOT NULL DEFAULT '{}',          -- field penyedia yang belum dipetakan

    active              BOOLEAN NOT NULL DEFAULT true,
    state               TEXT NULL,                           -- status dari sumber
    source              TEXT NOT NULL DEFAULT 'manual'
                        CHECK (source IN ('kfa','bpom','manual','other')),
    source_updated_at   TIMESTAMPTZ NULL,
    snapshot_date       DATE NOT NULL DEFAULT CURRENT_DATE,
    owner_id            UUID NULL,                           -- FK ditambah setelah tabel users dibuat
    is_preset           BOOLEAN NOT NULL DEFAULT false,
    curation_status     TEXT NOT NULL DEFAULT 'published'
                        CHECK (curation_status IN ('draft','published','archived','user_private')),
    curated_by          UUID NULL,                           -- admin yang publish
    curated_at          TIMESTAMPTZ NULL,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at          TIMESTAMPTZ NULL,

    -- Hanya admin/importer boleh membuat preset (owner_id kosong +
    -- curation_status published). User biasa hanya boleh 'user_private'
    -- dan WAJIB punya owner_id, sehingga tidak mungkin masuk preset publik.
    CONSTRAINT ck_catalog_scope CHECK (
        (is_preset AND owner_id IS NULL AND curation_status IN ('draft','published','archived'))
        OR (NOT is_preset AND owner_id IS NOT NULL AND curation_status = 'user_private')
    ),

    -- PG18 generated column untuk full-text search (STORED agar bisa di-index)
    search_tsv          tsvector GENERATED ALWAYS AS (
                            to_tsvector('simple',
                                coalesce(display_name,'')  || ' ' ||
                                coalesce(nama_dagang,'')   || ' ' ||
                                coalesce(generik_nama,'')  || ' ' ||
                                coalesce(rxterm,'')        || ' ' ||
                                coalesce(atc_l5_name,'')   || ' ' ||
                                coalesce(dosage_form_name,''))
                        ) STORED
);
CREATE UNIQUE INDEX uq_catalog_kfa     ON drug_catalog (kfa_code)     WHERE kfa_code IS NOT NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX uq_catalog_nie     ON drug_catalog (bpom_nie)     WHERE bpom_nie IS NOT NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX uq_catalog_barcode ON drug_catalog (barcode)      WHERE barcode IS NOT NULL AND deleted_at IS NULL;
-- Nama unik: preset satu namespace global; katalog pribadi unik per user.
CREATE UNIQUE INDEX uq_catalog_preset_name
    ON drug_catalog (lower(display_name), coalesce(dosage_form_name,''), coalesce(uom_name,''))
    WHERE deleted_at IS NULL AND owner_id IS NULL;
CREATE UNIQUE INDEX uq_catalog_owner_name
    ON drug_catalog (owner_id, lower(display_name), coalesce(dosage_form_name,''), coalesce(uom_name,''))
    WHERE deleted_at IS NULL AND owner_id IS NOT NULL;
CREATE INDEX ix_catalog_search ON drug_catalog USING GIN (search_tsv);
CREATE INDEX ix_catalog_atc    ON drug_catalog (atc_l5_code) WHERE active AND deleted_at IS NULL;
CREATE INDEX ix_catalog_form   ON drug_catalog (dosage_form_code) WHERE active AND deleted_at IS NULL;
CREATE INDEX ix_catalog_name   ON drug_catalog (lower(display_name)) WHERE deleted_at IS NULL;
CREATE INDEX ix_catalog_tags   ON drug_catalog USING GIN (tags);
-- Pencarian user: preset publik ATAU katalog pribadinya sendiri.
CREATE INDEX ix_catalog_scope  ON drug_catalog (owner_id) WHERE deleted_at IS NULL;
CREATE INDEX ix_catalog_preset ON drug_catalog (curation_status) WHERE deleted_at IS NULL AND owner_id IS NULL;
CREATE TRIGGER trg_catalog_updated BEFORE UPDATE ON drug_catalog
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
COMMENT ON TABLE  drug_catalog IS 'Katalog obat. Tahap 1: input manual user. Tahap 2: importer KFA/BPOM mengisi kolom sumber + extra_meta. Tidak bergantung pada satu penyedia.';
COMMENT ON COLUMN drug_catalog.id IS 'PK internal stabil, dipakai FK. Kode penyedia (kfa_code/bpom_nie) nullable dan terpisah.';
COMMENT ON COLUMN drug_catalog.extra_meta IS 'Payload mentah dari penyedia. Dipakai agar importer baru tidak butuh migrasi skema.';

-- +goose Down
DROP TRIGGER IF EXISTS trg_catalog_updated ON drug_catalog;
DROP INDEX IF EXISTS ix_catalog_preset;
DROP INDEX IF EXISTS ix_catalog_scope;
DROP INDEX IF EXISTS ix_catalog_tags;
DROP INDEX IF EXISTS ix_catalog_name;
DROP INDEX IF EXISTS ix_catalog_form;
DROP INDEX IF EXISTS ix_catalog_atc;
DROP INDEX IF EXISTS ix_catalog_search;
DROP INDEX IF EXISTS uq_catalog_owner_name;
DROP INDEX IF EXISTS uq_catalog_preset_name;
DROP INDEX IF EXISTS uq_catalog_barcode;
DROP INDEX IF EXISTS uq_catalog_nie;
DROP INDEX IF EXISTS uq_catalog_kfa;
DROP TABLE IF EXISTS drug_catalog CASCADE;
