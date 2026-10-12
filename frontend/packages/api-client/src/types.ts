/**
 * Tipe kontrak API Pantau Obat v1.
 *
 * Ditulis tangan dari `docs/design-v2/api-v2.md` dan bentuk balasan handler
 * backend (lihat `backend/internal/transport/http/handlers/*.go`), karena
 * backend belum mengekspos `/openapi.json`.
 *
 * Konvensi (api-v2 bagian 1):
 * - Nama field memakai `snake_case`, mengikuti JSON backend apa adanya.
 * - Field yang kolomnya ada di skema tetap dikirim walau bernilai `null`.
 * - Enum dikirim dalam bahasa Inggris (`taken`, `before_meal`, …); UI yang
 *   menerjemahkan lewat paket `@pantau-obat/i18n`.
 * - Tanggal: `YYYY-MM-DD`. Waktu: RFC 3339 UTC. Jam lokal: `HH:MM`.
 */

/* ============================================================
   Enum
   ============================================================ */

export type MealRelation = 'before_meal' | 'after_meal' | 'with_meal' | 'not_related';
export type TimeLabel = 'morning' | 'afternoon' | 'evening' | 'dawn' | 'other';
export type FrequencyPattern = 'daily' | 'interval' | 'weekly' | 'monthly_cycle' | 'prn';
export type CycleAnchor = 'dom' | 'end_of_month' | 'from_start';
export type LogStatus = 'taken' | 'late' | 'skipped' | 'snoozed';
export type LogSource = 'self' | 'caregiver' | 'system';
export type StockTxnKind = 'initial' | 'restock' | 'consume' | 'adjust' | 'expire';
export type Channel = 'push' | 'email' | 'telegram' | 'in_app';
export type DevicePlatform = 'android' | 'ios' | 'web';
export type CareRole = 'viewer' | 'helper' | 'manager';
export type CareStatus = 'pending' | 'accepted' | 'rejected' | 'revoked' | 'expired';
export type CareRelation = 'family' | 'child' | 'parent' | 'spouse' | 'friend' | 'nurse' | 'other';
export type UserRole = 'user' | 'admin';
export type AdminScope = 'users' | 'catalog' | 'notifications' | 'reports';
export type CurationStatus = 'draft' | 'published' | 'archived' | 'user_private';
export type CatalogSource = 'kfa' | 'bpom' | 'manual' | 'other';
export type UnitKind = 'dose' | 'count' | 'volume' | 'application';

/* ============================================================
   Pembungkus error standar
   ============================================================ */

export interface ApiErrorBody {
	error: {
		code: string;
		message: string;
		/** Rincian per field untuk error validasi (`details` di backend). */
		details?: Array<{ field: string; issue: string }>;
	};
}

/** Kode error yang dipakai backend (lihat `respond.go`). */
export const ERROR_CODES = {
	VALIDATION_FAILED: 'VALIDATION_FAILED',
	INVALID_JSON: 'INVALID_JSON',
	UNAUTHENTICATED: 'UNAUTHENTICATED',
	INVALID_CREDENTIALS: 'INVALID_CREDENTIALS',
	TOKEN_REVOKED: 'TOKEN_REVOKED',
	FORBIDDEN: 'FORBIDDEN',
	ADMIN_SCOPE_DENIED: 'ADMIN_SCOPE_DENIED',
	CARE_NOT_ACCEPTED: 'CARE_NOT_ACCEPTED',
	NOT_FOUND: 'NOT_FOUND',
	DUPLICATE_MEDICATION: 'DUPLICATE_MEDICATION',
	LOG_OCCURRENCE_TAKEN: 'LOG_OCCURRENCE_TAKEN',
	DUPLICATE_TIMES: 'DUPLICATE_TIMES',
	INVITE_PENDING: 'INVITE_PENDING',
	STOCK_INSUFFICIENT: 'STOCK_INSUFFICIENT',
	CATALOG_IMMUTABLE: 'CATALOG_IMMUTABLE',
	CATALOG_DUPLICATE_NAME: 'CATALOG_DUPLICATE_NAME',
	RATE_LIMITED: 'RATE_LIMITED',
	INTERNAL: 'INTERNAL',
} as const;

export type ErrorCode = (typeof ERROR_CODES)[keyof typeof ERROR_CODES];

/* ============================================================
   Auth & user
   ============================================================ */

export interface User {
	id: string;
	email: string;
	full_name: string;
	phone: string | null;
	timezone: string;
	is_active: boolean;
	role: UserRole;
	/** `null` berarti admin dengan akses penuh. */
	admin_scope: AdminScope[] | null;
	admin_granted_at: string | null;
	admin_granted_by: string | null;
	last_login_at: string | null;
	created_at: string;
	updated_at: string;
}

/**
 * Balasan login/register.
 *
 * `access_token` hanya ada di balasan; refresh token TIDAK ada di JSON — ia
 * dikirim sebagai cookie `po_refresh` (HttpOnly, Path=/api/v1/auth).
 * Lihat `session/` untuk cara menyimpannya.
 */
export interface AuthResult {
	access_token: string;
	token_type: 'Bearer';
	expires_in: number;
	user: User;
}

export interface RegisterInput {
	email: string;
	password: string;
	full_name: string;
	timezone?: string;
	phone?: string;
}

export interface LoginInput {
	email: string;
	password: string;
}

export interface ChangePasswordInput {
	current_password: string;
	new_password: string;
}

export interface UpdateUserInput {
	full_name?: string;
	phone?: string | null;
	timezone?: string;
}

export interface AuditLogEntry {
	id: string;
	actor_id: string;
	actor_role: string;
	action: string;
	resource_type: string | null;
	resource_id: string | null;
	created_at: string;
	/** Hanya ada bila tercatat; alamat kosong tidak dikirim. */
	ip?: string;
}

/* ============================================================
   Obat
   ============================================================ */

export interface MedicationFrequency {
	id: string;
	medication_id: string;
	owner_id: string;
	pattern: FrequencyPattern;

	times_per_day: number | null;
	interval_hours: number | null;
	days_of_week: number | null;
	interval_min_hours: number | null;

	cycle_intake_days: number | null;
	cycle_pause_days: number | null;
	cycle_anchor: CycleAnchor | null;
	cycle_anchor_dom: number | null;

	prn_max_per_day: number | null;

	tolerance_minutes: number;
	start_time: string;
	schedule_timezone: string;
}

export interface MedicationTime {
	id: string;
	medication_id: string;
	frequency_id: string | null;
	owner_id: string;
	/** `HH:MM` waktu lokal. */
	time_local: string;
	label: TimeLabel | null;
	weekdays: number;
	dose_override: string | null;
	sort_order: number;
	is_active: boolean;
}

/**
 * Saldo stok sebuah obat.
 *
 * Bentuknya mengikuti `stockDetailResponse` di backend. `doses_remaining`,
 * `needs_refill`, dan `estimated_days_left` dihitung server, bukan klien —
 * khususnya `estimated_days_left` yang memakai LAJU PEMAKAIAN NYATA 30 hari
 * terakhir, bukan jumlah dosis terjadwal. Pasien yang sering melewatkan dosis
 * akan kehabisan obat lebih lambat daripada jadwalnya, dan perkiraan berbasis
 * jadwal akan terus salah untuknya.
 */
export interface MedicationStock {
	medication_id: string;
	unit_code: string;
	initial_qty: number;
	current_qty: number;
	qty_per_dose: number;
	refill_threshold_qty: number | null;
	refill_reminder: boolean;
	doses_remaining: number;
	needs_refill: boolean;
	/** `null` berarti belum cukup data untuk memperkirakan — bukan nol. */
	estimated_days_left: number | null;
	last_restocked_at: string | null;
	last_consumed_at: string | null;
}

export interface Medication {
	id: string;
	owner_id: string;
	catalog_id: string | null;

	name: string;
	dose_value: number;
	dose_unit_code: string;
	strength_note: string | null;
	route_code: string;
	dosage_form_code: string | null;
	meal_relation: MealRelation;

	as_needed: boolean;
	must_finish: boolean;

	start_date: string;
	start_time: string;
	end_date: string | null;

	notes: string | null;
	notes_reminder: string | null;
	prescription_photo_url: string | null;
	color_tag: string | null;

	is_active: boolean;
	created_at: string;
	updated_at: string;

	/** Hanya ada pada balasan detail (GET satu obat). */
	frequency?: MedicationFrequency | null;
	times?: MedicationTime[];
	stock?: MedicationStock | null;
}

export interface CreateMedicationInput {
	name: string;
	dose_value: number;
	dose_unit_code: string;
	meal_relation: MealRelation;
	route_code?: string;
	dosage_form_code?: string | null;
	strength_note?: string | null;
	as_needed?: boolean;
	must_finish?: boolean;
	start_date: string;
	start_time: string;
	end_date?: string | null;
	notes?: string | null;
	notes_reminder?: string | null;
	color_tag?: string | null;
	catalog_id?: string | null;
	frequency: FrequencyInput;
}

export interface FrequencyInput {
	pattern: FrequencyPattern;
	times_per_day?: number;
	interval_hours?: number;
	days_of_week?: number;
	interval_min_hours?: number;
	cycle_intake_days?: number;
	cycle_pause_days?: number;
	cycle_anchor?: CycleAnchor;
	cycle_anchor_dom?: number;
	prn_max_per_day?: number;
	tolerance_minutes?: number;
	start_time: string;
}

/* ============================================================
   Catatan dosis (log)
   ============================================================ */

export interface MedicationLog {
	id: string;
	medication_id: string;
	schedule_id: string | null;
	owner_id: string;
	recorded_by: string;

	slot_date: string;
	scheduled_at: string | null;
	scheduled_hour: number | null;
	scheduled_minute: number | null;
	scheduled_tz: string | null;
	/**
	 * Jam jadwal `HH:MM` yang sudah diformat backend (dihitung dari
	 * `scheduled_hour` + `scheduled_minute`). `null` untuk dosis PRN.
	 */
	scheduled_time: string | null;
	/** Kunci identitas dosis (`schedule_id|tanggal|HH:MM`), untuk mencocokkan pengingat. */
	occurrence_key: string | null;

	status: LogStatus;
	taken_at: string | null;
	note: string | null;
	source: LogSource;

	created_at: string;
	updated_at: string;
}

/* ============================================================
   Tabel referensi (GET /refs)
   ============================================================ */

/** Satu entri tabel referensi: kode yang dikirim ke API + nama tampilan. */
export interface RefItem {
	code: string;
	name: string;
}

/**
 * Tabel referensi untuk formulir obat.
 *
 * Diambil dari backend, bukan ditulis di klien: kolom-kolom ini punya foreign
 * key ke tabel referensinya, jadi pilihan yang tidak ada di database akan
 * berakhir sebagai 500 saat menyimpan.
 */
export interface Refs {
	units: RefItem[];
	routes: RefItem[];
	dosage_forms: RefItem[];
}

/* ============================================================
   Jadwal harian (GET /schedule)
   ============================================================ */

/**
 * Status satu dosis terjadwal.
 *
 * `pending` dan `due` dihitung dari waktu; sisanya berasal dari catatan yang
 * sudah ada. Bedanya penting bagi UI: hanya `due` yang perlu ditindak.
 */
export type SlotStatus = 'pending' | 'due' | 'taken' | 'late' | 'skipped' | 'snoozed';

/**
 * Satu dosis pada satu tanggal.
 *
 * `schedule_id` adalah id baris `medication_times` — nilai yang sama yang
 * dikirim sebagai `schedule_id` saat mencatat dosis, jadi slot bisa langsung
 * dicatat tanpa penerjemahan id.
 */
export interface DaySlot {
	medication_id: string;
	medication_name: string;
	dose_value: number;
	dose_unit_code: string;
	meal_relation: MealRelation;
	color_tag: string | null;

	schedule_id: string;
	/** `HH:MM` waktu lokal pemilik. */
	time_local: string;
	time_label: TimeLabel | null;
	dose_override: string | null;

	/** Tanggal lokal pemilik, `YYYY-MM-DD`. */
	slot_date: string;
	/** Waktu jatuh tempo dalam RFC 3339. */
	scheduled_at: string;
	scheduled_tz: string;
	scheduled_hour: number;
	scheduled_minute: number;

	status: SlotStatus;
	log_id: string | null;
	taken_at: string | null;
	note: string | null;

	/** Sisa stok saat ini, bila obat punya stok. */
	stock_remaining: number | null;
	stock_unit_code: string | null;
}

export interface CreateLogInput {
	schedule_id: string;
	slot_date: string;
	scheduled_hour: number;
	scheduled_minute: number;
	scheduled_tz: string;
	status: LogStatus;
	taken_at?: string | null;
	note?: string | null;
}

export interface CreatePrnLogInput {
	slot_date: string;
	status: LogStatus;
	taken_at?: string | null;
	note?: string | null;
}

export interface UpdateLogInput {
	status?: LogStatus;
	taken_at?: string | null;
	note?: string | null;
}

export interface SnoozeLogInput {
	/** Menunda berapa menit dari waktu jadwal. */
	minutes: number;
}

/* ============================================================
   Stok
   ============================================================ */

export interface StockTransaction {
	id: string;
	kind: StockTxnKind;
	/** Perubahan saldo (positif menambah, negatif mengurangi). */
	qty_delta: number;
	balance_after: number;
	log_id: string | null;
	note: string | null;
	recorded_by: string;
	created_at: string;
}

export interface SetStockInput {
	unit_code: string;
	initial_qty: number;
	qty_per_dose: number;
	refill_threshold_qty?: number | null;
	refill_reminder?: boolean;
}

export interface RestockInput {
	/** Jumlah yang ditambahkan, dalam satuan stok. Field backend: `qty`. */
	qty: number;
	note?: string | null;
}

export interface AdjustStockInput {
	/** Perubahan saldo; negatif mengurangi. Field backend: `qty_delta`. */
	qty_delta: number;
	/** Jenis penyesuaian (mis. `adjust`). */
	kind?: StockTxnKind;
	/** Alasan koreksi — WAJIB menurut backend, dan ikut tercatat di audit. */
	note: string;
}

/* ============================================================
   Kepatuhan
   ============================================================ */

export interface AdherenceSummary {
	from: string;
	to: string;
	total: number;
	taken: number;
	late: number;
	skipped: number;
	missed: number;
	/** Persentase 0–100. */
	rate: number;
}

export interface AdherenceDailyPoint {
	date: string;
	total: number;
	taken: number;
	late: number;
	skipped: number;
	missed: number;
	rate: number;
}

/* ============================================================
   Katalog
   ============================================================ */

export interface CatalogItem {
	id: string;
	name: string;
	generic_name: string | null;
	form_code: string | null;
	route_code: string | null;
	strength_note: string | null;
	unit_code: string | null;
	source: CatalogSource;
	curation_status: CurationStatus;
	owner_id: string | null;
	created_at: string;
	updated_at: string;
}

export interface CatalogSuggestion {
	name: string;
	generic_name: string | null;
	form_code: string | null;
	route_code: string | null;
	strength_note: string | null;
}

/* ============================================================
   Caregiver (care-links)
   ============================================================ */

export interface CareLink {
	id: string;
	owner_id: string;
	caregiver_id: string | null;
	invite_email: string | null;
	invite_expires_at: string | null;
	relation: CareRelation;
	role: CareRole;
	status: CareStatus;
	decided_at: string | null;
	created_at: string;
}

export interface CreateInviteInput {
	/** Email yang diundang. Boleh belum terdaftar — undangan menunggu. */
	invite_email: string;
	relation: CareRelation;
	role: CareRole;
}

export interface AcceptInviteInput {
	/** Token dari tautan undangan. */
	token: string;
}

export interface UpdateCareLinkInput {
	relation?: CareRelation;
	role?: CareRole;
}

export interface CareOverview {
	owner: Pick<User, 'id' | 'full_name' | 'timezone'>;
	medications_count: number;
	adherence: AdherenceSummary;
	low_stock: MedicationStock[];
	recent_logs: MedicationLog[];
}

/* ============================================================
   Notifikasi
   ============================================================ */

export interface NotificationPrefs {
	channel: Channel;
	enabled: boolean;
	quiet_start: string;
	quiet_end: string;
	advance_minutes: number;
	throttled: boolean;
}

export interface UpdateNotificationPrefsInput {
	channel: Channel;
	enabled?: boolean;
	quiet_start?: string;
	quiet_end?: string;
	advance_minutes?: number;
}

export interface InAppNotification {
	id: string;
	kind: string;
	title: string;
	body: string;
	read_at: string | null;
	created_at: string;
}

export interface DeviceToken {
	id: string;
	platform: DevicePlatform;
	app_version: string | null;
	last_seen_at: string;
	created_at: string;
}

export interface RegisterDeviceInput {
	token: string;
	platform: DevicePlatform;
	app_version?: string;
}

/* ============================================================
   Paginasi (cursor)
   ============================================================ */

export interface CursorPage<T> {
	items: T[];
	next_cursor: string | null;
}
