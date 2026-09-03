<script lang="ts">
    import { RefreshCw } from '@lucide/svelte';
    import { Button } from '$lib/components/ui/button';
    let { title, connected = false, refreshing = false, onrefresh }: { title: string; connected?: boolean; refreshing?: boolean; onrefresh: () => void } = $props();
</script>

<header class="flex h-16 shrink-0 items-center justify-between bg-sidebar px-6 text-sidebar-foreground max-sm:px-4">
    <h1 class="text-lg font-semibold leading-tight">{title}</h1>
    <Button variant="ghost" size="icon" aria-label={`${title} 새로고침`} title="새로고침" disabled={!connected || refreshing} onclick={onrefresh}>
        <RefreshCw class={refreshing ? 'animate-spin' : ''} />
    </Button>
</header>
