import { Plugin } from 'obsidian';
import { DEFAULT_SETTINGS, type PluginSettings, SvelteSettingTab } from './settings';
import './styles.css';
import { SvelteModal } from './ui/SvelteModal';

export default class SvelteVitePlugin extends Plugin {
	settings!: PluginSettings;

	async onload(): Promise<void> {
		await this.loadSettings();
		this.addRibbonIcon('sparkles', 'Open Svelte view', () => {
			new SvelteModal(this.app, this.settings.greeting).open();
		});
		this.addCommand({
			id: 'open-svelte-modal',
			name: 'Open Svelte modal',
			callback: () => new SvelteModal(this.app, this.settings.greeting).open(),
		});
		this.addSettingTab(new SvelteSettingTab(this.app, this));
	}

	private async loadSettings(): Promise<void> {
		this.settings = Object.assign({}, DEFAULT_SETTINGS, (await this.loadData()) as Partial<PluginSettings>);
	}

	async saveSettings(): Promise<void> {
		await this.saveData(this.settings);
	}
}
