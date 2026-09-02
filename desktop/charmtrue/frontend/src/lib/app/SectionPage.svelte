<script lang="ts">
    import PlaceholderView from './PlaceholderView.svelte';
    import { getAppContext } from './context.svelte';
    import { navigationItems, type View } from './types';

    let { view }: { view: Exclude<View, 'overview'> } = $props();
    const app = getAppContext();
    const item = $derived(navigationItems.find(({ id }) => id === view)!);
</script>

<PlaceholderView
    title={item.label}
    description={`${item.label} 관리 기능을 사용하려면 TrueNAS 시스템을 먼저 연결하세요.`}
    onconnect={() => app.openConnectModal()}
/>
