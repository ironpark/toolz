<script lang="ts">
    import PlaceholderView from '$lib/app/PlaceholderView.svelte';
    import SharingManager from '$lib/app/SharingManager.svelte';
    import { getAppContext } from '$lib/app/context.svelte';
    const app=getAppContext();
</script>

{#if app.connection?.connected}<SharingManager/>{:else}<PlaceholderView title="공유 서비스" description="SMB, NFS와 Rsync 공유를 관리하려면 TrueNAS 시스템을 먼저 연결하세요." onconnect={()=>app.openConnectModal()}/>{/if}
