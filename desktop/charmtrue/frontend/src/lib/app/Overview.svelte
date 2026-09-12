<script lang="ts">
    import { onMount } from 'svelte';
    import { Activity, ArrowDown, ArrowUp, Cpu, Database, Network, Server } from '@lucide/svelte';
    import * as Alert from '$lib/components/ui/alert';
    import { Badge } from '$lib/components/ui/badge';
    import { Button } from '$lib/components/ui/button';
    import * as Card from '$lib/components/ui/card';
    import { Progress } from '$lib/components/ui/progress';
    import * as Table from '$lib/components/ui/table';
    import { TrueNASService, type ConnectionInfo, type OverviewLiveStats } from '../../../bindings/github.com/ironpark/toolz/desktop/charmtrue';
    import { formatBytes, formatTraffic, formatUptime } from './format';
    import { getAppContext } from './context.svelte';
    import type { View } from './types';

    let { connection, onnavigate }: { connection: ConnectionInfo | null; onnavigate: (view: View) => void } = $props();
    const app = getAppContext();
    const connected = $derived(connection?.connected === true);
    const system = $derived(connection?.system);
    const pools = $derived(app.storage?.pools ?? []);
    const total = $derived(app.storage?.totalSize ?? 0);
    const used = $derived(app.storage?.totalAllocated ?? 0);
    const free = $derived(app.storage?.totalFree ?? 0);
    const usage = $derived(total ? Math.min(100, used / total * 100) : 0);
    const runningServices = $derived((app.systemManagement?.services ?? []).filter(service => service.state === 'RUNNING').length);
    const unhealthyPools = $derived(pools.filter(pool => !pool.healthy).length);
    let now = $state(Date.now());
    let stats = $state<OverviewLiveStats | null>(null);
    let receivedAt = $state(0);
    let liveError = $state('');
    const fresh = $derived(receivedAt > 0 && now - receivedAt < 15000);
    const uptime = $derived(connected && fresh && stats?.uptimeSeconds != null
        ? stats.uptimeSeconds + Math.max(0, now - stats.uptimeSampledAt) / 1000 : null);
    const network = $derived(stats?.network ?? app.network?.summary);
    const interfaces = $derived([...new Set([...Object.keys(network?.ips ?? {}), ...(stats?.traffic ?? []).map(item => item.name)])].sort());

    onMount(() => {
        const timer = setInterval(() => now = Date.now(), 1000);
        return () => clearInterval(timer);
    });

    $effect(() => {
        const endpoint = connection?.endpoint;
        if (!connected || !endpoint) { stats = null; receivedAt = 0; return; }
        let disposed = false;
        let pending = false;
        async function refresh(): Promise<void> {
            if (pending || document.hidden) return;
            pending = true;
            try {
                const result = await TrueNASService.OverviewStats();
                if (!disposed) {
                    stats = result;
                    receivedAt = Date.now();
                    now = receivedAt;
                    liveError = '';
                }
            } catch (error) {
                if (!disposed) {
                    stats = null;
                    receivedAt = 0;
                    liveError = error instanceof Error ? error.message : String(error);
                }
            } finally { pending = false; }
        }
        void refresh();
        const timer = setInterval(() => void refresh(), 5000);
        const resume = () => { if (!document.hidden) void refresh(); };
        document.addEventListener('visibilitychange', resume);
        return () => { disposed = true; clearInterval(timer); document.removeEventListener('visibilitychange', resume); };
    });
</script>

<section class="space-y-6">
    {#if liveError}<Alert.Root variant="destructive"><Alert.Title>실시간 정보 조회 실패</Alert.Title><Alert.Description>{liveError}</Alert.Description></Alert.Root>{/if}
    <div class="grid gap-4 md:grid-cols-3" aria-label="시스템 요약">
        <Card.Root>
            <Card.Header class="flex flex-row items-center justify-between pb-2"><Card.Description>시스템</Card.Description><Server class="size-4 text-muted-foreground" /></Card.Header>
            <Card.Content class="space-y-3"><div><div class="break-all text-2xl font-semibold">{connected ? system?.hostname || 'TrueNAS' : '연결되지 않음'}</div><p class="mt-1 text-xs text-muted-foreground">{connected ? system?.version : '서버를 연결하세요'}</p></div>
                <div class="flex flex-wrap gap-2"><Badge variant="secondary">{connected ? app.systemManagement?.state || '상태 조회 중' : '미연결'}</Badge>{#if connected && app.systemManagement}<Badge variant="outline">서비스 {runningServices}개 실행</Badge>{/if}</div>
                {#if app.systemError}<p class="text-xs text-destructive">{app.systemError}</p>{/if}
            </Card.Content>
        </Card.Root>
        <Card.Root>
            <Card.Header class="flex flex-row items-center justify-between pb-2"><Card.Description>가동 시간</Card.Description><Activity class="size-4 text-muted-foreground" /></Card.Header>
            <Card.Content><div class="text-2xl font-semibold tabular-nums">{formatUptime(uptime)}</div><p class="mt-3 text-xs text-muted-foreground">{stats?.uptimeError || (connected ? '1초마다 갱신 · 서버 시간으로 주기적 보정' : '서버 연결 후 표시됩니다')}</p></Card.Content>
        </Card.Root>
        <Card.Root>
            <Card.Header class="flex flex-row items-center justify-between pb-2"><Card.Description>스토리지 사용량</Card.Description><Database class="size-4 text-muted-foreground" /></Card.Header>
            <Card.Content class="space-y-3"><div class="flex items-baseline justify-between gap-2"><span class="text-2xl font-semibold tabular-nums">{connected && total ? Math.round(usage) + '%' : '—'}</span><span class="text-xs text-muted-foreground">{connected && total ? formatBytes(used) + ' / ' + formatBytes(total) : ''}</span></div><Progress value={connected ? usage : 0} /><p class="text-xs text-muted-foreground">{connected && app.storage ? pools.length + '개 풀 · 가용 ' + (free === 0 ? '0 B' : formatBytes(free)) + (unhealthyPools ? ' · 확인 필요 ' + unhealthyPools + '개' : '') : '스토리지 정보 대기 중'}</p>{#if app.storageError}<p class="text-xs text-destructive">{app.storageError}</p>{/if}</Card.Content>
        </Card.Root>
    </div>
    <div class="grid items-start gap-4 xl:grid-cols-3">
        <Card.Root class="min-w-0 xl:col-span-2">
            <Card.Header class="flex flex-row items-start justify-between gap-3"><div><Card.Title class="flex items-center gap-2"><Network class="size-4" />네트워크 트래픽 / IP</Card.Title><Card.Description class="mt-2">인터페이스별 최신 송수신 속도 · 5초마다 갱신</Card.Description></div><Button variant="ghost" size="sm" onclick={() => onnavigate('network')}>네트워크 관리</Button></Card.Header>
            <Card.Content class="space-y-4">
                {#if stats?.trafficError}<p role="status" class="text-sm text-destructive">트래픽 조회 실패: {stats.trafficError}</p>{/if}
                {#if stats?.networkError || app.networkError && !stats}<p role="status" class="text-sm text-destructive">IP 조회 실패: {stats?.networkError || app.networkError}</p>{/if}
                {#if connected && interfaces.length}
                    <Table.Root><Table.Header><Table.Row><Table.Head>인터페이스 / IP 주소</Table.Head><Table.Head class="text-right"><span class="inline-flex items-center gap-1"><ArrowDown class="size-3" />수신</span></Table.Head><Table.Head class="text-right"><span class="inline-flex items-center gap-1"><ArrowUp class="size-3" />송신</span></Table.Head></Table.Row></Table.Header><Table.Body>
                        {#each interfaces as name (name)}
                            {@const traffic = stats?.traffic?.find(item => item.name === name)}
                            {@const ips = network?.ips?.[name]}
                            {@const addresses = [...(ips?.ipv4 ?? []), ...(ips?.ipv6 ?? [])]}
                            {@const sampleFresh = fresh && traffic && now / 1000 - traffic.sampledAt < 30}
                            <Table.Row><Table.Cell class="max-w-64"><p class="font-medium">{name}</p>{#each addresses as address}<p class="break-all whitespace-normal font-mono text-xs text-muted-foreground">{address}</p>{:else}<p class="text-xs text-muted-foreground">IP 주소 없음</p>{/each}</Table.Cell><Table.Cell class="text-right tabular-nums">{formatTraffic(sampleFresh ? traffic?.receivedKbps : null)}</Table.Cell><Table.Cell class="text-right tabular-nums">{formatTraffic(sampleFresh ? traffic?.sentKbps : null)}</Table.Cell></Table.Row>
                        {/each}
                    </Table.Body></Table.Root>
                    <p class="text-xs text-muted-foreground">—: 최신 측정값 없음. 가상 인터페이스와 물리 인터페이스는 트래픽이 중복될 수 있어 합산하지 않습니다.</p>
                {:else}<p class="py-8 text-center text-sm text-muted-foreground">{connected ? stats ? '표시할 네트워크 인터페이스가 없습니다.' : '네트워크 정보를 불러오는 중…' : '서버 연결 후 트래픽과 IP 주소가 표시됩니다.'}</p>{/if}
            </Card.Content>
        </Card.Root>
        <Card.Root>
            <Card.Header><Card.Title class="flex items-center gap-2"><Cpu class="size-4" />하드웨어 / 시스템 정보</Card.Title></Card.Header>
            <Card.Content><dl class="divide-y text-sm">
                <div class="space-y-1 py-3"><dt class="text-muted-foreground">CPU</dt><dd class="break-words font-medium">{connected ? system?.model || '—' : '—'}</dd><dd class="text-xs text-muted-foreground">{connected && system?.cores ? system.cores + ' 코어' : ''}</dd></div>
                <div class="flex justify-between gap-3 py-3"><dt class="text-muted-foreground">설치된 메모리</dt><dd>{connected ? formatBytes(system?.physmem) : '—'}</dd></div>
                <div class="space-y-1 py-3"><dt class="text-muted-foreground">연결 주소</dt><dd class="break-all font-mono text-xs">{connected ? connection?.endpoint : '—'}</dd></div>
                <div class="space-y-1 py-3"><dt class="text-muted-foreground">DNS</dt><dd class="break-all font-mono text-xs">{connected ? network?.nameServers?.join(', ') || '—' : '—'}</dd></div>
                <div class="space-y-1 py-3"><dt class="text-muted-foreground">기본 경로</dt><dd class="break-all font-mono text-xs">{connected ? network?.defaultRoutes?.join(', ') || '—' : '—'}</dd></div>
            </dl></Card.Content>
            <Card.Footer class="flex-wrap gap-2"><Button variant="outline" size="sm" onclick={() => onnavigate('system')}>시스템 관리</Button><Button variant="outline" size="sm" onclick={() => onnavigate('storage')}>저장소 관리</Button></Card.Footer>
        </Card.Root>
    </div>
</section>
