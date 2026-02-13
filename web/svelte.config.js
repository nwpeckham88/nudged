import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		adapter: adapter({
			pages: '../internal/webui/dist',
			assets: '../internal/webui/dist',
			fallback: 'index.html',
			precompress: false,
			strict: true
		}),
		paths: {
			base: '/ui'
		}
	}
};

export default config;
