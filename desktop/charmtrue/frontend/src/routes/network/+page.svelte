<script lang="ts">
    import NetworkManager from '$lib/app/NetworkManager.svelte';
    import PlaceholderView from '$lib/app/PlaceholderView.svelte';
    import { getAppContext } from '$lib/app/context.svelte';

    const app = getAppContext();
</script>

{#if app.connection?.connected}
    <NetworkManager />
{:else}
    <PlaceholderView title="네트워크" description="네트워크 인터페이스와 라우팅을 관리하려면 TrueNAS 시스템을 먼저 연결하세요." onconnect={() => app.openConnectModal()} />
{/if}
