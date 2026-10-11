package service

import (
	"github.com/jackc/pgx/v5"

	"github.com/frndly-bot/pantau-obat/backend/internal/repo/postgres"
)

// postgresQuerier adalah irisan yang dipenuhi pgxpool.Pool maupun pgx.Tx.
//
// Tujuannya: satu fungsi repo bisa dipakai baik di dalam transaksi maupun di
// luar, tanpa menulis dua versi. Ini yang membuat jalur tulis multi-tabel
// (registrasi + sesi + audit) tetap atomik.
//
// Didefinisikan sebagai alias, bukan tipe baru, supaya nilainya bisa
// diteruskan langsung ke fungsi repo yang menerima postgres.Querier.
type postgresQuerier = postgres.Querier

// pgx.Tx memenuhi postgres.Querier. Kalau kontrak ini berubah, build gagal
// di sini, bukan saat request pertama datang.
var _ postgres.Querier = (pgx.Tx)(nil)
