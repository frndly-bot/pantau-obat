# Pantau Obat — Design (UI/UX Dummy, Tanpa Integrasi)

Folder ini berisi **prototipe UI/UX statis** untuk aplikasi Pantau Obat.
Semua angka dan data adalah **dummy** yang hidup di `localStorage` browser.
Gaya antarmuka mengikuti **shadcn/ui** (token oklch, kontrol h-8/h-9, radius
`--radius`, focus ring 3px, tabel bergaris) dengan aksen teal sebagai warna
brand. Ikon memakai **Lucide**, di-inline lewat `assets/icons.js` sehingga
prototipe tetap utuh tanpa internet.
Tidak ada backend, tidak ada API, tidak ada auth nyata.

Tujuan: melihat bentuk antarmuka dan **alur interaksi antar halaman** sebelum
implementasi SvelteKit.

## Struktur

```
design/
  index.html              # hub peluncur + peta interaksi
  README.md               # dokumen ini
  assets/
    tokens.css            # design token gaya shadcn/ui (oklch, radius, kontrol)
    icons.js              # 104 ikon Lucide inline (offline, dari lucide-static)
    app.css               # komponen shadcn + shell user & admin
    data.js               # data dummy + state localStorage
    app.js                # helper bersama: render, toast, nav, drawer, bagan
  _review/                # tangkapan layar hasil verifikasi (bukan bagian UI)
  user/                   # aplikasi user — mobile-first (360px)
    login.html
    register.html
    hari-ini.html
    obat.html
    obat-detail.html
    riwayat.html
    caregiver.html
    pengaturan.html
  admin/                  # aplikasi admin — desktop-first (1280px+)
    login.html
    dashboard.html
    users.html
    user-detail.html
    catalog.html
    catalog-detail.html
    catalog-import.html
    notifications.html
    stats.html
```

## Cara pakai

Buka `design/index.html` langsung di browser (tanpa server), atau sajikan:

```bash
cd design && python3 -m http.server 8099
# lalu buka http://localhost:8099/
```

## Interaksi antar halaman (tanpa integrasi)

State disimpan di `localStorage` lewat `assets/data.js`:

| Aksi | Efek yang persist antar halaman |
|---|---|
| Login user (`budi@example.com`) | masuk `hari-ini.html`, sesi tersimpan |
| Login admin (`admin@example.com`) | masuk `admin/dashboard.html`, role + scope tersimpan |
| Tandai dosis diminum | status kartu berubah, kepatuhan naik, stok turun |
| Tunda (snooze) | badge "ditunda 10 mnt", maksimal 3x |
| Lewati | status `skipped` + alasan |
| Tambah obat (wizard 5 pola) | muncul di `hari-ini.html` dan `obat.html` |
| Restock | saldo stok naik, riwayat transaksi bertambah |
| Undang caregiver | status `pending` → `accepted`/ditolak |
| Admin publish preset | status `draft` → `published`, tercatat di audit |
| Admin disable user | badge nonaktif, statistik berubah |
| Reset data | tombol "Reset dummy" di `index.html` |

Semua tombol yang berada di luar cakupan dummy menampilkan toast
"belum diintegrasi" alih-alih diam, supaya batas jelas.

## Aturan yang dipatuhi

- **Mobile responsive = requirement.** Semua halaman user terbaca di 360px
  tanpa zoom; target sentuh utama ≥ 44px (ditegakkan media query di app.css).
- **Verifikasi nyata.** Prototipe ini diuji lewat Chrome headless: 18 halaman
  tanpa error JS, 27 aksi dummy berpengaruh pada state, overflow 0px di
  360px/390px/1440px, dan tema terang+gelap berganti dengan benar.
- **Aksi dosis selalu terlihat.** Kartu dosis di Hari Ini memakai tombol ikon
  berlabel (Diminum / Tunda / Lewati / Koreksi) yang sejajar satu baris —
  sengaja TIDAK memakai menu tersembunyi "Lainnya" karena menyembunyikan
  aksi yang penting dan menambah ketukan tanpa makna.
- **Jam minum bisa diisi/dikoreksi.** Menekan "Diminum" membuka modal jam
  (default jam sekarang, dengan pintasan "Sekarang"/"Jam jadwal") supaya
  dosis yang sudah diminum tapi belum dicatat tetap tercatat pada jam
  sebenarnya. "Koreksi" membuka modal yang sama untuk mengubah jam atau
  membatalkan catatan. Status `taken`/`late` dihitung dari jam ini terhadap
  toleransi jadwal, bukan dari jam sistem.
- **Enum bahasa Inggris, label bahasa Indonesia.** Database/API pakai
  `after_meal`, `morning`, `viewer`; UI menampilkan "Sesudah makan", "Pagi".
- **Admin tidak melihat isi obat pasien.** Halaman admin hanya agregat dan
  metadata akun. Rincian obat pasien individual tidak pernah ditampilkan.
- **Caregiver viewer read-only.** Tombol tulis tidak muncul saat role `viewer`.
- **Contoh obat memakai nama umum** (Paracetamol, Amlodipine, Metformin)
  sebagai bahan demo, bukan data pasien nyata.

## Cara mengubah ikon

Ikon dibuat otomatis dari `lucide-static` (npm, v1.55.0). Untuk menambah
ikon, petakan slug baru di skrip generator lalu jalankan ulang; jangan
menyunting `assets/icons.js` secara manual.

```bash
# contoh: ambil isi ikon baru dari paket lucide-static
npm pack lucide-static@latest && tar xzf lucide-static-*.tgz
# lihat package/icons/<nama>.svg lalu tambahkan pemetaannya ke icons.js
```

## Yang belum ada (sengaja)

- Integrasi API / backend.
- Auth nyata (login dummy meloloskan kredensial apa pun).
- Service worker / PWA install.
- Grafik yang benar-benar terhubung data server.

Kontrak API acuan: `docs/design-v2/api-v2.md`.