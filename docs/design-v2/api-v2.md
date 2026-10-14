# API v2 — Pantau Obat

Status: v2, selaras dengan migrasi goose
Tanggal: 2026-10-10
Backend: Go 1.27 · REST · JSON · prefix `/api/v1`

> Menggantikan `docs/design/api-draft.md`.

## 1. Konvensi

1. Base URL `/api/v1`. Versi di path, bukan header. Breaking change = `/api/v2`.
2. Body JSON `application/json; charset=utf-8`. Kolom `snake_case`.
3. Semua waktu ISO-8601 UTC (`2026-10-10T10:29:00Z`). Semua tanggal `YYYY-MM-DD`
   lokal per `users.timezone`.
4. Response selalu JSON, termasuk error.

### Envelope

Bentuk tunggal dipakai di semua endpoint. Ini menggantikan bentuk ganda di
dokumen lama (`{ok, data, meta}` vs `{data, page}`).

Sukses:

```json
{"data": {...}}
```

Sukses dengan list:

```json
{"data": [...], "page": {"next_cursor": "MjAyNi…", "has_more": false, "limit": 20}}
```

Gagal:

```json
{"error": {"code": "VALIDATION_FAILED", "message": "dose_value wajib diisi",
           "details": [{"field": "dose_value", "issue": "required"}]}}
```

`code` = kode mesin stabil (SCREAMING_SNAKE). `message` = aman untuk ditampilkan
ke user. `details` = array opsional.

### Kode error

| code | HTTP | Kapan |
|---|---|---|
| `VALIDATION_FAILED` | 400 | input salah / field hilang |
| `INVALID_JSON` | 400 | body bukan JSON |
| `UNAUTHENTICATED` | 401 | token hilang / kedaluwarsa |
| `INVALID_CREDENTIALS` | 401 | email/password salah |
| `TOKEN_REVOKED` | 401 | refresh token sudah dipakai / dicabut |
| `FORBIDDEN` | 403 | login valid tapi tidak berhak |
| `ADMIN_SCOPE_DENIED` | 403 | admin melampaui `admin_scope` |
| `CARE_NOT_ACCEPTED` | 403 | `care_link` belum accepted / di-revoke |
| `NOT_FOUND` | 404 | objek tidak ada / sudah soft-delete |
| `DUPLICATE_MEDICATION` | 409 | obat sama untuk user yang sama |
| `LOG_OCCURRENCE_TAKEN` | 409 | dosis jam itu sudah punya log hari itu |
| `DUPLICATE_TIMES` | 409 | jam dan pola hari itu sudah terdaftar |
| `INVITE_PENDING` | 409 | undangan untuk email sama masih menganggur |
| `STOCK_INSUFFICIENT` | 409 | pengurangan stok melebihi saldo |
| `CATALOG_IMMUTABLE` | 409 | mencoba mengubah preset publik |
| `RATE_LIMITED` | 429 | kuota terlampaui |
| `SNOOZE_LIMIT` | 429 | dosis ini sudah ditunda 3×; kini dianggap terlewat |
| `NOT_AVAILABLE` | 503 | fitur opsional belum dikonfigurasi operator (mis. penyimpanan foto) |
| `INTERNAL` | 500 | bug server |

`SNOOZE_LIMIT` sengaja dipisah dari `RATE_LIMITED`: yang satu soal kuota
permintaan, yang lain soal aturan dosis. Klien menampilkan pesan berbeda
untuk keduanya — pengguna yang membaca "terlalu banyak permintaan" akan
mencoba lagi nanti, padahal mencoba lagi tidak akan berhasil sampai besok.

### Aturan lain

5. Pagination cursor-based: `?limit=20&cursor=<opaque>`. Cursor =
   base64(`created_at|id`) yang ditandatangani. Tidak ada offset.
6. Filter tanggal: `from=YYYY-MM-DD&to=YYYY-MM-DD` (inklusif), dikonversi
   backend ke rentang UTC memakai timezone pemilik data.
7. Idempotensi: POST mutasi menerima `Idempotency-Key: <uuid>`. Server menyimpan
   hasil 24 jam.
8. Field `null` tetap dikirim bila kolomnya ada di skema.
9. Setiap response membawa `X-Request-Id` dan `X-Audit-Ref`.
10. **Enum di API memakai nilai bahasa Inggris** sama persis dengan CHECK di DB
    (`after_meal`, `morning`, `child`, `imported`). Frontend yang menerjemahkan
    ke Bahasa Indonesia.

## 2. Autentikasi

- Password hash Argon2id (`m=64MiB, t=3, p=4`), format PHC.
- Access token: JWT, TTL 15 menit, claim `sub`, `sid`, `exp`.
- Refresh token: opaque 32 byte, hanya SHA-256 disimpan di `refresh_sessions`.
  TTL 30 hari, rotasi tiap pakai (`rotated_from`). Reuse = tandai seluruh rantai
  `revoked_at` dan balas `TOKEN_REVOKED`.
- Cookie refresh: `po_refresh=<token>; HttpOnly; Secure; SameSite=Strict; Path=/api/v1/auth`.
- `role` **tidak pernah** diterima dari klien. Peran admin hanya lewat DB/seed.

| Method | Path | Auth | Keterangan |
|---|---|---|---|
| POST | `/auth/register` | — | daftar user baru; peran selalu `user` |
| POST | `/auth/login` | — | tukar kredensial jadi sesi |
| POST | `/auth/refresh` | cookie | perbarui access token, rotasi refresh |
| POST | `/auth/logout` | cookie | cabut sesi ini, atau semua dengan `all_devices=true` |
| GET | `/auth/me` | bearer | profil + `role` + `admin_scope` |
| POST | `/auth/change-password` | bearer | cabut seluruh sesi lain |
| POST | `/auth/forgot-password` | — | kirim tautan pemulihan; **selalu** balas sukses |
| POST | `/auth/reset-password` | — | tukar token jadi sandi baru; cabut semua sesi |

**Lupa sandi.** `POST /auth/forgot-password` menerima `{"email"}` dan
SELALU membalas `200 {"sent": true}`, termasuk untuk alamat yang tidak
terdaftar. Ini disengaja: bila endpoint membedakan keduanya, siapa pun bisa
memakainya untuk memeriksa apakah seseorang punya akun di sini — dan pada
aplikasi kesehatan, keanggotaan itu sendiri sudah informasi pribadi.
Batasnya: satu permintaan per alamat per menit (dijawab `429
RATE_LIMITED`), dan token berumur 15 menit serta sekali pakai.

`POST /auth/reset-password` menerima `{"token", "password"}`. Berbeda dari
endpoint di atas, endpoint ini MEMBERI TAHU bila tokennya tidak sah — di
sini pengguna sudah memegang tokennya, jadi tidak ada informasi baru yang
bocor, dan menyembunyikannya hanya membuat pengguna menunggu surat yang
tidak akan datang. Sukses berarti seluruh sesi lain dicabut.

```json
// POST /api/v1/auth/register
{"email":"budi@example.com","password":"Str0ngPass!","full_name":"Budi Santoso",
 "timezone":"Asia/Jakarta","phone":"+62812345678"}

// 201
{"data":{"id":"019...","email":"budi@example.com","full_name":"Budi Santoso",
         "timezone":"Asia/Jakarta","role":"user","created_at":"2026-10-10T10:29:00Z"}}

// POST /api/v1/auth/login -> 200
{"data":{"access_token":"<jwt>","token_type":"Bearer","expires_in":900,
         "user":{"id":"019...","full_name":"Budi Santoso","timezone":"Asia/Jakarta",
                 "role":"user","admin_scope":null}}}

// POST /api/v1/auth/logout
{"all_devices":false}

// GET /api/v1/auth/me -> 200
{"data":{"id":"019...","email":"budi@example.com","full_name":"Budi Santoso",
         "timezone":"Asia/Jakarta","role":"user","admin_scope":null,
         "is_active":true}}
```

## 3. Profil

| Method | Path | Auth | Keterangan |
|---|---|---|---|
| GET | `/users/me` | bearer | profil + timezone + preferensi ringkas |
| PATCH | `/users/me` | bearer | ubah `full_name`, `phone`, `timezone` |
| DELETE | `/users/me` | bearer | soft-delete + anonimkan, cabut semua sesi |
| GET | `/users/me/audit-log` | bearer | riwayat akses data dirinya |

Ganti timezone tidak mengubah log lama. Hanya dosis ke depan yang dihitung
dengan zona baru. Blok `schedule_timezone` pada `medication_frequency` menjaga
blok jadwal yang sudah berjalan tetap konsisten.

## 4. Katalog Obat

Dua sumber dalam satu endpoint: **preset publik** dan **katalog pribadi user**.
User melihat gabungan keduanya. User tidak pernah melihat katalog pribadi user lain.

| Method | Path | Auth | Keterangan |
|---|---|---|---|
| GET | `/catalog` | bearer | cari preset + katalog pribadi; `?q=&form=&route=&limit=&cursor=` |
| GET | `/catalog/{id}` | bearer | detail satu entri |
| POST | `/catalog` | bearer | buat entri katalog **pribadi** |
| PATCH | `/catalog/{id}` | bearer | ubah katalog pribadi miliknya saja |
| DELETE | `/catalog/{id}` | bearer | soft-delete katalog pribadi miliknya |
| GET | `/catalog/suggest` | bearer | saran nama (autocomplete) |

```json
// GET /api/v1/catalog?q=paracetamol
{"data":[
  {"id":"019aaa...","display_name":"Paracetamol 500 mg Tablet","nama_dagang":"Paracetamol",
   "is_preset":true,"curation_status":"published","source":"kfa",
   "dosage_form_name":"Tablet","route_name":"Oral",
   "dose_value_text":"500 mg","ucum_code":"mg","uom_name":"Tablet","atc_l5_code":"N02BE01"},
  {"id":"019bbb...","display_name":"Obat Warung ABC","is_preset":false,
   "curation_status":"user_private","source":"manual","dosage_form_name":"Tablet"}
],"page":{"next_cursor":null,"has_more":false,"limit":20}}

// POST /api/v1/catalog  (dibuat sebagai katalog pribadi)
{"display_name":"Obat Warung ABC","dosage_form_code":"BS001","route_code":"O",
 "ucum_code":"mg","uom_name":"Tablet","dose_value_text":"500 mg"}
// 201
{"data":{"id":"019bbb...","is_preset":false,"curation_status":"user_private",
         "owner_id":"019...","created_at":"2026-10-10T10:29:00Z"}}

// PATCH preset publik -> 409
{"error":{"code":"CATALOG_IMMUTABLE",
          "message":"Preset katalog hanya bisa diubah admin","details":null}}
```

Catatan: pencarian memakai `search_tsv` (index GIN) yang mencakup nama tampilan,
nama dagang, nama generik, `rxterm`, nama ATC level 5, dan bentuk sediaan.

## 5. Obat (Medications)

| Method | Path | Auth | Role | Keterangan |
|---|---|---|---|---|
| GET | `/medications` | bearer | owner, caregiver viewer/helper/manager | `?status=active&q=&limit=&cursor=` |
| POST | `/medications` | bearer | owner | buat obat + frekuensi + jam + stok |
| GET | `/medications/{id}` | bearer | owner, caregiver viewer/helper/manager | detail lengkap |
| PATCH | `/medications/{id}` | bearer | owner, caregiver manager | ubah data obat; jadwal terpisah |
| DELETE | `/medications/{id}` | bearer | owner, caregiver manager | soft-delete obat + jam, log dipertahankan |
| GET | `/medications/{id}/adherence` | bearer | owner, caregiver viewer/helper/manager | `?from=&to=` |
| POST | `/medications/{id}/photo` | bearer | owner | unggah foto resep (`multipart`, field `file`) |
| GET | `/medications/{id}/photo` | bearer | owner | sajikan foto resep |
| DELETE | `/medications/{id}/photo` | bearer | owner | hapus foto resep |

**Foto resep.** Hanya pemilik yang boleh mengaksesnya — berbeda dari
`GET /medications/{id}` yang juga terbuka untuk caregiver, karena foto
resep memuat identitas pasien. Foto disajikan lewat server ini, bukan
dialihkan ke object storage: CSP aplikasi menetapkan `connect-src 'self'`,
dan URL penyimpanan tidak perlu dibuka ke publik.

Yang disimpan di kolom `prescription_photo_url` adalah KUNCI OBJEK
(`photos/<owner>/<medication>/<waktu>.jpg`), bukan URL. Tautan ke bucket
privat adalah tautan bertanda tangan yang berlaku terbatas, sehingga URL
yang disimpan akan menjadi tautan mati dalam hitungan menit. Nama kolomnya
dipertahankan demi kompatibilitas klien. Batas 8 MiB; jenis berkas
diperiksa dari isinya, bukan dari header yang dikirim klien.

Field input lengkap (semua sesuai kolom schema):

```json
// POST /api/v1/medications
{
  "name": "Amlodipine 10",
  "catalog_id": "019aaa...",
  "dose_value": 1,
  "dose_unit_code": "tablet",
  "strength_note": null,
  "route_code": "O",
  "dosage_form_code": "BS001",
  "meal_relation": "after_meal",
  "as_needed": false,
  "must_finish": false,
  "start_date": "2026-10-11",
  "start_time": "07:00",
  "end_date": null,
  "notes": "Minum setelah sarapan",
  "notes_reminder": "Jangan lupa tensi pagi",
  "prescription_photo_url": "photos/019ccc.../019ddd.../1730000000000000000.jpg",
  "color_tag": "#4F46E5",
  "frequency": {
    "pattern": "daily",
    "times_per_day": 2,
    "start_time": "07:00",
    "schedule_timezone": "Asia/Jakarta"
  },
  "times": [
    {"time_local": "07:00", "label": "morning", "weekdays": 127},
    {"time_local": "19:00", "label": "evening", "weekdays": 127}
  ],
  "stock": {
    "unit_code": "tablet",
    "initial_qty": 30,
    "qty_per_dose": 1,
    "refill_threshold_qty": 5,
    "refill_reminder": true
  }
}
```

```json
// 201
{"data":{"id":"019ccc...","owner_id":"019...","name":"Amlodipine 10",
         "dose_value":1,"dose_unit_code":"tablet","route_code":"O",
         "meal_relation":"after_meal","as_needed":false,"must_finish":false,
         "start_date":"2026-10-11","start_time":"07:00","end_date":null,
         "is_active":true,"color_tag":"#4F46E5",
         "frequency":{"pattern":"daily","times_per_day":2,
                      "schedule_timezone":"Asia/Jakarta"},
         "times":[{"id":"019d1...","time_local":"07:00","label":"morning","weekdays":127,"is_active":true},
                  {"id":"019d2...","time_local":"19:00","label":"evening","weekdays":127,"is_active":true}],
         "stock":{"unit_code":"tablet","current_qty":30,"qty_per_dose":1,
                  "refill_threshold_qty":5,"refill_reminder":true},
         "created_at":"2026-10-10T10:29:00Z"}}

// 409 duplikat
{"error":{"code":"DUPLICATE_MEDICATION",
          "message":"Obat dengan nama dan dosis itu sudah ada",
          "details":[{"field":"name","issue":"duplicate"}]}}
```

Validasi yang menegakkan `ck_freq_params` di DB:

| `pattern` | Wajib ada |
|---|---|
| `daily` | `times_per_day` 1–24 |
| `interval` | `interval_hours` 1–168 |
| `weekly` | `days_of_week` bitmask, minimal satu hari |
| `monthly_cycle` | `cycle_intake_days`, `cycle_pause_days`, `cycle_anchor` |
| `prn` | tidak ada parameter wajib; `prn_max_per_day` opsional |

## 6. Jam Minum (Times)

Menggantikan `medication_schedules` di dokumen lama.

| Method | Path | Auth | Role | Keterangan |
|---|---|---|---|---|
| GET | `/medications/{id}/times` | bearer | owner, caregiver viewer/helper/manager | daftar jam |
| POST | `/medications/{id}/times` | bearer | owner, caregiver manager | tambah jam |
| PATCH | `/medications/{id}/times/{tid}` | bearer | owner, caregiver manager | ubah jam/label/hari |
| DELETE | `/medications/{id}/times/{tid}` | bearer | owner, caregiver manager | soft-delete satu jam |

```json
// POST /api/v1/medications/019ccc.../times
{"time_local":"12:30","label":"afternoon","weekdays":31,"dose_override":null}
// 201
{"data":{"id":"019d3...","medication_id":"019ccc...","time_local":"12:30",
         "label":"afternoon","weekdays":31,"dose_override":null,"is_active":true}}

// 409 jam itu sudah ada
{"error":{"code":"DUPLICATE_TIMES",
          "message":"Jam 12:30 untuk pola hari itu sudah ada",
          "details":[{"field":"time_local","issue":"duplicate"}]}}
```

Mengubah `time_local` hanya berlaku untuk dosis ke depan. Log yang sudah
tercatat dipertahankan apa adanya. Perubahan pola dicatat sebagai
`medication_dose_events` dengan `kind='dose_shift'`.

## 7. Log Kepatuhan

| Method | Path | Auth | Role | Keterangan |
|---|---|---|---|---|
| GET | `/medications/{id}/logs` | bearer | owner, caregiver viewer/helper/manager | `?from=&to=&status=` |
| GET | `/logs` | bearer | owner, caregiver viewer/helper/manager | feed semua obat per hari |
| POST | `/medications/{id}/logs` | bearer | owner, caregiver helper/manager | catat dosis |
| POST | `/logs/from-reminder` | bearer | owner, caregiver helper/manager | catat dari notifikasi |
| PATCH | `/logs/{logId}` | bearer | owner, caregiver helper/manager | koreksi status/catatan |
| POST | `/logs/{logId}/snooze` | bearer | owner, caregiver helper/manager | tunda pengingat |
| GET | `/logs/today` | bearer | owner | ringkasan hari ini (hanya yang SUDAH dicatat) |
| GET | `/logs/export` | bearer | owner | ekspor CSV `?from=&to=` (maks 366 hari) |
| GET | `/schedule` | bearer | owner | **jadwal dosis satu hari, termasuk yang belum dicatat** (`?date=YYYY-MM-DD`) |
| GET | `/refs` | bearer | semua | satuan, rute, bentuk sediaan — satu panggilan untuk formulir obat |

`/refs` penting bagi klien: `dose_unit_code`, `route_code`, dan
`dosage_form_code` punya **foreign key** ke tabel referensinya. Formulir yang
menawarkan pilihan dari daftar yang ditulis di klien bisa mengirim kode yang
tidak ada di database, dan pelanggaran foreign key sampai ke pengguna sebagai
**500** — padahal itu kesalahan pilihan, bukan kerusakan server. Urutannya
mengikuti `sort_order` (bukan alfabetis) supaya tampilannya masuk akal:
Oral sebelum Sublingual, bukan "B" (Buccal) di puncak.

`GET /logs/today` dan `GET /schedule` menjawab pertanyaan yang berbeda dan
keduanya diperlukan:

- `/logs/today` — "apa yang sudah saya catat hari ini". Hanya berisi catatan.
- `/schedule` — "dosis apa yang seharusnya saya minum hari ini". Dihitung dari
  aturan frekuensi, jadi dosis yang **belum** dicatat tetap terlihat.

Layar Hari Ini memakai `/schedule`; tanpa itu layar akan kosong sampai pengguna
mencatat sesuatu, justru saat pengingat paling dibutuhkan.

```json
// GET /api/v1/schedule  → 200
{"data":[{"medication_id":"01a13...","medication_name":"Amlodipine 5mg",
          "dose_value":1,"dose_unit_code":"tablet","meal_relation":"after_meal",
          "schedule_id":"01a13...","time_local":"07:00","time_label":"morning",
          "slot_date":"2026-10-12","scheduled_at":"2026-10-12T07:00:00+07:00",
          "scheduled_hour":7,"scheduled_minute":0,
          "status":"due","log_id":null,"taken_at":null,"note":null,
          "stock_remaining":29,"stock_unit_code":"tablet"}],
 "page":{"next_cursor":null,"has_more":false,"limit":1}}
```

`status`: `pending` (belum waktunya) · `due` (perlu ditindak) · `taken` ·
`late` · `skipped` · `snoozed`. `schedule_id` sama dengan yang diharapkan
`POST /medications/{id}/logs`, jadi klien mencatat langsung dari slot.

```json
// POST /api/v1/medications/019ccc.../logs
{"schedule_id":"019d1...","slot_date":"2026-10-10",
 "scheduled_hour":7,"scheduled_minute":0,"scheduled_tz":"Asia/Jakarta",
 "status":"taken","taken_at":"2026-10-10T00:31:00Z","note":null}
// 201
{"data":{"id":"019e1...","medication_id":"019ccc...","schedule_id":"019d1...",
         "owner_id":"019...","recorded_by":"019...","slot_date":"2026-10-10",
         "scheduled_hour":7,"scheduled_minute":0,"scheduled_tz":"Asia/Jakarta",
         "status":"taken","taken_at":"2026-10-10T00:31:00Z",
         "source":"self","note":null,"created_at":"2026-10-10T00:32:00Z",
         "stock":{"current_qty":29,"transaction_id":"019f1..."}}}

// 409 dosis itu sudah tercatat
{"error":{"code":"LOG_OCCURRENCE_TAKEN",
          "message":"Dosis 07:00 pada 2026-10-10 sudah tercatat",
          "details":[{"schedule_id":"019d1...","slot_date":"2026-10-10",
                      "existing_status":"taken"}]}}

// Log PRN: schedule_id null, boleh berkali-kali per hari
{"medication_id":"019ccc...","schedule_id":null,"slot_date":"2026-10-10",
 "status":"taken","taken_at":"2026-10-10T04:00:00Z","note":"demam 38.5"}
```

Aturan:

- Server memakai waktu sendiri untuk `taken_at` bila tidak dikirim.
- `taken` bila selisih ≤ `tolerance_minutes` (default 60); `late` bila lebih.
- Menandai `taken`/`late` memicu pengurangan stok (bagian 8).
- Koreksi = soft-delete baris lama + insert baris baru, satu transaksi.

## 8. Stok dan Restock

| Method | Path | Auth | Role | Keterangan |
|---|---|---|---|---|
| GET | `/medications/{id}/stock` | bearer | owner, caregiver viewer/helper/manager | saldo + perkiraan habis |
| PUT | `/medications/{id}/stock` | bearer | owner, caregiver manager | set/ubah pengaturan stok |
| GET | `/medications/{id}/stock/transactions` | bearer | owner, caregiver viewer/helper/manager | riwayat transaksi |
| POST | `/medications/{id}/stock/restock` | bearer | owner, caregiver manager | catat restock |
| POST | `/medications/{id}/stock/adjust` | bearer | owner, caregiver manager | koreksi manual |
| GET | `/stock/low` | bearer | owner | daftar obat yang perlu restock |

```json
// GET /api/v1/medications/019ccc.../stock -> 200
{"data":{"medication_id":"019ccc...","unit_code":"tablet","initial_qty":30,
         "current_qty":29,"qty_per_dose":1,"refill_threshold_qty":5,
         "refill_reminder":true,"doses_remaining":29,"needs_refill":false,
         "last_restocked_at":null,
         "estimated_days_left":14.5}}

// POST /api/v1/medications/019ccc.../stock/restock
{"qty":30,"note":"beli di apotek"}
// 201
{"data":{"kind":"restock","qty_delta":30,"balance_after":59,
         "transaction_id":"019f2...","last_restocked_at":"2026-10-10T11:00:00Z"}}

// 409 stok tidak cukup
{"error":{"code":"STOCK_INSUFFICIENT",
          "message":"Stok tidak cukup untuk mencatat dosis ini",
          "details":[{"current_qty":0,"requested":1}]}}
```

Aturan stok:

- `current_qty` hanya berubah lewat `stock_transactions`.
- `balance_after` diisi trigger `apply_stock_txn()`, bukan aplikasi.
- Saldo negatif ditolak di trigger dan di CHECK constraint.
- Setiap konsumsi menaut `log_id` supaya bisa ditelusuri asalnya.
- `log_id` unik (`uq_stock_txn_log`): satu dosis hanya boleh memotong saldo
  sekali, walau request-nya dikirim berulang.

## 9. Caregiver

> **STATUS IMPLEMENTASI.** Tingkat izin `manager` beserta endpoint tulis
> `/care/{ownerId}/medications*` dan `/care/{ownerId}/medications/{id}/stock/*`
> **belum ada di backend**. Saat ini `backend/internal/domain/enums.go` hanya
> punya `viewer` + `helper`, dan `00014_care_links.sql` CHECK-nya masih
> `('viewer','helper')`. Bagian ini adalah **kontrak yang dituju**; backend,
> migrasi, dan `enum_contract_test.go` perlu disesuaikan agar cocok.
> Sampai itu terjadi, permintaan dengan `role=manager` akan ditolak validasi.

Alur: pasien kirim undangan (`POST /care-links`) → caregiver terima token →
`POST /care-links/accept` → status `accepted`.

| Method | Path | Auth | Role | Keterangan |
|---|---|---|---|---|
| POST | `/care-links` | bearer | any | kirim undangan |
| GET | `/care-links` | bearer | any | `?as=owner` / `?as=caregiver` |
| POST | `/care-links/accept` | bearer | caregiver | terima via token |
| POST | `/care-links/reject` | bearer | caregiver | tolak |
| PATCH | `/care-links/{id}` | bearer | owner | ubah `role` (`viewer`↔`helper`) atau `relation` |
| DELETE | `/care-links/{id}` | bearer | owner atau caregiver | status → `revoked` |

Dashboard caregiver:

| Method | Path | Auth | Keterangan |
|---|---|---|---|
| GET | `/care/{ownerId}/overview` | bearer | kartu hari ini |
| GET | `/care/{ownerId}/medications` | bearer | obat yang dipantau |
| GET | `/care/{ownerId}/adherence` | bearer | `?from=&to=` |
| GET | `/care/{ownerId}/logs` | bearer | feed kronologis |
| GET | `/care/{ownerId}/logs/export` | bearer | **`manager`** — ekspor CSV, `?from=&to=&reason=` wajib |
| POST | `/care/{ownerId}/logs` | bearer | **`helper` atau `manager`** |
| POST | `/care/{ownerId}/medications` | bearer | **hanya `manager`** |
| PATCH | `/care/{ownerId}/medications/{id}` | bearer | **hanya `manager`** |
| DELETE | `/care/{ownerId}/medications/{id}` | bearer | **hanya `manager`** (arsip) |
| POST | `/care/{ownerId}/medications/{id}/stock/restock` | bearer | **hanya `manager`** |
| POST | `/care/{ownerId}/medications/{id}/stock/adjust` | bearer | **hanya `manager`** |

Aturan:

- `viewer`: hanya GET. Tidak ada aksi tulis.
- `helper`: boleh POST/PATCH log dengan `recorded_by = caregiver_id`,
  `source='caregiver'`. Status: `taken`, `late`, `snoozed`, `skipped`, koreksi.
- `manager`: semua hak `helper`, **ditambah** mengelola obat & stok pasien
  (tambah/ubah/arsip obat, ubah jam, restock, koreksi stok). Menulis audit
  dengan `actor_id` caregiver; `owner_id` data tetap milik pasien.
- `viewer` dan `helper` **tidak** boleh mengelola obat dan jadwal.
- Caregiver (ketiga tingkat) tidak pernah melihat `prescription_photo_url`,
  `notes` pribadi pasien, dan `audit_log` pasien. Field di-strip di query layer,
  bukan UI.
- Setiap akses caregiver menulis `audit_log` (`action='read'`).
- Relasi diverifikasi tiap request, bukan di-cache di UI.

```json
// GET /api/v1/care/019.../overview?date=2026-10-10
{"data":{"date":"2026-10-10","owner_id":"019...","timezone":"Asia/Jakarta",
         "doses_total":6,"taken":4,"late":1,"skipped":0,"missed":0,"pending":1,
         "adherence_ratio":0.83,
         "items":[{"medication_id":"019ccc...","name":"Amlodipine 10",
                   "schedule_id":"019d1...","time_local":"07:00",
                   "status":"taken","taken_at":"2026-10-10T00:31:00Z",
                   "recorded_by":"self"}]}}
```

## 10. Notifikasi

| Method | Path | Auth | Keterangan |
|---|---|---|---|
| GET | `/notification-prefs` | bearer | preferensi per kanal |
| PATCH | `/notification-prefs` | bearer | ubah kanal / jam sunyi / `advance_minutes` |
| POST | `/device-tokens` | bearer | daftar perangkat (idempoten per token) |
| DELETE | `/device-tokens/{token}` | bearer | cabut perangkat |
| GET | `/notifications` | bearer | inbox riwayat pengingat |

Kanal yang didukung: `push`, `email`, `telegram`, `in_app`.
Semua nilai ini ada di CHECK `notification_prefs.channel`.

```json
// PATCH /api/v1/notification-prefs
{"channel":"push","enabled":true,"quiet_start":"22:00","quiet_end":"06:00",
 "advance_minutes":10}
```

Payload push memakai kunci dosis yang sama dengan `uq_log_occurrence`:

```json
{
  "notification":{"title":"Waktunya minum Amlodipine 10","body":"1 tablet, sesudah makan",
                  "tag":"po-019ccc-019d1-2026-10-10-0700"},
  "data":{"type":"dose_reminder","medication_id":"019ccc...","schedule_id":"019d1...",
          "slot_date":"2026-10-10","scheduled_hour":7,"scheduled_minute":0,
          "deep_link":"/obat/019ccc?date=2026-10-10&hh=07&mm=00",
          "actions":"taken,snoozed,skipped"}
}
```

## 11. Admin

Semua rute admin di bawah prefix `/admin/*` dan dilindungi middleware
`RequireAdmin(scope)`. `admin_scope` NULL berarti akses penuh.

| Method | Path | Scope | Keterangan |
|---|---|---|---|
| GET | `/admin/users` | `users` | daftar akun, pencarian, filter |
| GET | `/admin/users/{id}` | `users` | detail akun (tanpa isi obat) |
| POST | `/admin/users/{id}/disable` | `users` | `is_active=false` |
| POST | `/admin/users/{id}/enable` | `users` | `is_active=true` |
| POST | `/admin/users/{id}/erase` | `users` | hapus atas permintaan |
| POST | `/admin/users/{id}/grant-admin` | `users` | beri peran + scope |
| POST | `/admin/users/{id}/revoke-admin` | `users` | cabut peran |
| GET | `/admin/catalog` | `catalog` | daftar termasuk `draft` |
| POST | `/admin/catalog` | `catalog` | buat preset baru (`draft`) |
| PATCH | `/admin/catalog/{id}` | `catalog` | ubah preset |
| POST | `/admin/catalog/{id}/publish` | `catalog` | `draft` → `published` |
| POST | `/admin/catalog/{id}/unpublish` | `catalog` | `published` → `draft` |
| POST | `/admin/catalog/{id}/archive` | `catalog` | → `archived` |
| POST | `/admin/catalog/import` | `catalog` | jalankan importer |
| GET | `/admin/notifications/jobs` | `notifications` | daftar job, filter status |
| POST | `/admin/notifications/jobs/{id}/retry` | `notifications` | ulangi job gagal |
| POST | `/admin/notifications/jobs/{id}/cancel` | `notifications` | batalkan job |
| GET | `/admin/stats` | `reports` | agregat tanpa identitas pasien |

```json
// POST /api/v1/admin/catalog/019aaa.../publish
{"reason":"verifikasi NIE BPOM"}
// 200
{"data":{"id":"019aaa...","curation_status":"published",
         "curated_by":"019admin...","curated_at":"2026-10-10T11:00:00Z"}}

// 403 admin dengan scope catalog mencoba akses users
{"error":{"code":"ADMIN_SCOPE_DENIED",
          "message":"Peran admin Anda tidak mencakup bidang ini",
          "details":{"required_scope":"users","your_scope":["catalog"]}}}

// GET /api/v1/admin/stats -> 200
{"data":{"users_active":128,"users_disabled":3,"medications_active":412,
         "catalog_presets":86,"jobs_failed_24h":2,"deliveries_sent_24h":1530}}
```

Aturan admin:

- Admin **tidak** bisa membaca isi obat atau log pasien individual lewat rute ini.
- Setiap tulisan menulis `audit_log` dengan `actor_role='admin'`, plus
  `before_state`/`after_state` untuk perubahan sensitif.
- `catalog_publish` wajib mengirim `reason`, tersimpan di `meta`.
- Batch importer bersifat idempoten; menjalankan dua kali tidak menggandakan data.

## 12. Webhook

| Method | Path | Auth | Keterangan |
|---|---|---|---|
| POST | `/webhooks/email-status` | HMAC | status kirim dari penyedia email |
| POST | `/webhooks/telegram-update` | secret token | update bot Telegram |

Verifikasi: header `X-Signature: sha256=<hmac_hex>` dari raw body dengan secret
per kanal. `X-Timestamp` harus ±5 menit. Replay ditolak karena `provider_id`
unik.

## 13. Header

| Header | Isi |
|---|---|
| `X-Request-Id` | UUID per request, sama dengan yang dicatat server |
| `X-Audit-Ref` | id baris `audit_log` untuk aksi yang berjejak |
| `X-RateLimit-Limit` | kuota menit ini |
| `X-RateLimit-Remaining` | sisa kuota |
| `X-RateLimit-Degraded` | `1` kalau penampung limit sedang bermasalah |

## 14. Rate Limit

- Default: 600 req/menit per user, 60 req/menit per IP belum login.
- Login/register: 5 per 5 menit per IP dan per email.
- Tulis log dan `from-reminder`: 120/menit.
- Endpoint stok: 60/menit.
- Balasan `429` menyertakan `Retry-After: 30`.
