/**
 * Opsi halaman: landing page dirender saat BUILD (prerender), bukan per
 * permintaan. Karena produksi memakai `adapter-static` (ADR-001 — tanpa
 * runtime Node di VPS), halaman ini menjadi berkas HTML terisi penuh:
 * SEO baik, muat cepat, dan tetap berfungsi di dalam Capacitor.
 */
export const prerender = true;
