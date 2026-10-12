/**
 * Adapter native (Capacitor) — BELUM DIPAKAI.
 *
 * Kerangka ini disiapkan supaya saat frontend dibungkus Capacitor (rencana
 * jauh), hanya berkas ini yang perlu diisi: UI dan pemanggilan API tidak
 * berubah. Yang dibutuhkan hanya plugin secure storage
 * (`@capacitor/preferences` atau `capacitor-secure-storage-plugin`).
 *
 * Cara mengaktifkan nanti:
 *   1. `pnpm add @capacitor/core capacitor-secure-storage-plugin`
 *   2. Isi implementasi di bawah memakai plugin tersebut.
 *   3. Di `apps/web/src/lib/api.ts`, pilih adapter:
 *        isNativePlatform() ? createNativeSessionAdapter() : createWebSessionAdapter()
 *   4. Tambahkan origin `capacitor://localhost` (iOS) dan `http://localhost`
 *      (Android) ke daftar CORS backend.
 */
import type { SessionAdapter } from './adapter';

/** Nama kunci di secure storage. */
export const NATIVE_REFRESH_KEY = 'po_refresh_token';

/**
 * Adapter native. Saat ini hanya melempar error supaya tidak diam-diam
 * dipakai sebelum Capacitor benar-benar terpasang.
 */
export function createNativeSessionAdapter(): SessionAdapter {
	const notReady = async (): Promise<never> => {
		throw new Error(
			'Adapter sesi native belum diimplementasikan. ' +
				'Pasang plugin secure storage Capacitor lalu isi createNativeSessionAdapter().'
		);
	};

	return {
		getRefresh: notReady,
		setRefresh: notReady,
		clear: notReady,
		hasSession: async () => false,
	};
}
