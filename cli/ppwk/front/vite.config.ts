import tailwindcss from '@tailwindcss/vite';
import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

// 빌드 산출물은 Go 패키지 안으로 바로 떨어뜨린다.
//
// go:embed 는 상위 디렉터리를 볼 수 없다. 산출물을 front/build 에 두면
// 그것을 다시 복사하는 단계가 필요하고, 그 단계는 언젠가 빠뜨린다.
const outDir = '../internal/web/dist';

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit({
			compilerOptions: {
				// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
				runes: ({ filename }) => filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},

			// SPA 로 만든다. 데이터는 전부 로컬 API 에서 오고 SEO 대상이
			// 아니므로, 서버 렌더링은 얻는 것 없이 빌드만 복잡하게 한다.
			adapter: adapter({
				pages: outDir,
				assets: outDir,
				fallback: 'index.html',
				precompress: false,
				strict: false
			})
		})
	]
});
