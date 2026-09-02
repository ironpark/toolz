<script lang="ts">
    import { page } from '$app/state';
    import { onMount } from 'svelte';
    import type { Snippet } from 'svelte';
    import ConnectModal from '$lib/app/ConnectModal.svelte';
    import Sidebar from '$lib/app/Sidebar.svelte';
    import Topbar from '$lib/app/Topbar.svelte';
    import { createAppContext } from '$lib/app/context.svelte';
    import { pageTitles, viewFromPath } from '$lib/app/types';
    import '$lib/styles/app.css';

    let { children }: { children: Snippet } = $props();
    const app = createAppContext();
    const activeView = $derived(viewFromPath(page.url.pathname));

    onMount(() => app.restoreConnection());
</script>

<div class="grid h-dvh overflow-hidden grid-cols-[240px_minmax(0,1fr)] bg-background max-lg:grid-cols-[72px_minmax(0,1fr)]">
    <Sidebar {activeView} connection={app.connection} onconnect={() => app.openConnectModal()} />
    <main class="min-w-0 overflow-y-auto overscroll-none px-8 py-6 max-sm:px-4">
        <Topbar title={pageTitles[activeView]} connected={app.connection?.connected === true} onconnect={() => app.openConnectModal()} />
        {@render children()}
    </main>
</div>

{#if app.modalOpen}
    <ConnectModal
        loading={app.loading}
        message={app.message}
        savedServers={app.savedServers}
        savedServersError={app.savedServersError}
        onclose={() => app.closeConnectModal()}
        onconnect={(...args) => app.connect(...args)}
        onconnectsaved={(id) => app.connectSavedServer(id)}
        ondelete={(id) => app.deleteSavedServer(id)}
    />
{/if}
