-- +goose Up
-- -----------------------------------------------------------------------------
-- Seed tabel rujukan — cukup untuk input manual user, TANPA importer KFA.
--
-- Kenapa wajib ada: medications.dose_unit_code dan route_code adalah
-- NOT NULL REFERENCES ref_unit/ref_route. Selama tabel ini kosong, TIDAK ADA
-- satu obat pun yang bisa disimpan. Backend tidak bisa diuji sama sekali.
--
-- BAHASA: semua `name` berbahasa INGGRIS. Data referensi netral bahasa;
-- frontend yang menerjemahkan, memakai `code` sebagai kunci. Menyimpan label
-- Indonesia di DB akan mengikat satu-satunya sumber data referensi pada satu
-- bahasa. Rincian: data-model-v2.md bagian 1 butir 5.
--
-- Sumber kode: subset UCUM untuk satuan, dan konvensi rute KFA (O, IV, IM,
-- dst) supaya selaras saat importer resmi dipasang tanpa memetakan ulang.
--
-- Semua ON CONFLICT DO NOTHING supaya migrasi ini idempoten. Kalau importer
-- KFA menyusul, ia cukup menambah baris yang belum ada.
-- -----------------------------------------------------------------------------

-- -----------------------------------------------------------------------------
-- ref_unit — satuan dosis. kind: dose | count | volume | application
-- -----------------------------------------------------------------------------
INSERT INTO ref_unit (code, name, ucum_code, kind, sort_order) VALUES
    ('mg',      'Milligram',                'mg',       'dose',        10),
    ('mcg',     'Microgram',                'ug',       'dose',        20),
    ('g',       'Gram',                     'g',        'dose',        30),
    ('IU',      'International Unit',       '[IU]',     'dose',        40),
    ('mEq',     'Milliequivalent',          'meq',      'dose',        50),
    ('mL',      'Milliliter',               'mL',       'volume',      60),
    ('L',       'Liter',                    'L',        'volume',      70),
    ('drop',    'Drop',                     'drop',     'volume',      80),
    ('tsp',     'Teaspoon',                 '[tsp_us]', 'volume',      90),
    ('tbsp',    'Tablespoon',               '[tbsp_us]','volume',     100),
    ('tablet',  'Tablet',                   NULL,       'count',      110),
    ('caplet',  'Caplet',                   NULL,       'count',      120),
    ('capsule', 'Capsule',                  NULL,       'count',      130),
    ('supp',    'Suppository',              NULL,       'application',140),
    ('sachet',  'Sachet',                   NULL,       'count',      150),
    ('patch',   'Patch',                    NULL,       'application',160),
    ('ampoule', 'Ampoule',                  NULL,       'application',170),
    ('vial',    'Vial',                     NULL,       'application',180),
    ('tube',    'Tube',                     NULL,       'application',190),
    ('puff',    'Puff',                     NULL,       'application',200)
ON CONFLICT (code) DO NOTHING;

-- -----------------------------------------------------------------------------
-- ref_route — rute pemberian. Kode mengikuti KFA rute_pemberian.
-- -----------------------------------------------------------------------------
INSERT INTO ref_route (code, name, sort_order) VALUES
    ('O',      'Oral',                  10),
    ('SL',     'Sublingual',            20),
    ('B',      'Buccal',                30),
    ('IV',     'Intravenous',           40),
    ('IM',     'Intramuscular',         50),
    ('SC',     'Subcutaneous',          60),
    ('ID',     'Intradermal',           70),
    ('TD',     'Transdermal',           80),
    ('TOP',    'Topical',               90),
    ('OPH',    'Ophthalmic',           100),
    ('OT',     'Otic',                 110),
    ('N',      'Nasal',                120),
    ('INH',    'Inhalation',           130),
    ('PR',     'Rectal',               140),
    ('PV',     'Vaginal',              150),
    ('NGT',    'Nasogastric',          160),
    ('IRR',    'Irrigation',           170)
ON CONFLICT (code) DO NOTHING;

-- -----------------------------------------------------------------------------
-- ref_dosage_form — bentuk sediaan. Kode mengikuti KFA (BSxxx).
-- Yang belum punya kode KFA resmi memakai awalan LOCAL, supaya jelas mana
-- yang lokal dan bisa digantikan saat importer dipasang.
-- -----------------------------------------------------------------------------
INSERT INTO ref_dosage_form (code, name, group_name, sort_order) VALUES
    ('LOCAL-TAB',   'Tablet',                'solid',      10),
    ('LOCAL-TABS',  'Film-Coated Tablet',    'solid',      20),
    ('LOCAL-TABG',  'Sugar-Coated Tablet',   'solid',      30),
    ('LOCAL-TABE',  'Effervescent Tablet',   'solid',      40),
    ('LOCAL-TABT',  'Chewable Tablet',       'solid',      50),
    ('LOCAL-KAP',   'Capsule',               'solid',      60),
    ('LOCAL-KAPK',  'Hard Capsule',          'solid',      70),
    ('LOCAL-KAPL',  'Soft Capsule',          'solid',      80),
    ('LOCAL-SYR',   'Syrup',                 'liquid',     90),
    ('LOCAL-SUS',   'Suspension',            'liquid',    100),
    ('LOCAL-DROP',  'Drops',                 'liquid',    110),
    ('LOCAL-INJ',   'Injection',             'sterile',   120),
    ('LOCAL-INF',   'Infusion',              'sterile',   130),
    ('LOCAL-SAL',   'Ointment',              'semi-solid',140),
    ('LOCAL-CRM',   'Cream',                 'semi-solid',150),
    ('LOCAL-GEL',   'Gel',                   'semi-solid',160),
    ('LOCAL-SUP',   'Suppository',           'solid',     170),
    ('LOCAL-INH',   'Inhaler',               'sterile',   180),
    ('LOCAL-SPRY',  'Spray',                 'liquid',    190),
    ('LOCAL-PATCH', 'Transdermal Patch',     'semi-solid',200)
ON CONFLICT (code) DO NOTHING;

-- -----------------------------------------------------------------------------
-- ref_controlled_drug — golongan obat. Kode mengikuti KFA controlled_drug.
-- -----------------------------------------------------------------------------
INSERT INTO ref_controlled_drug (code, name, sort_order) VALUES
    ('0',   'Over-the-Counter',         10),
    ('1',   'Limited OTC',              20),
    ('2',   'Prescription Only',        30),
    ('3',   'Prescription (Rx)',        40),
    ('4',   'Psychotropic',             50),
    ('5',   'Narcotic',                 60),
    ('9',   'Unclassified',             70)
ON CONFLICT (code) DO NOTHING;

-- -----------------------------------------------------------------------------
-- Template sistem — mempercepat input pola lazim.
-- owner_id NULL + is_system true, ditegakkan CHECK ck_tpl_system_owner.
-- -----------------------------------------------------------------------------
INSERT INTO medication_templates
    (owner_id, name, route_code, meal_relation, is_system, default_frequency, notes_reminder)
VALUES
    (NULL, 'Once daily (morning)', 'O', 'after_meal', true,
     '{"pattern":"daily","times_per_day":1,"start_time":"07:00"}'::jsonb,
     'Take after breakfast'),
    (NULL, 'Twice daily', 'O', 'after_meal', true,
     '{"pattern":"daily","times_per_day":2,"start_time":"07:00"}'::jsonb,
     'Morning and evening, after meals'),
    (NULL, 'Three times daily', 'O', 'after_meal', true,
     '{"pattern":"daily","times_per_day":3,"start_time":"07:00"}'::jsonb,
     'Morning, noon, evening, after meals'),
    (NULL, 'Four times daily', 'O', 'after_meal', true,
     '{"pattern":"daily","times_per_day":4,"start_time":"06:00"}'::jsonb,
     'Every 6 hours'),
    (NULL, 'Every 8 hours', 'O', 'not_related', true,
     '{"pattern":"interval","interval_hours":8,"start_time":"06:00"}'::jsonb,
     'Must be exactly 8 hours apart'),
    (NULL, 'Every 12 hours', 'O', 'not_related', true,
     '{"pattern":"interval","interval_hours":12,"start_time":"07:00"}'::jsonb,
     'Morning and evening, 12 hours apart'),
    (NULL, 'Antibiotic 5-day course', 'O', 'after_meal', true,
     '{"pattern":"daily","times_per_day":3,"start_time":"07:00"}'::jsonb,
     'MUST BE FINISHED even if you feel better')
ON CONFLICT DO NOTHING;

-- +goose Down
-- -----------------------------------------------------------------------------
-- Hapus dalam urutan yang menghormati FK: template dulu (mereka merujuk
-- ref_route dan ref_dosage_form), baru tabel rujukannya.
--
-- Template TIDAK difilter berdasarkan nama. Alasannya nyata: kalau migrasi
-- Up pernah mengisi nama versi lama (atau bahasa lain), filter nama akan
-- meleset, template tertinggal, dan DELETE ref_route GAGAL karena RESTRICT.
--
-- PENJAGAAN DATA NYATA. Mengosongkan tabel rujukan hanya boleh dilakukan
-- kalau belum ada data pengguna yang merujuknya. Tanpa penjagaan ini,
-- rollback di server produksi akan mengubah kegagalan FK menjadi pesan
-- samar dari PostgreSQL; dengan penjagaan ini, operator mendapat pesan
-- yang menjelaskan kenapa dan apa yang harus dilakukan.
--
-- Kirim sebagai exception. goose membatalkan migrasi, dan skema tetap utuh
-- (lebih baik gagal jelas daripada merusak data nyata).
-- -----------------------------------------------------------------------------
-- +goose StatementBegin
DO $$
DECLARE
    n_medications BIGINT;
    n_templates   BIGINT;
BEGIN
    SELECT count(*) INTO n_medications FROM medications;
    SELECT count(*) INTO n_templates
      FROM medication_templates WHERE NOT is_system;

    IF n_medications > 0 OR n_templates > 0 THEN
        RAISE EXCEPTION
            'Tidak bisa rollback seed rujukan: masih ada % obat dan % template pribadi yang merujuknya. '
            'Kosongkan data itu lebih dulu. Ini memang disengaja: mengosongkan ref_* di sistem yang sudah dipakai akan merusak data nyata.',
            n_medications, n_templates
            USING ERRCODE = 'restrict_violation';
    END IF;
END $$;
-- +goose StatementEnd

DELETE FROM medication_templates WHERE is_system AND owner_id IS NULL;

DELETE FROM ref_unit;
DELETE FROM ref_route;
DELETE FROM ref_dosage_form;
DELETE FROM ref_controlled_drug;
