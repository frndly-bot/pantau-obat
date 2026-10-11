-- +goose Up
-- -----------------------------------------------------------------------------
-- Anti dobel-konsumsi stok.
--
-- MASALAH: satu catatan dosis (medication_logs) boleh menghasilkan paling
-- banyak satu pengurangan stok. Tanpa batasan ini, dua request POST log yang
-- sama-sama lolos (mis. retry jaringan, atau dua tab) bisa memotong saldo
-- dua kali untuk satu dosis. Karena balance_after diisi trigger, kerusakannya
-- tidak terlihat: saldo turun 2, padahal obat diminum 1.
--
-- Kenapa unique index, bukan menambah kolom: stock_transactions sudah punya
-- log_id yang menautkan transaksi ke catatan dosis. Kalau log_id sama muncul
-- dua kali, itu memang kejadian yang tidak boleh ada. ENFORCE di DB, jangan
-- hanya mengandalkan kehati-hatian aplikasi.
--
-- Sifat partial (WHERE log_id IS NOT NULL): transaksi yang memang tidak
-- berasal dari dosis — initial, restock, adjust, expire — tidak punya log_id,
-- jadi tidak terpengaruh.
-- -----------------------------------------------------------------------------
CREATE UNIQUE INDEX uq_stock_txn_log
    ON stock_transactions (log_id) WHERE log_id IS NOT NULL;

-- Untuk perbaikan stok: transaksi initial hanya boleh satu per baris stok,
-- supaya stok awal tidak bisa disuntik dua kali dan membuat saldo melompat.
CREATE UNIQUE INDEX uq_stock_txn_initial
    ON stock_transactions (stock_id) WHERE kind = 'initial';

-- +goose Down
DROP INDEX IF EXISTS uq_stock_txn_initial;
DROP INDEX IF EXISTS uq_stock_txn_log;
