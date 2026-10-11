-- +goose Up
CREATE TABLE ref_unit (
    code        TEXT PRIMARY KEY,                 -- UCUM-ish: mg, g, mcg, IU, mL, drop...
    name        TEXT NOT NULL,
    ucum_code   TEXT NULL,
    kind        TEXT NOT NULL DEFAULT 'dose'
                CHECK (kind IN ('dose','count','volume','application')),
    sort_order  SMALLINT NOT NULL DEFAULT 0
);
COMMENT ON TABLE ref_unit IS 'Satuan dosis/konsumsi. Seed dari UCUM subset + KFA uom.';

CREATE TABLE ref_dosage_form (
    code        TEXT PRIMARY KEY,                 -- BS077, dll dari KFA
    name        TEXT NOT NULL,                    -- "Tablet Salut Selaput"
    group_name  TEXT NULL,                        -- padat / cair / semi-padat / steril
    sort_order  SMALLINT NOT NULL DEFAULT 0
);
COMMENT ON TABLE ref_dosage_form IS 'Bentuk sediaan. Seed dari KFA dosage_form + Kemenkes.';

CREATE TABLE ref_route (
    code        TEXT PRIMARY KEY,                 -- KFA rute_pemberian: O, ...
    name        TEXT NOT NULL,                    -- "Oral"
    sort_order  SMALLINT NOT NULL DEFAULT 0
);
COMMENT ON TABLE ref_route IS 'Rute pemberian. Seed dari KFA rute_pemberian.';

CREATE TABLE ref_controlled_drug (
    code        TEXT PRIMARY KEY,                 -- KFA controlled_drug: 3, ...
    name        TEXT NOT NULL,                    -- "Obat Keras"
    sort_order  SMALLINT NOT NULL DEFAULT 0
);
COMMENT ON TABLE ref_controlled_drug IS 'Golongan obat. Seed dari KFA controlled_drug.';

--    Tahap 1: cukup untuk input manual user (tanpa importer).
--    Tahap 2: importer eksternal (KFA/BPOM/dll) mengisi kolom sumber + meta.
--    Desainnya sengaja longgar: kolom sumber boleh NULL, jadi skema ini
--    tidak bergantung pada satu penyedia data.

-- +goose Down
DROP TABLE IF EXISTS ref_controlled_drug CASCADE;
DROP TABLE IF EXISTS ref_route CASCADE;
DROP TABLE IF EXISTS ref_dosage_form CASCADE;
DROP TABLE IF EXISTS ref_unit CASCADE;
