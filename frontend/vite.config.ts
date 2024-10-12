import { sveltekit } from '@sveltejs/kit/vite';

export default {
	plugins: [sveltekit()],
	build: {
		outDir: 'sn/frontend'
	}
};
