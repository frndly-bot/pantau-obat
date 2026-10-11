package http

// File ini menjelaskan bagaimana setiap kelompok rute didaftarkan.
//
// Router generik ada di router.go; file ini memuat catatan kelompok dan
// daftar rute yang menjadi tanggung jawab masing-masing handler.
//
// ATURAN: satu kelompok = satu file handler = satu method Routes.
// Menambah endpoint baru cukup menyentuh file kelompoknya. router.go dan
// file ini hanya berubah kalau ada kelompok BARU.
//
// Kondisi saat ini: hanya kelompok `auth` dan `health` yang punya handler.
// Kelompok lain sengaja belum didaftarkan supaya endpoint yang belum ada
// membalas 404, bukan 200 dengan data palsu. Itu membuat uji e2e jujur:
// kalau koleksi apidoc hijau, berarti backend benar-benar jalan.
//
// Daftar rute yang menjadi tanggung jawab tiap kelompok:
//
//	user         GET PATCH DELETE /users/me, GET /users/me/audit-log
//	medication   GET POST /medications, GET PATCH DELETE /medications/{id},
//	             GET /medications/{id}/adherence
//	             termasuk sub-jalur /medications/{id}/times
//	log          GET /medications/{id}/logs, GET /logs, POST /logs/from-reminder,
//	             PATCH /logs/{logId}, POST /logs/{logId}/snooze, GET /logs/today
//	stock        GET PUT /medications/{id}/stock, GET .../transactions,
//	             POST .../restock, POST .../adjust, GET /stock/low
//	catalog      GET POST /catalog, GET PATCH DELETE /catalog/{id},
//	             GET /catalog/suggest
//	care         POST GET /care-links, POST /care-links/accept, /reject,
//	             PATCH DELETE /care-links/{id},
//	             GET /care/{ownerId}/overview|medications|adherence|logs,
//	             POST /care/{ownerId}/logs
//	notification GET PATCH /notification-prefs, POST DELETE /device-tokens,
//	             GET /notifications
//	admin        /admin/users*, /admin/catalog*, /admin/notifications/jobs*,
//	             /admin/stats — masing-masing dengan RequireAdmin(scope)
//	webhook      POST /webhooks/email-status, POST /webhooks/telegram-update
