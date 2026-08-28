<script lang="ts">
	let { greeting: initialGreeting, onSave }: {
		greeting: string;
		onSave: (greeting: string) => Promise<void>;
	} = $props();
	let greeting = $derived(initialGreeting);
	let saved = $state(false);

	async function save(): Promise<void> {
		await onSave(greeting.trim());
		saved = true;
		window.setTimeout(() => (saved = false), 1500);
	}
</script>

<div class="svelte-vite-settings">
	<h2>Svelte Vite plugin</h2>
	<p class="setting-item-description">This settings screen is a Svelte component.</p>
	<label for="svelte-vite-greeting">Greeting</label>
	<input id="svelte-vite-greeting" type="text" bind:value={greeting} />
	<div class="svelte-vite-settings__actions">
		<button class="mod-cta" type="button" onclick={save}>Save</button>
		{#if saved}<span aria-live="polite">Saved</span>{/if}
	</div>
</div>
