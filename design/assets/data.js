/* ============================================================
   Pantau Obat — Design v2 · Data dummy + state localStorage
   Semua enum BAHASA INGGRIS (sama dengan CHECK di migrasi goose).
   Terjemahan ke Bahasa Indonesia dilakukan di UI (lihat LABEL).
   Tidak ada backend, tidak ada API, tidak ada auth nyata.
   ============================================================ */
(function () {
  "use strict";

  var KEY = "po.design.v2";

  /* ---------- terjemahan enum (EN -> ID) ---------- */
  var LABEL = {
    meal_relation: { before_meal: "Sebelum makan", after_meal: "Sesudah makan", with_meal: "Bersama makan", not_related: "Tidak terkait makan" },
    label: { morning: "Pagi", afternoon: "Siang", evening: "Malam", dawn: "Subuh", other: "Lainnya" },
    pattern: { daily: "Sekian kali sehari", interval: "Tiap sekian jam", weekly: "Hari tertentu", monthly_cycle: "Siklus (minum–jeda)", prn: "Bila perlu (PRN)" },
    status: { taken: "Diminum", late: "Terlambat", skipped: "Dilewati", snoozed: "Ditunda", pending: "Belum", missed: "Terlewat" },
    relation: { family: "Keluarga", child: "Anak", parent: "Orang tua", spouse: "Pasangan", friend: "Sahabat", nurse: "Perawat", other: "Lainnya" },
    care_role: { viewer: "Baca saja", helper: "Bisa edit" },
    care_status: { pending: "Menunggu", accepted: "Diterima", rejected: "Ditolak", revoked: "Dicabut", expired: "Kedaluwarsa" },
    curation_status: { draft: "Draf", published: "Terbit", archived: "Arsip", user_private: "Pribadi" },
    source: { kfa: "KFA Kemenkes", bpom: "BPOM", manual: "Manual", other: "Lainnya" },
    channel: { push: "Notifikasi push", email: "Email", telegram: "Telegram", in_app: "Dalam aplikasi" },
    job_kind: { dose_reminder: "Pengingat dosis", missed_escalation: "Eskalasi terlewat", care_digest: "Ringkasan caregiver", retention: "Retensi data", refill_reminder: "Peringatan restock" },
    job_status: { pending: "Menunggu", claimed: "Diproses", done: "Selesai", failed: "Gagal", cancelled: "Dibatalkan" },
    delivery_status: { pending: "Menunggu", sent: "Terkirim", failed: "Gagal", skipped: "Dilewati" },
    route: { O: "Mulut", T: "Topikal", S: "Injeksi", I: "Inhalasi", R: "Rektal", N: "Hidung", E: "Mata" },
    unit: { tablet: "Tablet", kapsul: "Kapsul", kaplet: "Kaplet", mg: "mg", mL: "mL", sachet: "Sachet", tetes: "Tetes", puff: "Puff", IU: "IU", unit: "Unit", spuit: "Spuit", oles: "Oles" },
    action: {
      login: "Masuk", logout: "Keluar", create: "Membuat", read: "Membaca", update: "Mengubah", delete: "Menghapus",
      invite: "Mengundang", accept: "Menerima", revoke: "Mencabut", export: "Mengekspor", log_taken: "Catat diminum",
      log_skipped: "Catat dilewati", restock: "Restock", catalog_search: "Cari katalog", admin_grant: "Beri peran admin",
      admin_revoke: "Cabut peran admin", user_disable: "Nonaktifkan akun", user_enable: "Aktifkan akun",
      user_erase: "Hapus akun", catalog_create: "Buat preset", catalog_update: "Ubah preset",
      catalog_publish: "Terbitkan preset", catalog_unpublish: "Tarik preset", catalog_archive: "Arsipkan preset",
      catalog_import: "Impor katalog", notif_retry: "Ulangi job", notif_cancel: "Batalkan job", retention_purge: "Purge retensi",
      dose_shift: "Jadwal bergeser", missed: "Terlewat"
    }
  };

  /* ---------- waktu dummy (relatif hari ini) ---------- */
  var NOW = new Date();
  function iso(d) { return d.toISOString(); }
  function ymd(d) {
    return d.getFullYear() + "-" + String(d.getMonth() + 1).padStart(2, "0") + "-" + String(d.getDate()).padStart(2, "0");
  }
  function shiftDays(n, h, m) {
    var d = new Date(NOW);
    d.setDate(d.getDate() + n);
    if (h !== undefined) d.setHours(h, m || 0, 0, 0);
    return d;
  }
  var TODAY = ymd(NOW);

  var HARI = ["Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"];
  var BULAN = ["Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"];

  /* ---------- zona waktu IANA (untuk pemilih di Pengaturan) ---------- */
  var TIMEZONES = [
    ["UTC","UTC+0"],
    ["Africa/Abidjan","UTC+0"],
    ["Africa/Accra","UTC+0"],
    ["Africa/Addis_Ababa","UTC+3"],
    ["Africa/Algiers","UTC+1"],
    ["Africa/Asmera","UTC+3"],
    ["Africa/Bamako","UTC+0"],
    ["Africa/Bangui","UTC+1"],
    ["Africa/Banjul","UTC+0"],
    ["Africa/Bissau","UTC+0"],
    ["Africa/Blantyre","UTC+2"],
    ["Africa/Brazzaville","UTC+1"],
    ["Africa/Bujumbura","UTC+2"],
    ["Africa/Cairo","UTC+2 / UTC+3"],
    ["Africa/Casablanca","UTC+1"],
    ["Africa/Ceuta","UTC+1 / UTC+2"],
    ["Africa/Conakry","UTC+0"],
    ["Africa/Dakar","UTC+0"],
    ["Africa/Dar_es_Salaam","UTC+3"],
    ["Africa/Djibouti","UTC+3"],
    ["Africa/Douala","UTC+1"],
    ["Africa/El_Aaiun","UTC+1"],
    ["Africa/Freetown","UTC+0"],
    ["Africa/Gaborone","UTC+2"],
    ["Africa/Harare","UTC+2"],
    ["Africa/Johannesburg","UTC+2"],
    ["Africa/Juba","UTC+2"],
    ["Africa/Kampala","UTC+3"],
    ["Africa/Khartoum","UTC+2"],
    ["Africa/Kigali","UTC+2"],
    ["Africa/Kinshasa","UTC+1"],
    ["Africa/Lagos","UTC+1"],
    ["Africa/Libreville","UTC+1"],
    ["Africa/Lome","UTC+0"],
    ["Africa/Luanda","UTC+1"],
    ["Africa/Lubumbashi","UTC+2"],
    ["Africa/Lusaka","UTC+2"],
    ["Africa/Malabo","UTC+1"],
    ["Africa/Maputo","UTC+2"],
    ["Africa/Maseru","UTC+2"],
    ["Africa/Mbabane","UTC+2"],
    ["Africa/Mogadishu","UTC+3"],
    ["Africa/Monrovia","UTC+0"],
    ["Africa/Nairobi","UTC+3"],
    ["Africa/Ndjamena","UTC+1"],
    ["Africa/Niamey","UTC+1"],
    ["Africa/Nouakchott","UTC+0"],
    ["Africa/Ouagadougou","UTC+0"],
    ["Africa/Porto-Novo","UTC+1"],
    ["Africa/Sao_Tome","UTC+0"],
    ["Africa/Tripoli","UTC+2"],
    ["Africa/Tunis","UTC+1"],
    ["Africa/Windhoek","UTC+2"],
    ["America/Adak","UTC-10 / UTC-9"],
    ["America/Anchorage","UTC-9 / UTC-8"],
    ["America/Anguilla","UTC-4"],
    ["America/Antigua","UTC-4"],
    ["America/Araguaina","UTC-3"],
    ["America/Argentina/La_Rioja","UTC-3"],
    ["America/Argentina/Rio_Gallegos","UTC-3"],
    ["America/Argentina/Salta","UTC-3"],
    ["America/Argentina/San_Juan","UTC-3"],
    ["America/Argentina/San_Luis","UTC-3"],
    ["America/Argentina/Tucuman","UTC-3"],
    ["America/Argentina/Ushuaia","UTC-3"],
    ["America/Aruba","UTC-4"],
    ["America/Asuncion","UTC-3"],
    ["America/Bahia","UTC-3"],
    ["America/Bahia_Banderas","UTC-6"],
    ["America/Barbados","UTC-4"],
    ["America/Belem","UTC-3"],
    ["America/Belize","UTC-6"],
    ["America/Blanc-Sablon","UTC-4"],
    ["America/Boa_Vista","UTC-4"],
    ["America/Bogota","UTC-5"],
    ["America/Boise","UTC-7 / UTC-6"],
    ["America/Buenos_Aires","UTC-3"],
    ["America/Cambridge_Bay","UTC-7 / UTC-6"],
    ["America/Campo_Grande","UTC-4"],
    ["America/Cancun","UTC-5"],
    ["America/Caracas","UTC-4"],
    ["America/Catamarca","UTC-3"],
    ["America/Cayenne","UTC-3"],
    ["America/Cayman","UTC-5"],
    ["America/Chicago","UTC-6 / UTC-5"],
    ["America/Chihuahua","UTC-6"],
    ["America/Ciudad_Juarez","UTC-7 / UTC-6"],
    ["America/Coral_Harbour","UTC-5"],
    ["America/Cordoba","UTC-3"],
    ["America/Costa_Rica","UTC-6"],
    ["America/Coyhaique","UTC-3"],
    ["America/Creston","UTC-7"],
    ["America/Cuiaba","UTC-4"],
    ["America/Curacao","UTC-4"],
    ["America/Danmarkshavn","UTC+0"],
    ["America/Dawson","UTC-7"],
    ["America/Dawson_Creek","UTC-7"],
    ["America/Denver","UTC-7 / UTC-6"],
    ["America/Detroit","UTC-5 / UTC-4"],
    ["America/Dominica","UTC-4"],
    ["America/Edmonton","UTC-7 / UTC-6"],
    ["America/Eirunepe","UTC-5"],
    ["America/El_Salvador","UTC-6"],
    ["America/Fort_Nelson","UTC-7"],
    ["America/Fortaleza","UTC-3"],
    ["America/Glace_Bay","UTC-4 / UTC-3"],
    ["America/Godthab","UTC-2 / UTC-1"],
    ["America/Goose_Bay","UTC-4 / UTC-3"],
    ["America/Grand_Turk","UTC-5 / UTC-4"],
    ["America/Grenada","UTC-4"],
    ["America/Guadeloupe","UTC-4"],
    ["America/Guatemala","UTC-6"],
    ["America/Guayaquil","UTC-5"],
    ["America/Guyana","UTC-4"],
    ["America/Halifax","UTC-4 / UTC-3"],
    ["America/Havana","UTC-5 / UTC-4"],
    ["America/Hermosillo","UTC-7"],
    ["America/Indiana/Knox","UTC-6 / UTC-5"],
    ["America/Indiana/Marengo","UTC-5 / UTC-4"],
    ["America/Indiana/Petersburg","UTC-5 / UTC-4"],
    ["America/Indiana/Tell_City","UTC-6 / UTC-5"],
    ["America/Indiana/Vevay","UTC-5 / UTC-4"],
    ["America/Indiana/Vincennes","UTC-5 / UTC-4"],
    ["America/Indiana/Winamac","UTC-5 / UTC-4"],
    ["America/Indianapolis","UTC-5 / UTC-4"],
    ["America/Inuvik","UTC-7 / UTC-6"],
    ["America/Iqaluit","UTC-5 / UTC-4"],
    ["America/Jamaica","UTC-5"],
    ["America/Jujuy","UTC-3"],
    ["America/Juneau","UTC-9 / UTC-8"],
    ["America/Kentucky/Monticello","UTC-5 / UTC-4"],
    ["America/Kralendijk","UTC-4"],
    ["America/La_Paz","UTC-4"],
    ["America/Lima","UTC-5"],
    ["America/Los_Angeles","UTC-8 / UTC-7"],
    ["America/Louisville","UTC-5 / UTC-4"],
    ["America/Lower_Princes","UTC-4"],
    ["America/Maceio","UTC-3"],
    ["America/Managua","UTC-6"],
    ["America/Manaus","UTC-4"],
    ["America/Marigot","UTC-4"],
    ["America/Martinique","UTC-4"],
    ["America/Matamoros","UTC-6 / UTC-5"],
    ["America/Mazatlan","UTC-7"],
    ["America/Mendoza","UTC-3"],
    ["America/Menominee","UTC-6 / UTC-5"],
    ["America/Merida","UTC-6"],
    ["America/Metlakatla","UTC-9 / UTC-8"],
    ["America/Mexico_City","UTC-6"],
    ["America/Miquelon","UTC-3 / UTC-2"],
    ["America/Moncton","UTC-4 / UTC-3"],
    ["America/Monterrey","UTC-6"],
    ["America/Montevideo","UTC-3"],
    ["America/Montserrat","UTC-4"],
    ["America/Nassau","UTC-5 / UTC-4"],
    ["America/New_York","UTC-5 / UTC-4"],
    ["America/Nome","UTC-9 / UTC-8"],
    ["America/Noronha","UTC-2"],
    ["America/North_Dakota/Beulah","UTC-6 / UTC-5"],
    ["America/North_Dakota/Center","UTC-6 / UTC-5"],
    ["America/North_Dakota/New_Salem","UTC-6 / UTC-5"],
    ["America/Ojinaga","UTC-6 / UTC-5"],
    ["America/Panama","UTC-5"],
    ["America/Paramaribo","UTC-3"],
    ["America/Phoenix","UTC-7"],
    ["America/Port-au-Prince","UTC-5 / UTC-4"],
    ["America/Port_of_Spain","UTC-4"],
    ["America/Porto_Velho","UTC-4"],
    ["America/Puerto_Rico","UTC-4"],
    ["America/Punta_Arenas","UTC-3"],
    ["America/Rankin_Inlet","UTC-6 / UTC-5"],
    ["America/Recife","UTC-3"],
    ["America/Regina","UTC-6"],
    ["America/Resolute","UTC-6 / UTC-5"],
    ["America/Rio_Branco","UTC-5"],
    ["America/Santarem","UTC-3"],
    ["America/Santiago","UTC-3 / UTC-4"],
    ["America/Santo_Domingo","UTC-4"],
    ["America/Sao_Paulo","UTC-3"],
    ["America/Scoresbysund","UTC-2 / UTC-1"],
    ["America/Sitka","UTC-9 / UTC-8"],
    ["America/St_Barthelemy","UTC-4"],
    ["America/St_Johns","UTC-3:30 / UTC-2:30"],
    ["America/St_Kitts","UTC-4"],
    ["America/St_Lucia","UTC-4"],
    ["America/St_Thomas","UTC-4"],
    ["America/St_Vincent","UTC-4"],
    ["America/Swift_Current","UTC-6"],
    ["America/Tegucigalpa","UTC-6"],
    ["America/Thule","UTC-4 / UTC-3"],
    ["America/Tijuana","UTC-8 / UTC-7"],
    ["America/Toronto","UTC-5 / UTC-4"],
    ["America/Tortola","UTC-4"],
    ["America/Vancouver","UTC-8 / UTC-7"],
    ["America/Whitehorse","UTC-7"],
    ["America/Winnipeg","UTC-6 / UTC-5"],
    ["America/Yakutat","UTC-9 / UTC-8"],
    ["Antarctica/Casey","UTC+8"],
    ["Antarctica/Davis","UTC+7"],
    ["Antarctica/DumontDUrville","UTC+10"],
    ["Antarctica/Macquarie","UTC+11 / UTC+10"],
    ["Antarctica/Mawson","UTC+5"],
    ["Antarctica/McMurdo","UTC+13 / UTC+12"],
    ["Antarctica/Palmer","UTC-3"],
    ["Antarctica/Rothera","UTC-3"],
    ["Antarctica/Syowa","UTC+3"],
    ["Antarctica/Troll","UTC+0 / UTC+2"],
    ["Antarctica/Vostok","UTC+5"],
    ["Arctic/Longyearbyen","UTC+1 / UTC+2"],
    ["Asia/Aden","UTC+3"],
    ["Asia/Almaty","UTC+5"],
    ["Asia/Amman","UTC+3"],
    ["Asia/Anadyr","UTC+12"],
    ["Asia/Aqtau","UTC+5"],
    ["Asia/Aqtobe","UTC+5"],
    ["Asia/Ashgabat","UTC+5"],
    ["Asia/Atyrau","UTC+5"],
    ["Asia/Baghdad","UTC+3"],
    ["Asia/Bahrain","UTC+3"],
    ["Asia/Baku","UTC+4"],
    ["Asia/Bangkok","UTC+7"],
    ["Asia/Barnaul","UTC+7"],
    ["Asia/Beirut","UTC+2 / UTC+3"],
    ["Asia/Bishkek","UTC+6"],
    ["Asia/Brunei","UTC+8"],
    ["Asia/Calcutta","UTC+5:30"],
    ["Asia/Chita","UTC+9"],
    ["Asia/Colombo","UTC+5:30"],
    ["Asia/Damascus","UTC+3"],
    ["Asia/Dhaka","UTC+6"],
    ["Asia/Dili","UTC+9"],
    ["Asia/Dubai","UTC+4"],
    ["Asia/Dushanbe","UTC+5"],
    ["Asia/Famagusta","UTC+2 / UTC+3"],
    ["Asia/Gaza","UTC+2 / UTC+3"],
    ["Asia/Hebron","UTC+2 / UTC+3"],
    ["Asia/Hong_Kong","UTC+8"],
    ["Asia/Hovd","UTC+7"],
    ["Asia/Irkutsk","UTC+8"],
    ["Asia/Jakarta","UTC+7"],
    ["Asia/Jayapura","UTC+9"],
    ["Asia/Jerusalem","UTC+2 / UTC+3"],
    ["Asia/Kabul","UTC+4:30"],
    ["Asia/Kamchatka","UTC+12"],
    ["Asia/Karachi","UTC+5"],
    ["Asia/Katmandu","UTC+5:45"],
    ["Asia/Khandyga","UTC+9"],
    ["Asia/Krasnoyarsk","UTC+7"],
    ["Asia/Kuala_Lumpur","UTC+8"],
    ["Asia/Kuching","UTC+8"],
    ["Asia/Kuwait","UTC+3"],
    ["Asia/Macau","UTC+8"],
    ["Asia/Magadan","UTC+11"],
    ["Asia/Makassar","UTC+8"],
    ["Asia/Manila","UTC+8"],
    ["Asia/Muscat","UTC+4"],
    ["Asia/Nicosia","UTC+2 / UTC+3"],
    ["Asia/Novokuznetsk","UTC+7"],
    ["Asia/Novosibirsk","UTC+7"],
    ["Asia/Omsk","UTC+6"],
    ["Asia/Oral","UTC+5"],
    ["Asia/Phnom_Penh","UTC+7"],
    ["Asia/Pontianak","UTC+7"],
    ["Asia/Pyongyang","UTC+9"],
    ["Asia/Qatar","UTC+3"],
    ["Asia/Qostanay","UTC+5"],
    ["Asia/Qyzylorda","UTC+5"],
    ["Asia/Rangoon","UTC+6:30"],
    ["Asia/Riyadh","UTC+3"],
    ["Asia/Saigon","UTC+7"],
    ["Asia/Sakhalin","UTC+11"],
    ["Asia/Samarkand","UTC+5"],
    ["Asia/Seoul","UTC+9"],
    ["Asia/Shanghai","UTC+8"],
    ["Asia/Singapore","UTC+8"],
    ["Asia/Srednekolymsk","UTC+11"],
    ["Asia/Taipei","UTC+8"],
    ["Asia/Tashkent","UTC+5"],
    ["Asia/Tbilisi","UTC+4"],
    ["Asia/Tehran","UTC+3:30"],
    ["Asia/Thimphu","UTC+6"],
    ["Asia/Tokyo","UTC+9"],
    ["Asia/Tomsk","UTC+7"],
    ["Asia/Ulaanbaatar","UTC+8"],
    ["Asia/Urumqi","UTC+6"],
    ["Asia/Ust-Nera","UTC+10"],
    ["Asia/Vientiane","UTC+7"],
    ["Asia/Vladivostok","UTC+10"],
    ["Asia/Yakutsk","UTC+9"],
    ["Asia/Yekaterinburg","UTC+5"],
    ["Asia/Yerevan","UTC+4"],
    ["Atlantic/Azores","UTC-1 / UTC+0"],
    ["Atlantic/Bermuda","UTC-4 / UTC-3"],
    ["Atlantic/Canary","UTC+0 / UTC+1"],
    ["Atlantic/Cape_Verde","UTC-1"],
    ["Atlantic/Faeroe","UTC+0 / UTC+1"],
    ["Atlantic/Madeira","UTC+0 / UTC+1"],
    ["Atlantic/Reykjavik","UTC+0"],
    ["Atlantic/South_Georgia","UTC-2"],
    ["Atlantic/St_Helena","UTC+0"],
    ["Atlantic/Stanley","UTC-3"],
    ["Australia/Adelaide","UTC+10:30 / UTC+9:30"],
    ["Australia/Brisbane","UTC+10"],
    ["Australia/Broken_Hill","UTC+10:30 / UTC+9:30"],
    ["Australia/Darwin","UTC+9:30"],
    ["Australia/Eucla","UTC+8:45"],
    ["Australia/Hobart","UTC+11 / UTC+10"],
    ["Australia/Lindeman","UTC+10"],
    ["Australia/Lord_Howe","UTC+11 / UTC+10:30"],
    ["Australia/Melbourne","UTC+11 / UTC+10"],
    ["Australia/Perth","UTC+8"],
    ["Australia/Sydney","UTC+11 / UTC+10"],
    ["Europe/Amsterdam","UTC+1 / UTC+2"],
    ["Europe/Andorra","UTC+1 / UTC+2"],
    ["Europe/Astrakhan","UTC+4"],
    ["Europe/Athens","UTC+2 / UTC+3"],
    ["Europe/Belgrade","UTC+1 / UTC+2"],
    ["Europe/Berlin","UTC+1 / UTC+2"],
    ["Europe/Bratislava","UTC+1 / UTC+2"],
    ["Europe/Brussels","UTC+1 / UTC+2"],
    ["Europe/Bucharest","UTC+2 / UTC+3"],
    ["Europe/Budapest","UTC+1 / UTC+2"],
    ["Europe/Busingen","UTC+1 / UTC+2"],
    ["Europe/Chisinau","UTC+2 / UTC+3"],
    ["Europe/Copenhagen","UTC+1 / UTC+2"],
    ["Europe/Dublin","UTC+0 / UTC+1"],
    ["Europe/Gibraltar","UTC+1 / UTC+2"],
    ["Europe/Guernsey","UTC+0 / UTC+1"],
    ["Europe/Helsinki","UTC+2 / UTC+3"],
    ["Europe/Isle_of_Man","UTC+0 / UTC+1"],
    ["Europe/Istanbul","UTC+3"],
    ["Europe/Jersey","UTC+0 / UTC+1"],
    ["Europe/Kaliningrad","UTC+2"],
    ["Europe/Kiev","UTC+2 / UTC+3"],
    ["Europe/Kirov","UTC+3"],
    ["Europe/Lisbon","UTC+0 / UTC+1"],
    ["Europe/Ljubljana","UTC+1 / UTC+2"],
    ["Europe/London","UTC+0 / UTC+1"],
    ["Europe/Luxembourg","UTC+1 / UTC+2"],
    ["Europe/Madrid","UTC+1 / UTC+2"],
    ["Europe/Malta","UTC+1 / UTC+2"],
    ["Europe/Mariehamn","UTC+2 / UTC+3"],
    ["Europe/Minsk","UTC+3"],
    ["Europe/Monaco","UTC+1 / UTC+2"],
    ["Europe/Moscow","UTC+3"],
    ["Europe/Oslo","UTC+1 / UTC+2"],
    ["Europe/Paris","UTC+1 / UTC+2"],
    ["Europe/Podgorica","UTC+1 / UTC+2"],
    ["Europe/Prague","UTC+1 / UTC+2"],
    ["Europe/Riga","UTC+2 / UTC+3"],
    ["Europe/Rome","UTC+1 / UTC+2"],
    ["Europe/Samara","UTC+4"],
    ["Europe/San_Marino","UTC+1 / UTC+2"],
    ["Europe/Sarajevo","UTC+1 / UTC+2"],
    ["Europe/Saratov","UTC+4"],
    ["Europe/Simferopol","UTC+3"],
    ["Europe/Skopje","UTC+1 / UTC+2"],
    ["Europe/Sofia","UTC+2 / UTC+3"],
    ["Europe/Stockholm","UTC+1 / UTC+2"],
    ["Europe/Tallinn","UTC+2 / UTC+3"],
    ["Europe/Tirane","UTC+1 / UTC+2"],
    ["Europe/Ulyanovsk","UTC+4"],
    ["Europe/Vaduz","UTC+1 / UTC+2"],
    ["Europe/Vatican","UTC+1 / UTC+2"],
    ["Europe/Vienna","UTC+1 / UTC+2"],
    ["Europe/Vilnius","UTC+2 / UTC+3"],
    ["Europe/Volgograd","UTC+3"],
    ["Europe/Warsaw","UTC+1 / UTC+2"],
    ["Europe/Zagreb","UTC+1 / UTC+2"],
    ["Europe/Zurich","UTC+1 / UTC+2"],
    ["Indian/Antananarivo","UTC+3"],
    ["Indian/Chagos","UTC+6"],
    ["Indian/Christmas","UTC+7"],
    ["Indian/Cocos","UTC+6:30"],
    ["Indian/Comoro","UTC+3"],
    ["Indian/Kerguelen","UTC+5"],
    ["Indian/Mahe","UTC+4"],
    ["Indian/Maldives","UTC+5"],
    ["Indian/Mauritius","UTC+4"],
    ["Indian/Mayotte","UTC+3"],
    ["Indian/Reunion","UTC+4"],
    ["Pacific/Apia","UTC+13"],
    ["Pacific/Auckland","UTC+13 / UTC+12"],
    ["Pacific/Bougainville","UTC+11"],
    ["Pacific/Chatham","UTC+13:45 / UTC+12:45"],
    ["Pacific/Easter","UTC-5 / UTC-6"],
    ["Pacific/Efate","UTC+11"],
    ["Pacific/Enderbury","UTC+13"],
    ["Pacific/Fakaofo","UTC+13"],
    ["Pacific/Fiji","UTC+12"],
    ["Pacific/Funafuti","UTC+12"],
    ["Pacific/Galapagos","UTC-6"],
    ["Pacific/Gambier","UTC-9"],
    ["Pacific/Guadalcanal","UTC+11"],
    ["Pacific/Guam","UTC+10"],
    ["Pacific/Honolulu","UTC-10"],
    ["Pacific/Kiritimati","UTC+14"],
    ["Pacific/Kosrae","UTC+11"],
    ["Pacific/Kwajalein","UTC+12"],
    ["Pacific/Majuro","UTC+12"],
    ["Pacific/Marquesas","UTC-9:30"],
    ["Pacific/Midway","UTC-11"],
    ["Pacific/Nauru","UTC+12"],
    ["Pacific/Niue","UTC-11"],
    ["Pacific/Norfolk","UTC+12 / UTC+11"],
    ["Pacific/Noumea","UTC+11"],
    ["Pacific/Pago_Pago","UTC-11"],
    ["Pacific/Palau","UTC+9"],
    ["Pacific/Pitcairn","UTC-8"],
    ["Pacific/Ponape","UTC+11"],
    ["Pacific/Port_Moresby","UTC+10"],
    ["Pacific/Rarotonga","UTC-10"],
    ["Pacific/Saipan","UTC+10"],
    ["Pacific/Tahiti","UTC-10"],
    ["Pacific/Tarawa","UTC+12"],
    ["Pacific/Tongatapu","UTC+13"],
    ["Pacific/Truk","UTC+10"],
    ["Pacific/Wake","UTC+12"],
    ["Pacific/Wallis","UTC+12"]
  ];

  /* ---------- katalog ---------- */
  var catalog = [
    { id: "cat-01", display_name: "Amlodipine 10 mg Tablet", nama_dagang: "Amlodipine", generik_nama: "Amlodipine Besylate", is_preset: true, curation_status: "published", source: "kfa", dosage_form_name: "Tablet", route_code: "O", dose_value_text: "10 mg", ucum_code: "mg", uom_name: "Tablet", atc_l5_code: "C08CA01", kfa_code: "93000123", bpom_nie: "GKL1234567890A1", packaging_options: ["Dus isi 30"], active_ingredients: ["Amlodipine Besylate 10 mg"], indication: "Hipertensi, angina stabil kronis.", warning: "Hindari pada gagal hati berat.", curated_by: "admin@example.com", curated_at: iso(shiftDays(-42, 9, 0)) },
    { id: "cat-02", display_name: "Metformin 500 mg Tablet", nama_dagang: "Metformin", generik_nama: "Metformin HCl", is_preset: true, curation_status: "published", source: "kfa", dosage_form_name: "Tablet", route_code: "O", dose_value_text: "500 mg", ucum_code: "mg", uom_name: "Tablet", atc_l5_code: "A10BA02", kfa_code: "93000456", bpom_nie: "GKL9876543210B1", packaging_options: ["Dus isi 100"], active_ingredients: ["Metformin HCl 500 mg"], indication: "Diabetes melitus tipe 2.", warning: "Hentikan sebelum pemeriksaan kontras.", curated_by: "admin@example.com", curated_at: iso(shiftDays(-40, 10, 0)) },
    { id: "cat-03", display_name: "Paracetamol 500 mg Tablet", nama_dagang: "Paracetamol", generik_nama: "Paracetamol", is_preset: true, curation_status: "published", source: "bpom", dosage_form_name: "Tablet", route_code: "O", dose_value_text: "500 mg", ucum_code: "mg", uom_name: "Tablet", atc_l5_code: "N02BE01", kfa_code: "93000789", bpom_nie: "GKL1112223330C1", packaging_options: ["Dus isi 10", "Strip isi 10"], active_ingredients: ["Paracetamol 500 mg"], indication: "Demam dan nyeri ringan–sedang.", warning: "Maksimal 4 g per hari.", curated_by: "admin@example.com", curated_at: iso(shiftDays(-38, 8, 30)) },
    { id: "cat-04", display_name: "Amlodipine 5 mg (sediaan baru)", nama_dagang: "Amlodipine", generik_nama: "Amlodipine Besylate", is_preset: true, curation_status: "draft", source: "kfa", dosage_form_name: "Tablet", route_code: "O", dose_value_text: "5 mg", ucum_code: "mg", uom_name: "Tablet", atc_l5_code: "C08CA01", kfa_code: "93000999", bpom_nie: null, packaging_options: ["Dus isi 30"], active_ingredients: ["Amlodipine Besylate 5 mg"], indication: null, warning: null, curated_by: null, curated_at: null },
    { id: "cat-05", display_name: "Vitamin D3 1000 IU", nama_dagang: "Vit D3", generik_nama: "Cholecalciferol", is_preset: true, curation_status: "archived", source: "manual", dosage_form_name: "Soft Capsule", route_code: "O", dose_value_text: "1000 IU", ucum_code: "IU", uom_name: "Kapsul", atc_l5_code: "A11CC05", kfa_code: null, bpom_nie: "GKL5556667770D1", packaging_options: ["Botol isi 60"], active_ingredients: ["Cholecalciferol 1000 IU"], indication: "Suplemen vitamin D.", warning: null, curated_by: "admin@example.com", curated_at: iso(shiftDays(-90, 9, 0)) },
    { id: "cat-06", display_name: "Obat Warung ABC (racikan)", nama_dagang: "ABC", generik_nama: null, is_preset: false, curation_status: "user_private", source: "manual", dosage_form_name: "Kapsul", route_code: "O", dose_value_text: null, ucum_code: "kapsul", uom_name: "Kapsul", atc_l5_code: null, kfa_code: null, bpom_nie: null, packaging_options: ["Plastik"], active_ingredients: [], indication: "Pusing dan pegal.", warning: null, owner_id: "u-01", curated_by: null, curated_at: null },
    { id: "cat-07", display_name: "Amoxicillin 500 mg Kapsul", nama_dagang: "Amoxicillin", generik_nama: "Amoxicillin Trihydrate", is_preset: true, curation_status: "published", source: "kfa", dosage_form_name: "Kapsul", route_code: "O", dose_value_text: "500 mg", ucum_code: "mg", uom_name: "Kapsul", atc_l5_code: "J01CA04", kfa_code: "93000222", bpom_nie: "GKL4445556660E1", packaging_options: ["Dus isi 30"], active_ingredients: ["Amoxicillin Trihydrate 500 mg"], indication: "Infeksi bakteri sensitif.", warning: "Habiskan sesuai resep walau sudah membaik.", curated_by: "admin@example.com", curated_at: iso(shiftDays(-35, 11, 0)) }
  ];

  /* ---------- obat + jadwal (aturan, bukan baris per hari) ---------- */
  var medications = [
    {
      id: "med-01", owner_id: "u-01", catalog_id: "cat-01", name: "Amlodipine 10", dose_value: 1, dose_unit_code: "tablet",
      strength_note: null, route_code: "O", dosage_form_code: "LOCAL-0001", meal_relation: "after_meal",
      as_needed: false, must_finish: false, start_date: ymd(shiftDays(-120)), start_time: "07:00", end_date: null,
      notes: "Minum setelah sarapan", notes_reminder: "Jangan lupa tensi pagi", prescription_photo_url: "https://cdn.example.com/rx/amlodipine.jpg",
      color_tag: "#2dd4bf", is_active: true, deleted_at: null,
      frequency: { id: "fq-01", pattern: "daily", times_per_day: 2, schedule_timezone: "Asia/Jakarta", tolerance_minutes: 60, start_time: "07:00" },
      times: [
        { id: "tm-01", time_local: "07:00", label: "morning", weekdays: 127, is_active: true, sort_order: 0 },
        { id: "tm-02", time_local: "19:00", label: "evening", weekdays: 127, is_active: true, sort_order: 1 }
      ],
      stock: { id: "st-01", unit_code: "tablet", initial_qty: 60, current_qty: 24, qty_per_dose: 1, refill_threshold_qty: 10, refill_reminder: true, last_restocked_at: iso(shiftDays(-14, 17, 0)) }
    },
    {
      id: "med-02", owner_id: "u-01", catalog_id: "cat-02", name: "Metformin 500", dose_value: 2, dose_unit_code: "tablet",
      strength_note: null, route_code: "O", dosage_form_code: "LOCAL-0001", meal_relation: "with_meal",
      as_needed: false, must_finish: false, start_date: ymd(shiftDays(-200)), start_time: "08:00", end_date: null,
      notes: "Selalu bersama makanan", notes_reminder: null, prescription_photo_url: null,
      color_tag: "#5aa9ff", is_active: true, deleted_at: null,
      frequency: { id: "fq-02", pattern: "daily", times_per_day: 2, schedule_timezone: "Asia/Jakarta", tolerance_minutes: 60, start_time: "08:00" },
      times: [
        { id: "tm-03", time_local: "08:00", label: "morning", weekdays: 127, is_active: true, sort_order: 0 },
        { id: "tm-04", time_local: "20:00", label: "evening", weekdays: 127, is_active: true, sort_order: 1 }
      ],
      stock: { id: "st-02", unit_code: "tablet", initial_qty: 200, current_qty: 148, qty_per_dose: 2, refill_threshold_qty: 40, refill_reminder: true, last_restocked_at: iso(shiftDays(-21, 9, 0)) }
    },
    {
      id: "med-03", owner_id: "u-01", catalog_id: "cat-07", name: "Amoxicillin 500", dose_value: 1, dose_unit_code: "kapsul",
      strength_note: null, route_code: "O", dosage_form_code: "LOCAL-0002", meal_relation: "after_meal",
      as_needed: false, must_finish: true, start_date: ymd(shiftDays(-3)), start_time: "06:00", end_date: ymd(shiftDays(2)),
      notes: "Antibiotik, wajib dihabiskan", notes_reminder: "Habiskan walau sudah membaik", prescription_photo_url: "https://cdn.example.com/rx/amoxicillin.jpg",
      color_tag: "#f59e0b", is_active: true, deleted_at: null,
      frequency: { id: "fq-03", pattern: "interval", interval_hours: 8, interval_min_hours: 8, schedule_timezone: "Asia/Jakarta", tolerance_minutes: 60, start_time: "06:00" },
      times: [
        { id: "tm-05", time_local: "06:00", label: "dawn", weekdays: 127, is_active: true, sort_order: 0 },
        { id: "tm-06", time_local: "14:00", label: "afternoon", weekdays: 127, is_active: true, sort_order: 1 },
        { id: "tm-07", time_local: "22:00", label: "evening", weekdays: 127, is_active: true, sort_order: 2 }
      ],
      stock: { id: "st-03", unit_code: "kapsul", initial_qty: 30, current_qty: 21, qty_per_dose: 1, refill_threshold_qty: 6, refill_reminder: false, last_restocked_at: null }
    },
    {
      id: "med-04", owner_id: "u-01", catalog_id: null, name: "Vitamin D3 1000", dose_value: 1, dose_unit_code: "kapsul",
      strength_note: "1000 IU per kapsul", route_code: "O", dosage_form_code: "LOCAL-0005", meal_relation: "after_meal",
      as_needed: false, must_finish: false, start_date: ymd(shiftDays(-60)), start_time: "09:00", end_date: null,
      notes: null, notes_reminder: null, prescription_photo_url: null,
      color_tag: "#f7bb59", is_active: true, deleted_at: null,
      frequency: { id: "fq-04", pattern: "weekly", days_of_week: 5, schedule_timezone: "Asia/Jakarta", tolerance_minutes: 60, start_time: "09:00" },
      times: [{ id: "tm-08", time_local: "09:00", label: "morning", weekdays: 5, is_active: true, sort_order: 0 }],
      stock: { id: "st-04", unit_code: "kapsul", initial_qty: 60, current_qty: 9, qty_per_dose: 1, refill_threshold_qty: 10, refill_reminder: true, last_restocked_at: iso(shiftDays(-60, 10, 0)) }
    },
    {
      id: "med-05", owner_id: "u-01", catalog_id: null, name: "Kontrasepsi siklus 21/7", dose_value: 1, dose_unit_code: "tablet",
      strength_note: null, route_code: "O", dosage_form_code: "LOCAL-0001", meal_relation: "not_related",
      as_needed: false, must_finish: false, start_date: ymd(shiftDays(-30)), start_time: "21:00", end_date: null,
      notes: "21 hari minum, 7 hari jeda", notes_reminder: null, prescription_photo_url: null,
      color_tag: "#c792ea", is_active: true, deleted_at: null,
      frequency: { id: "fq-05", pattern: "monthly_cycle", cycle_intake_days: 21, cycle_pause_days: 7, cycle_anchor: "from_start", schedule_timezone: "Asia/Jakarta", tolerance_minutes: 60, start_time: "21:00" },
      times: [{ id: "tm-09", time_local: "21:00", label: "evening", weekdays: 127, is_active: true, sort_order: 0 }],
      stock: { id: "st-05", unit_code: "tablet", initial_qty: 63, current_qty: 44, qty_per_dose: 1, refill_threshold_qty: 21, refill_reminder: true, last_restocked_at: iso(shiftDays(-30, 12, 0)) }
    },
    {
      id: "med-06", owner_id: "u-01", catalog_id: "cat-03", name: "Paracetamol 500 (bila perlu)", dose_value: 1, dose_unit_code: "tablet",
      strength_note: null, route_code: "O", dosage_form_code: "LOCAL-0001", meal_relation: "after_meal",
      as_needed: true, must_finish: false, start_date: ymd(shiftDays(-90)), start_time: "00:00", end_date: null,
      notes: "Bila demam atau nyeri", notes_reminder: null, prescription_photo_url: null,
      color_tag: "#ef4444", is_active: true, deleted_at: null,
      frequency: { id: "fq-06", pattern: "prn", prn_max_per_day: 4, schedule_timezone: "Asia/Jakarta", tolerance_minutes: 60, start_time: "00:00" },
      times: [],
      stock: { id: "st-06", unit_code: "tablet", initial_qty: 40, current_qty: 32, qty_per_dose: 1, refill_threshold_qty: 8, refill_reminder: false, last_restocked_at: null }
    },
    {
      id: "med-07", owner_id: "u-01", catalog_id: null, name: "Obat Warung ABC", dose_value: 1, dose_unit_code: "kapsul",
      strength_note: null, route_code: "O", dosage_form_code: "LOCAL-0002", meal_relation: "after_meal",
      as_needed: false, must_finish: false, start_date: ymd(shiftDays(-45)), start_time: "12:00", end_date: ymd(shiftDays(-15)),
      notes: "Sudah selesai", notes_reminder: null, prescription_photo_url: null,
      color_tag: "#14b8a6", is_active: false, deleted_at: iso(shiftDays(-15, 10, 0)),
      frequency: { id: "fq-07", pattern: "daily", times_per_day: 1, schedule_timezone: "Asia/Jakarta", tolerance_minutes: 60, start_time: "12:00" },
      times: [{ id: "tm-10", time_local: "12:00", label: "afternoon", weekdays: 127, is_active: false, sort_order: 0 }],
      stock: { id: "st-07", unit_code: "kapsul", initial_qty: 30, current_qty: 0, qty_per_dose: 1, refill_threshold_qty: 5, refill_reminder: false, last_restocked_at: null }
    }
  ];

  /* ---------- log kepatuhan (dummy beberapa hari) ---------- */
  function lg(id, medId, schedId, date, hh, mm, status, takenAt, extra) {
    var o = {
      id: id, medication_id: medId, schedule_id: schedId, owner_id: "u-01", recorded_by: "u-01",
      slot_date: date, scheduled_hour: hh, scheduled_minute: mm, scheduled_tz: "Asia/Jakarta",
      status: status, taken_at: takenAt, note: (extra && extra.note) || null, source: (extra && extra.source) || "self",
      deleted_at: null
    };
    return o;
  }
  var logs = [
    lg("lg-01", "med-01", "tm-01", TODAY, 7, 0, "taken", iso(shiftDays(0, 7, 4))),
    lg("lg-02", "med-01", "tm-02", ymd(shiftDays(-1)), 19, 0, "taken", iso(shiftDays(-1, 19, 12))),
    lg("lg-03", "med-01", "tm-01", ymd(shiftDays(-1)), 7, 0, "late", iso(shiftDays(-1, 8, 25))),
    lg("lg-04", "med-02", "tm-03", TODAY, 8, 0, "taken", iso(shiftDays(0, 8, 5))),
    lg("lg-05", "med-02", "tm-04", ymd(shiftDays(-1)), 20, 0, "taken", iso(shiftDays(-1, 20, 3))),
    lg("lg-06", "med-03", "tm-05", TODAY, 6, 0, "taken", iso(shiftDays(0, 6, 2))),
    lg("lg-07", "med-03", "tm-06", ymd(shiftDays(-1)), 14, 0, "taken", iso(shiftDays(-1, 14, 9))),
    lg("lg-08", "med-03", "tm-07", ymd(shiftDays(-1)), 22, 0, "skipped", null, { note: "Muntah", source: "self" }),
    lg("lg-09", "med-04", "tm-08", ymd(shiftDays(-2)), 9, 0, "taken", iso(shiftDays(-2, 9, 30))),
    lg("lg-10", "med-05", "tm-09", ymd(shiftDays(-1)), 21, 0, "taken", iso(shiftDays(-1, 21, 15))),
    lg("lg-11", "med-06", null, ymd(shiftDays(-2)), null, null, "taken", iso(shiftDays(-2, 15, 0)), { note: "Demam 38.5", source: "self" }),
    lg("lg-12", "med-06", null, ymd(shiftDays(-4)), null, null, "taken", iso(shiftDays(-4, 20, 0)), { note: "Nyeri kepala", source: "self" }),
    lg("lg-13", "med-02", "tm-03", ymd(shiftDays(-4)), 8, 0, "taken", iso(shiftDays(-4, 8, 40)), { source: "caregiver" }),
    lg("lg-14", "med-01", "tm-01", ymd(shiftDays(-3)), 7, 0, "missed", null, { note: null }),
    lg("lg-15", "med-01", "tm-02", ymd(shiftDays(-3)), 19, 0, "taken", iso(shiftDays(-3, 19, 25)))
  ];

  /* ---------- penyimpangan (dose_events) ---------- */
  var doseEvents = [
    { event_key: "tm-01|" + ymd(shiftDays(-3)) + "|07:00", medication_id: "med-01", kind: "missed", reason: "Tidak ada catatan", event_date: ymd(shiftDays(-3)) },
    { event_key: "tm-06|" + ymd(shiftDays(-6)) + "|14:00", medication_id: "med-03", kind: "dose_shift", reason: "Interval diubah dari 12 jam ke 8 jam", event_date: ymd(shiftDays(-6)) },
    { event_key: "tm-09|" + ymd(shiftDays(-2)) + "|21:00", medication_id: "med-05", kind: "paused", reason: "Masa jeda siklus (hari 22–28)", event_date: ymd(shiftDays(-2)) }
  ];

  /* ---------- caregiver (care_links) ---------- */
  var careLinks = [
    { id: "cl-01", owner_id: "u-01", caregiver_id: "u-02", invite_email: "sari@example.com", relation: "spouse", role: "viewer", status: "accepted", invited_by: "u-01", decided_at: iso(shiftDays(-30, 8, 0)), created_at: iso(shiftDays(-31, 8, 0)), caregiver_name: "Sari Wulandari" },
    { id: "cl-02", owner_id: "u-01", caregiver_id: "u-03", invite_email: "andi@example.com", relation: "child", role: "helper", status: "accepted", invited_by: "u-01", decided_at: iso(shiftDays(-20, 15, 0)), created_at: iso(shiftDays(-21, 15, 0)), caregiver_name: "Andi Santoso" },
    { id: "cl-03", owner_id: "u-01", caregiver_id: null, invite_email: "nenek@example.com", relation: "parent", role: "viewer", status: "pending", invited_by: "u-01", decided_at: null, created_at: iso(shiftDays(-2, 10, 0)), invite_expires_at: iso(shiftDays(5, 10, 0)), caregiver_name: "Nenek (belum terdaftar)" },
    { id: "cl-04", owner_id: "u-04", caregiver_id: "u-01", invite_email: "budi@example.com", relation: "parent", role: "viewer", status: "accepted", invited_by: "u-04", decided_at: iso(shiftDays(-45, 9, 0)), created_at: iso(shiftDays(-46, 9, 0)), caregiver_name: "Budi Santoso", as: "caregiver", owner_name: "Ibu Sumarni" },
    { id: "cl-05", owner_id: "u-05", caregiver_id: "u-01", invite_email: "budi@example.com", relation: "nurse", role: "helper", status: "accepted", invited_by: "u-05", decided_at: iso(shiftDays(-12, 11, 0)), created_at: iso(shiftDays(-13, 11, 0)), caregiver_name: "Budi Santoso", as: "caregiver", owner_name: "Pak Hendra" },
    { id: "cl-06", owner_id: "u-06", caregiver_id: "u-01", invite_email: "budi@example.com", relation: "friend", role: "viewer", status: "pending", invited_by: "u-06", decided_at: null, created_at: iso(shiftDays(-1, 16, 0)), invite_expires_at: iso(shiftDays(6, 16, 0)), caregiver_name: "Budi Santoso", as: "caregiver", owner_name: "Rina Kartika" }
  ];

  /* ---------- stok: transaksi ---------- */
  var stockTx = [
    { id: "tx-01", medication_id: "med-01", kind: "initial", qty_delta: 60, balance_after: 60, note: "Stok awal", created_at: iso(shiftDays(-120, 8, 0)), log_id: null },
    { id: "tx-02", medication_id: "med-01", kind: "restock", qty_delta: 30, balance_after: 90, note: "Beli di apotek", created_at: iso(shiftDays(-14, 17, 0)), log_id: null },
    { id: "tx-03", medication_id: "med-01", kind: "consume", qty_delta: -1, balance_after: 25, note: null, created_at: iso(shiftDays(0, 7, 4)), log_id: "lg-01" },
    { id: "tx-04", medication_id: "med-01", kind: "consume", qty_delta: -1, balance_after: 24, note: null, created_at: iso(shiftDays(-1, 19, 12)), log_id: "lg-02" },
    { id: "tx-05", medication_id: "med-01", kind: "adjust", qty_delta: -5, balance_after: 85, note: "Koreksi: 5 tablet rusak", created_at: iso(shiftDays(-10, 10, 0)), log_id: null },
    { id: "tx-06", medication_id: "med-02", kind: "initial", qty_delta: 200, balance_after: 200, note: "Stok awal", created_at: iso(shiftDays(-200, 8, 0)), log_id: null },
    { id: "tx-07", medication_id: "med-02", kind: "consume", qty_delta: -2, balance_after: 148, note: null, created_at: iso(shiftDays(0, 8, 5)), log_id: "lg-04" },
    { id: "tx-08", medication_id: "med-04", kind: "expire", qty_delta: -3, balance_after: 9, note: "Kedaluwarsa" + " " + ymd(shiftDays(-5)), created_at: iso(shiftDays(-5, 9, 0)), log_id: null },
    { id: "tx-09", medication_id: "med-05", kind: "consume", qty_delta: -1, balance_after: 44, note: null, created_at: iso(shiftDays(-1, 21, 15)), log_id: "lg-10" },
    { id: "tx-10", medication_id: "med-03", kind: "initial", qty_delta: 30, balance_after: 30, note: "Stok awal resep", created_at: iso(shiftDays(-3, 6, 0)), log_id: null },
    { id: "tx-11", medication_id: "med-03", kind: "consume", qty_delta: -1, balance_after: 21, note: null, created_at: iso(shiftDays(0, 6, 2)), log_id: "lg-06" }
  ];

  /* ---------- preferensi notifikasi ---------- */
  var notifPrefs = [
    { channel: "push", enabled: true, quiet_start: "22:00", quiet_end: "06:00", advance_minutes: 10, notif_throttled: false },
    { channel: "in_app", enabled: true, quiet_start: null, quiet_end: null, advance_minutes: 0, notif_throttled: false },
    { channel: "telegram", enabled: true, quiet_start: "22:00", quiet_end: "06:00", advance_minutes: 5, notif_throttled: false },
    { channel: "email", enabled: false, quiet_start: null, quiet_end: null, advance_minutes: 0, notif_throttled: false }
  ];

  var deviceTokens = [
    { id: "dv-01", platform: "android", label: "Samsung A54 — Budi", last_seen_at: iso(shiftDays(0, 7, 12)), revoked_at: null },
    { id: "dv-02", platform: "web", label: "Chrome Desktop — friendly-pc", last_seen_at: iso(shiftDays(-1, 21, 30)), revoked_at: null },
    { id: "dv-03", platform: "ios", label: "iPhone lama Budi", last_seen_at: iso(shiftDays(-60, 10, 0)), revoked_at: iso(shiftDays(-45, 9, 0)) }
  ];

  var inbox = [
    { id: "nb-01", kind: "dose_reminder", title: "Waktunya minum Amlodipine 10", body: "1 tablet, sesudah makan", created_at: iso(shiftDays(0, 7, 0)), read: true, channel: "push" },
    { id: "nb-02", kind: "dose_reminder", title: "Waktunya minum Metformin 500", body: "2 tablet, bersama makan", created_at: iso(shiftDays(0, 8, 0)), read: true, channel: "push" },
    { id: "nb-03", kind: "refill_reminder", title: "Vitamin D3 hampir habis", body: "Sisa 9 kapsul — perkiraan habis 9 hari lagi", created_at: iso(shiftDays(0, 8, 5)), read: false, channel: "push" },
    { id: "nb-04", kind: "missed_escalation", title: "Amlodipine 10 terlewat 3 kali", body: "Pengingat dikirim ke Sari Wulandari", created_at: iso(shiftDays(-1, 8, 10)), read: false, channel: "telegram" },
    { id: "nb-05", kind: "care_digest", title: "Ringkasan harian", body: "5 dosis diminum, 1 terlewat, kepatuhan 83%", created_at: iso(shiftDays(-1, 20, 0)), read: true, channel: "in_app" }
  ];

  /* ---------- admin: users ---------- */
  var users = [
    { id: "u-01", email: "budi@example.com", full_name: "Budi Santoso", phone: "+628123456789", timezone: "Asia/Jakarta", role: "user", admin_scope: null, is_active: true, last_login_at: iso(shiftDays(0, 7, 2)), created_at: iso(shiftDays(-120, 9, 0)), meds_active: 6, adherence_7d: 0.86, deleted_at: null },
    { id: "u-02", email: "sari@example.com", full_name: "Sari Wulandari", phone: "+628129876543", timezone: "Asia/Jakarta", role: "user", admin_scope: null, is_active: true, last_login_at: iso(shiftDays(0, 6, 45)), created_at: iso(shiftDays(-100, 10, 0)), meds_active: 2, adherence_7d: 0.94, deleted_at: null },
    { id: "u-03", email: "andi@example.com", full_name: "Andi Santoso", phone: null, timezone: "Asia/Makassar", role: "user", admin_scope: null, is_active: true, last_login_at: iso(shiftDays(-2, 20, 10)), created_at: iso(shiftDays(-80, 14, 0)), meds_active: 1, adherence_7d: 0.71, deleted_at: null },
    { id: "u-04", email: "sumarni@example.com", full_name: "Sumarni", phone: "+628551234000", timezone: "Asia/Jakarta", role: "user", admin_scope: null, is_active: true, last_login_at: iso(shiftDays(-1, 19, 45)), created_at: iso(shiftDays(-60, 11, 0)), meds_active: 4, adherence_7d: 0.62, deleted_at: null },
    { id: "u-05", email: "hendra@example.com", full_name: "Hendra Wijaya", phone: "+6287888111222", timezone: "Asia/Jayapura", role: "user", admin_scope: null, is_active: true, last_login_at: iso(shiftDays(-5, 8, 30)), created_at: iso(shiftDays(-50, 9, 30)), meds_active: 3, adherence_7d: 0.78, deleted_at: null },
    { id: "u-06", email: "rina@example.com", full_name: "Rina Kartika", phone: null, timezone: "Asia/Jakarta", role: "user", admin_scope: null, is_active: false, last_login_at: iso(shiftDays(-14, 16, 0)), created_at: iso(shiftDays(-40, 8, 0)), meds_active: 0, adherence_7d: null, deleted_at: null },
    { id: "u-07", email: "admin@example.com", full_name: "Admin Pantau", phone: null, timezone: "Asia/Jakarta", role: "admin", admin_scope: null, is_active: true, last_login_at: iso(shiftDays(0, 9, 0)), created_at: iso(shiftDays(-150, 8, 0)), meds_active: 0, adherence_7d: null, admin_granted_at: iso(shiftDays(-150, 9, 0)), deleted_at: null },
    { id: "u-08", email: "kurator@example.com", full_name: "Kurator Katalog", phone: null, timezone: "Asia/Jakarta", role: "admin", admin_scope: ["catalog"], is_active: true, last_login_at: iso(shiftDays(-1, 14, 0)), created_at: iso(shiftDays(-90, 8, 0)), meds_active: 0, adherence_7d: null, admin_granted_at: iso(shiftDays(-90, 9, 0)), deleted_at: null },
    { id: "u-09", email: "support@example.com", full_name: "Support Notifikasi", phone: null, timezone: "Asia/Jakarta", role: "admin", admin_scope: ["notifications", "reports"], is_active: true, last_login_at: iso(shiftDays(-3, 10, 0)), created_at: iso(shiftDays(-70, 8, 0)), meds_active: 0, adherence_7d: null, admin_granted_at: iso(shiftDays(-70, 9, 0)), deleted_at: null },
    { id: "u-10", email: "lintang@example.com", full_name: "Lintang P.", phone: "+6281200099988", timezone: "Asia/Jakarta", role: "user", admin_scope: null, is_active: true, last_login_at: iso(shiftDays(-8, 7, 0)), created_at: iso(shiftDays(-30, 8, 0)), meds_active: 5, adherence_7d: 0.9, deleted_at: null },
    { id: "u-11", email: "yusuf@example.com", full_name: "Yusuf Maulana", phone: null, timezone: "Asia/Jakarta", role: "user", admin_scope: null, is_active: true, last_login_at: iso(shiftDays(-1, 6, 0)), created_at: iso(shiftDays(-25, 8, 0)), meds_active: 2, adherence_7d: 0.45, deleted_at: null },
    { id: "u-12", email: "dewi@example.com", full_name: "Dewi Anggraini", phone: "+6289900112233", timezone: "Asia/Jakarta", role: "user", admin_scope: null, is_active: false, last_login_at: iso(shiftDays(-90, 8, 0)), created_at: iso(shiftDays(-200, 8, 0)), meds_active: 0, adherence_7d: null, deleted_at: null },
    { id: "u-13", email: "dihapus@example.com", full_name: "Akun Dihapus", phone: null, timezone: "Asia/Jakarta", role: "user", admin_scope: null, is_active: false, last_login_at: null, created_at: iso(shiftDays(-300, 8, 0)), meds_active: 0, adherence_7d: null, deleted_at: iso(shiftDays(-20, 8, 0)) }
  ];

  /* ---------- admin: job notifikasi ---------- */
  var jobs = [
    { id: 90124, kind: "dose_reminder", status: "done", dedup_key: "job|tm-01|" + TODAY + "|07:00|dose_reminder", owner_email: "budi@example.com", run_at: iso(shiftDays(0, 7, 0)), attempts: 1, max_attempts: 5, last_error: null, channels: { push: "sent", in_app: "sent", telegram: "sent" } },
    { id: 90125, kind: "dose_reminder", status: "done", dedup_key: "job|tm-03|" + TODAY + "|08:00|dose_reminder", owner_email: "budi@example.com", run_at: iso(shiftDays(0, 8, 0)), attempts: 1, max_attempts: 5, last_error: null, channels: { push: "sent", in_app: "sent", telegram: "skipped" } },
    { id: 90126, kind: "refill_reminder", status: "done", dedup_key: "refill|med-04|" + TODAY, owner_email: "budi@example.com", run_at: iso(shiftDays(0, 8, 5)), attempts: 1, max_attempts: 5, last_error: null, channels: { push: "sent", in_app: "sent" } },
    { id: 90127, kind: "dose_reminder", status: "failed", dedup_key: "job|tm-07|" + ymd(shiftDays(-1)) + "|22:00|dose_reminder", owner_email: "andi@example.com", run_at: iso(shiftDays(-1, 22, 0)), attempts: 5, max_attempts: 5, last_error: "push: 410 Gone (token dicabut); telegram: 403 bot blocked", channels: { push: "failed", telegram: "failed", email: "skipped" } },
    { id: 90128, kind: "missed_escalation", status: "done", dedup_key: "esc|u-01|" + ymd(shiftDays(-1)), owner_email: "budi@example.com", run_at: iso(shiftDays(-1, 8, 10)), attempts: 1, max_attempts: 5, last_error: null, channels: { telegram: "sent", in_app: "sent" } },
    { id: 90129, kind: "care_digest", status: "pending", dedup_key: "digest|u-04|" + TODAY, owner_email: "sumarni@example.com", run_at: iso(shiftDays(0, 20, 0)), attempts: 0, max_attempts: 5, last_error: null, channels: { push: "pending", in_app: "pending" } },
    { id: 90130, kind: "dose_reminder", status: "pending", dedup_key: "job|tm-02|" + TODAY + "|19:00|dose_reminder", owner_email: "budi@example.com", run_at: iso(shiftDays(0, 19, 0)), attempts: 0, max_attempts: 5, last_error: null, channels: { push: "pending", telegram: "pending", in_app: "pending" } },
    { id: 90131, kind: "retention", status: "cancelled", dedup_key: "retention|" + TODAY, owner_email: null, run_at: iso(shiftDays(0, 2, 0)), attempts: 0, max_attempts: 1, last_error: null, channels: {} },
    { id: 90132, kind: "dose_reminder", status: "failed", dedup_key: "job|tm-05|" + ymd(shiftDays(-2)) + "|06:00|dose_reminder", owner_email: "hendra@example.com", run_at: iso(shiftDays(-2, 6, 0)), attempts: 3, max_attempts: 5, last_error: "SMTP: 421 service not available", channels: { email: "failed", in_app: "sent" } },
    { id: 90133, kind: "missed_escalation", status: "pending", dedup_key: "esc|u-04|" + TODAY, owner_email: "sumarni@example.com", run_at: iso(shiftDays(0, 8, 10)), attempts: 0, max_attempts: 5, last_error: null, channels: { telegram: "pending" } }
  ];

  /* ---------- admin: audit ---------- */
  var audit = [
    { id: 55210, actor_email: "admin@example.com", actor_role: "admin", action: "catalog_publish", resource_type: "catalog", resource_id: "cat-01", created_at: iso(shiftDays(-42, 9, 0)), ip: "10.20.0.5", meta: { reason: "verifikasi NIE BPOM" }, before_state: { curation_status: "draft" }, after_state: { curation_status: "published" } },
    { id: 55211, actor_email: "admin@example.com", actor_role: "admin", action: "user_disable", resource_type: "user", resource_id: "u-06", created_at: iso(shiftDays(-14, 16, 5)), ip: "10.20.0.5", meta: { reason: "permintaan pengguna" }, before_state: { is_active: true }, after_state: { is_active: false } },
    { id: 55212, actor_email: "admin@example.com", actor_role: "admin", action: "user_enable", resource_type: "user", resource_id: "u-06", created_at: iso(shiftDays(-13, 9, 30)), ip: "10.20.0.5", meta: {}, before_state: { is_active: false }, after_state: { is_active: true } },
    { id: 55213, actor_email: "admin@example.com", actor_role: "admin", action: "notif_retry", resource_type: "notification", resource_id: null, created_at: iso(shiftDays(-1, 22, 40)), ip: "10.20.0.5", meta: { job_id: 90127 }, before_state: { status: "failed" }, after_state: { status: "pending" } },
    { id: 55214, actor_email: "admin@example.com", actor_role: "admin", action: "admin_grant", resource_type: "admin", resource_id: "u-08", created_at: iso(shiftDays(-90, 9, 0)), ip: "10.20.0.5", meta: { scope: ["catalog"] }, before_state: { role: "user" }, after_state: { role: "admin", admin_scope: ["catalog"] } },
    { id: 55215, actor_email: "budi@example.com", actor_role: "user", action: "log_taken", resource_type: "log", resource_id: "lg-01", created_at: iso(shiftDays(0, 7, 4)), ip: "114.79.12.3", meta: { medication_id: "med-01" }, before_state: null, after_state: { status: "taken" } },
    { id: 55216, actor_email: "budi@example.com", actor_role: "user", action: "restock", resource_type: "stock", resource_id: "med-01", created_at: iso(shiftDays(-14, 17, 0)), ip: "114.79.12.3", meta: { qty: 30 }, before_state: { current_qty: 60 }, after_state: { current_qty: 90 } },
    { id: 55217, actor_email: "budi@example.com", actor_role: "user", action: "invite", resource_type: "care_link", resource_id: "cl-03", created_at: iso(shiftDays(-2, 10, 0)), ip: "114.79.12.3", meta: { invite_email: "nenek@example.com" }, before_state: null, after_state: { status: "pending" } },
    { id: 55218, actor_email: null, actor_role: "system", action: "retention_purge", resource_type: "retention", resource_id: null, created_at: iso(shiftDays(-1, 2, 0)), ip: null, meta: { sessions_deleted: 412, tokens_deleted: 18 }, before_state: null, after_state: null },
    { id: 55219, actor_email: "sari@example.com", actor_role: "user", action: "read", resource_type: "medication", resource_id: null, created_at: iso(shiftDays(0, 6, 46)), ip: "36.85.4.19", meta: { via: "care_link", care_link_id: "cl-01" }, before_state: null, after_state: null },
    { id: 55220, actor_email: "admin@example.com", actor_role: "admin", action: "catalog_import", resource_type: "catalog", resource_id: null, created_at: iso(shiftDays(-40, 10, 0)), ip: "10.20.0.5", meta: { source: "kfa", imported: 128, skipped: 12 }, before_state: null, after_state: null },
    { id: 55221, actor_email: "andi@example.com", actor_role: "user", action: "log_taken", resource_type: "log", resource_id: "lg-13", created_at: iso(shiftDays(-4, 8, 40)), ip: "103.20.9.7", meta: { recorded_for: "u-01", source: "caregiver" }, before_state: null, after_state: { status: "taken" } }
  ];

  /* ---------- admin: stats ---------- */
  var stats = {
    users_active: 128, users_disabled: 3, users_new_7d: 14, users_new_prev_7d: 9,
    medications_active: 412, catalog_presets: 86, catalog_draft: 7, catalog_archived: 12,
    jobs_failed_24h: 2, deliveries_sent_24h: 1530, deliveries_failed_24h: 11,
    adherence_avg_7d: 0.79, adherence_avg_30d: 0.74,
    push_success_rate: 0.973, telegram_success_rate: 0.991, email_success_rate: 0.86,
    series_7d: [
      { d: "Sen", ok: 0.88, sent: 1480, failed: 8 },
      { d: "Sel", ok: 0.91, sent: 1560, failed: 5 },
      { d: "Rab", ok: 0.76, sent: 1390, failed: 14 },
      { d: "Kam", ok: 0.93, sent: 1620, failed: 4 },
      { d: "Jum", ok: 0.85, sent: 1505, failed: 9 },
      { d: "Sab", ok: 0.68, sent: 1120, failed: 21 },
      { d: "Min", ok: 0.81, sent: 1240, failed: 7 }
    ]
  };

  /* ---------- pengguna: ringkasan hari ini ---------- */
  var todayOverview = {
    date: TODAY, timezone: "Asia/Jakarta",
    doses_total: 7, taken: 3, late: 0, skipped: 0, missed: 0, pending: 4, prn_taken: 0,
    adherence_ratio: 0.86, streak_days: 6, best_streak: 21
  };

  /* ---------- pengguna: kepatuhan 30 hari ---------- */
  var adherenceDaily = (function () {
    var out = [];
    for (var i = 29; i >= 0; i--) {
      var r = [1, 1, 0.5, 1, 1, 0.75, 1, 1, 1, 0, 1, 1, 0.5, 1, 1, 1, 0.75, 1, 1, 1, 0.5, 1, 1, 1, 0.75, 1, 0, 1, 1, 1][29 - i];
      out.push({
        date: ymd(shiftDays(-i)), scheduled: 7,
        taken: Math.round(7 * (r === 1 ? 1 : r === 0.75 ? 0.85 : r === 0.5 ? 0.57 : 0.3)),
        ratio: r === 1 ? 0.98 : r === 0.75 ? 0.85 : r === 0.5 ? 0.57 : 0.3
      });
    }
    return out;
  })();

  /* ---------- caregiver view: pasien yang dipantau ---------- */
  var carePatients = [
    {
      owner_id: "u-04", name: "Ibu Sumarni", relation: "parent", role: "viewer", timezone: "Asia/Jakarta",
      doses_total: 6, taken: 3, late: 1, skipped: 0, missed: 1, pending: 1, adherence_ratio: 0.67, adherence_7d: 0.62, streak_missed: 2,
      items: [
        { medication_id: "med-04", name: "Amlodipine 10", color_tag: "#2dd4bf", schedule_id: "tm-01", time_local: "07:00", status: "taken", taken_at: iso(shiftDays(0, 7, 20)), recorded_by: "self" },
        { medication_id: "med-05", name: "Metformin 500", color_tag: "#5aa9ff", schedule_id: "tm-03", time_local: "08:00", status: "late", taken_at: iso(shiftDays(0, 9, 35)), recorded_by: "caregiver" },
        { medication_id: "med-04", name: "Amlodipine 10", color_tag: "#2dd4bf", schedule_id: "tm-02", time_local: "14:00", status: "missed", taken_at: null, recorded_by: null },
        { medication_id: "med-05", name: "Metformin 500", color_tag: "#5aa9ff", schedule_id: "tm-04", time_local: "19:00", status: "taken", taken_at: iso(shiftDays(0, 19, 5)), recorded_by: "self" },
        { medication_id: "med-06", name: "Vitamin D3", color_tag: "#f7bb59", schedule_id: "tm-08", time_local: "09:00", status: "taken", taken_at: iso(shiftDays(0, 9, 40)), recorded_by: "self" },
        { medication_id: "med-07", name: "Furosemide 40", color_tag: "#c792ea", schedule_id: "tm-11", time_local: "20:30", status: "pending", taken_at: null, recorded_by: null }
      ]
    },
    {
      owner_id: "u-05", name: "Pak Hendra", relation: "nurse", role: "helper", timezone: "Asia/Jayapura",
      doses_total: 4, taken: 4, late: 0, skipped: 0, missed: 0, pending: 0, adherence_ratio: 1, adherence_7d: 0.94, streak_missed: 0,
      items: [
        { medication_id: "med-08", name: "Amlodipine 5", color_tag: "#2dd4bf", schedule_id: "tm-12", time_local: "06:00", status: "taken", taken_at: iso(shiftDays(0, 6, 10)), recorded_by: "self" },
        { medication_id: "med-08", name: "Amlodipine 5", color_tag: "#2dd4bf", schedule_id: "tm-13", time_local: "18:00", status: "taken", taken_at: iso(shiftDays(0, 18, 2)), recorded_by: "self" },
        { medication_id: "med-09", name: "Glibenclamide 5", color_tag: "#f59e0b", schedule_id: "tm-14", time_local: "07:00", status: "taken", taken_at: iso(shiftDays(0, 7, 1)), recorded_by: "self" },
        { medication_id: "med-09", name: "Glibenclamide 5", color_tag: "#f59e0b", schedule_id: "tm-15", time_local: "19:00", status: "taken", taken_at: iso(shiftDays(0, 19, 3)), recorded_by: "caregiver" }
      ]
    }
  ];

  /* ---------- sesi (dummy) ---------- */
  var session = { user: null, role: null, admin_scope: null, logged_in_at: null };

  /* ---------- API publik ---------- */
  var API = {
    LABEL: LABEL, HARI: HARI, BULAN: BULAN, TODAY: TODAY, NOW: NOW,
    TIMEZONES: TIMEZONES,
    ymd: ymd, shiftDays: shiftDays, iso: iso,
    catalog: catalog, medications: medications, logs: logs, doseEvents: doseEvents,
    careLinks: careLinks, stockTx: stockTx, notifPrefs: notifPrefs, deviceTokens: deviceTokens,
    inbox: inbox, users: users, jobs: jobs, audit: audit, stats: stats,
    todayOverview: todayOverview, adherenceDaily: adherenceDaily, carePatients: carePatients,
    session: session,

    /* enum -> label Indonesia */
    t: function (group, value) {
      if (value === null || value === undefined || value === "") return "—";
      var g = LABEL[group];
      return (g && g[value]) || value;
    },

    /* Zona waktu Indonesia (3) + sisanya dikelompokkan per wilayah untuk
       dropdown bertingkat. Label wilayah memakai nama Indonesia. */
    ID_TIMEZONES: ["Asia/Jakarta", "Asia/Makassar", "Asia/Jayapura"],
    TZ_REGION_ID: {
      Asia: "Asia", Africa: "Afrika", America: "Amerika", Antarctica: "Antarktika",
      Arctic: "Arktik", Atlantic: "Atlantik", Australia: "Australia", Europe: "Eropa",
      Indian: "Samudra Hindia", Pacific: "Pasifik", UTC: "UTC"
    },
    timezoneGroups: function () {
      var self = this, seen = {}, groups = [];
      var idSet = {};
      this.ID_TIMEZONES.forEach(function (z) { idSet[z] = true; });
      this.TIMEZONES.forEach(function (t) {
        var id = t[0], off = t[1];
        if (seen[id]) return;
        seen[id] = true;
        var region = id.indexOf("/") >= 0 ? id.split("/")[0] : "UTC";
        var label = self.TZ_REGION_ID[region] || region;
        var g = groups.filter(function (x) { return x.key === label; })[0];
        if (!g) { g = { key: label, items: [] }; groups.push(g); }
        g.items.push({ id: id, offset: off, indonesia: !!idSet[id] });
      });
      return groups;
    },

    /* obat yang masih hidup */
    activeMeds: function () { return this.medications.filter(function (m) { return !m.deleted_at; }); },
    archivedMeds: function () { return this.medications.filter(function (m) { return !!m.deleted_at || !m.is_active; }); },
    med: function (id) { return this.medications.filter(function (m) { return m.id === id; })[0] || null; },
    cat: function (id) { return this.catalog.filter(function (c) { return c.id === id; })[0] || null; },
    user: function (id) { return this.users.filter(function (u) { return u.id === id; })[0] || null; },

    /* jadwal hari ini dari aturan (bukan baris per hari) */
    todaySchedule: function () {
      var day = NOW.getDay(); // 0=Min .. 6=Sab
      var bit = Math.pow(2, (day + 6) % 7); // Senin=1 ... Minggu=64
      var out = [];
      var self = this;
      this.activeMeds().forEach(function (m) {
        var f = m.frequency;
        if (f.pattern === "prn") return;
        if (f.pattern === "weekly" && !(f.days_of_week & bit)) return;
        // cek rentang terapi
        if (m.end_date && TODAY > m.end_date) return;
        if (m.start_date && TODAY < m.start_date) return;
        // cek siklus: hari ke-N sejak start
        if (f.pattern === "monthly_cycle") {
          var start = new Date(m.start_date + "T00:00:00");
          var diff = Math.floor((new Date(TODAY + "T00:00:00") - start) / 86400000);
          var cycle = f.cycle_intake_days + f.cycle_pause_days;
          if (cycle > 0 && (diff % cycle) >= f.cycle_intake_days) return; // masa jeda
        }
        m.times.filter(function (t) { return t.is_active; }).forEach(function (t) {
          if (f.pattern === "weekly" && !(t.weekdays & bit)) return;
          if (f.pattern === "interval") {
            var hh = parseInt(t.time_local.slice(0, 2), 10);
            var sh = parseInt((f.start_time || "00:00").slice(0, 2), 10);
            var d = ((hh - sh) % 24 + 24) % 24;
            if (d % f.interval_hours !== 0) return;
          }
          var log = self.logFor(m.id, t.id, TODAY, +t.time_local.slice(0, 2), +t.time_local.slice(3, 5));
          out.push({ med: m, time: t, log: log });
        });
      });
      out.sort(function (a, b) { return a.time.time_local < b.time.time_local ? -1 : 1; });
      return out;
    },

    logFor: function (medId, schedId, date, hh, mm) {
      return this.logs.filter(function (l) {
        return !l.deleted_at && l.medication_id === medId && l.schedule_id === schedId &&
          l.slot_date === date && l.scheduled_hour === hh && l.scheduled_minute === mm;
      })[0] || null;
    },

    prnLogsToday: function () {
      return this.logs.filter(function (l) { return !l.medication_id || (l.schedule_id === null && l.slot_date === TODAY && !l.deleted_at); });
    },

    /* status untuk kartu dosis */
    doseStatus: function (item) {
      if (item.log) return item.log.status;
      var hh = +item.time.time_local.slice(0, 2), mm = +item.time.time_local.slice(3, 5);
      var nowM = NOW.getHours() * 60 + NOW.getMinutes();
      var schM = hh * 60 + mm;
      return nowM >= schM ? "pending" : "upcoming";
    },

    needsRefill: function () {
      var self = this;
      return this.activeMeds().filter(function (m) {
        var s = m.stock;
        return s && s.refill_reminder && s.current_qty <= s.refill_threshold_qty;
      });
    },

    lowStock: function () {
      var self = this;
      return this.activeMeds().filter(function (m) {
        var s = m.stock;
        return s && s.refill_threshold_qty != null && s.current_qty <= s.refill_threshold_qty;
      }).map(function (m) {
        var s = m.stock;
        var perDay = self.perDay(m);
        return { med: m, stock: s, perDay: perDay, daysLeft: perDay > 0 ? Math.floor(s.current_qty / perDay) : null, dosesLeft: Math.floor(s.current_qty / s.qty_per_dose) };
      });
    },

    perDay: function (m) {
      var f = m.frequency;
      if (f.pattern === "daily") return f.times_per_day;
      if (f.pattern === "interval") return 24 / f.interval_hours;
      if (f.pattern === "weekly") return this.bitCount(f.days_of_week) * (m.times.length || 1) / 7;
      if (f.pattern === "monthly_cycle") {
        var cycle = f.cycle_intake_days + f.cycle_pause_days;
        return cycle > 0 ? (f.cycle_intake_days / cycle) * (m.times.length || 1) : 0;
      }
      return 0;
    },

    bitCount: function (mask) {
      var n = 0; for (var i = 1; i <= 64; i *= 2) if (mask & i) n++; return n;
    },

    stockPercent: function (m) {
      if (!m.stock || !m.stock.initial_qty) return 0;
      return Math.max(0, Math.min(100, Math.round((m.stock.current_qty / m.stock.initial_qty) * 100)));
    },

    fmtDate: function (dstr, opts) {
      if (!dstr) return "—";
      var d = new Date(dstr + (dstr.length === 10 ? "T00:00:00" : ""));
      if (isNaN(d.getTime())) return dstr;
      var o = opts || {};
      if (o.long) return HARI[d.getDay()] + ", " + d.getDate() + " " + BULAN[d.getMonth()] + " " + d.getFullYear();
      if (o.mid) return d.getDate() + " " + BULAN[d.getMonth()].slice(0, 3) + " " + d.getFullYear();
      if (o.clock) return String(d.getHours()).padStart(2, "0") + ":" + String(d.getMinutes()).padStart(2, "0");
      return d.getDate() + " " + BULAN[d.getMonth()].slice(0, 3);
    },

    fmtDateTime: function (isostr) {
      if (!isostr) return "—";
      var d = new Date(isostr);
      if (isNaN(d.getTime())) return isostr;
      return d.getDate() + " " + BULAN[d.getMonth()].slice(0, 3) + " " + d.getFullYear() + " " +
        String(d.getHours()).padStart(2, "0") + ":" + String(d.getMinutes()).padStart(2, "0");
    },

    relTime: function (isostr) {
      if (!isostr) return "—";
      var diff = (NOW - new Date(isostr)) / 1000;
      if (diff < 60) return "baru saja";
      if (diff < 3600) return Math.floor(diff / 60) + " mnt lalu";
      if (diff < 86400) return Math.floor(diff / 3600) + " jam lalu";
      if (diff < 604800) return Math.floor(diff / 86400) + " hari lalu";
      return this.fmtDate(String(isostr).slice(0, 10), { mid: true });
    },

    /* ---------- persistensi dummy ---------- */
    save: function () {
      try {
        localStorage.setItem(KEY, JSON.stringify({
          logs: this.logs, medications: this.medications, catalog: this.catalog,
          careLinks: this.careLinks, stockTx: this.stockTx, notifPrefs: this.notifPrefs,
          inbox: this.inbox, users: this.users, jobs: this.jobs,
          carePatients: this.carePatients, doseEvents: this.doseEvents, deviceTokens: this.deviceTokens,
          session: this.session
        }));
      } catch (e) { /* mode privat: abaikan */ }
    },
    load: function () {
      try {
        var raw = localStorage.getItem(KEY);
        if (!raw) return false;
        var s = JSON.parse(raw);
        ["logs", "medications", "catalog", "careLinks", "stockTx", "notifPrefs", "inbox", "users", "jobs",
          "carePatients", "doseEvents", "deviceTokens"].forEach(function (k) {
          if (s[k]) API[k] = s[k];
        });
        if (s.session) API.session = s.session;
        return true;
      } catch (e) { return false; }
    },
    reset: function () {
      try { localStorage.removeItem(KEY); } catch (e) {}
      location.reload();
    },

    /* ---------- pencatatan dosis (dummy, memengaruhi stok) ---------- */
    /* takenTime (opsional, "HH:MM") = jam obat benar-benar diminum. Dipakai
       saat pengguna baru mencatat di aplikasi padahal sudah minum lebih awal;
       status late dihitung dari jam ini, bukan dari jam sistem. */
    markTaken: function (medId, schedId, hh, mm, takenTime) {
      var m = this.med(medId); if (!m) return null;
      var existing = this.logFor(medId, schedId, TODAY, hh, mm);
      var now = new Date();
      var takenAt = now;
      if (takenTime) {
        var tp = String(takenTime).split(":");
        takenAt = new Date(NOW);
        takenAt.setHours(+tp[0], +tp[1], 0, 0);
      }
      var sch = new Date(NOW); sch.setHours(hh, mm, 0, 0);
      var late = (takenAt - sch) / 60000 > (m.frequency.tolerance_minutes || 60);
      if (existing) {
        existing.status = late ? "late" : "taken";
        existing.taken_at = takenAt.toISOString();
      } else {
        existing = lg("lg-" + Math.random().toString(36).slice(2, 8), medId, schedId, TODAY, hh, mm,
          late ? "late" : "taken", takenAt.toISOString(), {});
        this.logs.push(existing);
      }
      if (m.stock && m.stock.current_qty >= m.stock.qty_per_dose) {
        m.stock.current_qty -= m.stock.qty_per_dose;
        this.stockTx.unshift({
          id: "tx-" + Math.random().toString(36).slice(2, 8), medication_id: medId, kind: "consume",
          qty_delta: -m.stock.qty_per_dose, balance_after: m.stock.current_qty, note: null,
          created_at: takenAt.toISOString(), log_id: existing.id
        });
      }
      this.save();
      return existing;
    },

    /* Ubah jam dosis yang sudah tercatat. Status taken/late dihitung ulang. */
    editTaken: function (medId, schedId, hh, mm, takenTime) {
      var m = this.med(medId); if (!m) return null;
      var existing = this.logFor(medId, schedId, TODAY, hh, mm);
      if (!existing) return null;
      var tp = String(takenTime).split(":");
      var takenAt = new Date(NOW);
      takenAt.setHours(+tp[0], +tp[1], 0, 0);
      var sch = new Date(NOW); sch.setHours(hh, mm, 0, 0);
      var late = (takenAt - sch) / 60000 > (m.frequency.tolerance_minutes || 60);
      existing.status = late ? "late" : "taken";
      existing.taken_at = takenAt.toISOString();
      this.save();
      return existing;
    },

    markSkipped: function (medId, schedId, hh, mm, reason) {
      var existing = this.logFor(medId, schedId, TODAY, hh, mm);
      if (existing) {
        existing.status = "skipped"; existing.taken_at = null; existing.note = reason || null;
      } else {
        existing = lg("lg-" + Math.random().toString(36).slice(2, 8), medId, schedId, TODAY, hh, mm, "skipped", null, { note: reason });
        this.logs.push(existing);
      }
      this.save(); return existing;
    },

    snooze: function (medId, schedId, hh, mm, minutes) {
      var existing = this.logFor(medId, schedId, TODAY, hh, mm);
      if (existing) { existing.status = "snoozed"; existing.snooze_minutes = minutes; }
      else {
        this.logs.push(lg("lg-" + Math.random().toString(36).slice(2, 8), medId, schedId, TODAY, hh, mm, "snoozed", null, { note: "Ditunda " + minutes + " menit" }));
      }
      this.save();
    },

    restock: function (medId, qty, note) {
      var m = this.med(medId); if (!m || !m.stock) return null;
      m.stock.current_qty += +qty;
      m.stock.last_restocked_at = new Date().toISOString();
      var t = {
        id: "tx-" + Math.random().toString(36).slice(2, 8), medication_id: medId, kind: "restock",
        qty_delta: +qty, balance_after: m.stock.current_qty, note: note || null,
        created_at: new Date().toISOString(), log_id: null
      };
      this.stockTx.unshift(t);
      this.save();
      return t;
    },

    adjustStock: function (medId, delta, note) {
      var m = this.med(medId); if (!m || !m.stock) return null;
      var next = m.stock.current_qty + +delta;
      if (next < 0) return null;
      m.stock.current_qty = next;
      var t = {
        id: "tx-" + Math.random().toString(36).slice(2, 8), medication_id: medId, kind: "adjust",
        qty_delta: +delta, balance_after: next, note: note || null,
        created_at: new Date().toISOString(), log_id: null
      };
      this.stockTx.unshift(t);
      this.save();
      return t;
    },

    addMedication: function (input) {
      var id = "med-" + Math.random().toString(36).slice(2, 8);
      var m = {
        id: id, owner_id: "u-01", catalog_id: input.catalog_id || null, name: input.name,
        dose_value: input.dose_value, dose_unit_code: input.dose_unit_code, strength_note: input.strength_note || null,
        route_code: input.route_code || "O", dosage_form_code: input.dosage_form_code || "LOCAL-0001",
        meal_relation: input.meal_relation || "not_related", as_needed: !!input.as_needed, must_finish: !!input.must_finish,
        start_date: input.start_date || TODAY, start_time: input.start_time || "00:00", end_date: input.end_date || null,
        notes: input.notes || null, notes_reminder: input.notes_reminder || null, prescription_photo_url: null,
        color_tag: input.color_tag || "#2dd4bf", is_active: true, deleted_at: null,
        frequency: input.frequency || { id: "fq-" + id, pattern: "daily", times_per_day: 1, schedule_timezone: "Asia/Jakarta", tolerance_minutes: 60, start_time: "08:00" },
        times: (input.times || []).map(function (t, i) {
          return { id: "tm-" + id + "-" + i, time_local: t.time_local, label: t.label || "other", weekdays: t.weekdays == null ? 127 : t.weekdays, is_active: true, sort_order: i };
        }),
        stock: input.stock ? {
          id: "st-" + id, unit_code: input.stock.unit_code, initial_qty: input.stock.initial_qty,
          current_qty: input.stock.initial_qty, qty_per_dose: input.stock.qty_per_dose,
          refill_threshold_qty: input.stock.refill_threshold_qty, refill_reminder: !!input.stock.refill_reminder,
          last_restocked_at: null
        } : null
      };
      this.medications.push(m);
      if (m.stock) {
        this.stockTx.unshift({
          id: "tx-" + id, medication_id: id, kind: "initial", qty_delta: m.stock.initial_qty,
          balance_after: m.stock.initial_qty, note: "Stok awal", created_at: new Date().toISOString(), log_id: null
        });
      }
      this.save();
      return m;
    },

    addCatalog: function (input) {
      var c = {
        id: "cat-" + Math.random().toString(36).slice(2, 8), display_name: input.display_name,
        nama_dagang: input.nama_dagang || null, generik_nama: input.generik_nama || null, is_preset: false,
        curation_status: "user_private", source: "manual", dosage_form_name: input.dosage_form_name || "Tablet",
        route_code: input.route_code || "O", dose_value_text: input.dose_value_text || null,
        ucum_code: input.ucum_code || null, uom_name: input.uom_name || null, atc_l5_code: null,
        kfa_code: null, bpom_nie: null, packaging_options: [], active_ingredients: [],
        indication: null, warning: null, owner_id: "u-01", curated_by: null, curated_at: null
      };
      this.catalog.push(c); this.save(); return c;
    },

    setCuration: function (catId, status, reason) {
      var c = this.cat(catId); if (!c || !c.is_preset) return null;
      var before = c.curation_status;
      c.curation_status = status;
      c.curated_by = "admin@example.com";
      c.curated_at = new Date().toISOString();
      this.audit.unshift({
        id: 55290 + this.audit.length, actor_email: "admin@example.com", actor_role: "admin",
        action: status === "published" ? "catalog_publish" : status === "draft" ? "catalog_unpublish" : "catalog_archive",
        resource_type: "catalog", resource_id: c.id, created_at: new Date().toISOString(), ip: "10.20.0.5",
        meta: { reason: reason || null }, before_state: { curation_status: before }, after_state: { curation_status: status }
      });
      this.save(); return c;
    },

    setUserActive: function (userId, active, reason) {
      var u = this.user(userId); if (!u) return null;
      var before = u.is_active;
      u.is_active = !!active;
      this.audit.unshift({
        id: 55300 + this.audit.length, actor_email: "admin@example.com", actor_role: "admin",
        action: active ? "user_enable" : "user_disable", resource_type: "user", resource_id: u.id,
        created_at: new Date().toISOString(), ip: "10.20.0.5", meta: { reason: reason || null },
        before_state: { is_active: before }, after_state: { is_active: !!active }
      });
      this.save(); return u;
    },

    retryJob: function (jobId) {
      var j = this.jobs.filter(function (x) { return x.id === jobId; })[0];
      if (!j) return null;
      j.status = "pending"; j.attempts = 0; j.last_error = null;
      this.audit.unshift({
        id: 55310 + this.audit.length, actor_email: "admin@example.com", actor_role: "admin",
        action: "notif_retry", resource_type: "notification", resource_id: null,
        created_at: new Date().toISOString(), ip: "10.20.0.5", meta: { job_id: jobId },
        before_state: { status: "failed" }, after_state: { status: "pending" }
      });
      this.save(); return j;
    },

    cancelJob: function (jobId, reason) {
      var j = this.jobs.filter(function (x) { return x.id === jobId; })[0];
      if (!j) return null;
      j.status = "cancelled";
      this.audit.unshift({
        id: 55320 + this.audit.length, actor_email: "admin@example.com", actor_role: "admin",
        action: "notif_cancel", resource_type: "notification", resource_id: null,
        created_at: new Date().toISOString(), ip: "10.20.0.5", meta: { job_id: jobId, reason: reason || null },
        before_state: { status: "pending" }, after_state: { status: "cancelled" }
      });
      this.save(); return j;
    },

    /* demo auth — kredensial apa pun lolos, kecuali email admin */
    login: function (email, password) {
      var isAdmin = /admin|kurator|support/.test(String(email).toLowerCase());
      this.session = {
        user: isAdmin ? { id: "u-07", email: email, full_name: "Admin Pantau", role: "admin", admin_scope: null } :
          { id: "u-01", email: email, full_name: "Budi Santoso", role: "user", admin_scope: null },
        role: isAdmin ? "admin" : "user",
        admin_scope: null,
        logged_in_at: new Date().toISOString()
      };
      this.save();
      return this.session;
    },

    logout: function () { this.session = { user: null, role: null, admin_scope: null, logged_in_at: null }; this.save(); }
  };

  API.load();
  window.PO = API;
})();