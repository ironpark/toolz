import { builtinModules } from 'node:module';
import { copyFile } from 'node:fs/promises';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vite';

const external = [
	'obsidian',
	'electron',
	'@codemirror/autocomplete',
	'@codemirror/collab',
	'@codemirror/commands',
	'@codemirror/language',
	'@codemirror/lint',
	'@codemirror/search',
	'@codemirror/state',
	'@codemirror/view',
	'@lezer/common',
	'@lezer/highlight',
	'@lezer/lr',
	...builtinModules,
	...builtinModules.map((module) => `node:${module}`),
];

const copyObsidianArtifacts = {
	name: 'copy-obsidian-artifacts',
	async closeBundle(): Promise<void> {
		await Promise.all([
			copyFile('.vite-build/main.js', 'main.js'),
			copyFile('.vite-build/styles.css', 'styles.css'),
		]);
	},
};

export default defineConfig(({ mode }) => ({
	plugins: [svelte(), copyObsidianArtifacts],
	build: {
		lib: {
			entry: 'src/main.ts',
			formats: ['cjs'],
			fileName: () => 'main.js',
			cssFileName: 'styles',
		},
		outDir: '.vite-build',
		emptyOutDir: true,
		copyPublicDir: false,
		minify: mode === 'production',
		sourcemap: mode === 'development' ? 'inline' : false,
		rollupOptions: {
			external,
			output: { exports: 'named' },
		},
	},
}));
