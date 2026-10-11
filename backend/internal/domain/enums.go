package domain

import "fmt"

// File ini adalah satu-satunya tempat nilai enum didefinisikan di sisi Go.
//
// ATURAN KERAS: nilai di sini harus SAMA PERSIS dengan CHECK constraint di
// backend/migrations/. Sinkronisasi dilakukan manual, jadi gampang drift.
// Yang mendeteksi drift = TestEnumContract (repo/postgres), yang membaca
// pg_constraint dari database hidup lalu membandingkannya dengan konstanta
// di file ini. Kalau kamu menambah nilai di migrasi, tambahkan juga di sini,
// atau test itu akan gagal. Itu memang tujuannya.

// MealRelation = medications.meal_relation, medication_templates.meal_relation.
type MealRelation string

const (
	MealBefore     MealRelation = "before_meal"
	MealAfter      MealRelation = "after_meal"
	MealWith       MealRelation = "with_meal"
	MealNotRelated MealRelation = "not_related"
)

// TimeLabel = medication_times.label.
type TimeLabel string

const (
	LabelMorning   TimeLabel = "morning"
	LabelAfternoon TimeLabel = "afternoon"
	LabelEvening   TimeLabel = "evening"
	LabelDawn      TimeLabel = "dawn"
	LabelOther     TimeLabel = "other"
)

// FrequencyPattern = medication_frequency.pattern.
type FrequencyPattern string

const (
	PatternDaily        FrequencyPattern = "daily"
	PatternInterval     FrequencyPattern = "interval"
	PatternWeekly       FrequencyPattern = "weekly"
	PatternMonthlyCycle FrequencyPattern = "monthly_cycle"
	PatternPRN          FrequencyPattern = "prn"
)

// CycleAnchor = medication_frequency.cycle_anchor.
//
//	end_of_month = selalu tanggal terakhir bulan berjalan, mengikuti
//	               tahun kabisat (Februari 28 atau 29).
type CycleAnchor string

const (
	AnchorDom        CycleAnchor = "dom"
	AnchorEndOfMonth CycleAnchor = "end_of_month"
	AnchorFromStart  CycleAnchor = "from_start"
)

// LogStatus = medication_logs.status.
type LogStatus string

const (
	StatusTaken   LogStatus = "taken"
	StatusLate    LogStatus = "late"
	StatusSkipped LogStatus = "skipped"
	StatusSnoozed LogStatus = "snoozed"
)

// LogSource = medication_logs.source.
type LogSource string

const (
	SourceSelf      LogSource = "self"
	SourceCaregiver LogSource = "caregiver"
	SourceSystem    LogSource = "system"
)

// DoseEventKind = medication_dose_events.kind.
type DoseEventKind string

const (
	EventMissed    DoseEventKind = "missed"
	EventDoseShift DoseEventKind = "dose_shift"
	EventPaused    DoseEventKind = "paused"
)

// StockTxnKind = stock_transactions.kind.
type StockTxnKind string

const (
	StockInitial StockTxnKind = "initial"
	StockRestock StockTxnKind = "restock"
	StockConsume StockTxnKind = "consume"
	StockAdjust  StockTxnKind = "adjust"
	StockExpire  StockTxnKind = "expire"
)

// BumpKind = bump_queue.kind. Menjawab "kapan sesuatu harus terjadi".
type BumpKind string

const (
	BumpDose   BumpKind = "dose"
	BumpRefill BumpKind = "refill"
	BumpDigest BumpKind = "digest"
)

// JobKind = notification_jobs.kind. Mencatat pengiriman yang sudah terjadi.
type JobKind string

const (
	JobDoseReminder     JobKind = "dose_reminder"
	JobMissedEscalation JobKind = "missed_escalation"
	JobCareDigest       JobKind = "care_digest"
	JobRetention        JobKind = "retention"
	JobRefillReminder   JobKind = "refill_reminder"
)

// JobStatus = notification_jobs.status.
type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobClaimed   JobStatus = "claimed"
	JobDone      JobStatus = "done"
	JobFailed    JobStatus = "failed"
	JobCancelled JobStatus = "cancelled"
)

// DeliveryStatus = notification_deliveries.status.
type DeliveryStatus string

const (
	DeliveryPending DeliveryStatus = "pending"
	DeliverySent    DeliveryStatus = "sent"
	DeliveryFailed  DeliveryStatus = "failed"
	DeliverySkipped DeliveryStatus = "skipped"
)

// Channel = notification_prefs.channel dan notification_deliveries.channel.
type Channel string

const (
	ChannelPush     Channel = "push"
	ChannelEmail    Channel = "email"
	ChannelTelegram Channel = "telegram"
	ChannelInApp    Channel = "in_app"
)

// DevicePlatform = device_tokens.platform.
type DevicePlatform string

const (
	PlatformAndroid DevicePlatform = "android"
	PlatformIOS     DevicePlatform = "ios"
	PlatformWeb     DevicePlatform = "web"
)

// CareRole = care_links.role. Tingkat izin caregiver.
type CareRole string

const (
	CareViewer CareRole = "viewer" // hanya GET
	CareHelper CareRole = "helper" // boleh catat log, tidak boleh kelola obat
)

// CareStatus = care_links.status.
type CareStatus string

const (
	CarePending  CareStatus = "pending"
	CareAccepted CareStatus = "accepted"
	CareRejected CareStatus = "rejected"
	CareRevoked  CareStatus = "revoked"
	CareExpired  CareStatus = "expired"
)

// CareRelation = care_links.relation.
type CareRelation string

const (
	RelationFamily CareRelation = "family"
	RelationChild  CareRelation = "child"
	RelationParent CareRelation = "parent"
	RelationSpouse CareRelation = "spouse"
	RelationFriend CareRelation = "friend"
	RelationNurse  CareRelation = "nurse"
	RelationOther  CareRelation = "other"
)

// UserRole = users.role.
//
// Peran ini HANYA ditetapkan lewat operasi DB/seed, tidak pernah lewat API.
// Tidak ada endpoint yang menerima `role` dari klien.
type UserRole string

const (
	RoleUser  UserRole = "user"
	RoleAdmin UserRole = "admin"
)

// AdminScope = elemen users.admin_scope. NULL (slice nil) berarti akses penuh.
type AdminScope string

const (
	ScopeUsers         AdminScope = "users"
	ScopeCatalog       AdminScope = "catalog"
	ScopeNotifications AdminScope = "notifications"
	ScopeReports       AdminScope = "reports"
)

// AllAdminScopes adalah daftar lengkap, dipakai untuk validasi dan tampilan.
var AllAdminScopes = []AdminScope{ScopeUsers, ScopeCatalog, ScopeNotifications, ScopeReports}

// Valid memeriksa apakah cakupan admin dikenal.
//
// Dipakai jalur yang menerima cakupan dari operator (mis. admin-bootstrap)
// supaya salah ketik ditolak di depan, bukan menghasilkan admin yang
// kehilangan kemampuan tanpa penjelasan.
func (s AdminScope) Valid() bool {
	switch s {
	case ScopeUsers, ScopeCatalog, ScopeNotifications, ScopeReports:
		return true
	}
	return false
}

// ActorRole = audit_log.actor_role.
//
// Disalin SAAT aksi terjadi, bukan di-join. Alasannya: kalau peran user
// berubah di kemudian hari, jejak lama harus tetap menunjukkan peran pada
// saat aksi itu dilakukan. Tanpa ini, riwayat admin yang sudah diturunkan
// jadi menyesatkan.
type ActorRole string

const (
	ActorUser   ActorRole = "user"
	ActorAdmin  ActorRole = "admin"
	ActorSystem ActorRole = "system"
)

// CurationStatus = drug_catalog.curation_status. Siklus kurasi preset.
type CurationStatus string

const (
	CurationDraft       CurationStatus = "draft"
	CurationPublished   CurationStatus = "published"
	CurationArchived    CurationStatus = "archived"
	CurationUserPrivate CurationStatus = "user_private"
)

// CatalogSource = drug_catalog.source. Asal data.
type CatalogSource string

const (
	CatalogFromKFA    CatalogSource = "kfa"
	CatalogFromBPOM   CatalogSource = "bpom"
	CatalogFromManual CatalogSource = "manual"
	CatalogFromOther  CatalogSource = "other"
)

// UnitKind = ref_unit.kind.
type UnitKind string

const (
	UnitDose        UnitKind = "dose"
	UnitCount       UnitKind = "count"
	UnitVolume      UnitKind = "volume"
	UnitApplication UnitKind = "application"
)

// Valid memeriksa apakah nilai termasuk himpunan yang diizinkan.
// Dipakai di service supaya input ditolak sebelum menyentuh database,
// sehingga pesan errornya jelas, bukan pelanggaran CHECK yang samar.
func (m MealRelation) Valid() bool {
	switch m {
	case MealBefore, MealAfter, MealWith, MealNotRelated:
		return true
	}
	return false
}

// Valid memeriksa apakah label jam termasuk himpunan yang diizinkan.
func (l TimeLabel) Valid() bool {
	switch l {
	case LabelMorning, LabelAfternoon, LabelEvening, LabelDawn, LabelOther:
		return true
	}
	return false
}

// Valid memeriksa apakah pola frekuensi dikenal.
func (p FrequencyPattern) Valid() bool {
	switch p {
	case PatternDaily, PatternInterval, PatternWeekly, PatternMonthlyCycle, PatternPRN:
		return true
	}
	return false
}

// Valid memeriksa apakah status log dikenal.
func (s LogStatus) Valid() bool {
	switch s {
	case StatusTaken, StatusLate, StatusSkipped, StatusSnoozed:
		return true
	}
	return false
}

// Valid memeriksa apakah tingkat izin caregiver dikenal.
func (r CareRole) Valid() bool {
	return r == CareViewer || r == CareHelper
}

// Valid memeriksa apakah jenis hubungan caregiver dikenal.
//
// Nilainya harus sama persis dengan CHECK di care_links.relation.
func (r CareRelation) Valid() bool {
	switch r {
	case RelationFamily, RelationChild, RelationParent, RelationSpouse,
		RelationFriend, RelationNurse, RelationOther:
		return true
	}
	return false
}

// Valid memeriksa apakah status relasi caregiver dikenal.
func (s CareStatus) Valid() bool {
	switch s {
	case CarePending, CareAccepted, CareRejected, CareRevoked, CareExpired:
		return true
	}
	return false
}

// Valid memeriksa apakah platform perangkat dikenal.
//
// Nilainya harus sama persis dengan CHECK di device_tokens.platform. Platform
// yang tidak dikenal ditolak lebih awal supaya pesan errornya jelas, bukan
// berupa pelanggaran CHECK yang samar.
func (p DevicePlatform) Valid() bool {
	switch p {
	case PlatformAndroid, PlatformIOS, PlatformWeb:
		return true
	}
	return false
}

// Valid memeriksa apakah kanal notifikasi dikenal.
//
// Nilainya harus sama persis dengan CHECK di notification_prefs.channel.
func (c Channel) Valid() bool {
	switch c {
	case ChannelPush, ChannelEmail, ChannelTelegram, ChannelInApp:
		return true
	}
	return false
}

// Valid memeriksa apakah peran user dikenal.
func (r UserRole) Valid() bool { return r == RoleUser || r == RoleAdmin }

// HasScope melaporkan apakah peran ini mencakup scope yang diminta.
//
//	role != admin            -> false (pemanggil harus cek peran lebih dulu)
//	admin_scope nil          -> true  (akses penuh)
//	admin_scope berisi x     -> hanya true untuk x
func HasScope(role UserRole, scope []AdminScope, want AdminScope) bool {
	if role != RoleAdmin {
		return false
	}
	if scope == nil {
		return true // NULL = akses penuh
	}
	for _, s := range scope {
		if s == want {
			return true
		}
	}
	return false
}

// ValidScopeList memastikan tidak ada scope asing di daftar.
func ValidScopeList(scopes []AdminScope) error {
	known := make(map[AdminScope]bool, len(AllAdminScopes))
	for _, s := range AllAdminScopes {
		known[s] = true
	}
	for _, s := range scopes {
		if !known[s] {
			return fmt.Errorf("admin_scope %q tidak dikenal", s)
		}
	}
	return nil
}
