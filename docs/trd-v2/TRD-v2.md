# TRD — Pantau Obat v2

Status: Draft v2, menunggu konfirmasi user
Tanggal: 2026-10-10
Stack: SvelteKit + Svelte 5 + shadcn-svelte | Go 1.27 | PostgreSQL 18 | PWA (native menyusul)
Sumber kebenaran teknis: `backend/migrations/`, `docs/design-v2/data-model-v2.md`

> Dokumen ini menggantikan `docs/trd/TRD-v0.1.md`. Berbeda dari versi sebelumnya,
> TRD ini sudah diselaraskan dengan skema database yang **sudah diuji** di
> PostgreSQL 18.6 (23 tabel, 2 view).

---

## 1. Ringkasan

Pantau Obat adalah aplikasi web untuk membantu orang minum obat sesuai jadwal,
sekaligus memungkinkan caregiver (keluarga, perawat) memantau apakah obat
benar-benar diminum.

Pasien menambah obat beserta jadwalnya, sistem mengirim pengingat, pasien
menandai sudah diminum. Caregiver yang sudah dihubungkan melihat dashboard
kepatuhan tanpa perlu mencatat. Ada juga halaman admin untuk mengelola akun dan
mengurasi katalog obat.

Aplikasi ini alat bantu, bukan alat diagnosis. Tidak ada saran dosis, tidak ada
cek interaksi obat, tidak ada pengganti resep dokter.

## 2. Masalah yang Diselesaikan

1. Pasien lupa minum obat karena tidak ada pengingat yang konsisten.
2. Caregiver tidak tahu obat terlewat sampai terlambat.
3. Aplikasi pengingat obat umumnya tidak punya sisi pemantau untuk keluarga.
4. Riwayat kepatuhan tidak tersimpan sehingga tidak bisa dibawa ke dokter.
5. Jadwal obat nyata tidak sesederhana "sekali sehari" — ada obat tiap 8 jam,
   hari tertentu, siklus (21 hari minum lalu 7 hari jeda), dan "bila perlu".
   Aplikasi yang memaksa satu pola membuat orang berhenti memakainya.
6. Pasien tidak tahu obat akan habis sampai keburu habis.

## 3. Tujuan (v0.1)

- Menambah obat beserta jadwal dalam kurang dari 60 detik per obat.
- Mendukung lima pola frekuensi: harian, interval, mingguan, siklus bulanan, PRN.
- Menandai obat diminum dengan satu ketukan, termasuk dari notifikasi.
- Caregiver melihat status kepatuhan harian dan mingguan tanpa bisa mengubah data.
- Sistem mengirim pengingat sesuai jadwal, dengan snooze.
- Terlewat N kali berturut-turut memberi tahu caregiver.
- Melacak sisa stok dan mengingatkan saat obat hampir habis.
- User bisa memilih obat dari preset katalog atau menambah sendiri.
- Admin bisa mengelola user dan mengurasi preset katalog.
- Data kepatuhan bisa diekspor untuk dibawa ke dokter.

## 4. Non-Tujuan (v0.1)

- Tidak ada diagnosis, saran dosis, atau cek interaksi obat.
- Tidak ada integrasi apotek, tidak ada e-resep.
- Tidak ada integrasi wearables atau EHR.
- Tidak ada reminder berbasis lokasi (geo-fence).
- Tidak ada i18n selain Bahasa Indonesia.
- Tidak ada billing, kuota, atau langganan.
- Tidak ada multi-bahasa pesan error.

## 5. Persona

### 5.1 Pasien (pemilik obat)

- Owner obat. Menambah, mengubah, menghapus obat sendiri.
- Mencatat obat sudah diminum, menunda, atau melewati.
- Mengatur preferensi notifikasi dan jam sunyi sendiri.
- Mengatur stok awal dan mencatat restock.
- Tidak bisa melihat data obat orang lain, kecuali lewat `care_links` yang diterima.

### 5.2 Caregiver / anggota keluarga

- Memantau kepatuhan satu atau lebih pasien.
- `role=viewer`: baca saja terhadap data obat pasien. Tidak boleh menulis apa pun.
- `role=helper`: selain membaca, boleh **mencatat log dosis** atas nama pasien
  (termasuk menunda dan mengoreksi catatan dosis).
  Tidak boleh menambah, mengubah, atau menghapus obat dan jadwal.
- `role=manager`: selain hak `helper`, boleh **mengelola obat dan stok** pasien
  — menambah/mengubah obat, mengatur jam minum, menghentikan (arsip) obat,
  dan mencatat restock atau koreksi stok. Diberikan hanya bila pasien
  menghendakinya; default tetap `viewer`.
  **Status:** ini kontrak yang dituju — belum ada di backend (`enums.go` masih
  `viewer`+`helper`, CHECK `00014_care_links.sql` masih dua nilai). Backend dan
  migrasi perlu disesuaikan; lihat `api-v2.md` bagian 9.
- Menerima notifikasi kalau pasien terlewat N kali berturut-turut.
- Tidak pernah melihat foto resep, catatan pribadi pasien, atau audit log pasien —
  berlaku untuk **ketiga** tingkat izin.

### 5.3 Admin (pengelola aplikasi)

- Mengelola akun: menonaktifkan, mengaktifkan, menghapus atas permintaan.
- Mengurasi preset katalog obat: membuat, mengubah, mempublikasikan, mengarsipkan.
- Melihat statistik agregat (jumlah akun, jumlah obat aktif, tingkat kirim notifikasi).
- Memicu ulang job notifikasi yang gagal.
- **Tidak** melihat isi obat atau log pasien individual lewat rute admin.
  Kalau perlu investigasi, lewat jalur khusus dengan alasan tertulis dan tercatat.
- Peran admin hanya ditetapkan lewat operasi database/seed, tidak lewat API.

## 6. User Stories dan Acceptance Criteria

### Epic A — Autentikasi dan Akun

**A1. Sebagai pengguna baru, saya ingin mendaftar dengan email dan password.**
- AC: Email divalidasi formatnya, password minimal 8 karakter.
- AC: Email duplikat ditolak dengan pesan jelas. Perbandingan tidak peduli huruf besar/kecil (`CITEXT`).
- AC: Password disimpan sebagai hash Argon2id (`m=64MiB, t=3, p=4`), format PHC.
- AC: Setelah daftar, langsung masuk dengan sesi aktif.
- AC: Peran default `user`. Tidak ada endpoint yang menerima `role` dari klien.

**A2. Sebagai pengguna, saya ingin login dan logout.**
- AC: Login gagal untuk email tidak terdaftar maupun password salah, dengan pesan generik yang sama.
- AC: Logout mencabut refresh token di server, bukan hanya cookie di browser.
- AC: Refresh token kedaluwarsa setelah 30 hari, atau saat ganti password.

**A3. Sebagai pengguna, saya ingin ganti password.**
- AC: Butuh password lama yang valid.
- AC: Setelah diganti, seluruh refresh token lain diinvalidasi.

**A4. Sebagai pengguna, saya ingin mengatur timezone.**
- AC: Default `Asia/Jakarta`.
- AC: Semua jadwal dan pengingat mengikuti timezone ini.
- AC: Ganti timezone tidak mengubah log lama; hanya dosis ke depan yang dihitung dengan zona baru.

### Epic B — Manajemen Obat (Pasien)

**B1. Sebagai pasien, saya ingin menambah obat dengan data lengkap.**
- AC: Field wajib: nama, nilai dosis, satuan, rute, pola frekuensi, tanggal mulai.
- AC: Waktu mulai (`start_time`) opsional, default `00:00`.
- AC: Tanggal selesai opsional. Kosong berarti terapi berjalan terus tanpa batas.
- AC: Ada pilihan "harus dihabiskan" (`must_finish`) untuk obat seperti antibiotik.
- AC: Ada pilihan "bila diperlukan" (`as_needed`) untuk obat PRN.
- AC: Ada pilihan waktu makan: `before_meal`, `after_meal`, `with_meal`, `not_related`.
- AC: Ada kolom catatan bebas (`notes`) dan catatan pengingat (`notes_reminder`).
- AC: Ada kolom satuan kekuatan (`strength_note`) untuk bentuk seperti `120mg/5mL`.

**B2. Sebagai pasien, saya ingin memilih obat dari preset katalog.**
- AC: Pencarian katalog memakai nama dagang, nama generik, nama resep (`rxterm`), dan nama ATC.
- AC: Memilih preset mengisi form, tapi tidak menyimpan obat secara diam-diam.
- AC: Preset hanya bisa dibaca; user tidak bisa mengubah preset publik.

**B3. Sebagai pasien, saya ingin menambah obat yang tidak ada di preset.**
- AC: Obat yang ditambah manual disimpan sebagai katalog pribadi milik user itu.
- AC: Katalog pribadi **tidak** pernah muncul di preset publik atau untuk user lain.
- AC: Nama boleh sama dengan milik user lain (keunikan per user).

**B4. Sebagai pasien, saya ingin memakai template untuk isi cepat.**
- AC: Template berisi nama generik, dosis lazim, rute, dan pola awal.
- AC: Ada template sistem dan template pribadi.
- AC: Template hanya mengisi form. Tidak menyimpan obat secara diam-diam.

**B5. Sebagai pasien, saya ingin mengubah dan menghapus obat.**
- AC: Edit tidak menghapus riwayat pemakaian yang sudah tercatat.
- AC: Hapus berarti soft delete (`deleted_at`). Riwayat tetap bisa dilihat di arsip.
- AC: Obat yang dihapus tidak lagi menerima pengingat.
- AC: Obat yang dihapus boleh didaftarkan ulang dengan nama sama.

**B6. Sebagai pasien, saya ingin memberi warna pada obat.**
- AC: Warna disimpan per obat (`color_tag`, format `#rrggbb`).
- AC: Dipakai konsisten di daftar, jadwal, dan dashboard caregiver.

### Epic C — Pola Frekuensi (BARU)

Ini inti perubahan v2. Jadwal disimpan sebagai **aturan**, bukan baris per hari.

**C1. Sebagai pasien, saya ingin mengatur obat sekian kali per hari.**
- AC: Pola `daily` dengan `times_per_day` 1–24.
- AC: Jam konkret diisi satu per satu: jam dan menit, plus label `morning`/`afternoon`/`evening`/`dawn`/`other`.
- AC: Contoh: 3× sehari jam 07:00 (morning), 13:00 (afternoon), 19:00 (evening).

**C2. Sebagai pasien, saya ingin mengatur obat tiap sekian jam.**
- AC: Pola `interval` dengan `interval_hours` 1–168.
- AC: Contoh: tiap 8 jam mulai jam 06:00.
- AC: Boleh menyimpan rentang (`interval_min_hours`) untuk informasi saja.

**C3. Sebagai pasien, saya ingin mengatur obat pada hari tertentu saja.**
- AC: Pola `weekly` dengan `days_of_week` sebagai bitmask (Senin=1 … Minggu=64).
- AC: Minimal satu hari dipilih.
- AC: Contoh: Senin dan Kamis.

**C4. Sebagai pasien, saya ingin mengatur obat bersiklus.**
- AC: Pola `monthly_cycle` dengan `cycle_intake_days` (1–365) dan `cycle_pause_days` (0–365).
- AC: `cycle_anchor` menentukan titik mulai: `from_start`, `dom`, atau `end_of_month`.
- AC: Contoh: 21 hari minum lalu 7 hari jeda. Contoh lain: hanya akhir bulan.
- AC: Saat masa jeda, tidak ada pengingat dan tidak dihitung terlewat.

**C5. Sebagai pasien, saya ingin obat yang diminum hanya bila diperlukan.**
- AC: Pola `prn`. Tidak ada jadwal tetap.
- AC: `prn_max_per_day` opsional sebagai batas peringatan.
- AC: Log PRN dibuat tanpa `schedule_id` dan boleh berkali-kali dalam sehari.
- AC: Log PRN tidak menyebabkan dosis lain dianggap terlewat.

**C6. Sistem menghitung waktu dosis dari aturan, tanpa menyimpan baris per hari.**
- AC: Terapi tanpa `end_date` ("selamanya") hanya memakai 1 baris pola + N baris jam.
- AC: Menambah hari tidak menambah baris apa pun.
- AC: Baris baru hanya lahir saat pasien benar-benar mencatat dosis.
- AC: Jam dosis memakai timezone pemilik.
- AC: Zona saat blok jadwal dibuat disimpan (`schedule_timezone`) agar perubahan zona tidak mengubah tanggal dosis yang sudah lewat.

### Epic D — Pencatatan Kepatuhan

**D1. Sebagai pasien, saya ingin menandai obat sudah diminum.**
- AC: Satu ketukan dari halaman utama atau dari notifikasi.
- AC: Server menyimpan waktu aktual (`taken_at`), bukan waktu kiriman klien.
- AC: Status `taken` bila selisih ≤ 60 menit dari jadwal; `late` bila lebih.

**D2. Saya bisa menunda pengingat (snooze).**
- AC: Pilihan 5, 10, atau 30 menit.
- AC: Pengingat ulang dikirim pada waktu snooze.
- AC: Maksimal 3 snooze beruntun per dosis. Setelah itu dosis dianggap terlewat.
- AC: Snooze disimpan sebagai satu baris di `bump_queue` (`kind='dose'`, `skip_tag='snooze'`).
  Snooze kedua memperbarui baris itu, bukan menambah baris baru.

**D3. Saya bisa menandai obat dilewati.**
- AC: Alasan opsional: lupa, muntah, obat tidak tersedia, lainnya.
- AC: Status `skipped` disimpan di `medication_logs` dengan `taken_at` NULL.

**D4. Sistem menandai dosis yang terlewat.**
- AC: Pass harian (tengah malam waktu pemilik) memeriksa dosis hari itu yang lewat jam tanpa log.
- AC: Yang terlewat dicatat di `medication_dose_events` dengan `kind='missed'`.
- AC: Kunci `event_key` (`schedule|tanggal|jam:menit`) mencegah pencatatan ganda.
- AC: Hari dalam masa jeda siklus atau tanggal di luar rentang terapi tidak dihitung terlewat.

**D5. Saya bisa mengoreksi catatan yang salah.**
- AC: Koreksi tidak menghapus fisik. Baris lama di-soft-delete, baris baru dibuat dalam satu transaksi.
- AC: Nilai lama tersimpan di audit log.

**D6. Satu dosis tidak boleh punya dua log aktif.**
- AC: Partial unique index pada `(medication_id, schedule_id, slot_date, scheduled_hour, scheduled_minute)` untuk log hidup.
- AC: Log PRN (`schedule_id` NULL) dikecualikan sehingga boleh banyak per hari.

**D7. Kalau jadwal berubah, dosis lama tetap utuh.**
- AC: Mengubah jam jadwal hanya berlaku untuk dosis ke depan.
- AC: Perubahan pola dicatat sebagai `medication_dose_events` dengan `kind='dose_shift'`.

### Epic E — Stok dan Restock (BARU)

**E1. Sebagai pasien, saya ingin mengisi stok awal obat.**
- AC: Saat menambah obat, isi `initial_qty` dan satuan.
- AC: Isi `qty_per_dose` sebagai jumlah yang dipakai per dosis.

**E2. Sistem mengurangi stok otomatis saat dosis dicatat.**
- AC: Menandai "sudah diminum" mengurangi `current_qty` sebanyak `qty_per_dose`.
- AC: Setiap perubahan stok menulis satu baris `stock_transactions`.
- AC: `balance_after` diisi otomatis oleh trigger, bukan oleh aplikasi.
- AC: Saldo tidak boleh negatif. Percobaan mengurangi melebihi saldo ditolak.

**E3. Saya bisa mencatat restock.**
- AC: Restock menambah `current_qty` dan mengisi `last_restocked_at`.
- AC: Setiap restock menulis baris `stock_transactions` dengan `kind='restock'`.

**E4. Sistem mengingatkan saat obat hampir habis.**
- AC: Pasien menetapkan ambang (`refill_threshold_qty`) dan menyalakan `refill_reminder`.
- AC: Saat saldo ≤ ambang, sistem membuat job `refill_reminder`.
- AC: Sistem memperkirakan berapa hari lagi obat habis dari laju pemakaian.

**E5. Saya bisa menyesuaikan stok manual.**
- AC: Ada jenis transaksi `adjust` untuk koreksi dan `expire` untuk obat kedaluwarsa.
- AC: Keduanya tercatat lengkap dengan alasan.

### Epic F — Caregiver

**F1. Sebagai pasien, saya ingin mengundang caregiver lewat email.**
- AC: Undangan memakai token satu pakai, berlaku 7 hari, bisa dibatalkan.
- AC: Boleh mengundang email yang belum terdaftar (`caregiver_id` NULL saat itu).
- AC: Satu pasangan pasien–caregiver hanya boleh punya satu relasi aktif.

**F2. Sebagai caregiver, saya ingin melihat dashboard kepatuhan pasien.**
- AC: Persentase kepatuhan 7 hari dan 30 hari.
- AC: Daftar obat beserta status hari ini.
- AC: Timeline minum harian.
- AC: Tidak ada aksi tulis selama izin `viewer`.

**F3. Sebagai caregiver `helper`, saya ingin mencatat dosis atas nama pasien.**
- AC: Log tercatat dengan `source='caregiver'` dan `recorded_by` = id caregiver.
- AC: Status yang didukung: diminum, terlambat, ditunda, dilewati, dan koreksi.
- AC: Hanya boleh pada relasi yang sudah `accepted`, dan diverifikasi tiap request.
- AC: Tidak boleh mengelola obat maupun jadwal.

**F3b. Sebagai caregiver `manager`, saya ingin mengelola obat dan stok pasien.**
- AC: Boleh menambah dan mengubah obat pasien, termasuk jam minumnya.
- AC: Boleh menghentikan (arsip) obat tanpa menghapus riwayat kepatuhan.
- AC: Boleh mencatat restock dan koreksi stok; saldo tidak boleh negatif.
- AC: Semua perubahan obat/stok menulis audit dengan `actor_id` caregiver.
- AC: Pasien tetap melihat siapa yang mengubah datanya.
- AC: Tidak boleh melihat foto resep, catatan pribadi pasien, maupun audit log pasien.

**F4. Sebagai caregiver, saya menerima peringatan saat pasien terlewat N kali.**
- AC: Default N = 3 dosis berturut-turut terlewat.
- AC: Notifikasi dikirim ke kanal caregiver sesuai preferensinya.
- AC: Maksimal satu eskalasi per hari per pasien.

**F5. Sebagai pasien, saya ingin mengatur siapa saja yang boleh memantau.**
- AC: Tiga tingkat izin: `viewer`, `helper`, dan `manager`. Default `viewer`.
- AC: Menaikkan ke `manager` adalah keputusan sadar pasien; UI menjelaskan
  bedanya sebelum pasien menyimpan.
- AC: Pasien bisa menurunkan tingkat izin kapan saja; berlaku pada request berikutnya.

**F6. Sebagai pasien, saya bisa mencabut akses caregiver.**
- AC: Pencabutan mengubah status menjadi `revoked`; akses langsung hilang.

### Epic G — Notifikasi

**G1. Sistem mengirim pengingat pada jam minum.**
- AC: Satu pengingat per dosis.
- AC: Ada opsi maju lebih awal (`advance_minutes`, 0–120).
- AC: Jam sunyi (`quiet_start`/`quiet_end`) menahan pengiriman sampai lewat.

**G2. Saya bisa snooze dari notifikasi.**
- AC: Aksi snooze tersedia langsung di notifikasi web push.

**G3. Sistem mengirim ringkasan harian.**
- AC: Default pukul 20:00 waktu lokal.
- AC: Isinya daftar obat diminum dan terlewat hari itu.

**G4. Notifikasi tidak boleh spam.**
- AC: Deduplikasi per kombinasi dosis, kanal, dan jenis pesan (`dedup_key`).
- AC: Retry dengan backoff, maksimal 5 percobaan.
- AC: Kalau semua kanal gagal, job ditandai `failed` beserta alasannya.

**G5. Sistem mengingatkan restock.**
- AC: Job `refill_reminder` terpisah dari pengingat dosis.

### Epic H — Laporan

**H1. Sebagai pasien, saya ingin melihat statistik kepatuhan.**
- AC: Angka kepatuhan harian, mingguan, dan bulanan.
- AC: Menampilkan obat mana yang paling sering terlewat.

**H2. Sebagai pasien, saya ingin mengekspor data.**
- AC: Ekspor CSV riwayat pemakaian untuk dibawa ke dokter.
- AC: Ekspor hanya milik sendiri.

### Epic I — Admin (BARU)

**I1. Sebagai admin, saya ingin mengelola akun pengguna.**
- AC: Melihat daftar akun dengan pencarian dan filter.
- AC: Menonaktifkan akun (`is_active=false`) tanpa mengubah peran.
- AC: Mengaktifkan kembali akun.
- AC: Setiap aksi menulis audit log dengan `actor_role='admin'`.

**I2. Sebagai admin, saya ingin mengurasi preset katalog obat.**
- AC: Membuat, mengubah, mempublikasikan, dan mengarsipkan preset.
- AC: Preset punya siklus status: `draft` → `published` → `archived`.
- AC: Perubahan status tercatat dengan nilai sebelum dan sesudah.
- AC: Preset yang sudah dipublikasikan boleh diarsipkan, tidak boleh dihapus permanen.

**I3. Sebagai admin, saya ingin mengimpor data katalog.**
- AC: Importer menulis dengan `source='kfa'` atau `'bpom'`.
- AC: Importer bersifat idempoten: menjalankan dua kali tidak menggandakan data.
- AC: Field yang belum dipetakan disimpan di `extra_meta`, tidak dibuang.

**I4. Sebagai admin, saya ingin melihat agregat.**
- AC: Jumlah akun aktif, jumlah obat aktif, jumlah job notifikasi gagal.
- AC: Tidak menampilkan nama obat atau identitas pasien individual.

**I5. Sebagai admin, saya ingin memicu ulang job notifikasi gagal.**
- AC: Aksi tercatat di audit log.
- AC: Boleh dibatasi per `admin_scope`.

**I6. Admin terbatas sesuai scope.**
- AC: `admin_scope` berisi subset `{users, catalog, notifications, reports}`.
- AC: `admin_scope` NULL berarti akses penuh.
- AC: Admin dengan scope `catalog` tidak bisa mengakses rute `/admin/users`.
- AC: Percobaan melampaui scope dibalas `403 FORBIDDEN`.

### Epic J — Mobile dan Desktop

**J1. Aplikasi user nyaman dipakai di layar 360px.**
- AC: Semua halaman utama terbaca dan dapat dioperasikan tanpa zoom.
- AC: Bottom navigation untuk aksi utama di layar kecil.
- AC: Di layar lebar, navigasi berpindah ke sidebar tanpa kehilangan fungsi.

**J2. Aplikasi admin nyaman dipakai di desktop.**
- AC: Sidebar tetap, tabel lebar, filter dan kolom banyak.
- AC: Tetap bisa dipakai di tablet dan ponsel untuk aksi darurat, meski bukan prioritas.

**J3. PWA installable dan bisa ditambah ke layar utama.**
- AC: Manifest valid, service worker menyimpan app shell.
- AC: Push notification berjalan setelah izin diberikan.

**J4. Rencana native setelah v1 stabil.**
- AC: API dan design system tidak terikat pada web, supaya bungkus Capacitor bisa dipakai nanti.

## 7. Kebutuhan Non-Fungsional

### 7.1 Performa

- p95 latency API baca di bawah 200ms, tulis di bawah 300ms.
- Halaman utama mobile muat di bawah 2,5 detik pada jaringan 4G.
- Worker memindai dosis jatuh tempo dalam 15 menit ke depan.
- Perhitungan dosis dari aturan harus O(jumlah jam per obat), bukan O(jumlah hari terapi).

### 7.2 Keamanan

- Hash Argon2id: memory 64MB, iterations 3, parallelism 2.
- Sesi lewat cookie httpOnly, Secure, SameSite=Lax. Refresh token disimpan di DB, bukan localStorage.
- Semua endpoint menolak akses ke obat milik user lain.
- Endpoint caregiver memverifikasi `care_links` aktif pada tiap request, bukan hanya di UI.
- Rate limit: login 5 per menit per IP, register 3 per jam per email, API umum 120 per menit.
- Audit log untuk setiap akses baca data obat oleh caregiver.
- Nama obat tidak boleh muncul di log aplikasi.
- HTTPS wajib di produksi.
- **Rute admin terpisah** (`/admin/*`) dengan middleware `RequireAdmin(scope)`.
- **Admin tidak boleh membaca isi obat pasien** lewat rute admin.
- Peran admin tidak bisa diberikan lewat API; hanya lewat operasi DB/seed.
- Setiap tulisan admin menulis audit dengan `actor_role='admin'` dan `before_state`/`after_state`.
- `actor_role` disalin saat aksi terjadi agar jejak tidak berubah bila peran user berubah.

### 7.3 Privasi

- Data kesehatan bersifat sensitif, tidak dikirim ke analytics pihak ketiga.
- Retensi: akun tidak aktif lebih dari 2 tahun diberi kesempatan ekspor, lalu hard delete atas permintaan.
- Penghapusan akun: soft delete 30 hari, lalu purge.
- Admin hanya melihat data agregat, bukan isi obat individual.

### 7.4 Aksesibilitas

- Kontras teks minimal WCAG AA.
- Semua tombol punya label yang terbaca screen reader.
- Form punya label eksplisit, pesan error terhubung ke input terkait.
- Target sentuh minimal 44×44 px di aplikasi user.

### 7.5 Reliability

- Backup Postgres harian, retensi 30 hari.
- Healthcheck endpoint untuk API dan koneksi DB.
- Graceful shutdown: proses menyelesaikan request yang sedang berjalan sebelum berhenti.
- Kegagalan notifikasi tidak memblokir pencatatan dosis.

## 8. Lingkup Versi

### v0.1 — MVP

Auth, obat, lima pola frekuensi, pencatatan, pengingat web push, caregiver
(`viewer` + `helper` + `manager`), stok dan peringatan restock, preset katalog dengan
katalog pribadi user, halaman admin dasar (kelola user + kurasi preset),
statistik dasar.

### v0.2

Kanal Telegram dan email, ringkasan harian, eskalasi caregiver lanjutan,
ekspor CSV, tuning jam sunyi per kanal.

### v0.3

Polish PWA dengan cache offline, bungkus native Capacitor, laporan adherence
bulanan, materialized view untuk laporan berat.

### Tidak direncanakan

Integrasi wearable, EHR, apotek online, saran obat berbasis AI, multi-bahasa.

## 9. Keputusan yang Sudah Diambil

Semua yang tadinya terbuka di TRD v0.1 sudah diputuskan:

1. **Timezone** — default `Asia/Jakarta`, diatur per user. Caregiver menerima
   notifikasi memakai timezone pasien, bukan timezone sendiri.
2. **Izin caregiver** — `viewer`, `helper`, dan `manager` ada di v0.1.
   `manager` adalah perluasan sadar atas `helper`: ia boleh mengelola obat dan
   stok pasien, bukan hanya mencatat dosis.
3. **Kanal notifikasi awal** — web push saja untuk v0.1. Telegram dan email di v0.2.
4. **Visibilitas data** — caregiver melihat nama obat, tetapi tidak foto resep
   dan tidak catatan pribadi pasien.
5. **Skala** — perkiraan v1: ratusan pengguna aktif. Index sudah disiapkan
   untuk pertumbuhan, bukan untuk skala besar.

## 10. Risiko

| Risiko | Dampak | Mitigasi |
|---|---|---|
| Pasien lupa membuka aplikasi | Fitur inti gagal | Notifikasi push, halaman utama sesederhana mungkin |
| Terlalu banyak notifikasi | User mematikan izin atau uninstall | Deduplikasi, jam sunyi, pengaturan granular |
| Pencatatan tidak akurat | Data kepatuhan tidak dipercaya | Konfirmasi eksplisit, caregiver bisa melihat pola |
| Data kesehatan bocor | Risiko hukum dan privasi | Enkripsi transit, akses ketat, audit log, admin tidak lihat isi |
| Pola frekuensi rumit membingungkan user | User berhenti memakai | Template dan preset mengisi pola lazim; form bertahap |
| Admin salah mengubah preset publik | Data katalog rusak | Siklus status draft → published → archived, before/after tercatat |
| Worker mati | Pengingat berhenti, dosis terlewat tidak tercatat | Healthcheck, restart policy, catch-up pass saat start |
| Scope creep | Proyek melambat | Non-tujuan eksplisit di bagian 4 |

## 11. Dokumen Terhubung

- `docs/design-v2/architecture.html` — arsitektur sistem, struktur folder, deployment
- `docs/design-v2/data-model.html` — entitas database lengkap
- `docs/design-v2/api.html` — kontrak endpoint
- `docs/design-v2/notification.html` — strategi pengingat dan antrean
- `docs/design-v2/erd.html` — diagram relasi visual
- `docs/decisions-v2/` — catatan keputusan arsitektur
