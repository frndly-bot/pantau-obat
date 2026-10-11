# apidoc — OpenCollection Spec (Kontrak API)

Status: **lengkap dan lulus.** 75 file endpoint di 12 folder; satu kali run
koleksi menghasilkan **75/75 request lulus dengan 159/159 assertion**, dan
idempoten (run kedua di database yang sama tetap lulus).
Dokumen kontrak API: `docs/design-v2/api-v2.md`.

Folder ini adalah **syarat kelayakan backend**. Backend belum dianggap selesai
sampai setiap endpoint di `docs/design-v2/api-v2.md` punya file di sini, dengan
contoh request dan contoh response yang benar-benar bisa dijalankan.

## Kenapa OpenCollection

[OpenCollection](https://spec.opencollection.com/) (spesifikasi v1.0.0) adalah
format terbuka untuk mendeskripsikan koleksi API: request, autentikasi,
variabel, dan skrip. Dipilih karena:

- **Format teks di repo.** Bisa di-diff, di-review, dan masuk git. Bukan file
  biner atau export dari tool tertentu.
- **Multi-file per folder.** Satu endpoint satu file. Tidak ada satu file raksasa
  yang bentrok saat beberapa orang menambah endpoint.
- **Bisa dijalankan sebagai test.** Bagian `runtime` mendukung skrip dan
  assertion, jadi folder ini bukan cuma dokumentasi, tapi juga mekanisme uji
  end-to-end.
- **Tidak mengunci tool.** Bisa dibuka di Bruno, atau tool lain yang mendukung
  spesifikasi ini.

## Struktur folder

```
apidoc/
  opencollection.yml              # metadata koleksi + variabel
  environments/
    local.yml                    # base_url lokal
    staging.yml                  # base_url staging
  authentication/
    register.yml
    login.yml
    refresh.yml
    logout.yml
    me.yml
    change-password.yml
  user/
    get-profile.yml
    update-profile.yml
    delete-account.yml
    audit-log.yml
  medication/
    list.yml
    create.yml
    get.yml
    update.yml
    delete.yml
    adherence.yml
  schedule/
    list-times.yml
    create-time.yml
    update-time.yml
    delete-time.yml
  log/
    list-by-medication.yml
    list-all.yml
    create.yml
    from-reminder.yml
    update.yml
    snooze.yml
    today.yml
  stock/
    get.yml
    set.yml
    transactions.yml
    restock.yml
    adjust.yml
    low.yml
  catalog/
    search.yml
    get.yml
    create.yml
    update.yml
    delete.yml
    suggest.yml
  caregiver/
    create-invite.yml
    list-links.yml
    accept.yml
    reject.yml
    update-link.yml
    revoke.yml
    overview.yml
    medications.yml
    adherence.yml
    logs.yml
    create-log.yml
  notification/
    get-prefs.yml
    update-prefs.yml
    register-device.yml
    unregister-device.yml
    inbox.yml
  admin/
    list-users.yml
    get-user.yml
    disable-user.yml
    enable-user.yml
    erase-user.yml
    grant-admin.yml
    revoke-admin.yml
    list-catalog.yml
    create-catalog.yml
    update-catalog.yml
    publish-catalog.yml
    unpublish-catalog.yml
    archive-catalog.yml
    import-catalog.yml
    list-jobs.yml
    retry-job.yml
    cancel-job.yml
    stats.yml
  webhook/
    email-status.yml
    telegram-update.yml
  health/
    healthz.yml
    readyz.yml
```

Folder mengikuti **resource**, bukan halaman UI. Satu endpoint = satu file.

## Format file

Mengikuti struktur OpenCollection. Contoh lengkap:

```yaml
info:
  name: Buat Obat Baru
  type: http
  seq: 2
  tags:
    - medication
    - smoke

http:
  method: POST
  url: "{{base_url}}/api/v1/medications"
  headers:
    - name: Content-Type
      value: application/json
  body:
    type: json
    data: |-
      {
        "name": "Amlodipine 10",
        "dose_value": 1,
        "dose_unit_code": "tablet",
        "route_code": "O",
        "meal_relation": "after_meal",
        "start_date": "2026-10-11",
        "frequency": { "pattern": "daily", "times_per_day": 2 },
        "times": [
          { "time_local": "07:00", "label": "morning" },
          { "time_local": "19:00", "label": "evening" }
        ]
      }
  auth:
    type: bearer
    token: "{{access_token}}"

runtime:
  assertions:
    - expression: res.status
      operator: eq
      value: "201"
    - expression: res.body.data.name
      operator: isString
  scripts:
    - type: after-response
      code: |-
        bru.setVar("medication_id", res.body.data.id);

docs: |-
  # Buat Obat Baru

  Membuat obat beserta pola frekuensi, jam minum, dan stok awal.

  ## Catatan
  - `frequency.pattern=daily` wajib menyertakan `times_per_day`.
  - `uq_meds_owner_name` menolak duplikat nama+dosis+satuan milik user sama.
  - Balasan 409 `DUPLICATE_MEDICATION` bila duplikat.

  Kontrak: `docs/design-v2/api-v2.md` bagian 5.
```

## Menyiapkan environment

Nilai rahasia (password admin uji, secret webhook) **tidak** disimpan di file
yang masuk git. Salin contohnya lalu isi:

```bash
cp apidoc/.env.example apidoc/.env
# lalu edit apidoc/.env: ADMIN_PASSWORD, EMAIL_WEBHOOK_SECRET,
# TELEGRAM_WEBHOOK_SECRET — nilainya harus cocok dengan backend yang dijalankan
```

`environments/local.yml` membaca nilai itu lewat interpolasi
`{{process.env.NAMA}}`, jadi `local.yml` tetap aman untuk repo publik. `apidoc/.env`
sendiri diabaikan oleh `.gitignore` root.

## Variabel

Didefinisikan di `opencollection.yml`, nilai per environment di
`environments/*.yml`. Rahasia diambil dari `apidoc/.env` (lihat di atas).

| Variabel | Isi |
|---|---|
| `base_url` | `http://localhost:8080` (local) |
| `access_token` | diisi otomatis oleh `authentication/login.yml` |
| `user_id` | diisi otomatis oleh `authentication/register.yml` |
| `medication_id` | diisi otomatis oleh `medication/create.yml` |
| `schedule_id` | diisi otomatis oleh `schedule/create-time.yml` |
| `log_id` | diisi otomatis oleh `log/create.yml` |
| `catalog_id` | diisi otomatis oleh `catalog/create.yml` |
| `care_link_id` | diisi otomatis oleh `caregiver/create-invite.yml` |
| `admin_token` | token akun admin (dari seed, bukan register) |

Rantai variabel ini yang membuat folder ini bisa dipakai sebagai **uji e2e
berurutan**: register → login → buat obat → catat dosis → cek stok → cek
kepatuhan.

## Urutan jalan (collection run)

Urutan `seq` disusun supaya satu kali run membentuk alur nyata:

1. `health/healthz` — pastikan API hidup
2. `authentication/register` — buat akun uji
3. `authentication/login` — dapat `access_token`
4. `catalog/search` — cari preset
5. `medication/create` — buat obat + frekuensi + jam + stok
6. `log/create` — catat dosis, cek stok berkurang
7. `stock/get` — pastikan saldo turun satu
8. `medication/adherence` — cek angka kepatuhan
9. `caregiver/create-invite` — undang caregiver
10. `admin/*` — dengan `admin_token` terpisah

## Aturan pengisian

1. **Satu file satu endpoint.** Jangan gabung.
2. **Setiap file punya `docs`.** Isinya ringkas: tujuan, catatan penting, kode
   error yang mungkin muncul, dan rujukan ke `api-v2.md`.
3. **Setiap file punya `runtime.assertions` minimal status code.** Untuk endpoint
   penting, tambah assertion pada field yang bermakna.
4. **Contoh request harus benar-benar jalan.** Bukan placeholder seperti
   `"name": "string"`. Pakai nilai nyata yang masuk akal.
5. **Contoh response disimpan di `docs`.** Sertakan satu contoh sukses dan satu
   contoh gagal yang relevan.
6. **Enum wajib pakai nilai Inggris** yang sama dengan CHECK di migrasi:
   `after_meal`, `morning`, `child`, `imported`. Jangan nilai Indonesia.
7. **Tidak ada secret di file.** Pakai variabel. Password contoh boleh literal,
   tapi token tidak. Untuk rahasia nyata (password admin uji, secret webhook),
   simpan di `apidoc/.env` dan rujuk lewat `{{process.env.NAMA}}`.
8. **Endpoint admin dipisah folder.** Jangan dicampur dengan endpoint user,
   karena `admin_scope` dan auditnya berbeda.

## Kelayakan backend

Backend dianggap layak kalau **semua** ini terpenuhi:

- [x] Semua endpoint di `docs/design-v2/api-v2.md` punya file di `apidoc/`.
- [x] Collection run dari nol (database kosong) lulus tanpa error.
- [x] Setiap endpoint punya minimal satu assertion status code.
- [x] Alur e2e bernilai: register → obat → catat → stok turun → kepatuhan naik.
- [x] Alur admin terpisah lulus, termasuk uji `ADMIN_SCOPE_DENIED`.
- [x] Tidak ada `access_token` atau secret yang ter-commit.
- [ ] Environment `staging.yml`.
- [ ] Target `apidoc-run` di CI dengan `set -o pipefail`.

Catatan pada butir terakhir: `make apidoc-run` memakai `bru run`, dan bila
keluarannya dipipe (`| tail`), status keluarnya bisa tertutup oleh pipe.
Untuk dipakai sebagai gerbang CI, pipa itu perlu `set -o pipefail` — kalau
tidak, run yang gagal akan lolos hijau.

## Yang sudah ada

**75 file endpoint** di 12 folder, ditambah 12 `folder.yml`.

| Folder | File | Isi |
|---|---|---|
| `health/` | 2 | `/healthz`, `/readyz` |
| `authentication/` | 9 | register, register-caregiver, login, login-admin, refresh, logout, me, change-password, relogin |
| `catalog/` | 6 | search, suggest, create, get, update, delete |
| `medication/` | 5 | create, list, get, update, delete |
| `log/` | 5 | create, from-reminder, update, snooze, today |
| `stock/` | 6 | get, set, restock, adjust, transactions, low |
| `caregiver/` | 11 | create-invite, list-links, accept, reject, update-link, revoke, overview, medications, adherence, logs, create-log |
| `adherence/` | 3 | summary, daily, by-medication |
| `notification/` | 6 | get-prefs, update-prefs, register-device, list-devices, unregister-device, inbox |
| `user/` | 4 | get-profile, update-profile, audit-log, delete-account |
| `admin/` | 16 | list-users, get-user, disable/enable/erase-user, grant/revoke-admin, list/create/publish/unpublish/archive-catalog, list/retry/cancel-job, stats |
| `webhook/` | 2 | email-status, telegram-update |

Hasil run terakhir: **75/75 request lulus, 159/159 assertion**, dan
**idempoten** — dijalankan dua kali di database yang sama tetap lulus.

## Urutan folder WAJIB pakai `folder.yml`

Bruno mengurutkan folder secara **alfabetis**, bukan berdasarkan alur. Tanpa
`seq`, `adherence/` dan `admin/` berjalan **sebelum** `authentication/`,
sehingga `access_token` belum ada dan puluhan request menanggung 401 berantai
— terlihat seperti bug backend padahal bukan.

Setiap folder karena itu punya `folder.yml` berisi `info.seq`:

| seq | Folder | seq | Folder |
|---|---|---|---|
| 1 | health | 7 | caregiver |
| 2 | authentication | 8 | adherence |
| 3 | catalog | 9 | notification |
| 4 | medication | 10 | user |
| 5 | log | 11 | admin |
| 6 | stock | 12 | webhook |

Menambah folder baru **wajib** memberinya `folder.yml` dengan `seq`, dan
menyisipkannya tanpa menggeser yang lain kalau memungkinkan.

## Yang belum ada

- Environment `staging.yml`.
- Target `apidoc-run` di CI.
- Uji `ADMIN_SCOPE_DENIED` yang benar-benar menembak admin bercakupan
  terbatas. Saat ini hanya diuji secara tidak langsung.

## Tiga jebakan yang sudah ditemukan (jangan diulang)

1. **`bru.getVar` tidak membaca variabel environment.** Ia hanya membaca
   variabel yang diset lewat `bru.setVar` (runtime). Variabel dari
   `environments/*.yml` harus dibaca dengan **`bru.getEnvVar`**. Terbukti:
   secret webhook terbaca kosong lewat `getVar`, sehingga tanda tangan HMAC
   selalu salah dan endpoint membalas 401.

2. **`isObject` bukan operator assertion yang dikenal Bruno.** Operator yang
   sah: `eq`, `neq`, `gt`, `gte`, `lt`, `lte`, `in`, `notIn`, `contains`,
   `notContains`, `length`, `matches`, `notMatches`, `startsWith`, `endsWith`,
   `between`, `isEmpty`, `isNotEmpty`, `isNull`, `isUndefined`, `isDefined`,
   `isTruthy`, `isFalsy`, `isJson`, `isNumber`, `isString`, `isBoolean`,
   `isArray`. Operator tak dikenal jatuh ke `eq` dan membandingkan dengan
   string namanya — assertion gagal walau responsnya benar.

3. **Body webhook harus bertipe `text`, bukan `json`.** Kalau bertipe `json`,
   Bruno mengurai lalu menyusun ulang body-nya, sehingga byte yang terkirim
   tidak lagi sama dengan byte yang ditandatangani. Server membalas 401 walau
   HMAC-nya benar.

## Rujukan

- Spesifikasi: <https://spec.opencollection.com/> (v1.0.0)
- Referensi struktur YAML: <https://docs.usebruno.com/v3/opencollection-yaml/structure-reference>
- Kontrak endpoint: `docs/design-v2/api-v2.md`
- Aturan backend: `backend/README.md`
