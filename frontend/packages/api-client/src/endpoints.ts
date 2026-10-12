/**
 * Endpoint API Pantau Obat — pembungkus tipis di atas `ApiClient`.
 *
 * Dipisah dari `client.ts` supaya pemanggilan di komponen tidak menulis path
 * mentah, dan supaya bentuk request/response satu tempat dengan tipenya.
 */
import type { ApiClient } from './client';
import type {
	AcceptInviteInput,
	AdherenceDailyPoint,
	AdherenceSummary,
	AuditLogEntry,
	AuthResult,
	CareLink,
	CareOverview,
	CatalogItem,
	CatalogSuggestion,
	ChangePasswordInput,
	CreateInviteInput,
	CreateLogInput,
	CreateMedicationInput,
	CreatePrnLogInput,
	DaySlot,
	DeviceToken,
	InAppNotification,
	LoginInput,
	Medication,
	MedicationLog,
	MedicationStock,
	MedicationTime,
	NotificationPrefs,
	RegisterDeviceInput,
	RegisterInput,
	Refs,
	RestockInput,
	SetStockInput,
	AdjustStockInput,
	SnoozeLogInput,
	StockTransaction,
	UpdateCareLinkInput,
	UpdateLogInput,
	UpdateNotificationPrefsInput,
	UpdateUserInput,
	User,
} from './types';

type Query = Record<string, string | number | boolean | null | undefined>;

export function createApi(client: ApiClient) {
	return {
		/* ---------------- Auth ---------------- */
		auth: {
			register: (input: RegisterInput) => client.post<AuthResult>('/auth/register', input),
			login: (input: LoginInput) => client.post<AuthResult>('/auth/login', input),
			logout: () => client.post<void>('/auth/logout'),
			me: () => client.get<User>('/auth/me'),
			changePassword: (input: ChangePasswordInput) =>
				client.post<void>('/auth/change-password', input),
		},

		/* ---------------- Profil ---------------- */
		users: {
			me: () => client.get<User>('/users/me'),
			update: (input: UpdateUserInput) => client.patch<User>('/users/me', input),
			remove: () => client.delete<void>('/users/me'),
			auditLog: (query?: Query) => client.get<AuditLogEntry[]>('/users/me/audit-log', query),
		},
		/* ---------------- Obat ---------------- */
		medications: {
			list: (query?: Query) => client.get<Medication[]>('/medications', query),
			create: (input: CreateMedicationInput) => client.post<Medication>('/medications', input),
			get: (id: string) => client.get<Medication>(`/medications/${id}`),
			update: (id: string, input: Partial<CreateMedicationInput>) =>
				client.patch<Medication>(`/medications/${id}`, input),
			remove: (id: string) => client.delete<void>(`/medications/${id}`),
			times: (id: string) => client.get<MedicationTime[]>(`/medications/${id}/times`),
			addTime: (id: string, input: { time_local: string; label?: string }) =>
				client.post<MedicationTime>(`/medications/${id}/times`, input),
			updateTime: (id: string, timeId: string, input: { time_local?: string; label?: string }) =>
				client.patch<MedicationTime>(`/medications/${id}/times/${timeId}`, input),
			removeTime: (id: string, timeId: string) =>
				client.delete<void>(`/medications/${id}/times/${timeId}`),
		},

		/* ---------------- Tabel referensi ---------------- */
		refs: {
			/** Satuan, rute, dan bentuk sediaan — satu panggilan untuk formulir obat. */
			all: () => client.get<Refs>('/refs'),
		},

		/* ---------------- Jadwal harian ---------------- */
		schedule: {
			/**
			 * Jadwal satu hari. Tanpa `date`, dipakai hari ini menurut zona
			 * waktu pengguna (bukan UTC).
			 */
			day: (date?: string) => client.get<DaySlot[]>('/schedule', date ? { date } : undefined),
		},

		/* ---------------- Catatan dosis ---------------- */
		logs: {
			today: (query?: Query) => client.get<MedicationLog[]>('/logs/today', query),
			list: (query?: Query) => client.get<MedicationLog[]>('/logs', query),
			forMedication: (medicationId: string, query?: Query) =>
				client.get<MedicationLog[]>(`/medications/${medicationId}/logs`, query),
			create: (medicationId: string, input: CreateLogInput) =>
				client.post<MedicationLog>(`/medications/${medicationId}/logs`, input),
			createPrn: (medicationId: string, input: CreatePrnLogInput) =>
				client.post<MedicationLog>(`/medications/${medicationId}/logs/prn`, input),
			update: (logId: string, input: UpdateLogInput) =>
				client.patch<MedicationLog>(`/logs/${logId}`, input),
			snooze: (logId: string, input: SnoozeLogInput) =>
				client.post<MedicationLog>(`/logs/${logId}/snooze`, input),
			/** Ekspor CSV (RFC 4180 + BOM) — dibalas sebagai berkas. */
			exportCsv: (query?: Query) => client.requestBlob('/logs/export', query),
		},

		/* ---------------- Stok ---------------- */
		stock: {
			get: (medicationId: string) =>
				client.get<MedicationStock>(`/medications/${medicationId}/stock`),
			set: (medicationId: string, input: SetStockInput) =>
				client.put<MedicationStock>(`/medications/${medicationId}/stock`, input),
			transactions: (medicationId: string, query?: Query) =>
				client.get<StockTransaction[]>(`/medications/${medicationId}/stock/transactions`, query),
			restock: (medicationId: string, input: RestockInput) =>
				client.post<MedicationStock>(`/medications/${medicationId}/stock/restock`, input),
			adjust: (medicationId: string, input: AdjustStockInput) =>
				client.post<MedicationStock>(`/medications/${medicationId}/stock/adjust`, input),
			low: (query?: Query) => client.get<MedicationStock[]>('/stock/low', query),
		},

		/* ---------------- Kepatuhan ---------------- */
		adherence: {
			summary: (query?: Query) => client.get<AdherenceSummary>('/adherence', query),
			daily: (query?: Query) => client.get<AdherenceDailyPoint[]>('/adherence/daily', query),
			forMedication: (id: string, query?: Query) =>
				client.get<AdherenceSummary>(`/medications/${id}/adherence`, query),
		},

		/* ---------------- Katalog ---------------- */
		catalog: {
			list: (query?: Query) => client.get<CatalogItem[]>('/catalog', query),
			suggest: (query?: Query) => client.get<CatalogSuggestion[]>('/catalog/suggest', query),
			get: (id: string) => client.get<CatalogItem>(`/catalog/${id}`),
		},

		/* ---------------- Caregiver ---------------- */
		care: {
			/** Relasi tempat SAYA pemilik data ("siapa yang memantau saya"). */
			list: (query?: Query) => client.get<CareLink[]>('/care-links', query),
			/** Relasi tempat SAYA pendamping ("siapa yang saya pantau"). */
			listAsCaregiver: (query?: Query) =>
				client.get<CareLink[]>('/care-links', { ...query, as: 'caregiver' }),
			invite: (input: CreateInviteInput) => client.post<CareLink>('/care-links', input),
			accept: (input: AcceptInviteInput) => client.post<CareLink>('/care-links/accept', input),
			reject: (input: AcceptInviteInput) => client.post<CareLink>('/care-links/reject', input),
			update: (id: string, input: UpdateCareLinkInput) =>
				client.patch<CareLink>(`/care-links/${id}`, input),
			revoke: (id: string) => client.delete<void>(`/care-links/${id}`),

			overview: (ownerId: string) => client.get<CareOverview>(`/care/${ownerId}/overview`),
			medications: (ownerId: string, query?: Query) =>
				client.get<Medication[]>(`/care/${ownerId}/medications`, query),
			adherence: (ownerId: string, query?: Query) =>
				client.get<AdherenceSummary>(`/care/${ownerId}/adherence`, query),
			logs: (ownerId: string, query?: Query) =>
				client.get<MedicationLog[]>(`/care/${ownerId}/logs`, query),
			createLog: (ownerId: string, input: CreateLogInput) =>
				client.post<MedicationLog>(`/care/${ownerId}/logs`, input),
			/** Ekspor CSV atas nama pasien — wajib menyertakan `reason`. */
			exportCsv: (ownerId: string, query: Query) =>
				client.requestBlob(`/care/${ownerId}/logs/export`, query),
		},

		/* ---------------- Notifikasi ---------------- */
		notifications: {
			prefs: () => client.get<NotificationPrefs[]>('/notification-prefs'),
			updatePrefs: (input: UpdateNotificationPrefsInput) =>
				client.patch<NotificationPrefs>('/notification-prefs', input),
			list: (query?: Query) => client.get<InAppNotification[]>('/notifications', query),
			devices: () => client.get<DeviceToken[]>('/device-tokens'),
			registerDevice: (input: RegisterDeviceInput) =>
				client.post<DeviceToken>('/device-tokens', input),
			unregisterDevice: (token: string) =>
				client.delete<void>(`/device-tokens/${encodeURIComponent(token)}`),
		},
	};
}

export type Api = ReturnType<typeof createApi>;
