import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	server: {
		// Dijalankan dengan --host 0.0.0.0 supaya bisa dibuka dari browser.
		port: 5173,
		strictPort: false,
	},
});
