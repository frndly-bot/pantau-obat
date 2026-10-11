/* ============================================================
   Pantau Obat — Design v2 · Helper bersama (app.js)
   Isi: ikon SVG, toast, tema, shell user/admin, drawer/modal/sheet,
        tab, segmen, bagan, kalender, heatmap, guard sesi dummy.

   Aturan yang dipatuhi:
   - TANPA dependensi eksternal. Semua ikon SVG inline (bisa offline).
   - Enum tetap BAHASA INGGRIS; terjemahan lewat PO.t().
   - Tombol di luar cakupan dummy memanggil POAPP.dead() → toast
     "belum diintegrasi", bukan diam.
   ============================================================ */
(function () {
  "use strict";

  /* ---------------- Ikon: Lucide (inline, offline) ----------------
     Peta path ada di assets/icons.js, dibuat dari lucide-static v1.55.0.
     Atribut mengikuti bawaan Lucide: fill none, stroke currentColor,
     stroke-width 2, linecap/linejoin round. Tidak ada CDN, jadi prototipe
     tetap utuh saat dibuka tanpa internet. */
  var I = window.LUCIDE || {};

  function icon(name, cls) {
    var body = I[name] || I.circle || "";
    return '<svg class="' + (cls || "ic") + '" viewBox="0 0 24 24" fill="none" stroke="currentColor" ' +
      'stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' + body + "</svg>";
  }

  /* ---------------- Utilitas ---------------- */
  function esc(s) {
    if (s === null || s === undefined) return "";
    return String(s).replace(/[&<>"']/g, function (c) {
      return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c];
    });
  }

  function pct(x, digits) {
    if (x === null || x === undefined || isNaN(x)) return "—";
    return (x * 100).toFixed(digits === undefined ? 0 : digits) + "%";
  }

  /* Jam lokal "HH:MM" dari timestamp ISO. Dipakai untuk menampilkan jam minum
     sebenarnya. Jangan pakai String(iso).slice(0,10) — itu membuang jamnya dan
     selalu menghasilkan 00:00. */
  function clockAt(iso) {
    if (!iso) return "";
    var d = new Date(iso);
    if (isNaN(d.getTime())) return "";
    return String(d.getHours()).padStart(2, "0") + ":" + String(d.getMinutes()).padStart(2, "0");
  }

  /* Terjemahan ringkas untuk teks yang dibangun app.js (bukan lewat data-i18n,
     karena string dibuat runtime). Fallback ke kunci bila mesin i18n belum ada. */
  function tr(key) {
    return window.POI18N ? POI18N.t(key) : key;
  }

  function money(n) {
    return String(n).replace(/\B(?=(\d{3})+(?!\d))/g, ".");
  }

  function qs(name) {
    var m = new RegExp("[?&]" + name + "=([^&]*)").exec(location.search);
    return m ? decodeURIComponent(m[1].replace(/\+/g, " ")) : null;
  }

  /* Minimal HTML: buang tag, sisakan teks (untuk konfirmasi & pencarian). */
  function text(s) { return esc(String(s === null || s === undefined ? "—" : s)); }

  function debounce(fn, ms) {
    var t;
    return function () {
      var a = arguments, self = this;
      clearTimeout(t);
      t = setTimeout(function () { fn.apply(self, a); }, ms || 220);
    };
  }

  /* ---------------- Toast ---------------- */
  var ICON_KIND = { ok: "check", warn: "alert", err: "alert", info: "info" };

  function toast(kind, title, body) {
    var box = document.querySelector(".toasts");
    if (!box) {
      box = document.createElement("div");
      box.className = "toasts";
      document.body.appendChild(box);
    }
    var el = document.createElement("div");
    el.className = "toast " + (kind || "info");
    el.innerHTML = icon(ICON_KIND[kind] || "info") +
      "<div class='grow'><b>" + esc(title || "") + "</b>" +
      (body ? "<span>" + esc(body) + "</span>" : "") + "</div>" +
      "<button class='btn-ghost' data-x aria-label='Tutup'>" + icon("x") + "</button>";
    box.appendChild(el);
    function kill() {
      el.classList.add("out");
      setTimeout(function () { if (el.parentNode) el.parentNode.removeChild(el); }, 200);
    }
    el.querySelector("[data-x]").addEventListener("click", kill);
    setTimeout(kill, kind === "err" ? 6000 : 3800);
    return el;
  }

  /* Tombol/tindakan di luar cakupan dummy — selalu memberi kabar. */
  function dead(label) {
    return toast("info", "Belum diintegrasi",
      (label ? label + " — " : "") + "prototipe statis, tanpa backend.");
  }

  function bindDead(root) {
    (root || document).querySelectorAll("[data-dead]").forEach(function (el) {
      el.addEventListener("click", function (e) {
        e.preventDefault();
        dead(el.getAttribute("data-dead"));
      });
    });
  }

  /* ---------------- Tema ---------------- */
  var TKEY = "po.design.theme";
  var theme = {
    get: function () {
      try { return localStorage.getItem(TKEY) || "dark"; } catch (e) { return "dark"; }
    },
    set: function (v) {
      var root = document.documentElement;
      root.dataset.theme = v;
      // konvensi shadcn: kelas `.dark` di <html>. Dipasang bersamaan dengan
      // data-theme supaya token gelap berlaku di kedua jalur.
      root.classList.toggle("dark", v === "dark");
      try { localStorage.setItem(TKEY, v); } catch (e) {}
      document.querySelectorAll("[data-theme-knob]").forEach(function (k) {
        k.innerHTML = v === "light" ? icon("sun") : icon("moon");
      });
    },
    init: function () { theme.set(theme.get()); },
    toggle: function () { theme.set(theme.get() === "light" ? "dark" : "light"); }
  };

  function themeToggle() {
    var v = theme.get();
    return '<button class="theme-toggle" data-theme-toggle aria-label="Ganti tema" title="Ganti tema">' +
      '<span class="knob" data-theme-knob>' + icon(v === "light" ? "sun" : "moon") + "</span></button>";
  }

  /* ---------------- Sesi dummy (guard) ---------------- */
  function userHome() { return "hari-ini.html"; }
  function adminHome() { return "dashboard.html"; }

  function requireUser() {
    if (!PO.session || !PO.session.user) {
      location.replace("login.html?next=" + encodeURIComponent(location.pathname.split("/").pop()));
      return null;
    }
    return PO.session;
  }

  function requireAdmin() {
    if (!PO.session || !PO.session.user || PO.session.role !== "admin") {
      location.replace("login.html?next=" + encodeURIComponent(location.pathname.split("/").pop()));
      return null;
    }
    return PO.session;
  }

  function logout(to) {
    var wasAdmin = !!(PO.session && PO.session.role === "admin");
    PO.logout();
    location.href = to || (wasAdmin ? "login.html" : "login.html");
  }

  function expired() { return toast("warn", "Sesi dummy", "Halaman ini butuh login. Kredensial apa pun diterima."); }

  /* ---------------- Popup notifikasi (lonceng di bilah atas user) ----------------
     Klik lonceng menampilkan ringkasan notifikasi langsung di tempat. Sumbernya
     sama dengan "Kotak masuk" di Pengaturan (PO.inbox) supaya tidak ada dua
     kebenaran. Aksi lanjutan (buka Pengaturan) tetap tersedia sebagai sekunder. */
  /* Ikon per jenis notifikasi. dose_reminder sengaja TIDAK memakai "bell"
     karena kanal push juga memakai "bell" — dua lonceng berdampingan terlihat
     seperti duplikasi. Jam lebih tepat untuk pengingat dosis terjadwal. */
  function notifIcon(kind) {
    return kind === "refill_reminder" ? "box"
      : kind === "missed_escalation" ? "alert"
      : kind === "care_digest" ? "activity"
      : kind === "dose_reminder" ? "clock" : "bell";
  }

  function notifChannelIcon(ch) {
    return ch === "push" ? "bell" : ch === "telegram" ? "send" : ch === "email" ? "mail" : "monitor";
  }

  /* Satu notifikasi = satu ikon jenis + satu ikon kanal (selalu berbeda, lihat
     notifIcon). Tanpa badge baca/baru karena daftar ini hanya memuat yang
     belum dibaca. */
  function notifItemHTML(n) {
    return '<button type="button" class="notif-item unread" data-notif="' + esc(n.id) + '">' +
      '<span class="avatar sm">' + icon(notifIcon(n.kind)) + "</span>" +
      '<span class="grow">' +
        '<span class="sm trunc"><b>' + esc(n.title) + "</b></span>" +
        '<span class="xs muted-3 trunc-2">' + esc(n.body) + "</span>" +
        '<span class="xs muted-3">' + icon(notifChannelIcon(n.channel)) +
          " " + esc(PO.t("channel", n.channel)) + " · " + esc(PO.relTime(n.created_at)) + "</span>" +
      "</span></button>";
  }

  /* Tandai satu notifikasi sudah dibaca, lalu keluarkan dari daftar dengan
     animasi. Dipakai oleh klik maupun geser-ke-kanan. */
  function markNotifRead(el, id) {
    var n = PO.inbox.filter(function (x) { return x.id === id; })[0];
    if (n && !n.read) { n.read = true; PO.save(); }
    if (el) {
      el.classList.add("np-out");
      el.style.transform = "translateX(110%)";
      el.style.opacity = "0";
    }
    syncBell();
    setTimeout(notifRender, 220);
  }

  /* Status geser untuk daftar notifikasi (geser kanan = tandai dibaca). */
  var npSwipe = null;
  var npSwipeMoved = false;

  function notifPopup() {
    var scrim = document.getElementById("notif-scrim");
    var pop = document.getElementById("notif-pop");
    if (!scrim || !pop) {
      scrim = document.createElement("div");
      scrim.className = "notif-scrim";
      scrim.id = "notif-scrim";
      pop = document.createElement("div");
      pop.className = "notif-pop";
      pop.id = "notif-pop";
      pop.setAttribute("role", "dialog");
      pop.setAttribute("aria-modal", "false");
      pop.setAttribute("aria-label", "Notifikasi");
      document.body.appendChild(scrim);
      document.body.appendChild(pop);

      /* Klik di luar popup menutupnya. */
      scrim.addEventListener("click", function () { notifClose(); });
      document.addEventListener("keydown", function (e) {
        if (e.key === "Escape" && pop.classList.contains("on")) notifClose();
      });
      /* Delegasi klik untuk item + aksi di dalam popup. */
      pop.addEventListener("click", function (e) {
        var close = e.target.closest("[data-np-close]");
        if (close) { notifClose(); return; }
        var all = e.target.closest("[data-np-read]");
        if (all) {
          PO.inbox.forEach(function (n) { n.read = true; });
          PO.save();
          notifRender();
          syncBell();
          toast("ok", tr("settings.all_read"), "");
          return;
        }
        var item = e.target.closest("[data-notif]");
        if (item) {
          /* Abaikan klik yang sebenarnya bagian dari geseran. */
          if (npSwipeMoved) { npSwipeMoved = false; return; }
          markNotifRead(item, item.getAttribute("data-notif"));
        }
      });

      /* Geser ke kanan untuk menandai sudah dibaca (di luar layar = hilang).
         Listener dipasang di pop (bukan .np-list) karena isi pop diganti
         ulang oleh notifRender(); event touch tetap menggelembung ke sini. */
      pop.addEventListener("touchstart", function (e) {
        var it = e.target.closest("[data-notif]");
        if (!it || e.touches.length !== 1) return;
        npSwipe = { el: it, x: e.touches[0].clientX, y: e.touches[0].clientY, dx: 0, active: false };
        npSwipeMoved = false;
      }, { passive: true });
      pop.addEventListener("touchmove", function (e) {
        if (!npSwipe || e.touches.length !== 1) return;
        var dx = e.touches[0].clientX - npSwipe.x;
        var dy = e.touches[0].clientY - npSwipe.y;
        /* Kunci arah: geser horizontal (kanan) saja yang dianggap swipe. */
        if (!npSwipe.active) {
          if (Math.abs(dx) < 8) return;
          if (Math.abs(dy) > Math.abs(dx)) { npSwipe = null; return; }
          npSwipe.active = true;
        }
        npSwipe.dx = Math.max(0, dx);
        npSwipe.el.style.transition = "none";
        npSwipe.el.style.transform = "translateX(" + npSwipe.dx + "px)";
        npSwipeMoved = true;
      }, { passive: true });
      pop.addEventListener("touchend", function () {
        if (!npSwipe) return;
        var s = npSwipe; npSwipe = null;
        if (!s.active) { npSwipeMoved = false; return; }
        s.el.style.transition = "";
        if (s.dx > 72) {
          markNotifRead(s.el, s.el.getAttribute("data-notif"));
        } else {
          s.el.style.transform = "";
        }
        setTimeout(function () { npSwipeMoved = false; }, 60);
      });
    }
    return { scrim: scrim, pop: pop };
  }

  function notifRender() {
    var refs = notifPopup();
    /* Daftar hanya memuat yang BELUM dibaca. Setelah ditandai dibaca, item
       keluar dari daftar — dan tidak muncul lagi saat popup dibuka ulang. */
    var unread = PO.inbox.filter(function (n) { return !n.read; });
    refs.pop.innerHTML =
      '<div class="np-head">' +
        '<div class="grow"><h3>' + icon("bell") + esc(tr("settings.inbox")) + "</h3>" +
          (unread.length ? badge(unread.length + " " + tr("settings.unread"), "badge-info badge-pulse", true) : "") + "</div>" +
        '<button type="button" class="icon-btn" data-np-close aria-label="' + esc(tr("common.close")) + '">' + icon("x") + "</button>" +
      "</div>" +
      '<div class="np-list">' +
        (unread.length
          ? unread.map(notifItemHTML).join("")
          : PO.inbox.length
            ? '<div class="empty">' + icon("check-check") + "<b>" + esc(tr("settings.all_caught_up")) + "</b>" +
              "<span>" + esc(tr("settings.all_caught_up_hint")) + "</span></div>"
            : '<div class="empty">' + icon("bell") + "<b>" + esc(tr("settings.no_notifications")) + "</b>" +
              "<span>" + esc(tr("settings.no_notifications_hint")) + "</span></div>") +
      "</div>" +
      '<div class="np-foot">' +
        (unread.length
          ? '<button type="button" class="btn btn-ghost btn-sm" data-np-read>' + esc(tr("settings.mark_all_as_read")) + "</button>"
          : '<span class="xs muted">' + icon("check") + esc(tr("settings.all_read")) + "</span>") +
        '<a class="btn btn-outline btn-sm" href="pengaturan.html#notifikasi">' + esc(tr("settings.manage_notifications")) + "</a>" +
      "</div>";
  }

  function notifPosition(btn) {
    var pop = document.getElementById("notif-pop");
    if (!pop || !btn) return;
    var r = btn.getBoundingClientRect();
    var w = pop.offsetWidth || 336;
    var h = pop.offsetHeight || 400;
    /* Rata kanan dengan tombol, dijepit ke tepi viewport supaya tidak meluber. */
    var left = Math.min(Math.max(8, r.right - w), Math.max(8, window.innerWidth - w - 8));
    var top = r.bottom + 8;
    if (top + h > window.innerHeight - 8) top = Math.max(8, window.innerHeight - h - 8);
    pop.style.left = left + "px";
    pop.style.top = top + "px";
    pop.style.right = "auto";
  }

  function notifOpen(btn) {
    var refs = notifPopup();
    notifRender();
    /* Tampilkan dulu supaya offsetWidth/Height valid, baru posisikan. */
    refs.scrim.classList.add("on");
    refs.pop.classList.add("on");
    notifPosition(btn || document.querySelector("[data-bell]"));
  }

  function notifClose() {
    var scrim = document.getElementById("notif-scrim");
    var pop = document.getElementById("notif-pop");
    if (scrim) scrim.classList.remove("on");
    if (pop) pop.classList.remove("on");
  }

  function notifToggle(btn) {
    var pop = document.getElementById("notif-pop");
    if (pop && pop.classList.contains("on")) notifClose();
    else notifOpen(btn);
  }

  /* Sinkronkan titik merah lonceng dengan jumlah notifikasi belum dibaca. */
  function syncBell() {
    var bell = document.querySelector("[data-bell]");
    if (!bell) return;
    var unread = PO.inbox.filter(function (n) { return !n.read; }).length;
    var dot = bell.querySelector(".dot-badge");
    if (unread && !dot) bell.insertAdjacentHTML("beforeend", '<span class="dot-badge"></span>');
    else if (!unread && dot) dot.remove();
  }

  /* ---------------- Shell USER (mobile-first) ---------------- */
  var USER_NAV = [
    { id: "hari-ini", href: "hari-ini.html", label: "Hari Ini", i18n: "hari_ini", ic: "home" },
    { id: "obat", href: "obat.html", label: "Obat", i18n: "obat", ic: "pill" },
    { id: "riwayat", href: "riwayat.html", label: "Riwayat", i18n: "riwayat", ic: "activity" },
    { id: "caregiver", href: "caregiver.html", label: "Caregiver", i18n: "caregiver", ic: "users" },
    { id: "pengaturan", href: "pengaturan.html", label: "Atur", i18n: "pengaturan", ic: "sliders" }
  ];

  function userShell(opt) {
    var top = document.getElementById("po-top");
    var nav = document.getElementById("po-nav");
    var unread = PO.inbox.filter(function (n) { return !n.read; }).length;
    var due = PO.todaySchedule().filter(function (s) {
      var st = PO.doseStatus(s);
      return st === "pending" || st === "snoozed";
    }).length;

    if (top) {
      top.outerHTML =
        '<header class="u-topbar">' +
          '<div class="row-b">' +
            /* Tombol kembali duduk di KIRI, sebelum judul. Panah kiri di sisi
               kanan topbar membingungkan (arahnya berlawanan dengan posisinya).
               Halaman tanpa opsi `back` tidak menampilkan apa pun di sini. */
            (opt.back ? '<a class="icon-btn back-btn" href="' + esc(opt.back) + '" aria-label="Kembali">' + icon("arrow-left") + "</a>" : "") +
            '<div class="grow">' +
              '<div class="t trunc">' + esc(opt.title || "") + "</div>" +
              /* sub adalah teks biasa — di-esc supaya tidak pernah merender
                 markup tak sengaja dari pemanggil. */
              (opt.sub ? '<div class="sub trunc">' + esc(opt.sub) + "</div>" : "") +
            "</div>" +
            '<div class="acts">' +
              /* actions memang berisi markup tombol yang dibangun pemanggil;
                 after TIDAK boleh di sini — ia callback, dipanggil boot(). */
              (opt.actions || "") +
              '<button class="icon-btn" data-bell aria-label="Notifikasi">' + icon("bell") +
                (unread ? '<span class="dot-badge"></span>' : "") + "</button>" +
            "</div>" +
          "</div>" +
        "</header>";
    }
    var bellBtn = document.querySelector("[data-bell]");
    if (bellBtn) {
      /* Klik lonceng = buka popup notifikasi di tempat, bukan pindah halaman. */
      bellBtn.addEventListener("click", function (e) {
        e.preventDefault();
        notifToggle(bellBtn);
      });
    }

    if (nav) {
      nav.outerHTML =
        '<nav class="u-bottomnav nav-5" aria-label="Navigasi utama">' +
          USER_NAV.map(function (n) {
            var badge = "";
            if (n.id === "hari-ini" && due) badge = '<span class="u-nav-badge">' + due + "</span>";
            // label lewat data-i18n: POI18N.apply() (dipanggil setelah boot)
            // menggantinya dengan locale aktif — key-based, siap bahasa baru.
            return '<a class="u-nav-item' + (opt.active === n.id ? " active" : "") + '" href="' + n.href + '">' +
              icon(n.ic) + '<span data-i18n="nav.' + n.i18n + '">' + n.label + "</span>" + badge + "</a>";
          }).join("") +
        "</nav>";
    }
  }

  /* ---------------- Shell ADMIN (desktop-first) ---------------- */
  var ADMIN_NAV = [
    { grp: "Utama", i18n: "grp_main" },
    { id: "dashboard", href: "dashboard.html", label: "Dashboard", i18n: "dashboard", ic: "grid" },
    { grp: "Pengguna", i18n: "grp_users" },
    { id: "users", href: "users.html", label: "Pengguna", i18n: "users", ic: "users" },
    { grp: "Katalog", i18n: "grp_catalog" },
    { id: "catalog", href: "catalog.html", label: "Preset Katalog", i18n: "catalog", ic: "database" },
    { id: "catalog-import", href: "catalog-import.html", label: "Impor Katalog", i18n: "catalog_import", ic: "upload" },
    { grp: "Operasi", i18n: "grp_ops" },
    { id: "notifications", href: "notifications.html", label: "Notifikasi", i18n: "notifications", ic: "send" },
    { id: "stats", href: "stats.html", label: "Statistik", i18n: "stats", ic: "bar" }
  ];

  function adminShell(opt) {
    var side = document.getElementById("po-side");
    var top = document.getElementById("po-top");
    var s = PO.session;
    var jobsFailed = PO.jobs.filter(function (j) { return j.status === "failed"; }).length;
    var cnt = { users: PO.stats.users_active, catalog: PO.stats.catalog_presets, notifications: jobsFailed };

    if (side) {
      var html = '<div class="a-brand"><span class="logo-mark">' + icon("shield") + "</span>" +
        "<div><div class='nm'>Pantau Obat</div><div class='sb'>Konsol admin</div></div></div>" +
        '<nav class="a-nav" aria-label="Navigasi admin">';
      ADMIN_NAV.forEach(function (n) {
        if (n.grp) {
          html += '<div class="grp" data-i18n="nav.' + n.i18n + '">' + n.grp + "</div>";
          return;
        }
        var c = cnt[n.id];
        html += '<a class="' + (opt.active === n.id ? "active" : "") + '" href="' + n.href + '">' +
          icon(n.ic) + '<span data-i18n="nav.' + n.i18n + '">' + n.label + "</span>" +
          (c ? '<span class="cnt">' + c + "</span>" : "") + "</a>";
      });
      html += "</nav>" +
        '<div class="foot">' +
          '<a class="who" href="user-detail.html?id=u-07">' +
            '<span class="avatar sm">' + esc((s.user.full_name || "A").slice(0, 2).toUpperCase()) + "</span>" +
            '<div class="grow"><div class="nm">' + esc(s.user.full_name) + "</div>" +
            '<div class="rl">' + (s.admin_scope ? "admin · " + esc(s.admin_scope.join(", ")) : "admin · semua bidang") + "</div></div>" +
            icon("chev-right") +
          "</a>" +
          '<button class="btn btn-ghost btn-block mt-8" data-logout>' + icon("logout") + "Keluar</button>" +
        "</div>";
      // innerHTML, BUKAN outerHTML: elemen <aside class="a-side" id="po-side">
      // harus tetap ada supaya CSS sidebar (sticky desktop / drawer mobile)
      // dan tombol toggle tetap bekerja.
      side.innerHTML = html;
    }

    if (top) {
      top.outerHTML =
        '<header class="a-top">' +
          '<button class="icon-btn sidebar-toggle" data-side-toggle aria-label="Menu">' + icon("list") + "</button>" +
          '<div class="grow"><div class="ttl">' + esc(opt.title || "") + "</div>" +
            (opt.crumb ? '<div class="crumb">' + opt.crumb + "</div>" : "") + "</div>" +
          '<div class="row gap-8">' + (opt.actions || "") +
            '<button class="icon-btn" data-bell aria-label="Notifikasi">' + icon("bell") +
              (jobsFailed ? '<span class="dot-badge"></span>' : "") + "</button>" +
          "</div>" +
        "</header>";
    }

    var toggle = document.querySelector("[data-side-toggle]");
    if (toggle) {
      toggle.addEventListener("click", function () {
        var scrim = document.querySelector(".a-scrim");
        document.getElementById("po-side").classList.add("open");
        if (scrim) scrim.classList.add("on");
      });
    }
    var scrim = document.querySelector(".a-scrim");
    if (scrim) {
      scrim.addEventListener("click", function () {
        var side2 = document.getElementById("po-side");
        if (side2) side2.classList.remove("open");
        scrim.classList.remove("on");
      });
    }
    var bell = document.querySelector("[data-bell]");
    if (bell) bell.addEventListener("click", function () { location.href = "notifications.html"; });
  }

  /* ---------------- Overlay: drawer / modal / sheet ---------------- */
  function overlay(id, open) {
    var el = document.getElementById(id);
    if (!el) return;
    el.classList.toggle("on", open);
    var scrim = document.getElementById(id + "-scrim") || document.querySelector(".scrim[data-for='" + id + "']");
    if (scrim) scrim.classList.toggle("on", open);
    document.body.style.overflow = open ? "hidden" : "";
  }

  function initOverlays(root) {
    (root || document).querySelectorAll("[data-open]").forEach(function (b) {
      b.addEventListener("click", function (e) {
        e.preventDefault();
        overlay(b.getAttribute("data-open"), true);
      });
    });
    (root || document).querySelectorAll("[data-close]").forEach(function (b) {
      b.addEventListener("click", function (e) {
        e.preventDefault();
        var t = b.getAttribute("data-close");
        if (t === "#all") {
          document.querySelectorAll(".modal.on,.drawer.on,.sheet.on,.scrim.on").forEach(function (x) {
            x.classList.remove("on");
          });
          document.body.style.overflow = "";
          return;
        }
        overlay(t, false);
      });
    });
    document.addEventListener("keydown", function (e) {
      if (e.key !== "Escape") return;
      document.querySelectorAll(".modal.on,.drawer.on,.sheet.on,.scrim.on,.a-side.open").forEach(function (x) {
        x.classList.remove("on");
      });
      document.body.style.overflow = "";
    });
  }

  /* ---------------- Tab + segmen ---------------- */
  function initTabs(root) {
    var box = root || document;
    box.querySelectorAll(".tabs").forEach(function (tabs) {
      tabs.addEventListener("click", function (e) {
        var tab = e.target.closest(".tab");
        if (!tab || !tabs.contains(tab)) return;
        tabs.querySelectorAll(".tab").forEach(function (t) { t.classList.remove("active"); });
        tab.classList.add("active");
        var scope = tabs.getAttribute("data-scope") || "";
        var key = tab.getAttribute("data-tab");
        box.querySelectorAll("[data-panel]").forEach(function (p) {
          if (scope && p.getAttribute("data-scope") !== scope) return;
          p.classList.toggle("hide", p.getAttribute("data-panel") !== key);
        });
      });
    });
  }

  function initSeg(root) {
    (root || document).querySelectorAll(".seg").forEach(function (seg) {
      seg.addEventListener("click", function (e) {
        var b = e.target.closest("button");
        if (!b || !seg.contains(b)) return;
        seg.querySelectorAll("button").forEach(function (x) { x.classList.remove("on"); });
        b.classList.add("on");
        if (seg.hasAttribute("data-filter")) {
          var k = b.getAttribute("data-val");
          var target = document.querySelector(seg.getAttribute("data-target") || "");
          if (target) filterRows(target, k);
        }
      });
    });
  }

  function filterRows(scope, key) {
    scope.querySelectorAll("[data-kind]").forEach(function (el) {
      var ok = key === "all" || el.getAttribute("data-kind") === key;
      el.classList.toggle("hide", !ok);
    });
    var n = scope.querySelectorAll("[data-kind]:not(.hide)").length;
    var out = document.querySelector("[data-count-out]");
    if (out) out.textContent = n;
  }

  /* ---------------- Bagan ---------------- */
  /* Definisi gradien yang dirujuk CSS (.ring .val, .spark path.area). */
  function svgDefs() {
    return '<svg width="0" height="0" style="position:absolute" aria-hidden="true"><defs>' +
      '<linearGradient id="ringGrad" x1="0" y1="0" x2="1" y2="1">' +
        '<stop offset="0%" stop-color="var(--primary)"/><stop offset="100%" stop-color="var(--primary)"/>' +
      "</linearGradient>" +
      '<linearGradient id="sparkGrad" x1="0" y1="0" x2="0" y2="1">' +
        '<stop offset="0%" stop-color="var(--primary)" stop-opacity=".55"/>' +
        '<stop offset="100%" stop-color="var(--primary)" stop-opacity="0"/>' +
      "</linearGradient></defs></svg>";
  }

  function ring(value, label, sub, size) {
    var s = size || 104, r = (s / 2) - 8, c = 2 * Math.PI * r;
    var off = c * (1 - Math.max(0, Math.min(1, value || 0)));
    return '<div class="ring" style="width:' + s + "px;height:" + s + 'px">' +
      '<svg width="' + s + '" height="' + s + '" viewBox="0 0 ' + s + " " + s + '">' +
        '<circle class="track" cx="' + s / 2 + '" cy="' + s / 2 + '" r="' + r + '" fill="none" stroke-width="8"/>' +
        '<circle class="val" cx="' + s / 2 + '" cy="' + s / 2 + '" r="' + r + '" fill="none" stroke-width="8" ' +
          'stroke-dasharray="' + c.toFixed(1) + '" stroke-dashoffset="' + off.toFixed(1) + '"/>' +
      "</svg>" +
      '<div class="lbl"><b>' + esc(label || pct(value)) + "</b>" + (sub ? "<span>" + esc(sub) + "</span>" : "") + "</div>" +
    "</div>";
  }

  function bars(rows, opt) {
    var o = opt || {};
    var max = o.max || Math.max.apply(null, rows.map(function (r) { return r.v; })) || 1;
    return '<div class="bars">' + rows.map(function (r) {
      var h = Math.max(3, Math.round((r.v / max) * 100));
      var cls = r.v / max < 0.6 ? " low" : r.v / max < 0.8 ? " mid" : "";
      return '<div class="b' + cls + '">' +
        (o.tip === false ? "" : '<span class="tip">' + esc(o.fmt ? o.fmt(r.v) : r.v) + "</span>") +
        '<i style="height:' + h + '%"></i><span>' + esc(r.k) + "</span></div>";
    }).join("") + "</div>";
  }

  function spark(values, opt) {
    var o = opt || {}, w = 300, h = 38, pad = 3;
    var max = Math.max.apply(null, values), min = Math.min.apply(null, values);
    var span = (max - min) || 1;
    var pts = values.map(function (v, i) {
      var x = pad + (i / (values.length - 1)) * (w - pad * 2);
      var y = h - pad - ((v - min) / span) * (h - pad * 2);
      return [x, y];
    });
    var line = pts.map(function (p, i) { return (i ? "L" : "M") + p[0].toFixed(1) + " " + p[1].toFixed(1); }).join(" ");
    var area = line + "L" + (w - pad) + " " + h + "L" + pad + " " + h + "Z";
    return '<svg class="spark" viewBox="0 0 ' + w + " " + h + '" preserveAspectRatio="none" role="img" aria-label="tren">' +
      '<path class="area" d="' + area + '"/><path class="line" d="' + line + '"/>' +
      (o.dot === false ? "" : '<circle cx="' + pts[pts.length - 1][0].toFixed(1) + '" cy="' + pts[pts.length - 1][1].toFixed(1) + '" r="3"/>') +
    "</svg>";
  }

  function donut(value, label, sub, size) {
    var s = size || 132;
    return '<div class="donut" style="width:' + s + "px;height:" + s + 'px">' +
      ring(value, label, sub, s) + "</div>";
  }

  function heat(values, opt) {
    /* values: array {date, ratio} — kolom = minggu, baris = hari (Sen..Min) */
    var o = opt || {};
    return '<div class="heat" role="img" aria-label="heatmap kepatuhan">' +
      values.map(function (d) {
        var r = d.ratio;
        var cls = d.none ? "none" : r == null ? "none" : r >= 0.98 ? "l4" : r >= 0.85 ? "l3" : r >= 0.6 ? "l2" : r > 0.2 ? "l1" : "bad";
        return '<i class="' + cls + '" title="' + esc(d.date) + " — " + esc(pct(r)) + '"></i>';
      }).join("") + "</div>";
  }

  function meter(value, opt) {
    var o = opt || {};
    var cls = o.thin ? " thin" : "";
    var v = Math.max(0, Math.min(100, Math.round((value || 0) * 100)));
    var tone = o.tone || (v >= 80 ? "ok" : v >= 50 ? "warn" : "err");
    return '<div class="meter' + (o.tone === "none" ? "" : " " + tone) + cls + '"><i style="width:' + v + '%"></i></div>';
  }

  function badge(text, kind, dot) {
    return '<span class="badge ' + (kind || "") + (dot ? " badge-pulse" : "") + '">' +
      (dot ? '<span class="dot"></span>' : "") + esc(text) + "</span>";
  }

  function stat(o) {
    return '<div class="stat"><div class="k">' + (o.ic ? icon(o.ic) : "") + esc(o.k) + "</div>" +
      '<div class="v ' + (o.tone || "") + '">' + esc(o.v) + "</div>" +
      '<div class="s">' + (o.s || "") + "</div>" +
      (o.viz ? '<div class="viz">' + o.viz + "</div>" : "") + "</div>";
  }

  function empty(title, body, ic) {
    return '<div class="empty">' + icon(ic || "circle") + "<b>" + esc(title) + "</b>" +
      (body ? "<span>" + esc(body) + "</span>" : "") + "</div>";
  }

  /* Placeholder memuat: baris tabel & blok bergaris (gaya shadcn Skeleton).
     Dipakai saat data belum siap supaya tata letak tidak melompat. */
  function skel(w, h) {
    return '<span class="skel" style="display:block;width:' + (w || "100%") + ";height:" + (h || "0.75rem") + '"></span>';
  }

  function skelRows(n, cols, opt) {
    var o = opt || {};
    var out = "";
    for (var r = 0; r < n; r++) {
      out += "<tr>";
      for (var c = 0; c < cols; c++) {
        out += '<td class="' + (o.tight ? "tight" : "") + '">' +
          skel(c === 0 ? "70%" : c === cols - 1 ? "40%" : "85%") + "</td>";
      }
      out += "</tr>";
    }
    return out;
  }

  function skelLines(n) {
    var out = "";
    for (var i = 0; i < n; i++) out += '<div class="mt-8">' + skel(i % 3 === 2 ? "60%" : "100%") + "</div>";
    return out;
  }

  /* Status dosis -> badge + kelas kartu */
  function doseBadge(status) {
    var map = {
      taken: ["badge-ok", "check", "Diminum"],
      late: ["badge-late", "clock", "Terlambat"],
      missed: ["badge-miss", "alert", "Terlewat"],
      skipped: ["badge-na", "ban", "Dilewati"],
      snoozed: ["badge-info", "hourglass", "Ditunda"],
      pending: ["badge-info", "clock", "Belum"],
      upcoming: ["badge-na", "clock", "Akan datang"]
    };
    var m = map[status] || map.upcoming;
    return badge(m[2], m[0]);
  }

  function doseClass(status) { return "st-" + (status === "upcoming" ? "pending" : status); }

  /* ---------------- Sinkronisasi UI umum ---------------- */
  function boot(opt) {
    var o = opt || {};
    theme.init();
    document.body.insertAdjacentHTML("beforeend", svgDefs());
    if (o.shell === "user") userShell(o);
    else if (o.shell === "admin") adminShell(o);

    document.querySelectorAll("[data-theme-toggle]").forEach(function (b) {
      b.addEventListener("click", theme.toggle);
    });
    document.querySelectorAll("[data-logout]").forEach(function (b) {
      b.addEventListener("click", function () {
        toast("info", "Keluar (dummy)", "Sesi dummy dihapus.");
        setTimeout(function () { logout(o.loginTo); }, 500);
      });
    });
    document.querySelectorAll("[data-pct]").forEach(function (el) {
      el.textContent = pct(parseFloat(el.getAttribute("data-pct")));
    });
    if (o.after) o.after();
    initOverlays();
    initTabs();
    initSeg();
    bindDead();
    /* i18n: terjemahkan elemen beratribut data-i18n setelah render selesai.
       Pilihan bahasa & tema TIDAK di topbar — keduanya di Pengaturan. */
    if (window.POI18N) POI18N.apply();
  }

  window.POAPP = {
    icon: icon, esc: esc, text: text, pct: pct, money: money, qs: qs, debounce: debounce,
    clockAt: clockAt,
    toast: toast, dead: dead, bindDead: bindDead,
    theme: theme, themeToggle: themeToggle,
    requireUser: requireUser, requireAdmin: requireAdmin, logout: logout, expired: expired,
    boot: boot, userShell: userShell, adminShell: adminShell,
    overlay: overlay, initOverlays: initOverlays, initTabs: initTabs, initSeg: initSeg, filterRows: filterRows,
    svgDefs: svgDefs, ring: ring, donut: donut, bars: bars, spark: spark, heat: heat, meter: meter,
    badge: badge, stat: stat, empty: empty, doseBadge: doseBadge, doseClass: doseClass,
    notifIcon: notifIcon, notifChannelIcon: notifChannelIcon,
    skel: skel, skelRows: skelRows, skelLines: skelLines,
    USER_NAV: USER_NAV, ADMIN_NAV: ADMIN_NAV
  };
})();