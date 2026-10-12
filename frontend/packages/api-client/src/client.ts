/**
 * Klien HTTP Pantau Obat.
 *
 * Tanggung jawab:
 * - Menyisipkan `Authorization: Bearer <access token>` dari MEMORI.
 * - Menyegarkan access token otomatis saat backend membalas 401, lalu
 *   mengulang permintaan SEKALI (bukan berulang, supaya tidak ada loop).
 * - Menyeragamkan error menjadi `ApiError` bertipe.
 *
 * Access token sengaja HANYA di memori variabel modul — bukan localStorage,
 * bukan sessionStorage. Saat tab ditutup, token hilang; sesi dipulihkan lewat
 * `/auth/refresh` yang memakai cookie HttpOnly (lihat `session/`).
 */
import { ERROR_CODES, type ApiErrorBody, type AuthResult, type User } from './types';
import type { SessionAdapter } from './session/adapter';

/** Informasi paginasi berbasis cursor (lihat `respond.go` di backend). */
export interface PageInfo {
	next_cursor: string | null;
	has_more: boolean;
	limit: number;
	total?: number;
}

export interface ApiClientOptions {
	/** Basis URL API, mis. `http://127.0.0.1:8080/api/v1`. */
	baseUrl: string;
	/** Adapter sesi (web / native / memory). */
	session: SessionAdapter;
	/** Dipanggil saat sesi benar-benar habis (refresh gagal). */
	onSessionExpired?: () => void;
	/** Implementasi fetch; bisa ditukar saat pengujian. */
	fetchImpl?: typeof fetch;
}

/** Error API yang membawa kode dari backend. */
export class ApiError extends Error {
	readonly status: number;
	readonly code: string;
	/** Rincian per field untuk error validasi (`error.details` di backend). */
	readonly details?: Array<{ field: string; issue: string }>;

	constructor(
		status: number,
		code: string,
		message: string,
		details?: Array<{ field: string; issue: string }>
	) {
		super(message);
		this.name = 'ApiError';
		this.status = status;
		this.code = code;
		this.details = details;
	}

	get isUnauthenticated(): boolean {
		return this.code === ERROR_CODES.UNAUTHENTICATED || this.status === 401;
	}

	get isForbidden(): boolean {
		return this.code === ERROR_CODES.FORBIDDEN || this.status === 403;
	}

	get isNotFound(): boolean {
		return this.code === ERROR_CODES.NOT_FOUND || this.status === 404;
	}

	/** Rincian masalah untuk satu field, bila ada. */
	issueFor(field: string): string | undefined {
		return this.details?.find((d) => d.field === field)?.issue;
	}
}

type QueryValue = string | number | boolean | null | undefined;

interface SuccessEnvelope<T> {
	data: T;
	page?: PageInfo;
}

export class ApiClient {
	private readonly baseUrl: string;
	private readonly session: SessionAdapter;
	private readonly fetchImpl: typeof fetch;
	private readonly onSessionExpired?: () => void;

	/** Access token — di memori saja. */
	private accessToken: string | null = null;
	/** Mencegah banyak refresh bersamaan (single-flight). */
	private refreshInFlight: Promise<boolean> | null = null;

	constructor(options: ApiClientOptions) {
		this.baseUrl = options.baseUrl.replace(/\/$/, '');
		this.session = options.session;
		this.fetchImpl = options.fetchImpl ?? globalThis.fetch.bind(globalThis);
		this.onSessionExpired = options.onSessionExpired;
	}

	/** Memanggil `/auth/me` untuk mengambil profil pengguna yang sedang masuk. */
	async me(): Promise<User> {
		return this.get<User>('/auth/me');
	}

	/** Apakah ada access token di memori. */
	get hasAccessToken(): boolean {
		return this.accessToken !== null;
	}

	/**
	 * Memulihkan sesi saat pemuatan halaman: mencoba refresh memakai cookie
	 * HttpOnly, lalu mengambil profil. Aman dipanggil walau tidak ada sesi.
	 */
	async restore(): Promise<User | null> {
		if (!(await this.session.hasSession())) return null;
		const ok = await this.refresh();
		if (!ok) return null;
		return this.me();
	}

	/** Menyimpan access token setelah login/register. */
	async applyAuth(result: AuthResult): Promise<User> {
		this.accessToken = result.access_token;
		// Beri tahu adapter bahwa sekarang ada sesi. Di web ini hanya menulis
		// penanda non-rahasia (refresh token tetap di cookie HttpOnly); di
		// native ini yang menyimpan token ke secure storage.
		await this.session.setRefresh(result.access_token);
		return result.user;
	}

	/** Menghapus sesi lokal. */
	async clearSession(): Promise<void> {
		this.accessToken = null;
		await this.session.clear();
	}

	/** Memanggil `/auth/refresh` dan memperbarui access token. */
	async refresh(): Promise<boolean> {
		if (this.refreshInFlight) return this.refreshInFlight;
		this.refreshInFlight = this.doRefresh().finally(() => {
			this.refreshInFlight = null;
		});
		return this.refreshInFlight;
	}

	private async doRefresh(): Promise<boolean> {
		try {
			const res = await this.fetchImpl(`${this.baseUrl}/auth/refresh`, {
				method: 'POST',
				headers: { Accept: 'application/json' },
				// Cookie refresh (web) ikut terkirim. Di native, adapter
				// menyisipkan tokennya lewat header di bawah.
				credentials: 'include',
			});
			if (!res.ok) {
				await this.expire();
				return false;
			}
			const env = (await res.json()) as SuccessEnvelope<AuthResult>;
			this.accessToken = env.data.access_token;
			return true;
		} catch {
			await this.expire();
			return false;
		}
	}

	private async expire(): Promise<void> {
		this.accessToken = null;
		await this.session.clear();
		this.onSessionExpired?.();
	}

	/** Header untuk satu permintaan. */
	private async headers(extra?: HeadersInit): Promise<Headers> {
		const headers = new Headers(extra);
		headers.set('Accept', 'application/json');
		if (this.accessToken) headers.set('Authorization', `Bearer ${this.accessToken}`);
		return headers;
	}

	/* ---------------- Permintaan inti ---------------- */

	/**
	 * Satu permintaan; mengembalikan `data` dari envelope `{ data, page }`.
	 * Pakai `requestPage` bila informasi paginasi juga dibutuhkan.
	 */
	async request<T>(
		method: string,
		path: string,
		options?: { body?: unknown; query?: Record<string, QueryValue>; retryOn401?: boolean }
	): Promise<T> {
		const result = await this.requestPage<T>(method, path, options);
		return result.data;
	}

	/** Seperti `request`, tetapi juga mengembalikan `page`. */
	async requestPage<T>(
		method: string,
		path: string,
		options?: { body?: unknown; query?: Record<string, QueryValue>; retryOn401?: boolean }
	): Promise<{ data: T; page?: PageInfo }> {
		const url = this.buildUrl(path, options?.query);
		const headers = await this.headers(
			options?.body !== undefined ? { 'Content-Type': 'application/json' } : undefined
		);

		const res = await this.fetchImpl(url, {
			method,
			headers,
			credentials: 'include',
			body: options?.body !== undefined ? JSON.stringify(options.body) : undefined,
		});

		if (res.status === 401 && (options?.retryOn401 ?? true)) {
			// Access token kadaluwarsa — segarkan sekali lalu ulangi.
			const refreshed = await this.refresh();
			if (refreshed) {
				return this.requestPage<T>(method, path, { ...options, retryOn401: false });
			}
		}

		if (res.status === 204) return { data: undefined as T };

		const text = await res.text();
		const body = text ? safeJson(text) : undefined;

		if (!res.ok) throw toApiError(res.status, res.statusText, body);

		// Backend selalu membalas `{ data, page? }`. Bila bentuknya lain
		// (mis. dari proxy), data dikembalikan apa adanya.
		if (body && typeof body === 'object' && 'data' in body) {
			const env = body as SuccessEnvelope<T>;
			return { data: env.data, page: env.page };
		}
		return { data: body as T };
	}

	/** Permintaan yang mengembalikan berkas (mis. ekspor CSV). */
	async requestBlob(path: string, query?: Record<string, QueryValue>): Promise<Blob> {
		const url = this.buildUrl(path, query);
		const headers = await this.headers();
		const res = await this.fetchImpl(url, { method: 'GET', headers, credentials: 'include' });
		if (!res.ok) {
			const text = await res.text();
			throw toApiError(res.status, res.statusText, text ? safeJson(text) : undefined);
		}
		return res.blob();
	}

	get<T>(path: string, query?: Record<string, QueryValue>) {
		return this.request<T>('GET', path, { query });
	}

	post<T>(path: string, body?: unknown, query?: Record<string, QueryValue>) {
		return this.request<T>('POST', path, { body, query });
	}

	patch<T>(path: string, body?: unknown) {
		return this.request<T>('PATCH', path, { body });
	}

	put<T>(path: string, body?: unknown) {
		return this.request<T>('PUT', path, { body });
	}

	delete<T>(path: string, body?: unknown) {
		return this.request<T>('DELETE', path, { body });
	}

	private buildUrl(path: string, query?: Record<string, QueryValue>): string {
		const normalized = path.startsWith('/') ? path : `/${path}`;
		const url = `${this.baseUrl}${normalized}`;
		if (!query) return url;
		const params = new URLSearchParams();
		for (const [key, value] of Object.entries(query)) {
			if (value === undefined || value === null) continue;
			params.set(key, String(value));
		}
		const qs = params.toString();
		return qs ? `${url}?${qs}` : url;
	}
}

/** Mengubah balasan error backend menjadi `ApiError`. */
function toApiError(status: number, statusText: string, body: unknown): ApiError {
	const env = body as ApiErrorBody | undefined;
	const detail = env?.error;
	return new ApiError(
		status,
		detail?.code ?? `HTTP_${status}`,
		detail?.message ?? statusText,
		detail?.details
	);
}

function safeJson(text: string): unknown {
	try {
		return JSON.parse(text);
	} catch {
		return undefined;
	}
}
