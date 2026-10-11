# Database — Pantau Obat

Skema: **PostgreSQL 18**, 23 tabel + 2 view.
Migrasi dikelola **goose**, file ada di `backend/migrations/`.

## Cara pakai

Dari root repo (semua perintah lewat Makefile):

```bash
make db-up           # nyalakan PostgreSQL 18 (container)
make db-migrate      # jalankan migrasi yang belum naik
make db-status       # lihat mana yang sudah dan belum jalan
make db-shell        # psql interaktif
make db-rollback N=1 # turunkan 1 migrasi
make db-new NAME=tambah_kolom_x
make db-hard-reset   # HATI-HATI: rollback penuh lalu naik lagi
```

Kalau lebih suka langsung:

```bash
export GOOSE_DRIVER=postgres
export GOOSE_DBSTRING="postgres://pantau_obat:PASSWORD@127.0.0.1:5432/pantau_obat?sslmode=disable"
goose -dir backend/migrations status
goose -dir backend/migrations up
```

`GOOSE_DBSTRING` di Makefile dibaca dari `deploy/.env`, jadi tidak perlu
diisi manual kalau `.env` sudah ada.

## Daftar migrasi

| No | File | Isi |
|---|---|---|
| 1 | `00001_extensions.sql` | `pgcrypto`, `citext` |
| 2 | `00002_helpers.sql` | fungsi `set_updated_at()` |
| 3 | `00003_ref_tables.sql` | `ref_unit`, `ref_dosage_form`, `ref_route`, `ref_controlled_drug` |
| 4 | `00004_drug_catalog.sql` | katalog obat + 12 index + trigger + FTS |
| 5 | `00005_templates.sql` | `medication_templates` |
| 6 | `00006_users.sql` | `users` + self-FK admin + CHECK scope |
| 7 | `00007_medications.sql` | `medications` |
| 8 | `00008_frequency.sql` | `medication_frequency` (5 pola) |
| 9 | `00009_times.sql` | `medication_times` + `schedule_timezone` |
| 10 | `00010_logs.sql` | `medication_logs` + `uq_log_occurrence` |
| 11 | `00011_dose_events.sql` | `medication_dose_events` (append-only) |
| 12 | `00012_bump_queue.sql` | `bump_queue` |
| 13 | `00013_stock.sql` | stok + transaksi + trigger `apply_stock_txn` |
| 14 | `00014_care_links.sql` | relasi caregiver |
| 15 | `00015_notifications.sql` | prefs, device tokens, integrasi |
| 16 | `00016_jobs.sql` | job dan delivery notifikasi |
| 17 | `00017_sessions.sql` | `refresh_sessions` |
| 18 | `00018_audit.sql` | `audit_log` + index admin |
| 19 | `00019_views.sql` | `v_medication_stock_status`, `v_adherence_daily` |

**Urutan 6 sebelum 4 bukan salah tulis.** Tabel `users` dibuat di file 6,
sementara `drug_catalog` di file 4. FK `drug_catalog.owner_id → users(id)`
karena itu ditambahkan lewat `ALTER TABLE` di file 6. Ini pernah jadi bug
saat skema masih satu file: kalau urutannya terbalik, seluruh skema gagal.

## Aturan menulis migrasi

1. **Satu perubahan satu file.** Jangan gabung dua perubahan.
2. **Selalu tulis `Down`.** `make db-rollback` harus benar-benar bisa
   mengembalikan keadaan. Uji dengan `make db-hard-reset`.
3. **Jangan edit file yang sudah naik ke main.** Kalau perlu ubah, buat
   migrasi baru.
4. **Fungsi plpgsql wajib pakai `StatementBegin`/`StatementEnd`:**

```sql
-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION contoh() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
```

   Ini **bukan opsional**. goose memecah pernyataan berdasarkan `;` dan tidak
   paham dolar-quote PostgreSQL. Tanpa penanda itu, fungsi terpotong di tengah
   dan migrasi gagal dengan `unterminated dollar-quoted string`. Masalah ini
   sudah pernah terjadi di proyek ini.

5. **Perubahan enum = migrasi CHECK baru**, bukan edit manual. Pola:

```sql
-- +goose Up
ALTER TABLE medications DROP CONSTRAINT medications_meal_relation_check;
ALTER TABLE medications ADD CONSTRAINT medications_meal_relation_check
    CHECK (meal_relation IN ('before_meal','after_meal','with_meal','not_related','lain'));

-- +goose Down
ALTER TABLE medications DROP CONSTRAINT medications_meal_relation_check;
ALTER TABLE medications ADD CONSTRAINT medications_meal_relation_check
    CHECK (meal_relation IN ('before_meal','after_meal','with_meal','not_related'));
```

6. **Prefix nomor urut, bukan tanggal.** goose memakai nomor di awal nama file
   sebagai versi. File dari `make db-new` memakai timestamp dan tidak bentrok
   dengan nomor urut; keduanya jalan, tapi jangan dicampur tanpa alasan.

## Skema awal

`db/schema.sql` sudah dihapus karena isinya sudah sepenuhnya ada di berkas migrasi
migrasi. Kalau perlu referensi lengkap dalam satu file, dokumen
`docs/design-v2/data-model-v2.md` memuat seluruh tabel beserta alasan tiap
index.

## Membuat migrasi dari nol

Untuk lingkungan baru:

```bash
make db-up
make db-migrate
make verify
```

Hasil yang diharapkan: 23 tabel + 2 view, versi goose = 19.
