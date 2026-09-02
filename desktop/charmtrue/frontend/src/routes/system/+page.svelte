<script lang="ts">
    import PlaceholderView from '$lib/app/PlaceholderView.svelte';
    import SystemManager from '$lib/app/SystemManager.svelte';
    import { getAppContext } from '$lib/app/context.svelte';

    const app = getAppContext();
</script>

{#if app.connection?.connected}
    <SystemManager />
{:else}
    <PlaceholderView title="시스템" description="서비스, 업데이트와 전원 작업을 관리하려면 TrueNAS 시스템을 먼저 연결하세요." onconnect={() => app.openConnectModal()} />
{/if}
