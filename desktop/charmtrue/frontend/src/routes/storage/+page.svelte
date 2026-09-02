<script lang="ts">
    import PlaceholderView from '$lib/app/PlaceholderView.svelte';
    import StorageDashboard from '$lib/app/StorageDashboard.svelte';
    import { getAppContext } from '$lib/app/context.svelte';
    const app = getAppContext();
</script>

{#if app.connection?.connected}<StorageDashboard />{:else}<PlaceholderView title="저장소" description="스토리지 풀과 디스크를 관리하려면 TrueNAS 시스템을 먼저 연결하세요." onconnect={() => app.openConnectModal()} />{/if}
