-- +goose Up
CREATE OR REPLACE VIEW v_medication_stock_status AS
SELECT
    s.id                AS stock_id,
    s.owner_id,
    s.medication_id,
    m.name              AS medication_name,
    s.unit_code,
    s.current_qty,
    s.qty_per_dose,
    s.refill_threshold_qty,
    s.refill_reminder,
    CASE
        WHEN s.qty_per_dose > 0 THEN floor(s.current_qty / s.qty_per_dose)
        ELSE NULL
    END                 AS doses_remaining,
    CASE
        WHEN s.refill_threshold_qty IS NOT NULL
             AND s.current_qty <= s.refill_threshold_qty THEN true
        ELSE false
    END                 AS needs_refill
FROM medication_stock s
JOIN medications m ON m.id = s.medication_id
WHERE s.deleted_at IS NULL AND m.deleted_at IS NULL;

COMMENT ON VIEW v_medication_stock_status IS 'Sisa dosis per obat + flag butuh restock.';

-- -----------------------------------------------------------------------------
--     Dihitung on-the-fly untuk rentang pendek (7/30 hari). Karena jadwal
--     disimpan sebagai aturan, view ini yang menggantikan "status baris slot".
-- -----------------------------------------------------------------------------
CREATE OR REPLACE VIEW v_adherence_daily AS
WITH tz AS (
    SELECT t.id AS schedule_time_id, t.medication_id, t.owner_id,
           t.time_local, t.weekdays,
           coalesce(f.schedule_timezone, u.timezone) AS tz_name
    FROM medication_times t
    JOIN medications m       ON m.id = t.medication_id
    JOIN users u             ON u.id = t.owner_id
    LEFT JOIN medication_frequency f ON f.medication_id = t.medication_id
    WHERE t.deleted_at IS NULL AND t.is_active AND m.deleted_at IS NULL AND m.is_active
)
SELECT
    l.owner_id,
    l.medication_id,
    l.slot_date,
    count(*)                                          AS logged_doses,
    count(*) FILTER (WHERE l.status IN ('taken','late')) AS taken_doses,
    count(*) FILTER (WHERE l.status = 'skipped')       AS skipped_doses
FROM medication_logs l
WHERE l.deleted_at IS NULL AND l.schedule_id IS NOT NULL
GROUP BY l.owner_id, l.medication_id, l.slot_date;

COMMENT ON VIEW v_adherence_daily IS 'Rekap harian dari log terjadwal. Penyimpangan (missed) dibaca dari medication_dose_events.';

-- +goose Down
DROP VIEW IF EXISTS v_adherence_daily CASCADE;
DROP VIEW IF EXISTS v_medication_stock_status CASCADE;
