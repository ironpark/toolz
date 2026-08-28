import { App, Modal } from 'obsidian';
import { mount, unmount } from 'svelte';
import ModalContent from './ModalContent.svelte';

export class SvelteModal extends Modal {
	private component: ReturnType<typeof mount> | undefined;

	constructor(app: App, private readonly greeting: string) {
		super(app);
	}

	onOpen(): void {
		this.component = mount(ModalContent, {
			target: this.contentEl,
			props: { greeting: this.greeting, onClose: () => this.close() },
		});
	}

	onClose(): void {
		if (this.component) {
			void unmount(this.component);
			this.component = undefined;
		}
		this.contentEl.empty();
	}
}
