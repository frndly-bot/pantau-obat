/**
 * Titik tunggal klien API untuk aplikasi web.
 *
 * Adapter sesi dipilih di sini. Saat ini web; ketika Capacitor ditambahkan,
 * cukup tukar ke `createNativeSessionAdapter()` tanpa menyentuh komponen.
 */
import {
	ApiClient,
	createApi,
	createWebSessionAdapter,
	type Api,
} from '@pantau-obat/api-client';
import { browser } from '$app/environment';
import { API_BASE_URL } from './config';

const session = createWebSessionAdapter();

export const client = new ApiClient({
	baseUrl: API_BASE_URL,
	session,
	onSessionExpired: () => {
		// Sesi benar-benar habis (refresh gagal). Bersihkan state aplikasi;
		// pengalihan ke /login ditangani oleh penjaga rute.
		if (browser) {
			window.dispatchEvent(new CustomEvent('po:session-expired'));
		}
	},
});

export const api: Api = createApi(client);
