package domain

// File ini memuat aturan tentang jenis transaksi stok.
//
// Dipisah dari enums.go karena yang ada di sini bukan sekadar daftar nilai,
// melainkan ATURAN tentang nilai mana yang boleh dipakai di konteks apa.
// Aturan itu ikut diuji, dan testnya ada di paket ini.

// AdjustableStockKinds adalah jenis transaksi yang BOLEH dipakai untuk
// koreksi manual.
//
// `consume` dan `initial` sengaja TIDAK ada di sini:
//
//   - `consume` dipakai jalur pencatatan dosis, dan selalu menaut log_id.
//     Memakainya untuk koreksi manual akan membuat saldo tampak berkurang
//     karena dosis, padahal karena koreksi — dan saat ditelusuri nanti,
//     riwayatnya menyesatkan.
//
//   - `initial` hanya boleh lahir sekali per baris stok, saat stok dibuat.
//     Membiarkannya dipakai sebagai koreksi akan membuat "stok awal" bisa
//     disuntik berkali-kali, dan itu menghapus makna kolom initial_qty.
//
// `restock` diizinkan karena koreksi sering kali memang berupa penambahan
// yang lupa dicatat sebelumnya.
var AdjustableStockKinds = []StockTxnKind{
	StockRestock,
	StockAdjust,
	StockExpire,
}

// IsAdjustableStockKind melaporkan apakah jenis transaksi boleh dipakai
// untuk koreksi manual lewat endpoint adjust.
func IsAdjustableStockKind(k StockTxnKind) bool {
	for _, allowed := range AdjustableStockKinds {
		if k == allowed {
			return true
		}
	}
	return false
}

// StockKindLabel memberi label bahasa Inggris untuk ditampilkan.
//
// Label ada di sini, bukan di frontend, supaya nilai yang ditampilkan selalu
// cocok dengan nilai yang tersimpan. Frontend boleh menerjemahkannya lagi
// memakai kunci yang sama.
func StockKindLabel(k StockTxnKind) string {
	switch k {
	case StockInitial:
		return "initial stock"
	case StockRestock:
		return "restock"
	case StockConsume:
		return "consumed by dose"
	case StockAdjust:
		return "manual adjustment"
	case StockExpire:
		return "expired stock"
	}
	return string(k)
}
