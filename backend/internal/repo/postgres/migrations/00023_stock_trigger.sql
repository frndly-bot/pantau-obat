-- +goose Up
-- -----------------------------------------------------------------------------
-- Trigger stok: apply_stock_txn()
--
-- MASALAH: kolom stock_transactions.balance_after adalah NOT NULL, tapi tidak
-- ada yang mengisinya. data-model-v2.md bagian 7.2 sudah menjanjikan trigger
-- bernama apply_stock_txn() yang mengisi kolom itu sekaligus memutakhirkan
-- medication_stock.current_qty, tetapi migrasi 00013_stock.sql hanya membuat
-- tabel dan trigger set_updated_at. Akibatnya SETIAP insert transaksi stok
-- gagal dengan "null value in column balance_after violates not-null
-- constraint", dan obat dengan stok awal tidak bisa dibuat sama sekali.
--
-- Kenapa logikanya di trigger, bukan di aplikasi:
--
--   1. Saldo dan riwayat tidak bisa berbeda. Kalau aplikasi menghitung saldo,
--      dua request bersamaan bisa sama-sama membaca saldo 30, masing-masing
--      menulis -1, dan hasilnya 29 padahal seharusnya 28. Di dalam trigger,
--      UPDATE ... current_qty = current_qty + delta dijalankan di bawah kunci
--      baris yang sama dengan INSERT-nya, sehingga tidak ada jendela balapan.
--
--   2. Saldo negatif mustahil. Pemeriksaan dilakukan sebelum perubahan
--      diterapkan, di dalam transaksi yang sama. Aplikasi yang memeriksa dulu
--      lalu menulis tetap punya jendela antara periksa dan tulis.
--
--   3. balance_after selalu konsisten dengan current_qty, karena keduanya
--      dihitung dari nilai yang sama pada saat yang sama.
--
-- Kolom balance_after tetap NOT NULL, dan trigger BEFORE INSERT mengisinya
-- sebelum constraint diperiksa. Aplikasi TIDAK boleh mengisi kolom ini.
-- -----------------------------------------------------------------------------
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION apply_stock_txn() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    new_balance NUMERIC(12,3);
BEGIN
    -- Kunci baris stok supaya dua transaksi bersamaan untuk obat yang sama
    -- dijalankan berurutan, bukan saling menimpa. FOR UPDATE di sini yang
    -- membuat perhitungan saldo aman tanpa perlu locking di aplikasi.
    SELECT current_qty + NEW.qty_delta
      INTO new_balance
      FROM medication_stock
     WHERE id = NEW.stock_id
       FOR UPDATE;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'Baris stok % tidak ditemukan', NEW.stock_id
            USING ERRCODE = 'foreign_key_violation';
    END IF;

    -- Saldo negatif ditolak SEBELUM diterapkan, bukan sesudah. Menolak
    -- sesudah berarti sempat ada keadaan tidak sah yang terlihat oleh
    -- transaksi lain, walau akhirnya di-rollback.
    IF new_balance < 0 THEN
        RAISE EXCEPTION
            'Stok tidak cukup: saldo akan menjadi %, permintaan % %',
            new_balance, NEW.qty_delta, NEW.kind
            USING ERRCODE = 'check_violation';
    END IF;

    UPDATE medication_stock
       SET current_qty = new_balance
     WHERE id = NEW.stock_id;

    -- balance_after diisi dari nilai yang SAMA dengan yang baru saja
    -- disimpan, sehingga tidak mungkin berbeda.
    NEW.balance_after := new_balance;

    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER trg_stock_txn_apply
    BEFORE INSERT ON stock_transactions
    FOR EACH ROW EXECUTE FUNCTION apply_stock_txn();

COMMENT ON FUNCTION apply_stock_txn() IS
    'Mengisi balance_after dan memutakhirkan medication_stock.current_qty. Menolak saldo negatif.';

-- +goose Down
DROP TRIGGER IF EXISTS trg_stock_txn_apply ON stock_transactions;
DROP FUNCTION IF EXISTS apply_stock_txn();
