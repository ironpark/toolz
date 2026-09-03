<script lang="ts">
    import { ChevronRight, Database, HardDrive, Layers3, Network, Server, Settings, Share2, Users } from '@lucide/svelte';
    import { Button } from '$lib/components/ui/button';
    import type { ConnectionInfo } from '../../../bindings/github.com/ironpark/toolz/desktop/charmtrue';
    import { navigationItems, type View } from './types';

    let { activeView, connection, onconnect }: { activeView: View; connection: ConnectionInfo | null; onconnect: () => void } = $props();
    const connected = $derived(connection?.connected === true);
    const icons = { overview: Layers3, storage: HardDrive, datasets: Database, services: Share2, network: Network, identity: Users, system: Server } as const;
</script>

<aside class="flex h-dvh flex-col bg-sidebar text-sidebar-foreground">
    <a class="flex h-16 items-center gap-3 px-5 font-semibold max-lg:justify-center max-lg:px-0" href="/" aria-label="CharmTrue 홈"><span class="grid size-8 place-items-center rounded-lg bg-primary text-primary-foreground"><Layers3 class="size-4" /></span><span class="max-lg:hidden">CharmTrue</span></a>
    <nav class="flex-1 space-y-1 p-3" aria-label="주 메뉴">
        {#each navigationItems as item (item.id)}
            {@const NavIcon = icons[item.id]}
            <a class={`flex h-9 items-center gap-3 rounded-md px-3 text-sm transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground max-lg:justify-center ${activeView === item.id ? 'bg-sidebar-accent font-medium text-sidebar-accent-foreground' : 'text-muted-foreground'}`} href={item.href} aria-current={activeView === item.id ? 'page' : undefined}><NavIcon class="size-4" /><span class="max-lg:hidden">{item.label}</span></a>
        {/each}
    </nav>
    <div class="space-y-3 border-t p-3">
        <Button variant="outline" class="h-auto w-full justify-start gap-3 p-3 max-lg:justify-center" onclick={onconnect}><span class={`size-2 rounded-full ${connected ? 'bg-emerald-500' : 'bg-muted-foreground'}`}></span><span class="min-w-0 flex-1 text-left max-lg:hidden"><strong class="block truncate text-sm">{connection?.system.hostname || '시스템 연결'}</strong><small class="block truncate text-xs font-normal text-muted-foreground">{connection?.endpoint || '등록된 서버 없음'}</small></span><ChevronRight class="size-4 max-lg:hidden" /></Button>
        <Button variant="ghost" class="w-full justify-start gap-3 max-lg:justify-center" href="/system"><Settings class="size-4" /><span class="max-lg:hidden">설정</span></Button>
    </div>
</aside>
