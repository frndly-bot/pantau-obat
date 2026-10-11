-- +goose Up
-- -----------------------------------------------------------------------------
-- Perbaikan view kepatuhan harian.
--
-- TIGA MASALAH pada v_adherence_daily versi sebelumnya (00019_views.sql):
--
-- 1. CTE `tz` DIDEFINISIKAN TAPI TIDAK DIPAKAI. Blok tujuh baris itu di-JOIN
--    setiap kali view dipanggil, lalu hasilnya dibuang. Beban sia-sia, dan
--    menyiratkan ada logika zona waktu yang sebenarnya tidak ada.
--
-- 2. DOSIS TERLEWAT TIDAK DIHITUNG SAMA SEKALI. View hanya membaca
--    medication_logs. Dosis yang terlewat TIDAK punya baris di sana; ia
--    ditulis di medication_dose_events dengan kind='missed'. Akibatnya
--    pasien yang melewatkan separuh dosisnya bisa terlihat patuh 100%,
--    karena yang terlewat memang tidak pernah muncul di penyebut. Ini cacat
--    yang membuat angka kepatuhan tidak bisa dipercaya.
--
-- 3. PENYEBUTNYA SALAH. `logged_doses` menghitung baris log yang ADA, bukan
--    dosis yang SEHARUSNYA ada. Rasio taken/logged selalu mendekati 100%
--    selama user rajin mencatat apa yang diminum.
--
-- Yang tidak bisa diselesaikan di sini: view ini tetap tidak bisa menghitung
-- penyebut yang benar, karena jumlah dosis terjadwal pada satu hari bergantung
-- pada ATURAN frekuensi (pola, siklus, zona waktu). Itu urusan kode Go, bukan
-- SQL, dan sudah diputuskan demikian di ADR-005.
--
-- Jadi view ini diperbaiki untuk dua hal yang MEMANG bisa dilakukan di SQL:
-- mencatat `missed` sebagai fakta, dan menghitung `scheduled_denominator`
-- sebagai jumlah dosis terjadwal yang DIKETAHUI — yaitu log yang tercatat
-- ditambah penyimpangan yang tercatat. Penyebut yang sungguh lengkap dihitung
-- service/adherence.go, yang memakai mesin aturan domain.
-- -----------------------------------------------------------------------------
DROP VIEW IF EXISTS v_adherence_daily;

CREATE VIEW v_adherence_daily AS
SELECT
    d.owner_id,
    d.medication_id,
    d.slot_date,

    -- Dosis yang punya catatan, apa pun hasilnya.
    d.logged_doses,

    -- Dosis yang benar-benar diminum, termasuk yang telat.
    d.taken_doses,

    -- Dosis yang sengaja dilewati user.
    d.skipped_doses,

    -- Dosis yang lewat jam tanpa catatan. Dibaca dari dose_events, bukan
    -- dari logs, karena di sanalah sistem menuliskannya.
    d.missed_doses,

    -- Jumlah dosis yang keadaannya DIKETAHUI hari itu. Bukan jumlah dosis
    -- yang seharusnya ada — itu butuh mesin aturan. Yang bisa dijamin di sini
    -- adalah angka ini tidak pernah lebih kecil dari kenyataan.
    d.known_doses,

    -- Rasio kepatuhan terhadap dosis yang diketahui.
    -- NULL bila belum ada apa pun yang diketahui, supaya "belum ada data"
    -- tidak dilaporkan sebagai 0% (yang berarti "semua terlewat").
    CASE
        WHEN d.known_doses > 0
        THEN round((d.taken_doses::numeric / d.known_doses), 4)
        ELSE NULL
    END AS adherence_ratio
FROM (
    SELECT
        l.owner_id,
        l.medication_id,
        l.slot_date,
        count(*)::int AS logged_doses,
        count(*) FILTER (WHERE l.status IN ('taken','late'))::int AS taken_doses,
        count(*) FILTER (WHERE l.status = 'skipped')::int AS skipped_doses,
        0::int AS missed_doses,
        count(*)::int AS known_doses
    FROM medication_logs l
    WHERE l.deleted_at IS NULL AND l.schedule_id IS NOT NULL
    GROUP BY l.owner_id, l.medication_id, l.slot_date

    UNION ALL

    -- Penyimpangan yang terlewat, digabung sebagai baris sendiri lalu
    -- dijumlahkan di atas. LEFT JOIN akan lebih rumit karena satu tanggal
    -- bisa punya log dan penyimpangan sekaligus.
    SELECT
        e.owner_id,
        e.medication_id,
        e.event_date,
        0, 0, 0,
        count(*) FILTER (WHERE e.kind = 'missed')::int,
        count(*) FILTER (WHERE e.kind = 'missed')::int
    FROM medication_dose_events e
    WHERE e.kind = 'missed' AND e.schedule_id IS NOT NULL
    GROUP BY e.owner_id, e.medication_id, e.event_date
) AS d
GROUP BY d.owner_id, d.medication_id, d.slot_date,
         d.logged_doses, d.taken_doses, d.skipped_doses,
         d.missed_doses, d.known_doses;

COMMENT ON VIEW v_adherence_daily IS
    'Rekap harian: log, terlewat, dan rasio terhadap dosis yang diketahui. Penyebut lengkap dihitung di service/adherence.go memakai mesin aturan, karena bergantung pada pola frekuensi.';

-- +goose Down
DROP VIEW IF EXISTS v_adherence_daily;

-- Kembalikan ke definisi semula, apa adanya, termasuk cacatnya. Migrasi Down
-- harus mengembalikan keadaan sebelumnya, bukan keadaan yang "lebih baik".
CREATE VIEW v_adherence_daily AS
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
