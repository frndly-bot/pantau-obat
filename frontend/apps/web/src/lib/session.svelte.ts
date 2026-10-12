/**
 * State sesi aplikasi (Svelte 5 runes).
 *
 * Aturan penting (plan §4.3):
 * - Access token hidup di MEMORI, di dalam `ApiClient` — bukan di sini, bukan
 *   di localStorage. Komponen hanya melihat `user`.
 * - Refresh token tidak pernah terlihat JavaScript (cookie HttpOnly).
 *
 * Karena itu, `user` adalah satu-satunya hal yang dibagikan ke UI.
 */
import type { User } from '@pantau-obat/api-client';
import { api, client } from './api';

class SessionState {
	/** Profil pengguna yang sedang masuk, atau `null`. */
	user = $state<User | null>(null);
	/** Benar selama pemulihan sesi awal berjalan. */
	loading = $state(true);

	/** Pemulihan sesi dijalankan sekali saja, walau dipanggil berkali-kali. */
	private restorePromise: Promise<void> | null = null;

	get isAuthenticated(): boolean {
		return this.user !== null;
	}

	/**
	 * Memulihkan sesi sekali (idempoten). Layout akar memanggilnya saat mount;
	 * penjaga rute menunggu promise yang sama, sehingga tidak ada dua
	 * permintaan refresh berbarengan.
	 */
	init(): Promise<void> {
		if (!this.restorePromise) this.restorePromise = this.doRestore();
		return this.restorePromise;
	}

	private async doRestore(): Promise<void> {
		this.loading = true;
		try {
			this.user = await client.restore();
		} catch {
			this.user = null;
		} finally {
			this.loading = false;
		}
	}

	/** Memulihkan sesi saat aplikasi dimuat (nama lama, tetap didukung). */
	async restore(): Promise<void> {
		return this.init();
	}

	/** Setelah login/register berhasil. */
	async adopt(accessToken: string, user: User): Promise<void> {
		await client.applyAuth({
			access_token: accessToken,
			token_type: 'Bearer',
			expires_in: 0,
			user,
		});
		this.user = user;
	}

	/** Mengambil profil terbaru dari backend. */
	async refreshUser(): Promise<void> {
		this.user = await api.auth.me();
	}

	/** Keluar: beri tahu backend, lalu bersihkan state lokal. */
	async signOut(): Promise<void> {
		try {
			await api.auth.logout();
		} catch {
			// Keluar tetap harus berhasil walau backend tidak bisa dihubungi.
		}
		await client.clearSession();
		this.user = null;
	}

	/** Membersihkan state tanpa memanggil backend (sesi kadaluwarsa). */
	async forget(): Promise<void> {
		await client.clearSession();
		this.user = null;
	}
}

export const session = new SessionState();
