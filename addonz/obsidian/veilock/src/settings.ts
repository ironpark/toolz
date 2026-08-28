import { App, PluginSettingTab } from 'obsidian';
import { mount, unmount } from 'svelte';
import type SvelteVitePlugin from './main';
import SettingsPanel from './ui/SettingsPanel.svelte';

export interface PluginSettings { greeting: string; }

export const DEFAULT_SETTINGS: PluginSettings = { greeting: 'Hello from Svelte 5!' };

export class SvelteSettingTab extends PluginSettingTab {
	private component: ReturnType<typeof mount> | undefined;

	constructor(app: App, private readonly plugin: SvelteVitePlugin) {
		super(app, plugin);
	}

	display(): void {
		this.containerEl.empty();
		this.component = mount(SettingsPanel, {
			target: this.containerEl,
			props: {
				greeting: this.plugin.settings.greeting,
				onSave: async (greeting: string) => {
					this.plugin.settings.greeting = greeting;
					await this.plugin.saveSettings();
				},
			},
		});
	}

	hide(): void {
		if (this.component) {
			void unmount(this.component);
			this.component = undefined;
		}
	}
}
