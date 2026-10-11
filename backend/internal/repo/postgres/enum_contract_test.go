package postgres_test

import (
	"context"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/frndly-bot/pantau-obat/backend/internal/config"
	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	"github.com/frndly-bot/pantau-obat/backend/internal/repo/postgres"
)

// TEST KONTRAK ENUM.
//
// MASALAH YANG DISELESAIKAN: nilai enum di Go dan CHECK constraint di
// database disinkronkan MANUAL. Tidak ada yang memaksa keduanya cocok, dan
// ketidakcocokan tidak ketahuan sampai ada request yang gagal di runtime.
//
// Itu sudah terjadi TIGA kali selama pembangunan backend ini:
//
//   - `reject` dipakai service care-links, tapi tidak ada di
//     audit_log.action -> setiap penolakan undangan gagal.
//   - `log_snoozed` tidak ada, padahal snooze punya alurnya sendiri.
//   - Beberapa aksi katalog (create/update/unpublish/archive) dipakai admin
//     tapi belum ada di CHECK.
//
// Semuanya ketemu lewat kegagalan saat menjalankan, bukan lewat test. Test
// INI yang seharusnya menangkapnya lebih dulu.
//
// Cara kerjanya: membaca definisi CHECK langsung dari pg_constraint, lalu
// membandingkannya dengan konstanta di paket domain. Kalau ada yang beda,
// test GAGAL dan menyebutkan nilai mana yang hanya ada di satu sisi.
//
// Test ini butuh database. Kalau DB_URL tidak diisi, test dilewati dengan
// pesan yang jelas — bukan gagal, supaya `go test ./...` tetap berguna di
// mesin tanpa database.

// enumCase memetakan satu CHECK constraint ke nilai-nilai Go yang harus
// cocok dengannya.
type enumCase struct {
	// nama constraint di database, untuk pesan kesalahan.
	constraint string

	// values adalah nilai yang dikenal Go.
	values []string
}

func TestEnumContract(t *testing.T) {
	db := requireTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cases := []enumCase{
		// --- audit_log ---
		{
			constraint: "audit_log_actor_role_check",
			values: []string{
				string(domain.ActorUser), string(domain.ActorAdmin), string(domain.ActorSystem),
			},
		},
		{
			constraint: "ck_audit_action",
			values:     auditActionValues(),
		},
		{
			constraint: "audit_log_resource_type_check",
			values: []string{
				postgres.ResourceUser, postgres.ResourceAuth, postgres.ResourceMed,
				postgres.ResourceSched, postgres.ResourceLog, postgres.ResourceStock,
				postgres.ResourceCare, postgres.ResourceCat, postgres.ResourceNotif,
				postgres.ResourceAdmin, postgres.ResourceRetain,
			},
		},

		// --- bump_queue ---
		{
			constraint: "bump_queue_kind_check",
			values: []string{
				string(domain.BumpDose), string(domain.BumpRefill), string(domain.BumpDigest),
			},
		},

		// --- care_links ---
		{
			constraint: "care_links_relation_check",
			values: []string{
				string(domain.RelationFamily), string(domain.RelationChild),
				string(domain.RelationParent), string(domain.RelationSpouse),
				string(domain.RelationFriend), string(domain.RelationNurse),
				string(domain.RelationOther),
			},
		},
		{
			constraint: "care_links_role_check",
			values:     []string{string(domain.CareViewer), string(domain.CareHelper)},
		},
		{
			constraint: "care_links_status_check",
			values: []string{
				string(domain.CarePending), string(domain.CareAccepted),
				string(domain.CareRejected), string(domain.CareRevoked),
				string(domain.CareExpired),
			},
		},

		// --- device_tokens ---
		{
			constraint: "device_tokens_platform_check",
			values: []string{
				string(domain.PlatformAndroid), string(domain.PlatformIOS), string(domain.PlatformWeb),
			},
		},

		// --- drug_catalog ---
		{
			constraint: "drug_catalog_curation_status_check",
			values: []string{
				string(domain.CurationDraft), string(domain.CurationPublished),
				string(domain.CurationArchived), string(domain.CurationUserPrivate),
			},
		},
		{
			constraint: "drug_catalog_source_check",
			values: []string{
				string(domain.CatalogFromKFA), string(domain.CatalogFromBPOM),
				string(domain.CatalogFromManual), string(domain.CatalogFromOther),
			},
		},

		// --- medication_dose_events ---
		{
			constraint: "medication_dose_events_kind_check",
			values: []string{
				string(domain.EventMissed), string(domain.EventDoseShift), string(domain.EventPaused),
			},
		},

		// --- medication_frequency ---
		{
			constraint: "medication_frequency_pattern_check",
			values: []string{
				string(domain.PatternDaily), string(domain.PatternInterval),
				string(domain.PatternWeekly), string(domain.PatternMonthlyCycle),
				string(domain.PatternPRN),
			},
		},
		{
			constraint: "medication_frequency_cycle_anchor_check",
			values: []string{
				string(domain.AnchorDom), string(domain.AnchorEndOfMonth), string(domain.AnchorFromStart),
			},
		},

		// --- medication_logs ---
		{
			constraint: "medication_logs_status_check",
			values: []string{
				string(domain.StatusTaken), string(domain.StatusLate),
				string(domain.StatusSkipped), string(domain.StatusSnoozed),
			},
		},
		{
			constraint: "medication_logs_source_check",
			values: []string{
				string(domain.SourceSelf), string(domain.SourceCaregiver), string(domain.SourceSystem),
			},
		},

		// --- medications & templates ---
		{
			constraint: "medications_meal_relation_check",
			values: []string{
				string(domain.MealBefore), string(domain.MealAfter),
				string(domain.MealWith), string(domain.MealNotRelated),
			},
		},
		{
			constraint: "medication_templates_meal_relation_check",
			values: []string{
				string(domain.MealBefore), string(domain.MealAfter),
				string(domain.MealWith), string(domain.MealNotRelated),
			},
		},

		// --- medication_times ---
		{
			constraint: "medication_times_label_check",
			values: []string{
				string(domain.LabelMorning), string(domain.LabelAfternoon),
				string(domain.LabelEvening), string(domain.LabelDawn), string(domain.LabelOther),
			},
		},

		// --- notification_jobs & deliveries ---
		{
			constraint: "notification_jobs_kind_check",
			values: []string{
				string(domain.JobDoseReminder), string(domain.JobMissedEscalation),
				string(domain.JobCareDigest), string(domain.JobRetention),
				string(domain.JobRefillReminder),
			},
		},
		{
			constraint: "notification_jobs_status_check",
			values: []string{
				string(domain.JobPending), string(domain.JobClaimed), string(domain.JobDone),
				string(domain.JobFailed), string(domain.JobCancelled),
			},
		},
		{
			constraint: "notification_deliveries_status_check",
			values: []string{
				string(domain.DeliveryPending), string(domain.DeliverySent),
				string(domain.DeliveryFailed), string(domain.DeliverySkipped),
			},
		},

		// --- notification_prefs ---
		{
			constraint: "notification_prefs_channel_check",
			values: []string{
				string(domain.ChannelPush), string(domain.ChannelEmail),
				string(domain.ChannelTelegram), string(domain.ChannelInApp),
			},
		},

		// --- stock_transactions ---
		{
			constraint: "stock_transactions_kind_check",
			values: []string{
				string(domain.StockInitial), string(domain.StockRestock),
				string(domain.StockConsume), string(domain.StockAdjust),
				string(domain.StockExpire),
			},
		},

		// --- user_token_revocations ---
		{
			constraint: "user_token_revocations_reason_check",
			values: []string{
				domain.RevokeReasonPasswordChange, domain.RevokeReasonLogoutAll,
				domain.RevokeReasonTokenReuse, domain.RevokeReasonAdminDisable,
				domain.RevokeReasonAdminErase, domain.RevokeReasonAdminRevokeRole,
				domain.RevokeReasonUserRequest,
			},
		},
	}

	for _, c := range cases {
		t.Run(c.constraint, func(t *testing.T) {
			dbValues, err := readConstraintValues(ctx, db, c.constraint)
			if err != nil {
				t.Fatalf("tidak bisa membaca constraint %s: %v", c.constraint, err)
			}
			if len(dbValues) == 0 {
				t.Fatalf("constraint %s tidak ditemukan atau tanpa nilai", c.constraint)
			}

			goValues := sortedUnique(c.values)

			onlyDB := difference(dbValues, goValues)
			onlyGo := difference(goValues, dbValues)

			if len(onlyDB) > 0 {
				t.Errorf("nilai ada di DATABASE tapi TIDAK ada di Go: %v\n"+
					"  -> menambah nilai baru di migrasi wajib diikuti konstanta di domain/enums.go,\n"+
					"     kalau tidak: kode akan gagal saat menulis nilai itu", onlyDB)
			}
			if len(onlyGo) > 0 {
				t.Errorf("nilai ada di GO tapi TIDAK ada di database: %v\n"+
					"  -> konstanta ini akan GAGAL saat dipakai, karena CHECK menolaknya", onlyGo)
			}
		})
	}
}

// TestAuditActionCoverage memastikan setiap konstanta aksi audit benar-benar
// ada di CHECK, satu per satu.
//
// Test enum di atas sudah mencakupnya, tapi yang ini memberi pesan yang lebih
// langsung: "aksi ini dipakai kode, tapi database menolaknya".
func TestAuditActionCoverage(t *testing.T) {
	db := requireTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dbValues, err := readConstraintValues(ctx, db, "ck_audit_action")
	if err != nil {
		t.Fatalf("tidak bisa membaca ck_audit_action: %v", err)
	}
	if len(dbValues) == 0 {
		t.Fatal("ck_audit_action tidak ditemukan")
	}

	inDB := make(map[string]bool, len(dbValues))
	for _, v := range dbValues {
		inDB[v] = true
	}

	for _, v := range auditActionValues() {
		if !inDB[v] {
			t.Errorf("aksi audit %q dipakai kode tapi TIDAK ada di CHECK constraint.\n"+
				"  Tambahkan lewat migrasi baru, bukan dengan mengedit file yang sudah naik.", v)
		}
	}
}

// requireTestDB membuka koneksi uji, atau melewati test bila tidak ada.
//
// MELEWATI, bukan gagal. Test kontrak ini berguna, tapi `go test ./...` harus
// tetap bisa dijalankan di mesin tanpa database. Yang tidak boleh adalah
// test diam-diam hijau padahal tidak memeriksa apa pun — karena itu pesan
// lewatnya dibuat jelas.
func requireTestDB(t *testing.T) *postgres.DB {
	t.Helper()

	dsn := os.Getenv("TEST_DB_URL")
	if dsn == "" {
		dsn = os.Getenv("DB_URL")
	}
	if dsn == "" {
		t.Skip("TEST_DB_URL / DB_URL tidak diisi; test kontrak enum dilewati " +
			"(jalankan dengan DB_URL terisi untuk memeriksa sinkronisasi)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := postgres.NewDB(ctx, config.DBConfig{URL: dsn, MaxConns: 2, MinConns: 1})
	if err != nil {
		t.Skipf("tidak bisa terhubung ke database uji: %v", err)
	}

	// Pastikan skemanya sudah ada. Kalau belum, melewati lebih baik daripada
	// melaporkan puluhan kegagalan yang sebenarnya hanya "database kosong".
	var n int
	if err := db.Pool().QueryRow(ctx,
		`SELECT count(*) FROM information_schema.tables
          WHERE table_schema = 'public' AND table_name = 'audit_log'`).Scan(&n); err != nil || n == 0 {
		db.Close()
		t.Skip("skema belum dimigrasi di database uji; test kontrak enum dilewati")
	}

	return db
}

// readConstraintValues membaca daftar nilai dari satu CHECK constraint.
//
// Cara kerjanya: ambil definisi constraint sebagai teks, lalu ekstrak semua
// literal string di dalamnya. Pendekatan ini dipilih karena definisi CHECK
// bisa berbentuk apa saja (IN (...), ANY(ARRAY[...]), dan sebagainya), dan
// menulis parser khusus untuk setiap bentuk akan rapuh.
//
// Yang perlu disadari: kalau ada CHECK dengan literal string yang BUKAN nilai
// enum (mis. nama kolom dalam pesan), ia ikut terbaca. Untuk constraint di
// proyek ini, bentuknya selalu daftar nilai, jadi aman.
func readConstraintValues(ctx context.Context, db *postgres.DB, constraintName string) ([]string, error) {
	rows, err := db.Pool().Query(ctx, `
        SELECT m.val
          FROM pg_constraint c,
               LATERAL regexp_matches(
                   pg_get_constraintdef(c.oid), '''([^'']+)''', 'g'
               ) AS m(val)
         WHERE c.conname = $1
         ORDER BY 1`, constraintName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		// regexp_matches mengembalikan text[], dan tipe itu tidak bisa
		// di-scan langsung ke string.
		var arr []string
		if err := rows.Scan(&arr); err != nil {
			return nil, err
		}
		if len(arr) > 0 {
			out = append(out, arr[0])
		}
	}
	return out, rows.Err()
}

// sortedUnique mengurutkan dan membuang duplikat.
func sortedUnique(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// difference mengembalikan elemen a yang tidak ada di b.
func difference(a, b []string) []string {
	inB := make(map[string]bool, len(b))
	for _, v := range b {
		inB[v] = true
	}
	var out []string
	for _, v := range a {
		if !inB[v] {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

// auditActionValues mengumpulkan seluruh konstanta aksi audit.
//
// Ditulis manual, bukan lewat refleksi, supaya penambahan konstanta baru
// TERLIHAT di sini saat review. Refleksi akan membuatnya otomatis, tapi juga
// membuat konstanta yang lupa didaftarkan tidak pernah ketahuan.
func auditActionValues() []string {
	return []string{
		// aksi user
		postgres.ActionCreate, postgres.ActionRead, postgres.ActionUpdate,
		postgres.ActionDelete, postgres.ActionLogin, postgres.ActionLogout,
		postgres.ActionInvite, postgres.ActionAccept, postgres.ActionReject,
		postgres.ActionRevoke, postgres.ActionExport,
		postgres.ActionLogTaken, postgres.ActionLogSkipped, postgres.ActionLogSnoozed,
		postgres.ActionRestock, postgres.ActionCatalogSearch,
		// aksi admin
		postgres.ActionAdminGrant, postgres.ActionAdminRevoke,
		postgres.ActionAdminScopeChange,
		postgres.ActionUserDisable, postgres.ActionUserEnable, postgres.ActionUserErase,
		postgres.ActionCatalogCreate, postgres.ActionCatalogUpdate,
		postgres.ActionCatalogPublish, postgres.ActionCatalogUnpublish,
		postgres.ActionCatalogArchive, postgres.ActionCatalogImport,
		postgres.ActionCatalogMerge,
		postgres.ActionNotifRetry, postgres.ActionNotifCancel,
		postgres.ActionRetentionPurge,
	}
}

// TestEnumValuesNonEmpty memastikan tidak ada konstanta enum yang kosong.
//
// Konstanta kosong () akan lolos dari perbandingan himpunan karena difilter
// sortedUnique, tapi ia tetap bug: nilai kosong ditolak CHECK, dan pesan
// errornya membingungkan.
func TestEnumValuesNonEmpty(t *testing.T) {
	checks := map[string][]string{
		"ActorRole": {string(domain.ActorUser), string(domain.ActorAdmin), string(domain.ActorSystem)},
		"MealRelation": {
			string(domain.MealBefore), string(domain.MealAfter),
			string(domain.MealWith), string(domain.MealNotRelated),
		},
		"LogStatus": {
			string(domain.StatusTaken), string(domain.StatusLate),
			string(domain.StatusSkipped), string(domain.StatusSnoozed),
		},
		"Channel": {
			string(domain.ChannelPush), string(domain.ChannelEmail),
			string(domain.ChannelTelegram), string(domain.ChannelInApp),
		},
	}
	for name, vals := range checks {
		for i, v := range vals {
			if strings.TrimSpace(v) == "" {
				t.Errorf("%s[%d] kosong", name, i)
			}
		}
	}
}
