import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	server: {
		proxy: {
			'/agents': 'http://localhost:8080',
			'/apps': 'http://localhost:8080',
			'/health': 'http://localhost:8080'
		}
	}
});
