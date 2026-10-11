# Data Model v2 — Pantau Obat

Status: v2, selaras dengan migrasi goose (teruji di PostgreSQL 18.6)
Tanggal: 2026-10-10
DB: PostgreSQL 18 · 23 tabel · 2 view

> Dokumen ini menggantikan `docs/design/data-model.md`.
> Sumber kebenaran: `backend/migrations/`. Kalau ada perbedaan, migrasi yang benar.

---

## 1. Keputusan Desain

1. **PK entitas pakai `uuidv7()`.** Monotonik berdasarkan waktu, sehingga insert
   append-heavy (log, transaksi stok) tidak menyebar acak ke seluruh B-tree.
   Tetap 128-bit dan tidak membocorkan urutan antar tenant.
2. **`audit_log`, `notification_jobs`, `notification_deliveries` pakai `BIGINT GENERATED ALWAYS AS IDENTITY`.** Append-only, hemat, tidak perlu UUID.
3. **Soft delete via `deleted_at TIMESTAMPTZ`.** Riwayat kepatuhan tidak boleh
   hilang saat obat dihapus.
4. **Enum pakai `TEXT + CHECK`, bukan tipe ENUM Postgres.** Menambah nilai tidak
   perlu `ALTER TYPE ... USING`, dan rollback aman.
5. **Semua enum berbahasa Inggris.** `before_meal`, `evening`, `child`,
   `imported`. Tidak ada nilai enum berbahasa Indonesia di mana pun.

   **`ref_*.name` juga bahasa Inggris sebagai default.** Semula `name`
   direncanakan Indonesia karena dianggap label tampilan. Itu keliru: UI
   yang menerjemahkan, bukan database. Menyimpan label Indonesia di DB
   membuat satu-satunya sumber data referensi terikat pada satu bahasa,
   padahal non-tujuan v0.1 ("tidak ada i18n") soal antarmuka, bukan soal
   penyimpanan. Data referensi harus netral bahasa.

   Terjemahan Indonesia disimpan di frontend, dengan kode `code` sebagai
   kunci. Jadi `ref_route` menyimpan `O` / `Oral`, dan frontend yang
   menampilkan "Mulut" bila perlu.
6. **Jadwal disimpan sebagai aturan, bukan baris per hari.** Tidak ada tabel
   slot. Waktu dosis dihitung dari aturan. Ini yang membuat terapi
   "selamanya" tidak membengkakkan database.
7. **Katalog dipisah menjadi preset publik dan katalog pribadi user** dalam satu
   tabel, dibedakan `owner_id` dan `is_preset`.
8. **Semua FK `ON DELETE RESTRICT`.** Mencegah hapus berantai tidak sengaja.
9. **Semua waktu absolut `TIMESTAMPTZ`.** Tanggal lokal (`slot_date`) dihitung di
   backend dari `users.timezone`, bukan zona DB.

## 2. Diagram Relasi

Diagram visual lengkap: `docs/design-v2/erd.html`.

```
users 1───* medications 1───1 medication_frequency
  │             │        1───* medication_times
  │             │        1───* medication_logs
  │             │        1───1 medication_stock 1───* stock_transactions
  │             │        1───* medication_dose_events
  │             │        1───* bump_queue
  │             └───* medication_templates
  │
  ├──1───* notification_prefs
  ├──1───* device_tokens
  ├──1───* user_integrations
  ├──1───* refresh_sessions
  ├──1───* drug_catalog (katalog pribadi; NULL = preset publik)
  │
  │   owner_id        caregiver_id
  └───* care_links *───┘
         (viewer | helper)

audit_log *───1 users (actor_id, NULL untuk sistem)
  actor_role: user | admin | system
```

Kardinalitas penting:

- `users → medications` 1:N via `medications.owner_id`.
- `medications → medication_frequency` **1:1** (UNIQUE `medication_id`).
- `medications → medication_times` 1:N.
- `medications → medication_stock` **1:1** (UNIQUE `medication_id`).
- `medication_stock → stock_transactions` 1:N.
- `medications → medication_logs` 1:N. `schedule_id` NULL untuk dosis PRN.
- `medications → medication_dose_events` 1:N (append-only).
- `users ↔ users` lewat `care_links`, dua arah (`owner_id` dan `caregiver_id`).

## 3. Tabel Rujukan

Di-seed dari KFA Kemenkes, UCUM, dan daftar bentuk sediaan Kemenkes.

### 3.1 `ref_unit` — satuan dosis

| Kolom | Tipe | Constraint | Keterangan |
|---|---|---|---|
| `code` | TEXT | PK | kode UCUM-ish: `mg`, `tablet`, `IU` |
| `name` | TEXT | NOT NULL | label tampilan, mis. "Tablet" |
| `ucum_code` | TEXT | NULL | kode UCUM bila berbeda |
| `kind` | TEXT | CHECK `dose`/`count`/`volume`/`application` | kelompok satuan |
| `sort_order` | SMALLINT | NOT NULL DEFAULT 0 | urutan di dropdown |

Nilai awal: `mg`, `g`, `mcg`, `IU`, `mL`, `tetes`, `puff`, `spray`, `sachet`,
`tablet`, `kapsul`, `kaplet`, `suppositoria`, `unit`, `spuit`, `ampul`, `vial`,
`patch`, `oles`, `lain`.

### 3.2 `ref_dosage_form` — bentuk sediaan

| Kolom | Tipe | Constraint | Keterangan |
|---|---|---|---|
| `code` | TEXT | PK | kode KFA, mis. `BS077` |
| `name` | TEXT | NOT NULL | "Tablet Salut Selaput" |
| `group_name` | TEXT | NULL | `solid`/`liquid`/`semi_solid`/`sterile` |
| `sort_order` | SMALLINT | NOT NULL DEFAULT 0 | urutan |

### 3.3 `ref_route` — rute pemberian

| Kolom | Tipe | Constraint | Keterangan |
|---|---|---|---|
| `code` | TEXT | PK | kode KFA: `O` (Oral), `T` (Topikal), `S` (Injeksi) |
| `name` | TEXT | NOT NULL | label tampilan |
| `sort_order` | SMALLINT | NOT NULL DEFAULT 0 | urutan |

### 3.4 `ref_controlled_drug` — golongan obat

| Kolom | Tipe | Constraint | Keterangan |
|---|---|---|---|
| `code` | TEXT | PK | kode KFA: `3` (Obat Keras) |
| `name` | TEXT | NOT NULL | label tampilan |
| `sort_order` | SMALLINT | NOT NULL DEFAULT 0 | urutan |

## 4. Katalog Obat

### 4.1 `drug_catalog`

Satu tabel untuk dua peran, dibedakan lewat `owner_id` + `is_preset`.

- **Preset publik:** `owner_id` NULL, `is_preset` true, `curation_status`
  `draft`/`published`/`archived`. Diisi admin atau importer.
- **Katalog pribadi user:** `owner_id` terisi, `is_preset` false,
  `curation_status` `user_private`. Dibuat user saat menambah obat manual.
  Tidak pernah muncul untuk user lain.

Ditegakkan oleh CHECK `ck_catalog_scope`:

```
(is_preset AND owner_id IS NULL AND curation_status IN ('draft','published','archived'))
OR (NOT is_preset AND owner_id IS NOT NULL AND curation_status = 'user_private')
```

| Kolom | Tipe | Constraint | Keterangan |
|---|---|---|---|
| `id` | UUID | PK, `uuidv7()` | PK internal stabil, dipakai FK |
| `display_name` | TEXT | NOT NULL | nama lengkap |
| `nama_dagang` | TEXT | NULL | nama dagang |
| `generik_nama` | TEXT | NULL | INN |
| `is_generik` | BOOLEAN | NULL | penanda generik |
| `rxterm` | TEXT | NULL | istilah resep |
| `manufacturer`, `registrar` | TEXT | NULL | pabrik dan pendaftar |
| `produksi_buatan` | TEXT | CHECK `local`/`imported`/`other` | asal produksi |
| `country_of_origin` | TEXT | NULL | negara asal |
| `dosage_form_code` | TEXT | FK `ref_dosage_form` | bentuk sediaan |
| `dosage_form_name` | TEXT | NULL | salinan nama untuk tampilan cepat |
| `route_code` | TEXT | FK `ref_route` | rute |
| `route_name` | TEXT | NULL | salinan |
| `dose_value_text` | TEXT | NULL | "300 mg" |
| `ucum_code` | TEXT | NULL | satuan UCUM |
| `uom_name` | TEXT | NULL | unit kemasan |
| `controlled_code` | TEXT | FK `ref_controlled_drug` | golongan |
| `controlled_name` | TEXT | NULL | salinan |
| `atc_l1_code` … `atc_l5_name` | TEXT | NULL | klasifikasi ATC 5 level |
| `atc_ddd` | TEXT | NULL | dosis harian terdefinisi |
| `indication`, `warning`, `side_effect` | TEXT | NULL | boleh HTML dari sumber |
| `description`, `dosage_usage`, `storage_note` | TEXT | NULL | keterangan tambahan |
| `is_discontinued` | BOOLEAN | NULL | dihentikan? |
| `nie_start_date`, `nie_end_date` | DATE | NULL | masa berlaku izin edar |
| `kfa_code` | TEXT | NULL | kode KFA |
| `bpom_nie` | TEXT | NULL | nomor registrasi BPOM |
| `barcode` | TEXT | NULL | barcode |
| `kode_lkpp`, `kode_kbki` | TEXT | NULL | kode lain |
| `dose_per_unit` | NUMERIC(12,3) | NULL | dosis per unit |
| `pack_desc` | TEXT | NULL | "Dus isi 60" |
| `pack_qty` | NUMERIC(12,3) | NULL | jumlah per kemasan |
| `fix_price`, `het_price` | NUMERIC(14,2) | NULL | harga |
| `is_fornas` | BOOLEAN | NULL | masuk Fornas? |
| `reference_url` | TEXT | CHECK `https://%` | sumber |
| `active_ingredients` | JSONB | NOT NULL `[]` | `[{zat_aktif, kekuatan}]` |
| `packaging_options` | JSONB | NOT NULL `[]` | `[{nama, qty, uom}]` |
| `identifiers` | JSONB | NOT NULL `[]` | `[{code, source_name, url}]` |
| `tags` | TEXT[] | NOT NULL `{}` | penanda bebas |
| `extra_meta` | JSONB | NOT NULL `{}` | payload mentah penyedia |
| `active` | BOOLEAN | NOT NULL true | aktif? |
| `state` | TEXT | NULL | status dari sumber |
| `owner_id` | UUID | FK `users`, NULL = preset | pemilik katalog pribadi |
| `is_preset` | BOOLEAN | NOT NULL false | preset publik? |
| `curation_status` | TEXT | CHECK 4 nilai | siklus kurasi |
| `curated_by` | UUID | NULL | admin yang publish |
| `curated_at` | TIMESTAMPTZ | NULL | kapan publish |
| `source` | TEXT | CHECK `kfa`/`bpom`/`manual`/`other` | asal data |
| `source_updated_at` | TIMESTAMPTZ | NULL | kapan sumber diperbarui |
| `snapshot_date` | DATE | NOT NULL CURRENT_DATE | tanggal ambil |
| `created_at`, `updated_at`, `deleted_at` | TIMESTAMPTZ | — | — |
| `search_tsv` | tsvector | **GENERATED STORED** | FTS dari nama, dagang, generik, rxterm, ATC, bentuk |

Alasan `extra_meta` ada: kalau nanti sumber data punya field yang belum
dipetakan, importer menyimpannya di sana tanpa perlu migrasi skema.

Index:

| Nama | Definisi | Alasan |
|---|---|---|
| `uq_catalog_kfa` | UNIQUE `kfa_code` WHERE NOT NULL AND NOT deleted | importer idempoten |
| `uq_catalog_nie` | UNIQUE `bpom_nie` WHERE NOT NULL AND NOT deleted | cegah dobel dari BPOM |
| `uq_catalog_barcode` | UNIQUE `barcode` WHERE NOT NULL AND NOT deleted | lookup scan |
| `uq_catalog_preset_name` | UNIQUE nama+bentuk+uom WHERE `owner_id` NULL | nama preset unik global |
| `uq_catalog_owner_name` | UNIQUE owner+nama+bentuk+uom WHERE owner NOT NULL | nama pribadi unik per user |
| `ix_catalog_search` | GIN `search_tsv` | pencarian teks |
| `ix_catalog_atc` | `atc_l5_code` WHERE active | filter klasifikasi |
| `ix_catalog_form` | `dosage_form_code` WHERE active | filter bentuk |
| `ix_catalog_name` | `lower(display_name)` | lookup nama |
| `ix_catalog_tags` | GIN `tags` | filter tag |
| `ix_catalog_scope` | `owner_id` | pisah preset vs pribadi |
| `ix_catalog_preset` | `curation_status` WHERE `owner_id` NULL | daftar preset untuk admin |

Catatan implementasi: PG18 **menolak index pada `VIRTUAL` generated column**
(`ERROR: indexes on virtual generated columns are not supported`). Karena
`search_tsv` di-index, ia harus `STORED`.

### 4.2 `medication_templates`

| Kolom | Tipe | Constraint | Keterangan |
|---|---|---|---|
| `id` | UUID | PK `uuidv7()` | — |
| `owner_id` | UUID | FK `users`, NULL = template sistem | pemilik |
| `name` | TEXT | NOT NULL, 1–120 | nama template |
| `catalog_id` | UUID | FK `drug_catalog` | kaitan katalog |
| `dosage_form_code` | TEXT | FK `ref_dosage_form` | bentuk |
| `dose_value` | NUMERIC(12,3) | NULL | dosis lazim |
| `dose_unit_code` | TEXT | FK `ref_unit` | satuan |
| `route_code` | TEXT | FK `ref_route` | rute |
| `meal_relation` | TEXT | CHECK 4 nilai | waktu makan |
| `as_needed` | BOOLEAN | NOT NULL false | PRN |
| `must_finish` | BOOLEAN | NOT NULL false | harus habis |
| `default_frequency` | JSONB | NOT NULL `{}` | pola awal |
| `default_stock_qty` | NUMERIC(12,3) | NULL | stok awal |
| `default_refill_threshold` | NUMERIC(12,3) | NULL | ambang restock |
| `notes_reminder` | TEXT | ≤1000 | catatan pengingat |
| `is_system` | BOOLEAN | NOT NULL false | template sistem? |
| `usage_count` | INTEGER | NOT NULL 0 | berapa kali dipakai |
| `created_at`, `updated_at`, `deleted_at` | TIMESTAMPTZ | — | — |

CHECK `ck_tpl_system_owner`: `(is_system AND owner_id IS NULL) OR (NOT is_system AND owner_id IS NOT NULL)`.

Index: `uq_tpl_owner_name` (uniq per user), `uq_tpl_system_name` (uniq global
untuk sistem).

## 5. Pengguna

### 5.1 `users`

| Kolom | Tipe | Constraint | Keterangan |
|---|---|---|---|
| `id` | UUID | PK `uuidv7()` | — |
| `email` | CITEXT | NOT NULL UNIQUE | case-insensitive |
| `phone` | TEXT | CHECK 6–20 | opsional |
| `password_hash` | TEXT | NOT NULL | Argon2id PHC |
| `full_name` | TEXT | NOT NULL, 1–100 | nama tampilan |
| `timezone` | TEXT | NOT NULL DEFAULT `Asia/Jakarta` | basis jadwal lokal |
| `is_active` | BOOLEAN | NOT NULL true | false = diblokir |
| `role` | TEXT | CHECK `user`/`admin`, DEFAULT `user` | peran |
| `admin_scope` | TEXT[] | CHECK subset `{users,catalog,notifications,reports}` | NULL = penuh |
| `admin_granted_at` | TIMESTAMPTZ | NULL | kapan diberi |
| `admin_granted_by` | UUID | FK `users` (self) | siapa memberi |
| `last_login_at` | TIMESTAMPTZ | NULL | login terakhir |
| `created_at`, `updated_at`, `deleted_at` | TIMESTAMPTZ | — | — |

Aturan peran:

- `role` **terpisah dari** `is_active`. Blokir akun tidak mengubah peran.
- `role` tidak pernah diterima dari klien. Hanya ditetapkan lewat DB/seed.
- `ck_admin_scope`: `admin_scope IS NULL OR admin_scope <@ ARRAY['users','catalog','notifications','reports']`.
- `ck_admin_scope_only_admin`: `role = 'admin' OR admin_scope IS NULL`.

Index: `ix_users_inactive`, `ix_users_admins` (partial WHERE `role='admin'`).

### 5.2 `care_links`

| Kolom | Tipe | Constraint | Keterangan |
|---|---|---|---|
| `id` | UUID | PK `uuidv7()` | — |
| `owner_id` | UUID | NOT NULL FK `users` | pasien |
| `caregiver_id` | UUID | **NULL** FK `users` | NULL saat undangan belum diterima |
| `invite_email` | CITEXT | NULL | email tujuan |
| `relation` | TEXT | CHECK `family`/`child`/`parent`/`spouse`/`friend`/`nurse`/`other` | hubungan |
| `role` | TEXT | CHECK `viewer`/`helper`, DEFAULT `viewer` | tingkat izin |
| `status` | TEXT | CHECK `pending`/`accepted`/`rejected`/`revoked`/`expired` | siklus |
| `invited_by` | UUID | NOT NULL FK `users` | pengundang |
| `invite_token_hash` | TEXT | NULL | SHA-256, satu pakai |
| `invite_expires_at` | TIMESTAMPTZ | NULL | default +7 hari |
| `decided_at` | TIMESTAMPTZ | NULL | kapan accept/reject |
| `created_at`, `updated_at`, `deleted_at` | TIMESTAMPTZ | — | — |

CHECK `ck_care_not_self`: `caregiver_id IS NULL OR owner_id <> caregiver_id`.

Index unik: `uq_care_active_pair`, `uq_care_pending_invite`, `uq_care_token`.

### 5.3 `notification_prefs`

Satu baris per user per kanal. `channel` CHECK `push`/`email`/`telegram`/`in_app`.
Kolom: `enabled`, `quiet_start`, `quiet_end`, `advance_minutes` (0–120),
`notif_throttled`. UNIQUE `(user_id, channel)`.

### 5.4 `device_tokens`

Token push per perangkat. `platform` CHECK `android`/`ios`/`web`. `token` UNIQUE.
`revoked_at` diisi saat logout atau token basi.

### 5.5 `user_integrations`

Integrasi eksternal (Telegram, WA). `provider` CHECK `telegram`/`wa`,
`external_id` (chat id), UNIQUE `(provider, external_id)`.

### 5.6 `refresh_sessions`

Refresh token opaque. `token_hash` UNIQUE (SHA-256). `expires_at` +30 hari.
`rotated_from` FK self untuk deteksi pencurian token.

## 6. Obat dan Jadwal

### 6.1 `medications`

| Kolom | Tipe | Constraint | Keterangan |
|---|---|---|---|
| `id` | UUID | PK `uuidv7()` | — |
| `owner_id` | UUID | NOT NULL FK `users` | tenant |
| `catalog_id` | UUID | NULL FK `drug_catalog` | kaitan katalog |
| `name` | TEXT | NOT NULL, 1–120 | nama obat |
| `dose_value` | NUMERIC(12,3) | NOT NULL CHECK > 0 | jumlah per intake |
| `dose_unit_code` | TEXT | NOT NULL FK `ref_unit` | satuan |
| `strength_note` | TEXT | NULL | "120mg/5mL", "100IU/mL" |
| `route_code` | TEXT | NOT NULL FK `ref_route` | rute |
| `dosage_form_code` | TEXT | NULL FK `ref_dosage_form` | bentuk |
| `meal_relation` | TEXT | NOT NULL DEFAULT `not_related`, CHECK 4 nilai | sebelum/sesudah/saat/tidak |
| `as_needed` | BOOLEAN | NOT NULL false | bila diperlukan |
| `must_finish` | BOOLEAN | NOT NULL false | harus dihabiskan |
| `start_date` | DATE | NOT NULL | mulai terapi |
| `start_time` | TIME | NOT NULL DEFAULT `00:00` | jam mulai |
| `end_date` | DATE | NULL, CHECK ≥ `start_date` | NULL = selamanya |
| `notes` | TEXT | ≤2000 | catatan user |
| `notes_reminder` | TEXT | ≤1000 | catatan pengingat |
| `prescription_photo_url` | TEXT | CHECK `https://%` | URL object storage |
| `color_tag` | TEXT | CHECK `^#[0-9a-fA-F]{6}$` | label visual |
| `is_active` | BOOLEAN | NOT NULL true | dihentikan tanpa hapus |
| `created_at`, `updated_at`, `deleted_at` | TIMESTAMPTZ | — | — |

Index unik `uq_meds_owner_name`: `(owner_id, lower(name), dose_value, dose_unit_code)` WHERE `deleted_at IS NULL`.

Index: `ix_meds_owner`, `ix_meds_owner_active`, `ix_meds_catalog`.

### 6.2 `medication_frequency` — pola minum (1:1 dengan obat)

| Kolom | Tipe | Constraint | Keterangan |
|---|---|---|---|
| `id` | UUID | PK `uuidv7()` | — |
| `medication_id` | UUID | NOT NULL **UNIQUE** FK | 1:1 |
| `owner_id` | UUID | NOT NULL FK `users` | tenant |
| `pattern` | TEXT | CHECK `daily`/`interval`/`weekly`/`monthly_cycle`/`prn` | pola |
| `times_per_day` | SMALLINT | 1–24 | untuk `daily` |
| `interval_hours` | SMALLINT | 1–168 | untuk `interval` |
| `days_of_week` | SMALLINT | 1–127 | bitmask untuk `weekly` |
| `interval_min_hours` | SMALLINT | 1–168 | rentang (informasi) |
| `cycle_intake_days` | SMALLINT | 1–365 | hari minum |
| `cycle_pause_days` | SMALLINT | 0–365 | hari jeda |
| `cycle_anchor` | TEXT | CHECK `dom`/`end_of_month`/`from_start` | titik mulai siklus |
| `cycle_anchor_dom` | SMALLINT | 1–31 | tanggal anchor |
| `prn_max_per_day` | SMALLINT | 1–24 | batas PRN |
| `tolerance_minutes` | SMALLINT | 0–120 | toleransi |
| `start_time` | TIME | DEFAULT `08:00` | jam mulai |
| `schedule_timezone` | TEXT | NOT NULL DEFAULT `Asia/Jakarta` | zona blok jadwal |
| `created_at`, `updated_at`, `deleted_at` | TIMESTAMPTZ | — | — |

CHECK `ck_freq_params` memaksa parameter yang sesuai ada:

```
(pattern = 'daily'         AND times_per_day IS NOT NULL) OR
(pattern = 'interval'      AND interval_hours IS NOT NULL) OR
(pattern = 'weekly'        AND days_of_week IS NOT NULL) OR
(pattern = 'monthly_cycle' AND cycle_intake_days IS NOT NULL
                           AND cycle_pause_days IS NOT NULL
                           AND cycle_anchor IS NOT NULL) OR
(pattern = 'prn')
```

Bitmask `days_of_week`: Senin=1, Selasa=2, Rabu=4, Kamis=8, Jumat=16,
Sabtu=32, Minggu=64. `127` = setiap hari.

### 6.3 `medication_times` — jam konkret

| Kolom | Tipe | Constraint | Keterangan |
|---|---|---|---|
| `id` | UUID | PK `uuidv7()` | — |
| `medication_id` | UUID | NOT NULL FK `medications` | induk |
| `frequency_id` | UUID | NULL FK `medication_frequency` | kaitan pola |
| `owner_id` | UUID | NOT NULL FK `users` | tenant |
| `time_local` | TIME | NOT NULL | jam lokal |
| `label` | TEXT | CHECK `morning`/`afternoon`/`evening`/`dawn`/`other` | label |
| `weekdays` | SMALLINT | 1–127 DEFAULT 127 | override hari |
| `dose_override` | TEXT | NULL | dosis khusus jam ini |
| `sort_order` | SMALLINT | NOT NULL 0 | urutan tampil |
| `is_active` | BOOLEAN | NOT NULL true | — |
| `created_at`, `updated_at`, `deleted_at` | TIMESTAMPTZ | — | — |

UNIQUE `uq_times_med_time` pada `(medication_id, time_local, weekdays)` WHERE hidup.

### 6.4 Kenapa tidak ada tabel slot

Versi v0.1 memakai tabel `medication_slots` yang di-generate 14 hari ke depan.
Itu dibuang karena:

| Masalah | Dampak |
|---|---|
| Terapi "selamanya" | baris tumbuh tanpa batas, sebanding jumlah hari |
| Skala SaaS | biaya penyimpanan tumbuh tanpa kaitan dengan pemakaian nyata |
| Sinkronisasi | worker wajib rajin mengisi; kalau mati, pengingat berhenti |
| Perubahan jadwal | harus menghapus/menulis ulang slot masa depan |

Yang dipakai sekarang: **aturan + occurrence key**.

- Waktu dosis dihitung dari `medication_frequency` + `medication_times`.
- Identitas dosis = `(medication_id, schedule_id, slot_date, scheduled_hour, scheduled_minute)`.
- Keunikan ditegakkan `uq_log_occurrence` di `medication_logs`.
- Yang benar-benar perlu baris (`snooze`, job notifikasi) masuk `bump_queue`.

Hasil: satu obat 2× sehari selama 10 tahun tetap **3 baris** (1 pola + 2 jam).

### 6.5 `medication_logs` — catatan kepatuhan

| Kolom | Tipe | Constraint | Keterangan |
|---|---|---|---|
| `id` | UUID | PK `uuidv7()` | — |
| `medication_id` | UUID | NOT NULL FK | obat |
| `schedule_id` | UUID | NULL FK `medication_times` | NULL = dosis PRN |
| `owner_id` | UUID | NOT NULL FK `users` | pemilik obat |
| `recorded_by` | UUID | NOT NULL FK `users` | siapa mencatat |
| `slot_date` | DATE | NOT NULL | tanggal lokal pemilik |
| `scheduled_at` | TIMESTAMPTZ | NULL | waktu jadwal absolut |
| `scheduled_hour` | SMALLINT | 0–23 | **occurrence key** |
| `scheduled_minute` | SMALLINT | 0–59 | **occurrence key** |
| `scheduled_tz` | TEXT | NULL | zona saat dosis jatuh |
| `status` | TEXT | CHECK `taken`/`late`/`skipped`/`snoozed` | status |
| `taken_at` | TIMESTAMPTZ | NULL | waktu aktual diminum |
| `note` | TEXT | ≤500 | alasan |
| `source` | TEXT | CHECK `self`/`caregiver`/`system` | asal |
| `created_at`, `updated_at`, `deleted_at` | TIMESTAMPTZ | — | — |

CHECK `ck_log_occurrence`: `schedule_id` NULL harus berarti jam juga NULL, dan
sebaliknya. Ini mencegah baris setengah jadi.

CHECK `ck_log_taken_at`: status `taken`/`late` wajib punya `taken_at`.

Index unik `uq_log_occurrence`:

```sql
CREATE UNIQUE INDEX uq_log_occurrence
    ON medication_logs (medication_id, schedule_id, slot_date, scheduled_hour, scheduled_minute)
    WHERE deleted_at IS NULL AND schedule_id IS NOT NULL;
```

Ini pengganti `uq_log_slot` versi lama. `schedule_id` NULL (PRN) dikecualikan
sehingga boleh banyak per hari.

### 6.6 `medication_dose_events` — penyimpangan (append-only)

Dipakai untuk kejadian yang **bukan** pencatatan user. Tujuannya: jumlah baris
tetap sebanding jumlah kejadian nyata.

| Kolom | Tipe | Constraint | Keterangan |
|---|---|---|---|
| `event_key` | TEXT | **PK** | `schedule_id\|YYYY-MM-DD\|HH:MM` |
| `owner_id` | UUID | NOT NULL FK `users` | tenant |
| `medication_id` | UUID | NOT NULL FK `medications` | obat |
| `schedule_id` | UUID | NULL FK `medication_times` | jam |
| `event_date` | DATE | NOT NULL | tanggal |
| `scheduled_hour` | SMALLINT | 0–23 | jam |
| `scheduled_minute` | SMALLINT | 0–59 | menit |
| `scheduled_tz` | TEXT | NOT NULL | zona |
| `kind` | TEXT | CHECK `missed`/`dose_shift`/`paused` | jenis penyimpangan |
| `reason` | TEXT | NULL | alasan |
| `created_at` | TIMESTAMPTZ | NOT NULL now() | — |

Arti `kind`:

- `missed` — dosis lewat jam tanpa log.
- `dose_shift` — pola berubah (mis. dari tiap 8 jam jadi 3× sehari).
- `paused` — terapi dijeda, atau tanggal tidak cocok slot PRN.

`event_key` sebagai PK membuat penulisan idempoten: menjalankan pass dua kali
tidak menggandakan baris.

### 6.7 `bump_queue` — pekerjaan yang benar-benar perlu baris

Hanya untuk hal yang punya identitas dan jadwal sendiri.

| Kolom | Tipe | Constraint | Keterangan |
|---|---|---|---|
| `id` | BIGINT | IDENTITY PK | antrean |
| `kind` | TEXT | CHECK `dose`/`refill`/`digest` | jenis |
| `owner_id` | UUID | NOT NULL FK `users` | tenant |
| `medication_id` | UUID | NULL FK | obat |
| `schedule_id` | UUID | NULL FK `medication_times` | jam |
| `scheduled_at` | TIMESTAMPTZ | NOT NULL | kapan dorong |
| `slot_date` | DATE | NOT NULL | tanggal lokal |
| `dedup_key` | TEXT | NOT NULL | kunci anti-ganda |
| `attempts` | SMALLINT | NOT NULL 0 | percobaan |
| `claimed_at` | TIMESTAMPTZ | NULL | sedang diklaim |
| `done_at` | TIMESTAMPTZ | NULL | selesai |
| `skip_tag` | TEXT | NULL | `snooze` atau `quiet` |
| `created_at` | TIMESTAMPTZ | NOT NULL now() | — |

UNIQUE `uq_bump_dedup` pada `dedup_key` WHERE `done_at IS NULL`. Index
`ix_bump_due` (partial WHERE `done_at IS NULL`) untuk pengambilan cepat.

Bedanya dengan `notification_jobs`: `bump_queue` menentukan **kapan** sesuatu
harus terjadi (termasuk snooze yang bisa berubah), `notification_jobs`
mencatat **pengiriman** dan hasil per kanal.

## 7. Stok

### 7.1 `medication_stock` (1:1 dengan obat)

| Kolom | Tipe | Constraint | Keterangan |
|---|---|---|---|
| `id` | UUID | PK `uuidv7()` | — |
| `medication_id` | UUID | NOT NULL **UNIQUE** FK | 1:1 |
| `owner_id` | UUID | NOT NULL FK `users` | tenant |
| `unit_code` | TEXT | NOT NULL FK `ref_unit` | satuan stok |
| `initial_qty` | NUMERIC(12,3) | NOT NULL CHECK ≥ 0 | stok awal |
| `current_qty` | NUMERIC(12,3) | NOT NULL CHECK ≥ 0 | saldo berjalan |
| `qty_per_dose` | NUMERIC(12,3) | NOT NULL CHECK > 0 | pemakaian per dosis |
| `refill_threshold_qty` | NUMERIC(12,3) | NULL CHECK ≥ 0 | ambang peringatan |
| `refill_reminder` | BOOLEAN | NOT NULL false | nyalakan peringatan |
| `last_restocked_at` | TIMESTAMPTZ | NULL | restock terakhir |
| `created_at`, `updated_at`, `deleted_at` | TIMESTAMPTZ | — | — |

### 7.2 `stock_transactions` (append-only)

| Kolom | Tipe | Constraint | Keterangan |
|---|---|---|---|
| `id` | UUID | PK `uuidv7()` | — |
| `stock_id` | UUID | NOT NULL FK | stok |
| `medication_id` | UUID | NOT NULL FK | obat |
| `owner_id` | UUID | NOT NULL FK `users` | tenant |
| `kind` | TEXT | CHECK `initial`/`restock`/`consume`/`adjust`/`expire` | jenis |
| `qty_delta` | NUMERIC(12,3) | NOT NULL | positif masuk, negatif keluar |
| `balance_after` | NUMERIC(12,3) | CHECK ≥ 0, **diisi trigger** | saldo setelah |
| `log_id` | UUID | NULL FK `medication_logs` | kaitan ke dosis |
| `note` | TEXT | ≤500 | alasan |
| `recorded_by` | UUID | NULL FK `users` | pelaku |
| `created_at` | TIMESTAMPTZ | NOT NULL now() | — |

Trigger `apply_stock_txn()` (BEFORE INSERT) menjalankan tiga hal:

1. `UPDATE medication_stock SET current_qty = current_qty + NEW.qty_delta`.
2. Kalau saldo hasilnya negatif, `RAISE EXCEPTION`. Jadi saldo tidak bisa ditekan ke bawah nol.
3. Mengisi `NEW.balance_after` dengan saldo setelah perubahan.

Konsekuensi: aplikasi **tidak boleh** mengisi `balance_after` sendiri, dan
`current_qty` hanya berubah lewat transaksi.

**Diperbaiki di `00020_stock_txn_guard.sql`.** Satu dosis tidak boleh
mengurangi saldo dua kali. Ditegakkan di database, bukan hanya di aplikasi:

```sql
CREATE UNIQUE INDEX uq_stock_txn_log
    ON stock_transactions (log_id) WHERE log_id IS NOT NULL;

-- stok awal tidak bisa disuntik dua kali
CREATE UNIQUE INDEX uq_stock_txn_initial
    ON stock_transactions (stock_id) WHERE kind = 'initial';
```

Sifat partial: transaksi tanpa `log_id` (`initial`, `restock`, `adjust`,
`expire`) tidak terpengaruh. Sudah diuji: percobaan kedua menulis `consume`
untuk `log_id` yang sama ditolak dengan `uq_stock_txn_log`.

### 7.3 `v_medication_stock_status` — view

Menghitung `doses_remaining` (`floor(current_qty / qty_per_dose)`) dan
`needs_refill` (`current_qty <= refill_threshold_qty`).

## 8. Notifikasi

### 8.1 `notification_jobs`

| Kolom | Tipe | Constraint | Keterangan |
|---|---|---|---|
| `id` | BIGINT | IDENTITY PK | antrean |
| `kind` | TEXT | CHECK `dose_reminder`/`missed_escalation`/`care_digest`/`retention`/`refill_reminder` | jenis |
| `status` | TEXT | CHECK `pending`/`claimed`/`done`/`failed`/`cancelled` | status |
| `dedup_key` | TEXT | NOT NULL UNIQUE | anti spam |
| `owner_id` | UUID | NOT NULL FK `users` | tenant |
| `payload` | JSONB | NOT NULL `{}` | data pesan |
| `run_at` | TIMESTAMPTZ | NOT NULL | kapan dijalankan |
| `attempts` | SMALLINT | NOT NULL 0 | percobaan |
| `max_attempts` | SMALLINT | NOT NULL 5 | batas |
| `last_error` | TEXT | NULL | error terakhir |
| `claimed_at` | TIMESTAMPTZ | NULL | diklaim |
| `created_at`, `updated_at` | TIMESTAMPTZ | — | — |

Versi v0.1 memakai tipe ENUM Postgres di sini. Sekarang `TEXT + CHECK` agar
konsisten dengan sisa skema.

Index `ix_jobs_claimable` (partial WHERE `status='pending'`) dipakai bersama
`SELECT ... FOR UPDATE SKIP LOCKED`.

### 8.2 `notification_deliveries`

Satu baris per job per kanal. UNIQUE `(job_id, channel)`. Kolom: `channel`,
`status` CHECK `pending`/`sent`/`failed`/`skipped`, `provider_id`,
`error_code`, `error_msg`, `attempts`, `sent_at`.

## 9. Audit

### 9.1 `audit_log` (append-only)

| Kolom | Tipe | Constraint | Keterangan |
|---|---|---|---|
| `id` | BIGINT | IDENTITY PK | urutan |
| `actor_id` | UUID | NULL FK `users` | NULL = sistem |
| `owner_id` | UUID | NULL FK `users` | pemilik data |
| `actor_role` | TEXT | CHECK `user`/`admin`/`system`, DEFAULT `user` | peran saat aksi |
| `action` | TEXT | CHECK 30+ nilai | aksi |
| `resource_type` | TEXT | CHECK 11 nilai | tipe objek |
| `resource_id` | UUID | NULL | tanpa FK |
| `ip` | INET | NULL | alamat |
| `user_agent` | TEXT | NULL | agen |
| `before_state` | JSONB | NULL | nilai sebelum |
| `after_state` | JSONB | NULL | nilai sesudah |
| `meta` | JSONB | NOT NULL `{}` | detail tambahan |
| `created_at` | TIMESTAMPTZ | NOT NULL now() | — |

Aksi user: `create`, `read`, `update`, `delete`, `login`, `logout`, `invite`,
`accept`, `revoke`, `export`, `log_taken`, `log_skipped`, `restock`,
`catalog_search`.

Aksi admin: `admin_grant`, `admin_revoke`, `admin_scope_change`, `user_disable`,
`user_enable`, `user_erase`, `catalog_create`, `catalog_update`,
`catalog_publish`, `catalog_unpublish`, `catalog_archive`, `catalog_import`,
`catalog_merge`, `notif_retry`, `notif_cancel`, `retention_purge`.

`resource_type`: `medication`, `schedule`, `log`, `care_link`, `user`, `auth`,
`stock`, `catalog`, `admin`, `notification`, `retention`.

Kenapa `actor_role` disalin, bukan di-join: kalau peran user berubah di
kemudian hari, jejak lama harus tetap menunjukkan peran **saat aksi terjadi**.
Tanpa ini, riwayat admin yang sudah diturunkan jadi menyesatkan.

Index: `ix_audit_owner`, `ix_audit_actor`, `ix_audit_resource`,
`ix_audit_meta_gin` (GIN), `ix_audit_admin` (partial WHERE `actor_role='admin'`),
`ix_audit_action`.

Tidak ada `updated_at` dan `deleted_at`. Aplikasi tidak boleh UPDATE/DELETE.

## 10. Aturan Konsistensi

1. **Multi-tenant:** semua query `medications`/`medication_times`/`medication_logs`
   wajib filter `owner_id = auth.user_id` ATAU ada `care_links` accepted yang
   cocok. Dicek di service, bukan hanya handler.
2. **Soft delete berantai logis:** hapus obat → set `deleted_at` pada obat dan
   semua `medication_times` dalam satu transaksi. Log dipertahankan.
3. **Unik obat per user:** `(owner_id, lower(name), dose_value, dose_unit_code)`
   hanya untuk baris hidup. Obat yang sudah dihapus boleh didaftarkan ulang.
4. **Anti dosis ganda:** `uq_log_occurrence`. Koreksi = soft-delete baris lama +
   insert baris baru dalam satu transaksi.
5. **Caregiver:** `viewer` hanya GET. `helper` boleh POST/PATCH log dengan
   `recorded_by = caregiver_id` dan `source='caregiver'`. Tidak boleh kelola obat.
6. **Zona waktu:** simpan absolut `TIMESTAMPTZ`. `slot_date` dihitung backend
   dari `users.timezone`, bukan `CURRENT_DATE` DB (zona DB = UTC).
7. **Stok:** hanya berubah lewat `stock_transactions`. `balance_after` diisi
   trigger. Saldo tidak boleh negatif.
8. **Katalog:** preset publik tidak boleh diubah user. Katalog pribadi tidak
   boleh terlihat user lain.
9. **Admin:** tidak boleh membaca isi obat pasien lewat rute admin. Setiap
   tulisan menulis audit dengan `actor_role='admin'`.

## 11. Retensi Data

| Data | Retensi | Aksi |
|---|---|---|
| `medication_logs` hidup | 2 tahun sejak `slot_date` | arsip, lalu soft-delete |
| `medication_dose_events` | 2 tahun | arsip bersama log |
| `audit_log` | 1 tahun | hapus batch mingguan |
| `refresh_sessions` kedaluwarsa | 30 hari | DELETE fisik harian |
| `device_tokens` revoked | 90 hari | DELETE fisik |
| `care_links` revoked/expired | 1 tahun | DELETE fisik |
| `bump_queue` selesai | 7 hari | DELETE fisik |
| `notification_jobs` done/failed | 30 hari | DELETE fisik |
| foto resep yatim | 90 hari | hapus objek storage |

## 12. Migrasi

Strategi: **goose**. Satu perubahan satu file, selalu ada `-- +goose Down`.

Urutan yang mencerminkan `backend/migrations/`: 

| No | File | Isi |
|---|---|---|
| 1 | `00001_extensions.sql` | `pgcrypto`, `citext` |
| 2 | `00002_helpers.sql` | fungsi `set_updated_at()` |
| 3 | `00003_ref_tables.sql` | `ref_unit`, `ref_dosage_form`, `ref_route`, `ref_controlled_drug` |
| 4 | `00004_users.sql` | `users` + self-FK `admin_granted_by` |
| 5 | `00005_drug_catalog.sql` | `drug_catalog` + FK `owner_id` + index + trigger |
| 6 | `00006_templates.sql` | `medication_templates` |
| 7 | `00007_medications.sql` | `medications` |
| 8 | `00008_frequency.sql` | `medication_frequency` |
| 9 | `00009_times.sql` | `medication_times` |
| 10 | `00010_logs.sql` | `medication_logs` + `uq_log_occurrence` |
| 11 | `00011_dose_events.sql` | `medication_dose_events` |
| 12 | `00012_bump_queue.sql` | `bump_queue` |
| 13 | `00013_stock.sql` | `medication_stock`, `stock_transactions`, trigger stok |
| 14 | `00014_care_links.sql` | `care_links` |
| 15 | `00015_notifications.sql` | `notification_prefs`, `device_tokens`, `user_integrations` |
| 16 | `00016_jobs.sql` | `notification_jobs`, `notification_deliveries` |
| 17 | `00017_sessions.sql` | `refresh_sessions` |
| 18 | `00018_audit.sql` | `audit_log` |
| 19 | `00019_views.sql` | `v_medication_stock_status`, `v_adherence_daily` |
| 20 | `00020_stock_txn_guard.sql` | unique `stock_transactions.log_id` + `initial` per stok |
| 21 | `00021_seed_ref.sql` | seed tabel rujukan — **belum dibuat, lihat catatan** |

Urutan 4 sebelum 5 penting: `drug_catalog.owner_id` FK ke `users`, jadi
tabel `users` harus dibuat lebih dulu. Ini pernah jadi bug saat implementasi —
skema gagal total karena urutan ini terbalik.

Perubahan enum = migrasi CHECK baru, bukan edit manual file yang sudah naik.
