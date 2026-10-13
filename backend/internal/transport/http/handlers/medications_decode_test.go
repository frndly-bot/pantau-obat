package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDecoderObat_MenerimaIsActive mengunci perbaikan bug yang membuat
// "Hentikan sementara" SELALU gagal dari aplikasi.
//
// Latar belakang: seluruh handler obat memakai `decodeAuthBody` — dekoder
// milik AKUN, yang membawa daftar field terlarang berisi `is_active`. Di
// konteks akun, penolakan itu benar: status aktif akun hanya boleh diubah
// operator lewat database.
//
// Tetapi pada OBAT, `is_active` adalah field yang sah — itulah cara pengguna
// menghentikan obatnya sendiri. Akibat pemakaian decoder yang salah, server
// membalas "status aktif akun hanya bisa diubah lewat endpoint admin" untuk
// permintaan yang menyentuh obat, dan pesan itu menyesatkan siapa pun yang
// membacanya.
//
// Uji ini memanggil decoder dengan aturan yang benar-benar dipakai handler
// obat (nil), sehingga bug yang sama tidak bisa kembali tanpa ketahuan.
func TestDecoderObat_MenerimaIsActive(t *testing.T) {
	body := `{"is_active":false}`
	r := httptest.NewRequest(http.MethodPatch, "/api/v1/medications/x", strings.NewReader(body))
	w := httptest.NewRecorder()

	var dst struct {
		IsActive *bool `json:"is_active"`
	}

	// Aturan nil = netral, sama seperti yang dipakai handler obat sekarang.
	if err := decodeBodyWithRules(w, r, &dst, nil); err != nil {
		t.Fatalf("decoder obat menolak is_active: %v", err)
	}
	if dst.IsActive == nil || *dst.IsActive {
		t.Fatalf("is_active tidak terbaca dengan benar: %+v", dst.IsActive)
	}
}

// TestDecoderAkun_MenolakIsActive memastikan perbaikan di atas TIDAK
// melemahkan perlindungan pada data akun.
//
// Dua hal ini harus benar bersamaan: obat menerima `is_active`, akun
// menolaknya. Kalau seseorang menghapus aturan akun untuk "menyelesaikan"
// bug obat, uji ini yang akan menangkapnya.
func TestDecoderAkun_MenolakIsActive(t *testing.T) {
	body := `{"is_active":false}`
	r := httptest.NewRequest(http.MethodPatch, "/api/v1/users/me", strings.NewReader(body))
	w := httptest.NewRecorder()

	var dst struct {
		IsActive *bool `json:"is_active"`
	}

	err := decodeAuthBody(w, r, &dst)
	if err == nil {
		t.Fatal("decoder akun MENERIMA is_active — perlindungan data akun bocor")
	}

	var forbidden *forbiddenFieldError
	if !asForbidden(err, &forbidden) {
		t.Fatalf("error bukan forbiddenFieldError: %T", err)
	}
	if forbidden.Field != "is_active" {
		t.Fatalf("field yang ditolak salah: %q", forbidden.Field)
	}
}

// asForbidden adalah pembungkus kecil supaya uji tidak bergantung pada
// paket `errors` secara langsung — dan supaya niatnya terbaca.
func asForbidden(err error, target **forbiddenFieldError) bool {
	f, ok := err.(*forbiddenFieldError)
	if ok {
		*target = f
	}
	return ok
}

// TestDecoderObat_MengabaikanFieldAsing memastikan klien versi lebih baru
// tetap bisa mengirim field tambahan tanpa ditolak.
//
// Ini disengaja: menolak field asing akan memaksa klien dan server
// dideploy bersamaan, dan itu tidak selalu mungkin.
func TestDecoderObat_MengabaikanFieldAsing(t *testing.T) {
	body := `{"name":"Paracetamol","field_dari_versi_masa_depan":123}`
	r := httptest.NewRequest(http.MethodPatch, "/api/v1/medications/x", strings.NewReader(body))
	w := httptest.NewRecorder()

	var dst struct {
		Name *string `json:"name"`
	}
	if err := decodeBodyWithRules(w, r, &dst, nil); err != nil {
		t.Fatalf("field asing seharusnya diabaikan, bukan ditolak: %v", err)
	}
	if dst.Name == nil || *dst.Name != "Paracetamol" {
		t.Fatalf("name tidak terbaca: %+v", dst.Name)
	}
}
