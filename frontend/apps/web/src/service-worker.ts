/// <reference lib="webworker" />
/**
 * Service worker Pantau Obat.
 *
 * PRINSIP UTAMA: yang di-cache adalah KERANGKA aplikasi (HTML, JS, CSS, ikon),
 * BUKAN data pengguna. Data kesehatan tidak boleh tersimpan di cache peramban
 * yang bisa dibaca dari perangkat bersama, dan data basi lebih berbahaya
 * daripada tidak ada data: pengguna bisa mengira dosis hari ini sudah dicatat
 * padahal catatan itu dari kemarin.
 *
 * Karena itu:
 * - Permintaan ke `/api/**` SELALU lewat jaringan, tidak pernah di-cache.
 * - Hanya metode GET yang disentuh; POST/PATCH/DELETE tidak pernah di-cache.
 * - Navigasi memakai strategi network-first: halaman selalu dicoba dari
 *   jaringan dulu, cache hanya jaring pengaman saat offline.
 */
const CACHE_NAME = 'pantau-obat-v1';

/** Kerangka yang di-cache saat pemasangan. */
const APP_SHELL = ['/', '/login', '/hari-ini', '/obat', '/riwayat', '/caregiver', '/pengaturan'];

self.addEventListener('install', (event) => {
	event.waitUntil(
		caches.open(CACHE_NAME).then((cache) =>
			// Satu berkas gagal tidak boleh membatalkan seluruh pemasangan.
			Promise.allSettled(APP_SHELL.map((url) => cache.add(url)))
		)
	);
	// Ambil alih segera supaya perbaikan tidak menunggu tab ditutup.
	void self.skipWaiting();
});

self.addEventListener('activate', (event) => {
	event.waitUntil(
		caches
			.keys()
			.then((keys) =>
				Promise.all(keys.filter((k) => k !== CACHE_NAME).map((k) => caches.delete(k)))
			)
			.then(() => self.clients.claim())
	);
});

self.addEventListener('fetch', (event) => {
	const request = event.request;

	// Hanya GET. Permintaan yang mengubah data tidak boleh di-cache.
	if (request.method !== 'GET') return;

	const url = new URL(request.url);

	// Hanya origin sendiri.
	if (url.origin !== self.location.origin) return;

	// API: SELALU ke jaringan. Tidak ada data pengguna di cache.
	if (url.pathname.startsWith('/api/')) return;

	// Navigasi: network-first.
	if (request.mode === 'navigate') {
		event.respondWith(
			fetch(request)
				.then((response) => {
					// Simpan salinan terbaru untuk keadaan offline.
					const copy = response.clone();
					caches.open(CACHE_NAME).then((cache) => cache.put(request, copy)).catch(() => {});
					return response;
				})
				.catch(async () => {
					const cached = await caches.match(request);
					if (cached) return cached;
					// Halaman ber-guard tidak ada di cache (dirender di klien),
					// jadi kerangka terdekat dipakai sebagai jaring pengaman.
					const shell = await caches.match('/');
					return shell ?? Response.error();
				})
		);
		return;
	}

	// Aset statis: cache-first, lalu perbarui di latar.
	event.respondWith(
		caches.match(request).then((cached) => {
			const network = fetch(request)
				.then((response) => {
					if (response.ok) {
						const copy = response.clone();
						caches.open(CACHE_NAME).then((cache) => cache.put(request, copy)).catch(() => {});
					}
					return response;
				})
				.catch(() => cached);
			return cached ?? network;
		})
	);
});
