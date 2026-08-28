# Deref

문서 속 참조 토큰을 저장된 실제 값으로 동적으로 렌더링하는 Obsidian 플러그인입니다.

`${NAME}`처럼 작성하면 원본 문서는 그대로 유지하면서, 화면에는 연결된 값이 표시됩니다.

## Obsidian Svelte + Vite Plugin

A production-ready Obsidian community plugin boilerplate built with Vite 8, Svelte 5, and TypeScript. It keeps the release conventions of the official [Obsidian sample plugin](https://github.com/obsidianmd/obsidian-sample-plugin) while replacing esbuild and imperative UI examples with Vite and Svelte components.

## Included

- Vite library build that emits Obsidian-compatible `main.js`
- Svelte 5 runes and the official Vite plugin
- Svelte-powered modal and settings tab examples
- Clean Svelte mount/unmount handling for Obsidian lifecycle events
- Generated `styles.css` at the plugin root
- Type checking with `svelte-check`
- ESLint 10 with rules for Obsidian and Svelte
- Mobile-compatible defaults (no Electron or Node runtime APIs)

## Requirements

- Node.js 20.19+, 22.12+, or a newer supported release
- pnpm 11+
- Obsidian 1.0+

## Start developing

1. Rename the plugin in `manifest.json` and `package.json`. Keep the manifest `id` stable after the first release.
2. Run `pnpm install`.
3. Run `pnpm run dev` to start the watch build.
4. Copy or clone the project into `<Vault>/.obsidian/plugins/<manifest-id>/`, then reload Obsidian and enable it under **Settings → Community plugins**.

The watch command writes `main.js` and `styles.css` directly to the project root, where Obsidian expects them.

## Commands

| Command          | Purpose                                           |
| ---------------- | ------------------------------------------------- |
| `pnpm run dev`   | Build once and rebuild when source files change   |
| `pnpm run check` | Type-check TypeScript and Svelte components       |
| `pnpm run lint`  | Run Obsidian and Svelte lint rules                |
| `pnpm run build` | Type-check and create a minified production build |

## Project structure

```text
src/
  main.ts                  Plugin lifecycle and registrations
  settings.ts              Settings model and Svelte settings mount
  styles.css               Plugin styles bundled to root styles.css
  ui/
    ModalContent.svelte    Example Svelte modal content
    SettingsPanel.svelte   Example Svelte settings interface
    SvelteModal.ts         Obsidian-to-Svelte lifecycle adapter
vite.config.ts             Obsidian-compatible Vite library build
svelte.config.js           Svelte compiler configuration
manifest.json              Obsidian plugin metadata
versions.json              Plugin-to-Obsidian compatibility map
```

## Build output

Vite bundles Svelte and browser-compatible runtime dependencies. Obsidian, CodeMirror, Electron, Lezer, and Node built-ins are externalized because Obsidian provides them at runtime. The release artifacts are `main.js`, `manifest.json`, and `styles.css`.

Generated files are intentionally ignored by Git.

## Release

Update `minAppVersion` in `manifest.json` if needed, then run `pnpm version patch`, `pnpm version minor`, or `pnpm version major`.

Release tags must be the exact version number without a `v` prefix.

## License

0BSD, matching the official Obsidian sample plugin.
