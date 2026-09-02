<script lang="ts">
    import { Bell, Plus, Settings } from '@lucide/svelte';
    import { Button } from '$lib/components/ui/button';
    let { title, connected = false, onconnect }: { title: string; connected?: boolean; onconnect: () => void } = $props();
</script>

<header class="mb-8 flex h-10 items-center justify-between"><div><p class="text-xs text-muted-foreground">TrueNAS Workspace</p><h1 class="text-lg font-semibold">{title}</h1></div><div class="flex items-center gap-2"><Button variant="ghost" size="icon" aria-label="알림" disabled><Bell /></Button><Button variant={connected ? 'outline' : 'default'} onclick={onconnect}>{#if connected}<Settings />연결 설정{:else}<Plus />시스템 추가{/if}</Button></div></header>
