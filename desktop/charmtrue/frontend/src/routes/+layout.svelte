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
    const refreshing = $derived(app.isRefreshing(activeView));

    onMount(() => app.restoreConnection());
</script>

<div class="grid h-dvh overflow-hidden grid-cols-[220px_minmax(0,1fr)] bg-sidebar max-lg:grid-cols-[72px_minmax(0,1fr)]">
    <Sidebar {activeView} connection={app.connection} onconnect={() => app.openConnectModal()} />
    <div class="flex min-h-0 min-w-0 flex-col">
        <Topbar title={pageTitles[activeView]} connected={app.connection?.connected === true} {refreshing} onrefresh={() => app.refreshView(activeView)} />
        <main class="mb-3 mr-3 min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-none rounded-xl bg-background px-8 py-6 max-sm:mb-2 max-sm:mr-2 max-sm:px-4">
            {@render children()}
        </main>
    </div>
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
