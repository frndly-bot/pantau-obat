# Notifikasi v2 — Pantau Obat

Status: v2, selaras dengan migrasi goose
Tanggal: 2026-10-10

> Menggantikan `docs/design/notification.md`.

## 1. Prinsip

Pengingat harus tetap terkirim walaupun browser ditutup, HP restart, atau
service worker dimatikan. Karena itu **scheduler jalan di server**, bukan di
browser. Tidak ada cron di klien.

Perubahan utama dari v0.1: tidak ada tabel slot. Jadwal berupa aturan, dan
antrean kerja memakai dua tabel dengan peran berbeda.

## 2. Peran Dua Tabel Antrean

| Tabel | Menjawab pertanyaan | Isi |
|---|---|---|
| `bump_queue` | **kapan** sesuatu harus terjadi | dosis yang akan datang, snooze, restock, digest |
| `notification_jobs` | apa yang **sudah** dikirim dan hasilnya | job pengiriman per kanal |

Kenapa dipisah:

- Snooze mengubah **waktu**, bukan isi pesan. Kalau snooze disimpan di
  `notification_jobs`, riwayat pengiriman jadi kotor.
- `bump_queue` murah dibersihkan (7 hari). `notification_jobs` disimpan 30 hari
  untuk investigasi "kenapa saya tidak dapat notifikasi".
- Satu baris `bump_queue` bisa menghasilkan beberapa `notification_jobs`
  (retry), atau nol (kalau dosis sudah dicatat lebih dulu).

## 3. Tiga Loop

Semua di dalam satu proses Go.

```
scheduler (tick 60 detik)
   hitung dosis jatuh tempo dari aturan
   -> insert bump_queue (dedup_key, idempoten)
        |
        v
dispatcher (tick 5 detik)
   claim bump_queue yang due: SELECT ... FOR UPDATE SKIP LOCKED
   -> buat notification_jobs (dedup via unique)
   -> baca notification_prefs user (kanal aktif, jam sunyi, advance)
   -> fan-out per kanal: sender_push / sender_email / sender_telegram
   -> tulis notification_deliveries per kanal
   -> tandai bump_queue.done_at
        |
        v
missed_pass (tick harian, tengah malam waktu pemilik)
   hitung dosis hari itu yang lewat jam tanpa log
   -> insert medication_dose_events (kind='missed', event_key unik)
   -> kalau pasien terlewat N berturut-turut, buat job missed_escalation
```

Tambahan: `catch-up pass` saat start, untuk mengejar dosis yang terlewat
selama proses mati.

## 4. Menghitung Dosis Jatuh Tempo

Ini pengganti "scan tabel slot". Pseudocode:

```
func dueDoses(now time.Time) []Dose {
    for each medication where is_active and deleted_at is null:
        freq := medication.frequency
        tz   := freq.schedule_timezone     // bukan timezone user saat ini
        local := now.In(tz)

        if !withinTherapy(medication, local) { continue }
        if !patternMatches(freq, local)      { continue }
        if inPauseWindow(freq, local)        { continue }

        for each t in medication.times where t.is_active:
            if !weekdayMatches(t.weekdays, local.Weekday()) { continue }
            if !sameMinute(t.time_local, local)             { continue }
            yield Dose{medication, t, local.Date(), local.Hour(), local.Minute()}
    }
}
```

`patternMatches` per `pattern`:

| pattern | syarat |
|---|---|
| `daily` | selalu cocok |
| `interval` | `(jamSekarang - jamMulai) % interval_hours == 0` |
| `weekly` | bitmask hari cocok |
| `monthly_cycle` | dalam `cycle_intake_days` sejak anchor; tidak dalam `cycle_pause_days` |
| `prn` | tidak dijadwalkan; hanya dicatat manual |

Kenapa memakai `schedule_timezone`, bukan `users.timezone`:

Kalau pasien pindah zona atau mengubah preferensi timezone, dosis yang sudah
terjadi tidak boleh berubah tanggalnya. Zona disimpan saat blok jadwal dibuat.
Perubahan zona berlaku untuk blok berikutnya.

Identitas dosis (occurrence key):

```
medication_id | schedule_id | slot_date | scheduled_hour | scheduled_minute
```

Bentuk string: `schedule_id|YYYY-MM-DD|HH:MM`. Ini dipakai di:
- `medication_logs` (lewat `uq_log_occurrence`)
- `medication_dose_events` (lewat `event_key`)
- `bump_queue.dedup_key`

Satu kunci untuk satu dosis di semua tabel. Ini yang membuat idempotensi
konsisten — tidak ada tabel yang bisa punya dua baris untuk dosis yang sama.

## 5. Job dan Antrean

### `bump_queue`

| Kolom | Isi |
|---|---|
| `kind` | `dose` / `refill` / `digest` |
| `scheduled_at` | kapan harus didorong |
| `slot_date` | tanggal lokal pemilik |
| `dedup_key` | `dose\|<schedule>\|<date>\|<hh:mm>` atau `refill\|<med>\|<date>` |
| `skip_tag` | `snooze` (ditunda user) atau `quiet` (jam sunyi) |
| `attempts`, `claimed_at`, `done_at` | siklus hidup |

UNIQUE `uq_bump_dedup` pada `dedup_key` WHERE `done_at IS NULL`. Artinya:
satu dosis hanya punya satu pekerjaan aktif pada satu waktu.

### `notification_jobs`

| Kolom | Isi |
|---|---|
| `kind` | `dose_reminder` / `missed_escalation` / `care_digest` / `retention` / `refill_reminder` |
| `status` | `pending` / `claimed` / `done` / `failed` / `cancelled` |
| `dedup_key` | `job\|<dose-key>\|dose_reminder` |
| `run_at` | kapan dijalankan |
| `attempts`, `max_attempts` | retry (default maks 5) |

Di v0.1 tabel ini memakai tipe ENUM Postgres. Sekarang `TEXT + CHECK` agar
konsisten dengan sisa skema. Nilai `kind` ditambah `refill_reminder`.

### `notification_deliveries`

Satu baris per job per kanal. UNIQUE `(job_id, channel)`. Menyimpan
`provider_id`, `error_code`, `error_msg`, `attempts`, `sent_at`.

## 6. Kanal

| Kanal | Latensi | Kapan | Status v0.1 |
|---|---|---|---|
| In-app | saat app dibuka | fallback, selalu aktif | **AKTIF** |
| Web Push | < 2 detik | HP + desktop, perlu izin | ditunda (kandidat v0.2) |
| Telegram | 1–5 detik | semua perangkat dengan chat aktif | ditunda (v0.2) |
| Email | 30–120 detik | inbox, dibaca telat | ditunda (v0.2) |

Keputusan aktivasi (2026-10-12): v0.1 hanya mengirim **in-app**. "Mengirim"
untuk kanal ini berarti mencatat job `sent` yang dibaca inbox aplikasi
sendiri — tidak butuh kredensial operator apa pun. Kode tiga kanal lain
tetap ada di repo; aktivasi cukup mengisi kredensial (VAPID / SMTP / token
bot) dan mengubah `DefaultPrefs`.

Nilai `channel` di CHECK: `push`, `email`, `telegram`, `in_app`. Empat-empatnya
sudah ada di schema, jadi tidak perlu migrasi CHECK saat menambah kanal.

Catatan: kanal `wa` **tidak** dipakai di v0.1–v0.3. WA Business API butuh
verifikasi bisnis dan biaya per percakapan. Nilainya juga tidak ada di CHECK
migrasi `00015_notifications.sql` supaya tidak jadi janji palsu di UI.

### Urutan fallback

```
push?      -> ada device_tokens aktif & VAPID siap   -> web push
telegram?  -> user_integrations verified             -> Bot API
email?     -> prefs channel 'email' enabled          -> SMTP
in_app?    -> selalu true                            -> tulis baris inbox
```

Kalau push **dan** Telegram sama-sama aktif, keduanya dikirim. Pasien dan
caregiver memakai perangkat berbeda; satu pesan tidak cukup untuk dua orang.

## 7. Snooze

- Pilihan 5, 10, 30 menit.
- Snooze = satu baris `bump_queue` (`kind='dose'`, `skip_tag='snooze'`).
- Snooze kedua **memperbarui** `scheduled_at` baris itu, bukan menambah baris.
- Maksimal 3 snooze beruntun per dosis. Setelah itu tulis
  `medication_dose_events` dengan `kind='paused'` dan tandai dosis terlewat.
- Setelah dosis dicatat (taken/late/skipped), baris `bump_queue` diisi `done_at`.

Alasan tidak menyimpan snooze di `notification_jobs`: snooze mengubah waktu,
bukan pesan. Menaruhnya di riwayat pengiriman akan mengaburkan data.

## 8. Jam Sunyi

- `quiet_start` dan `quiet_end` per user per kanal.
- Dosis yang jatuh di dalam jam sunyi: baris `bump_queue` diberi
  `skip_tag='quiet'`, `scheduled_at` digeser ke `quiet_end`.
- **Penting:** penggeseran ini tidak mengubah `slot_date` dosis. Dosis tetap
  dihitung milik tanggal aslinya, sehingga laporan kepatuhan tidak bohong.
- Kalau pasien mencatat dosis lebih dulu, baris `bump_queue` di-`done` tanpa kirim.

## 9. Deteksi Terlewat

Satu pass per hari, bukan per menit. Alasan: per menit berarti query berat
tanpa manfaat. Keputusan `missed` bersifat final per hari.

```
setiap hari 00:05 waktu lokal pemilik:
    untuk setiap dosis terjadwal hari kemarin yang tidak punya log:
        insert medication_dose_events(
            event_key = schedule_id|tanggal|hh:mm,
            kind      = 'missed',
            reason    = 'tidak ada catatan'
        )
    hitung streak terlewat berturut-turut per pasien
    kalau streak >= N (default 3):
        buat job missed_escalation untuk caregiver
```

Yang **tidak** dihitung terlewat:

- Hari dalam `cycle_pause_days` (masa jeda siklus).
- Tanggal di luar `start_date`..`end_date`.
- Dosis `prn` (memang tidak dijadwalkan).
- Dosis yang sudah punya log dengan status apa pun.

`event_key` sebagai PRIMARY KEY membuat pass idempoten. Kalau dijalankan dua
kali karena restart, tidak ada baris ganda.

## 10. Hari Dihitung Ulang Saat Start

Kalau worker mati 3 hari, pass tengah malam tidak jalan. Saat start, `catch-up
pass` menjalankan ulang `missed_pass` untuk rentang hari yang terlewat. Karena
idempoten, aman dijalankan berkali-kali.

## 11. Retry dan Backoff

| Percobaan | Jeda |
|---|---|
| 1 | langsung |
| 2 | 1 menit |
| 3 | 5 menit |
| 4 | 30 menit |
| 5 | 2 jam |
| gagal | job `failed`, `last_error` diisi |

Backoff dihitung dari `attempts`, disimpan di `run_at`.

Kegagalan per kanal tidak menggagalkan job. Job `done` kalau minimal satu kanal
sukses. Semua kanal gagal baru `failed`.

## 12. Anti-Spam

1. Dedup per dosis + kanal + jenis pesan lewat `dedup_key`.
2. Maksimal 3 snooze per dosis.
3. Jam sunyi per kanal.
4. Eskalasi caregiver maksimal 1 kali per hari per pasien.
5. Digest harian hanya dikirim kalau ada masalah.
6. Kalau lebih dari 3 notifikasi masuk dalam 10 menit tanpa satu pun `taken`,
   turunkan otomatis ke email saja dan set `notif_throttled` di
   `notification_prefs`.
7. Semua throttle dicatat di `audit_log` beserta alasannya.

## 13. Peringatan Restock

Ini job baru di v2.

```
setiap hari 08:00 waktu lokal pemilik:
    untuk setiap medication_stock where refill_reminder and deleted_at is null:
        kalau current_qty <= refill_threshold_qty:
            buat job refill_reminder
            dedup_key = refill|<medication_id>|<tanggal>
```

Isi pesan menyertakan `doses_remaining` dan `estimated_days_left` dari
`v_medication_stock_status`.

Dedup harian: satu peringatan per obat per hari, tidak berulang sepanjang hari.

## 14. Payload

### Web Push

```json
{
  "notification": {
    "title": "Waktunya minum Amlodipine 10",
    "body": "1 tablet, sesudah makan",
    "tag": "po-019ccc-019d1-2026-10-10-0700"
  },
  "data": {
    "type": "dose_reminder",
    "medication_id": "019ccc...",
    "schedule_id": "019d1...",
    "slot_date": "2026-10-10",
    "scheduled_hour": 7,
    "scheduled_minute": 0,
    "deep_link": "/obat/019ccc?date=2026-10-10&hh=07&mm=00",
    "actions": "taken,snoozed,skipped"
  }
}
```

`tag` memakai occurrence key sehingga notifikasi untuk dosis yang sama saling
menimpa, bukan menumpuk.

### Telegram

Sama isinya, dikirim sebagai pesan dengan tombol inline:
`Minum` / `Tunda 10 menit` / `Lewati`.

### Restock

```json
{"type":"refill_reminder","medication_id":"019ccc...",
 "medication_name":"Amlodipine 10","current_qty":4,"doses_remaining":4,
 "estimated_days_left":2,"unit_code":"tablet"}
```

## 15. Service Worker

```
push event    -> tampilkan notifikasi dengan tag = occurrence key
notificationclick:
    action "taken"   -> POST /api/v1/logs/from-reminder  {status: taken}
    action "snooze"  -> POST /api/v1/logs/{id}/snooze    {minutes: 10}
    action "skipped" -> POST /api/v1/logs/from-reminder  {status: skipped}
    default          -> buka deep_link
```

Semua panggilan memakai `Idempotency-Key` yang sama dengan occurrence key,
supaya klik ganda tidak menghasilkan dua log.

## 16. Batas Gagal Kirim

| Kondisi | Perlakuan |
|---|---|
| Satu kanal gagal | kanal itu `failed`, kanal lain lanjut |
| Semua kanal gagal | job `failed` setelah 5 percobaan |
| Job gagal | `last_error` diisi, muncul di `/admin/notifications/jobs` |
| Gagal > 20 dalam 24 jam | log level `error`, perlu dilihat admin |
| Tidak ada perangkat | kanal push di-skip, status `skipped`, bukan error |
| Backlog > 1000 | backpressure: scheduler berhenti insert baru sampai turun |

Kegagalan notifikasi **tidak** memblokir pencatatan dosis. Log tetap tersimpan.
Notifikasi bersifat best-effort.

## 17. Log

| Event | Level |
|---|---|
| Dosis masuk `bump_queue` | `debug` |
| Job sukses di salah satu kanal | `info` |
| Job gagal di satu kanal | `warn` |
| Job gagal di semua kanal | `error` |
| Token perangkat dicabut otomatis | `warn` |
| Backpressure aktif | `error` |
| Catch-up pass dijalankan | `info` |
| Streak terlewat memicu eskalasi | `info` |

**Nama obat tidak pernah ditulis ke log.** Pakai `medication_id`.

## 18. Yang Perlu Dikonfirmasi

1. **Kanal email default mati atau hidup?** Rekomendasi: mati.
2. **Pengingat telat dikirim ke pasien juga, atau caregiver saja?**
   Rekomendasi: pasien dulu pada keterlambatan ringan, caregiver setelah `missed`.
3. **Jam sunyi default?** Rekomendasi 22:00–06:00 waktu pengguna.
4. **Telegram bot: satu global atau per user?** Rekomendasi satu bot global
   per instalasi, token di `.env`.
5. **Maksimal 5 percobaan dalam rentang ~3 jam cukup?** Kalau SMTP mati lebih
   lama, angka ini perlu dinaikkan.
