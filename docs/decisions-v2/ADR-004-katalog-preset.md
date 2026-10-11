# ADR-004 v2: Katalog Preset vs Data User

- Status: Accepted
- Tanggal: 2026-10-10
- Terkait: `ADR-001-stack-v2`, `docs/design-v2/data-model-v2.md` bagian 4

## Context

Aplikasi butuh daftar obat untuk dipilih pengguna. Ada dua jenis data:

1. **Preset publik** — kurasi admin atau importer dari sumber resmi (KFA, BPOM).
   Dipakai semua pengguna. Harus stabil dan terverifikasi.
2. **Data pribadi pengguna** — obat yang tidak ada di preset, ditambah sendiri
   oleh pengguna. Tidak boleh muncul untuk orang lain.

Kasus yang memicu keputusan:

- Pengguna di daerah tertentu punya obat warung atau racikan yang tidak ada di
  katalog resmi.
- Nama obat bisa sama antara dua pengguna dengan merek berbeda.
- Pengguna tidak boleh bisa "mencemari" preset publik dengan entri asal-asalan.
- Admin perlu bisa menarik kembali preset yang ternyata salah.

Pertanyaan: satu tabel dengan pembeda, atau dua tabel terpisah?

## Decision

**Satu tabel `drug_catalog`, dibedakan oleh `owner_id` + `is_preset` +
`curation_status`, dengan CHECK yang menegakkan aturan.**

```sql
CONSTRAINT ck_catalog_scope CHECK (
    (is_preset AND owner_id IS NULL AND curation_status IN ('draft','published','archived'))
    OR (NOT is_preset AND owner_id IS NOT NULL AND curation_status = 'user_private')
)
```

| Peran | `owner_id` | `is_preset` | `curation_status` |
|---|---|---|---|
| Preset publik | NULL | true | `draft`, `published`, `archived` |
| Katalog pribadi | terisi | false | `user_private` |

Konsekuensi CHECK:

- Pengguna biasa **tidak mungkin** membuat preset publik, karena butuh
  `owner_id` NULL + `is_preset` true.
- Entri pribadi **wajib** punya pemilik, sehingga isolasi antar pengguna
  langsung terjaga.

Keunikan nama juga dipisah:

- Preset: unik global pada `(lower(display_name), dosage_form_name, uom_name)`.
- Pribadi: unik pada `(owner_id, lower(display_name), dosage_form_name, uom_name)`.

Sehingga dua pengguna boleh punya "Obat Warung ABC" masing-masing, tapi preset
tidak boleh punya dua entri dengan nama dan bentuk yang sama.

PK internal adalah `id UUID`. Kode penyedia (`kfa_code`, `bpom_nie`, `barcode`)
menjadi kolom nullable dengan partial unique index. FK dari `medications` dan
`medication_templates` memakai `id`, bukan kode penyedia.

## Consequences

### Positive

1. **Satu jalur pencarian.** Query katalog = `WHERE owner_id IS NULL OR owner_id = :me`.
   Tidak perlu union dua tabel.
2. **Isolasi dijamin DB.** CHECK membuatnya tidak mungkin salah, bukan sekadar
   konvensi di kode.
3. **FK tetap satu arah.** `medications.catalog_id` menunjuk satu tabel, apa pun
   jenis entrinya.
4. **Siklus hidup jelas.** `draft → published → archived` hanya berlaku untuk
   preset; entri pribadi selamanya `user_private`.
5. **Importer idempoten.** Partial unique pada `kfa_code` dan `bpom_nie` membuat
   menjalankan importer dua kali tidak menggandakan data.
6. **Skema tahan sumber baru.** `extra_meta JSONB` menyimpan field penyedia yang
   belum dipetakan, jadi menambah sumber tidak butuh migrasi.

### Negative

1. **Tabel jadi lebar.** Sekitar 60 kolom. Sebagian besar nullable.
   Alternatifnya memecah tabel, tapi itu menambah join di jalur pencarian.
2. **Query harus selalu ingat filter kepemilikan.** Kalau lupa
   `owner_id IS NULL OR owner_id = :me`, data pengguna lain bisa bocor.
   Mitigasi: akses katalog hanya lewat satu fungsi repo (`catalog.SearchVisible`),
   bukan query lepas di handler.
3. **CHECK sedikit rumit.** Kesalahan tulis CHECK bisa membuat data tidak bisa
   di-insert sama sekali. Sudah diuji: preset palsu dan pribadi tanpa pemilik
   keduanya ditolak.
4. **Duplikasi antar-pengguna tidak terdeteksi.** Dua pengguna bisa menambah
   entri pribadi yang sebenarnya sama. Kalau ini jadi masalah, perlu
   penggabungan (kandidat fitur `catalog_merge`).

### Neutral

1. **`source` menandai asal** (`kfa`, `bpom`, `manual`, `other`), bukan jenis
   kepemilikan. Dua hal berbeda, dipisah.
2. **`search_tsv` mencakup entri pribadi juga.** Pengguna bisa mencari obat
   pribadinya sendiri dengan cara yang sama.
3. **Preset tidak bisa dihapus permanen**, hanya `archived`. Riwayat tetap.

## Alternatives Considered

| Alternatif | Kenapa Ditolak |
|---|---|
| **Dua tabel: `drug_preset` + `drug_user`** | `medications.catalog_id` jadi ambigu; butuh dua FK dan pengecekan mana yang terisi. Pencarian harus union. |
| **Satu tabel tanpa pemisahan, semua entri publik** | Pengguna bisa mencemari katalog publik. Tidak ada cara mencabut. |
| **Pisah lewat schema berbeda (`catalog.presets`)** | Postgres mendukung, tapi menambah kompleksitas izin dan `search_path`. Tidak sepadan. |
| **Preset disimpan di file statis, bukan DB** | Tidak bisa dikurasi lewat admin, tidak bisa dicari bersamaan, tidak ada riwayat perubahan. |
| **PK memakai `kfa_code`** | Kalau KFA tidak dipakai (sekarang tidak bisa diakses), PK jadi kosong. Sudah dicoba, gagal. |
| **`source` dipakai sebagai pembeda** | `source` menandai asal data, bukan kepemilikan. Mencampur keduanya bikin bingung. |

## Catatan Implementasi

- `ck_catalog_scope` sudah diuji: user biasa yang mencoba membuat preset
  ditolak, dan entri `user_private` tanpa `owner_id` ditolak.
- `deleted_at` tetap ada untuk katalog pribadi. Preset memakai `curation_status`
  untuk pensiun, bukan soft delete.
- Importer menulis `snapshot_date` dan `source_updated_at` supaya bisa
  dibandingkan dengan sumber saat berikutnya.
- Atribusi "Sumber: KFA Kemenkes / BPOM" wajib tampil di UI saat entri berasal
  dari sumber itu.

## Yang Belum Diputuskan

- Kebutuhan `catalog_merge` untuk menggabungkan entri pribadi dengan preset yang
  baru muncul. Kandidat v0.2.
- Apakah admin boleh mengangkat entri pribadi pengguna menjadi preset.
  Rekomendasi: ya, tetapi lewat alur terpisah dengan persetujuan dan audit.
