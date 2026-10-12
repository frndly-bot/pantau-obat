/**
 * Opsi halaman terima undangan: dirender saat build (prerender) sehingga
 * kerangka halaman langsung tampil sebelum bundle JavaScript selesai dimuat.
 *
 * Isinya sendiri (status sesi, hasil penerimaan) ditentukan di klien, karena
 * bergantung pada cookie refresh HttpOnly yang tidak terbaca saat build.
 */
export const prerender = true;
