# ADR-005 v2: Jadwal sebagai Aturan, bukan Baris per Hari

- Status: Accepted
- Tanggal: 2026-10-10
- Terkait: `ADR-002-postgresql-18`, `docs/design-v2/data-model-v2.md` bagian 6

## Context

Aplikasi harus mendukung obat yang diminum **selamanya** — hipertensi,
diabetes, tiroid. Juga lima pola frekuensi: harian, interval, mingguan, siklus
bulanan (intake lalu pause), dan PRN.

Versi v0.1 memakai tabel `medication_slots`: worker membuat baris untuk setiap
dosis 14 hari ke depan, dan menyegarkannya harian.

Masalahnya muncul saat skala dibayangkan sebagai SaaS.

### Hitungan dengan tabel slot

Asumsi konservatif: pengguna punya 3 obat, rata-rata 2 dosis per hari.

| Pengguna | Dosis/hari | Baris/hari | Baris/tahun |
|---|---|---|---|
| 100 | 600 | 600 | 219.000 |
| 1.000 | 6.000 | 6.000 | 2.190.000 |
| 10.000 | 60.000 | 60.000 | 21.900.000 |

Dua puluh dua juta baris per tahun untuk 10 ribu pengguna, dan itu **sebelum**
log dosis. Sebagian besar baris itu hanya berisi "belum terjadi" — status
`pending`.

Yang lebih buruk: mayoritas pengguna tidak pernah membuka aplikasi setiap hari,
tapi baris tetap dibuat untuk mereka.

### Masalah lain selain ukuran

1. **Worker jadi titik gagal.** Kalau worker mati lebih dari 14 hari, tidak ada
   pengingat dan tidak ada slot baru.
2. **Perubahan jadwal mahal.** Mengubah jam dari 07:00 ke 08:00 berarti
   menghapus dan menulis ulang semua slot masa depan.
3. **Menghapus obat** berarti memutus atau menghapus ratusan baris slot.
4. **Backfill.** Menambah pengguna lama ke fitur baru berarti memproduksi slot
   secara massal.
5. **Tidak ada nilai tambah.** Baris `pending` tidak menyimpan informasi yang
   tidak bisa dihitung dari aturan.

## Decision

**Jadwal disimpan sebagai ATURAN. Tidak ada tabel slot.**

Yang disimpan:

- `medication_frequency` — 1 baris per obat: pola dan parameternya.
- `medication_times` — N baris per obat: jam konkret.
- `medication_frequency.schedule_timezone` — zona saat blok dibuat.

Yang dihitung saat dibutuhkan:

- Tanggal dan jam dosis, dari aturan + timezone.

Yang tetap disimpan sebagai baris, tetapi hanya saat benar-benar terjadi:

- `medication_logs` — dosis yang dicatat pengguna.
- `medication_dose_events` — penyimpangan (`missed`, `dose_shift`, `paused`).
- `bump_queue` — pekerjaan yang akan datang (snooze, job notifikasi).

### Identitas dosis tanpa tabel slot

Masalah inti: kalau tidak ada baris slot, bagaimana mencegah satu dosis dicatat
dua kali?

Jawabannya **occurrence key**, yaitu string yang mewakili satu dosis:

```
<schedule_id>|<YYYY-MM-DD>|<HH>:<MM>
contoh: 019d1...|2026-10-10|07:00
```

Kunci ini dipakai di tiga tempat:

| Tabel | Bagaimana |
|---|---|
| `medication_logs` | partial unique index `uq_log_occurrence` pada `(medication_id, schedule_id, slot_date, scheduled_hour, scheduled_minute)` WHERE hidup AND `schedule_id IS NOT NULL` |
| `medication_dose_events` | `event_key` sebagai PRIMARY KEY |
| `bump_queue` | `dedup_key` dengan UNIQUE partial WHERE `done_at IS NULL` |

Satu kunci, satu dosis, konsisten di semua tabel.

### Perbandingan hasil

| | Dengan slot | Dengan aturan |
|---|---|---|
| Obat 2× sehari, 10 tahun | 7.300 baris | **3 baris** |
| 10.000 pengguna, 1 tahun | 21,9 juta baris | **~0 baris tambahan** |
| Worker mati 30 hari | pengingat berhenti | pengingat tetap jalan (aturan tetap ada) |
| Ubah jam 07:00 → 08:00 | tulis ulang slot masa depan | update 1 baris `medication_times` |
| Hapus obat | hapus/putus ratusan baris | soft delete 1 obat + N jam |

Yang tumbuh sekarang sebanding dengan **berapa kali orang benar-benar minum**,
bukan berapa hari yang dilewati kalender. Ini yang membuat model ini cocok
untuk SaaS.

## Consequences

### Positive

1. **Database tidak membengkak karena waktu.** Terapi seumur hidup tetap murah.
2. **Worker tidak lagi titik gagal kritis.** Kalau worker mati, aturan tetap ada;
   saat hidup lagi, satu pass menghitung ulang.
3. **Perubahan jadwal murah.** Update satu baris, bukan rewrite massal.
4. **Lebih sedikit state yang bisa tidak konsisten.** Tidak ada baris `pending`
   yang bisa tertinggal atau dobel.
5. **Biaya storage proporsional pemakaian nyata.**

### Negative

1. **Logika jadwal ada di kode, bukan di SQL.** Tidak bisa diverifikasi dengan
   query sederhana. Mitigasi: unit test domain yang ketat untuk setiap pola.
2. **Query laporan lebih berat.** Laporan kepatuhan harus menggabungkan log +
   dose_events terhadap jadwal hidup. Untuk rentang 7–30 hari masih murah;
   untuk laporan tahunan perlu materialized view.
3. **Timezone harus disimpan per blok jadwal.** Kalau memakai `users.timezone`
   langsung, mengubah preferensi zona akan mengubah tanggal dosis yang sudah
   lewat. Karena itu ada `schedule_timezone`.
4. **Occurrence key harus konsisten.** Kalau kode menghitung kunci dengan format
   berbeda di dua tempat, anti-ganda gagal. Mitigasi: satu fungsi pembentuk kunci,
   dipakai semua tempat.
5. **Snapshot pengingat bergantung pada akurasi perhitungan.** Kalau salah
   hitung, pengingat salah jam tanpa gejala yang kelihatan. Mitigasi: test per
   pola, termasuk kasus batas (pergantian hari, DST, akhir bulan).

### Neutral

1. **`bump_queue` tetap ada sebagai baris.** Tapi skalanya O(jadwal aktif × jam),
   bukan O(hari). Snooze memang butuh identitas sendiri.
2. **Perubahan pola dicatat** sebagai `dose_shift` supaya riwayat tetap bisa
   dijelaskan saat jadwal berubah di tengah terapi.

## Alternatives Considered

| Alternatif | Kenapa Ditolak |
|---|---|
| **Tabel slot di-generate 14 hari** | Sudah dipakai di v0.1. Membengkak, worker jadi titik gagal, perubahan jadwal mahal. |
| **Tabel slot di-generate selamanya** | Masalah sama, lebih parah. |
| **Slot dihitung di DB dengan view** | Lebih baik dari tabel, tapi pola `monthly_cycle` + pause + timezone sulit dan lambat di SQL murni, dan tetap tidak menjawab snooze. |
| **Materialized view slot** | Menambah langkah refresh dan tetap punya masalah staleness. |
| **Hybrid: slot hanya 7 hari ke depan** | Mengurangi ukuran tapi tidak menghilangkan masalah worker mati dan rewrite saat jadwal berubah. |
| **Simpan `next_run_at` per jadwal** | Perlu update setiap kali dosis lewat, dan tetap perlu menghitung untuk menampilkan riwayat. |

## Catatan Implementasi

- `patternMatches` per pola ada di `internal/domain/frequency.go`, diuji terpisah.
- **Perhatian khusus DST.** Timezone Indonesia tidak memakai DST, tapi kode
  harus tetap benar kalau ada pengguna di zona lain. Simpan `TIMESTAMPTZ`,
  hitung lokal dengan `time.LoadLocation`.
- **Akhir bulan.** `monthly_cycle` dengan `cycle_anchor='end_of_month'` harus
  menangani Februari (28/29 hari) dengan benar.
- **Catch-up pass saat start.** Worker menghitung ulang dosis yang terlewat
  selama proses mati. Idempoten karena occurrence key.
- `medication_dose_events.kind='dose_shift'` ditulis saat frekuensi berubah,
  supaya laporan bisa menjelaskan kenapa jumlah dosis berubah di tengah periode.

## Verifikasi yang Sudah Dilakukan

Dijalankan pada PostgreSQL 18.6:

| Yang diuji | Hasil |
|---|---|
| Obat 2× sehari terapi selamanya | 3 baris (1 pola + 2 jam) |
| `uq_log_occurrence` menolak dosis ganda | ditolak |
| Log PRN (`schedule_id` NULL) berkali-kali sehari | diterima |
| `ck_log_occurrence` menolak baris setengah jadi | ditolak |
| `medication_dose_events.event_key` menolak duplikat | ditolak |
| Hitung dosis 3 hari dari aturan | 6 dosis, dari 0 baris jadwal |

## Yang Belum Diverifikasi

- Beban query laporan kepatuhan pada rentang 1 tahun, ribuan pengguna.
- Perilaku saat pengguna berpindah timezone di tengah siklus.
- Kasus batas `monthly_cycle` dengan `end_of_month` pada Februari.
