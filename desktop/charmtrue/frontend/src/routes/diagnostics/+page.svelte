<script lang="ts">
    import DiagnosticsManager from '$lib/app/DiagnosticsManager.svelte';
    import PlaceholderView from '$lib/app/PlaceholderView.svelte';
    import { getAppContext } from '$lib/app/context.svelte';
    const app = getAppContext();
</script>

{#if app.connection?.connected}
    {#key app.connection.endpoint}<DiagnosticsManager endpoint={app.connection.endpoint} />{/key}
{:else}
    <PlaceholderView title="진단 및 해결" description="시스템 점검과 설정 최적화를 실행하려면 TrueNAS 시스템을 먼저 연결하세요." onconnect={() => app.openConnectModal()} />
{/if}
