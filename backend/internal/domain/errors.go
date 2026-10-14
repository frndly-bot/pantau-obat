// Package domain berisi tipe murni dan aturan bisnis.
//
// ATURAN IMPORT: paket ini TIDAK BOLEH mengimpor paket internal lain,
// dan tidak boleh mengimpor database/sql, net/http, atau driver apa pun.
// Semua tipe di sini harus bisa diuji tanpa database dan tanpa server.
package domain

import "errors"

// Sentinel error. Service mengembalikan salah satu dari ini; transport
// menerjemahkannya menjadi kode error API. Pemisahan ini yang membuat
// lapisan HTTP tidak perlu tahu tentang SQL, dan sebaliknya.
var (
	// ErrNotFound dipakai untuk objek yang tidak ada ATAU sudah soft-delete.
	// Sengaja tidak dibedakan, supaya tidak membocorkan keberadaan data.
	ErrNotFound = errors.New("objek tidak ditemukan")

	// ErrForbidden = login valid tapi tidak berhak atas objek ini.
	ErrForbidden = errors.New("tidak berhak mengakses objek ini")

	// ErrUnauthenticated = token hilang, kedaluwarsa, atau tidak valid.
	ErrUnauthenticated = errors.New("sesi tidak valid")

	// ErrInvalidCredentials = email tidak terdaftar ATAU password salah.
	// Digabung sengaja: membedakannya memudahkan enumerasi akun.
	ErrInvalidCredentials = errors.New("email atau password salah")

	// ErrTokenRevoked = refresh token sudah dipakai atau dicabut.
	ErrTokenRevoked = errors.New("sesi sudah dicabut")

	// ErrAdminScopeDenied = admin melampaui admin_scope miliknya.
	ErrAdminScopeDenied = errors.New("peran admin tidak mencakup bidang ini")

	// ErrCareNotAccepted = care_link belum accepted atau sudah dicabut.
	ErrCareNotAccepted = errors.New("relasi caregiver belum aktif")

	// ErrValidation = input tidak lolos validasi. Detail menyusul di layer atas.
	ErrValidation = errors.New("input tidak valid")

	// ErrDuplicateMedication = obat sama untuk user yang sama.
	ErrDuplicateMedication = errors.New("obat dengan nama dan dosis itu sudah ada")

	// ErrLogOccurrenceTaken = dosis jam itu sudah punya log di tanggal itu.
	ErrLogOccurrenceTaken = errors.New("dosis pada jam itu sudah tercatat")

	// ErrDuplicateTimes = jam dan pola hari itu sudah terdaftar.
	ErrDuplicateTimes = errors.New("jam untuk pola hari itu sudah ada")

	// ErrInvitePending = undangan untuk email sama masih menganggur.
	ErrInvitePending = errors.New("undangan untuk email itu masih menunggu")

	// ErrStockInsufficient = pengurangan stok melebihi saldo.
	ErrStockInsufficient = errors.New("stok tidak cukup")

	// ErrCatalogImmutable = mencoba mengubah preset publik.
	ErrCatalogImmutable = errors.New("preset katalog hanya bisa diubah admin")

	// ErrCatalogDuplicateName = nama preset publik sudah dipakai.
	//
	// Dipisah dari ErrDuplicateMedication karena pelakunya berbeda: yang
	// menghadapinya admin yang menambah preset, bukan pasien yang menambah
	// obat. Tanpa sentinel ini, bentrok nama preset jatuh ke cabang default
	// dan klien menerima 500 untuk kesalahan yang bisa dijelaskan.
	ErrCatalogDuplicateName = errors.New("nama katalog sudah dipakai")

	// ErrRateLimited = kuota permintaan terlampaui.
	ErrRateLimited = errors.New("terlalu banyak permintaan")

	// ErrSnoozeLimit = batas penundaan beruntun untuk satu dosis tercapai.
	//
	// Dipisah dari ErrValidation karena akibatnya berbeda bagi pengguna:
	// ini bukan "input salah", melainkan aturan yang sengaja menahan
	// penundaan tanpa akhir. Klien perlu membedakannya supaya bisa
	// menjelaskan bahwa dosisnya kini dianggap terlewat, bukan menyuruh
	// pengguna memperbaiki isian.
	ErrSnoozeLimit = errors.New("batas penundaan untuk dosis ini sudah tercapai")
)
