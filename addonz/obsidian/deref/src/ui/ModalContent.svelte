<script lang="ts">
	let { greeting, onClose }: { greeting: string; onClose: () => void } = $props();
</script>

<section class="svelte-vite-modal">
	<span class="svelte-vite-modal__badge">Svelte 5 + Vite</span>
	<h2>{greeting}</h2>
	<p>This interface is rendered by a Svelte component inside an Obsidian modal.</p>
	<button class="mod-cta" type="button" onclick={onClose}>Close</button>
</section>
