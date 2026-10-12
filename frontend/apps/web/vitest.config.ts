import { defineConfig } from 'vitest/config';

/**
 * Konfigurasi pengujian, dipisah dari `vite.config.ts` supaya plugin
 * SvelteKit (yang khusus build/dev) tidak ikut dimuat saat menjalankan tes.
 */
export default defineConfig({
	test: {
		include: ['src/**/*.{test,spec}.{js,ts}'],
		environment: 'node',
	},
});
