package handlers

import (
	"errors"
	"io"
	"net/http"
)

// maxJSONBody adalah batas ukuran body JSON.
//
// 1 MiB cukup untuk semua endpoint di API ini. Batas ini mencegah satu
// klien mengirim body raksasa dan menghabiskan memori server.
const maxJSONBody = 1 << 20

// ErrBodyTooLarge menandai body yang melebihi batas.
var ErrBodyTooLarge = errors.New("body terlalu besar")

// readLimitedBody membaca seluruh body dengan batas ukuran.
func readLimitedBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return nil, ErrBodyTooLarge
		}
		return nil, err
	}
	if len(raw) == 0 {
		// Body kosong dianggap objek kosong, bukan error. Ini yang membuat
		// logout tanpa body tetap bekerja.
		return []byte("{}"), nil
	}
	return raw, nil
}
