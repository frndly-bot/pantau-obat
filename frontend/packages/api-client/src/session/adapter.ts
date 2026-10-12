/**
 * Session adapter — titik tukar antara web dan mobile (Capacitor).
 *
 * KEPUTUSAN ARSITEKTUR (lihat plan §4.3):
 * Frontend ini akan dibungkus Capacitor menjadi aplikasi mobile. Capacitor
 * TIDAK bisa menjalankan server Node, jadi pola BFF (proxy sisi server) tidak
 * bisa dipakai. Karena itu sesi dibagi dua lapis:
 *
 *   - Refresh token  → TIDAK PERNAH ada di JavaScript.
 *                      Web    : cookie `po_refresh` (HttpOnly, SameSite=Strict)
 *                      Mobile : secure storage native (Keychain / Keystore)
 *   - Access token   → di MEMORI saja, umur pendek (~15 menit)
 *
 * Efeknya: token berumur panjang tidak bisa dieksfiltrasi lewat XSS. Yang ada
 * di JS hanya access token berumur pendek, dan itu dibatasi oleh rotasi
 * refresh + pencabutan sesi di backend.
 *
 * Antarmuka di bawah sengaja dibuat minimal supaya implementasi native nanti
 * cukup menukar adapter tanpa menyentuh UI maupun pemanggilan API.
 */

/** Penyimpan refresh token. */
export interface SessionAdapter {
	/**
	 * Mengembalikan refresh token, atau `null` bila tidak ada.
	 *
	 * Di web, ini selalu `null`: refresh token ada di cookie HttpOnly yang
	 * tidak bisa dibaca JavaScript. Nilai `null` di sini BUKAN berarti
	 * "belum login" — untuk itu pakai `hasSession()`.
	 */
	getRefresh(): Promise<string | null>;

	/**
	 * Menyimpan refresh token.
	 *
	 * Di web ini tidak melakukan apa pun (backend sudah memasang cookie-nya
	 * sendiri lewat `Set-Cookie`); di mobile menyimpannya ke secure storage.
	 */
	setRefresh(token: string): Promise<void>;

	/** Menghapus refresh token (dipakai saat logout / refresh gagal). */
	clear(): Promise<void>;

	/**
	 * Menandakan apakah mungkin ada sesi.
	 *
	 * Di web: bergantung pada penanda non-rahasia (`po_session=1`, bukan
	 * HttpOnly) supaya JS tahu kapan harus mencoba refresh. Di mobile:
	 * apakah secure storage berisi token.
	 */
	hasSession(): Promise<boolean>;
}

/**
 * Adapter web.
 *
 * Refresh token dikelola sepenuhnya oleh browser + backend lewat cookie
 * HttpOnly. JavaScript hanya menyimpan PENANDA non-rahasia (`po_session=1`)
 * supaya tahu kapan perlu memanggil `/auth/refresh` — tanpa penanda ini,
 * setiap kunjungan (termasuk tamu) akan memicu permintaan refresh yang gagal.
 */
export function createWebSessionAdapter(options?: {
	/** Nama penanda non-rahasia. Bukan token. */
	markerName?: string;
}): SessionAdapter {
	const markerName = options?.markerName ?? 'po_session';

	function readCookie(name: string): string | null {
		if (typeof document === 'undefined') return null;
		const prefix = `${name}=`;
		for (const part of document.cookie.split(';')) {
			const trimmed = part.trim();
			if (trimmed.startsWith(prefix)) return trimmed.slice(prefix.length);
		}
		return null;
	}

	return {
		async getRefresh() {
			// Sengaja selalu null: refresh token tidak boleh terbaca JS.
			return null;
		},
		async setRefresh() {
			// Backend yang memasang cookie refresh lewat `Set-Cookie`. Di sini
			// hanya dicatat bahwa ada sesi, supaya halaman berikutnya tahu
			// perlu mencoba refresh.
			if (typeof document === 'undefined') return;
			document.cookie = `${markerName}=1; Path=/; Max-Age=${60 * 60 * 24 * 30}; SameSite=Strict`;
		},
		async clear() {
			if (typeof document === 'undefined') return;
			document.cookie = `${markerName}=; Path=/; Max-Age=0; SameSite=Strict`;
		},
		async hasSession() {
			return readCookie(markerName) !== null;
		},
	};
}

/**
 * Adapter untuk pengujian & prerender (tanpa `document`).
 * Tidak menyimpan apa pun.
 */
export function createMemorySessionAdapter(initial?: string): SessionAdapter {
	let token: string | null = initial ?? null;
	return {
		async getRefresh() {
			return token;
		},
		async setRefresh(value: string) {
			token = value;
		},
		async clear() {
			token = null;
		},
		async hasSession() {
			return token !== null;
		},
	};
}
