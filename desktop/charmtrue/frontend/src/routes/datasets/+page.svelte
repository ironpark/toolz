<script lang="ts">
    import PlaceholderView from '$lib/app/PlaceholderView.svelte';
    import DatasetBrowser from '$lib/app/DatasetBrowser.svelte';
    import { getAppContext } from '$lib/app/context.svelte';
    const app = getAppContext();
</script>

{#if app.connection?.connected}<DatasetBrowser />{:else}<PlaceholderView title="데이터셋" description="데이터셋과 스냅샷을 관리하려면 TrueNAS 시스템을 먼저 연결하세요." onconnect={() => app.openConnectModal()} />{/if}
