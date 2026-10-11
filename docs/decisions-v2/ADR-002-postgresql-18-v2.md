# ADR-002 v2: PostgreSQL 18

- Status: Accepted (revisi v2)
- Tanggal: 2026-10-10
- Menggantikan: `docs/decisions/ADR-002-postgresql-18.md` versi pertama
- Terkait: `ADR-001-stack-v2`, `backend/migrations/`

## Context

ADR-001 awalnya memilih PostgreSQL 16 dengan alasan "versi stabil". Kebutuhan
berkembang:

1. DB dinaikkan ke **PostgreSQL 18** atas permintaan langsung.
2. Model data diperluas: katalog obat Indonesia, lima pola frekuensi, satuan
   lengkap, stok dan restock, template, serta peran admin.
3. Perluasan itu menambah tekanan pada dua hal: jumlah PK ber-UUID pada tabel
   append-heavy, dan kebutuhan pencarian teks pada katalog.

## Decision

**PostgreSQL 18**, terverifikasi pada 18.6.

### 1. `uuidv7()` untuk PK entitas

PG18 menyediakan `uuidv7()` bawaan. Menggantikan `gen_random_uuid()` (v4).

- UUID v7 berurutan berdasarkan waktu, sehingga insert append-heavy (log,
  transaksi stok, dose events) tidak menyebar acak ke seluruh B-tree.
- Tetap 128-bit dan tidak membocorkan urutan seperti `BIGSERIAL`.
- Tidak butuh ekstensi tambahan.

Dikecualikan: `audit_log`, `notification_jobs`, `notification_deliveries` tetap
`BIGINT GENERATED ALWAYS AS IDENTITY` karena append-only dan hemat.

### 2. Generated column untuk FTS katalog

`drug_catalog.search_tsv` adalah `tsvector` generated dari nama tampilan, nama
dagang, nama generik, `rxterm`, nama ATC level 5, dan bentuk sediaan. Di-index GIN.

**Temuan saat implementasi:** PG18 menolak index pada `VIRTUAL` generated column:

```
ERROR: indexes on virtual generated columns are not supported
```

Karena kolom ini dipakai sebagai target pencarian, dipakai `STORED`, bukan `VIRTUAL`.

### 3. Tuning hemat untuk VPS kecil

Konfigurasi di `deploy/postgres/postgresql.conf`:

| Parameter | Nilai | Alasan |
|---|---|---|
| `shared_buffers` | 128MB | ~25% RAM VPS, sisanya untuk OS |
| `work_mem` | 4MB | hindari swap saat banyak sort |
| `max_connections` | 20 | app kecil, pool sedikit |
| `max_parallel_workers_per_gather` | 1 | VPS 1 vCPU |
| `wal_compression` | on | hemat I/O |
| `statement_timeout` | 30s | cegah query menggantung |
| `idle_in_transaction_session_timeout` | 5min | cegah slot pool habis |

Container dibatasi `mem_limit: 320m`, `cpus: "1.0"`, `shm_size: 128mb`.

## Consequences

### Positive

1. Insert lebih rapat secara fisik (index locality) untuk tabel append-heavy.
2. `uuidv7()` mengurangi alasan memakai `uuid-ossp` atau library aplikasi.
3. Pencarian katalog sepenuhnya di DB, tanpa layanan pencarian tambahan.
4. Versi 18 masih menerima perbaikan keamanan.

### Negative

1. **PG18 belum jadi default di semua tooling.** Image `postgres:18-alpine`
   dipakai eksplisit, dan `testcontainers-go` harus dipin ke tag 18.
2. **Client `psql` di dev bisa lebih lama** (mesin ini 16.9). Protokol query
   tetap kompatibel, tetapi meta-command bisa berbeda. Kebijakan: kirim SQL
   lewat stdin, hindari meta-command yang berubah antar versi.
3. **`uuidv7()` tidak ada di PG ≤17.** Rollback ke versi lama butuh ganti PK
   default, jadi keputusan ini menaikkan biaya rollback.
4. **`STORED` generated column memakai ruang disk** dan dihitung saat insert.
   Untuk katalog yang berukuran ribuan baris, ini tidak signifikan.

### Neutral

1. `citext` dan `pgcrypto` tetap dipakai.
2. Tidak ada perubahan pada model transport atau bahasa pesan.

## Alternatives Considered

| Alternatif | Kenapa Ditolak |
|---|---|
| **Tetap PostgreSQL 16** | Diminta naik ke 18. Fitur yang diuntungkan tidak ada di 16. |
| **PostgreSQL 17** | Tidak punya `uuidv7()`. Kalau naik, sekalian ke 18. |
| **UUID v4 tetap** | Write spread lebih buruk pada tabel log yang tumbuh cepat. |
| **BIGSERIAL untuk PK entitas** | Membocorkan urutan, memudahkan enumerasi antar tenant. |
| **`search_tsv` biasa + trigger manual** | Lebih banyak kode dan rawan drift dibanding generated column. |
| **`VIRTUAL` generated column** | Tidak bisa di-index di PG18. Sudah diuji dan gagal. |

## Verifikasi yang Sudah Dilakukan

Dijalankan terhadap PostgreSQL 18.6, 23 tabel + 2 view, apply tanpa error.

| Yang diuji | Hasil |
|---|---|
| `uuidv7()` tersedia | ada |
| Virtual generated column | jalan, tapi **tidak bisa di-index** |
| `pgcrypto`, `citext` | terpasang |
| `ck_catalog_scope` menolak preset buatan user | ditolak |
| `uq_log_occurrence` menolak dosis ganda | ditolak |
| `ck_freq_params` menolak pattern tanpa parameter | ditolak |
| `ck_log_taken_at` menolak `taken` tanpa `taken_at` | ditolak |
| `ck_care_not_self` menolak caregiver = owner | ditolak |
| Trigger `apply_stock_txn` mengisi `balance_after` | 30 → 29 → 49 otomatis |
| Konsumsi melebihi saldo | ditolak |
| Urutan FK `users` sebelum `drug_catalog` | sempat gagal, sudah diperbaiki |

## Yang Belum Terverifikasi

- Beban tuning PG18 di VPS 1 GB belum diuji dengan data nyata.
- Importer katalog belum ada, jadi `drug_catalog` masih kosong.
- Rollback ke PG16 belum diuji dan kemungkinan butuh migrasi manual.
