package postgres

import (
	"context"
	"fmt"
)

// RefItem adalah satu entri tabel referensi (kode + nama tampilan).
type RefItem struct {
	Code string
	Name string
}

// Refs adalah seluruh tabel referensi yang dipakai formulir obat.
//
// Dikumpulkan dalam satu struct supaya pengambilan tidak tersebar dan
// pemanggil hanya perlu satu perjalanan ke database.
type Refs struct {
	Units       []RefItem
	Routes      []RefItem
	DosageForms []RefItem
}

// AllRefs mengambil unit, rute, dan bentuk sediaan sekaligus.
//
// KENAPA INI PENTING, bukan sekadar kenyamanan: kolom-kolom ini punya foreign
// key ke tabel referensinya. Formulir yang menawarkan pilihan dari daftar yang
// ditulis di klien bisa mengirim kode yang tidak ada di database — dan
// pelanggaran foreign key sampai ke pengguna sebagai 500, padahal itu
// kesalahan pilihan, bukan kerusakan server. Dengan membaca daftarnya dari
// sumbernya, pilihan yang ditawarkan selalu sah.
func (db *DB) AllRefs(ctx context.Context, q Querier) (*Refs, error) {
	units, err := refTable(ctx, q, "ref_unit")
	if err != nil {
		return nil, err
	}
	routes, err := refTable(ctx, q, "ref_route")
	if err != nil {
		return nil, err
	}
	forms, err := refTable(ctx, q, "ref_dosage_form")
	if err != nil {
		return nil, err
	}
	return &Refs{Units: units, Routes: routes, DosageForms: forms}, nil
}

// refTable membaca satu tabel referensi.
//
// Nama tabel TIDAK berasal dari masukan pengguna — hanya dari tiga konstanta
// di AllRefs — sehingga penggabungan string di sini tidak membuka celah
// injeksi. Parameter query tidak bisa dipakai untuk nama tabel.
//
// Urutan memakai sort_order lebih dulu, baru code. Tabel referensi punya
// sort_order justru supaya tampilannya masuk akal bagi manusia — mis. Oral
// sebelum Sublingual. Mengurutkan berdasarkan code akan menampilkan "B"
// (Buccal) di paling atas, dan itu tidak membantu siapa pun.
func refTable(ctx context.Context, q Querier, table string) ([]RefItem, error) {
	rows, err := q.Query(ctx, `SELECT code, name FROM `+table+` ORDER BY sort_order, code`)
	if err != nil {
		return nil, fmt.Errorf("membaca tabel referensi %s: %w", table, err)
	}
	defer rows.Close()

	var out []RefItem
	for rows.Next() {
		var it RefItem
		if err := rows.Scan(&it.Code, &it.Name); err != nil {
			return nil, fmt.Errorf("membaca baris %s: %w", table, err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
