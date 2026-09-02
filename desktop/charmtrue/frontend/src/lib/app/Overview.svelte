<script lang="ts">
    import { Activity, Database, HardDrive, Server } from '@lucide/svelte';
    import { Button } from '$lib/components/ui/button';
    import * as Card from '$lib/components/ui/card';
    import { Progress } from '$lib/components/ui/progress';
    import type { ConnectionInfo } from '../../../bindings/github.com/ironpark/toolz/desktop/charmtrue';
    import { formatBytes } from './format'; import { getAppContext } from './context.svelte'; import type { View } from './types';
    let { connection, onnavigate }: { connection: ConnectionInfo | null; onnavigate: (view: View) => void } = $props();
    const connected = $derived(connection?.connected === true); const system = $derived(connection?.system); const app = getAppContext();
    const pools = $derived(app.storage?.pools ?? []); const total = $derived(app.storage?.totalSize ?? 0); const used = $derived(app.storage?.totalAllocated ?? 0); const available = $derived(app.storage?.totalFree ?? 0);
    const healthy = $derived(pools.length > 0 && pools.every((pool) => pool.healthy)); const runningServices = $derived((app.systemManagement?.services ?? []).filter((service) => service.state === 'RUNNING').length);
</script>

<section class="space-y-6">
    <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4" aria-label="시스템 요약">
        <Card.Root><Card.Header class="flex flex-row items-center justify-between pb-2"><Card.Description>시스템</Card.Description><Server /></Card.Header><Card.Content><div class="text-2xl font-semibold">{system?.hostname || '연결되지 않음'}</div><p class="text-xs text-muted-foreground">{system?.version || '인스턴스를 추가하세요'}</p></Card.Content></Card.Root>
        <Card.Root><Card.Header class="flex flex-row items-center justify-between pb-2"><Card.Description>스토리지</Card.Description><Database /></Card.Header><Card.Content><div class="text-2xl font-semibold">{total ? formatBytes(total) : '—'}</div><p class="text-xs text-muted-foreground">{total ? `${pools.length}개 풀` : '풀 사용량'}</p></Card.Content></Card.Root>
        <Card.Root><Card.Header class="flex flex-row items-center justify-between pb-2"><Card.Description>가동 시간</Card.Description><Activity /></Card.Header><Card.Content><div class="text-2xl font-semibold">{system?.uptime || '—'}</div><p class="text-xs text-muted-foreground">{system?.model || '마지막 동기화 없음'}</p></Card.Content></Card.Root>
        <Card.Root><Card.Header class="flex flex-row items-center justify-between pb-2"><Card.Description>서비스</Card.Description><HardDrive /></Card.Header><Card.Content><div class="text-2xl font-semibold">{connected ? `${runningServices}개 실행` : '—'}</div><p class="text-xs text-muted-foreground">{app.systemManagement?.state || '시스템 상태'}</p></Card.Content></Card.Root>
    </div>
    <div class="grid gap-4 lg:grid-cols-2">
        <Card.Root><Card.Header class="flex flex-row items-center justify-between"><div><Card.Title>풀 사용량</Card.Title><Card.Description>전체 스토리지 풀 기준</Card.Description></div><Button variant="ghost" onclick={() => onnavigate('storage')}>전체 보기</Button></Card.Header><Card.Content class="space-y-4"><div class="text-3xl font-semibold">{total ? `${Math.round(used / total * 100)}%` : '—'}</div><Progress value={total ? used / total * 100 : 0} /><div class="flex justify-between text-sm text-muted-foreground"><span>사용 {formatBytes(used)}</span><span>가용 {formatBytes(available)}</span></div></Card.Content></Card.Root>
        <Card.Root><Card.Header><Card.Title>시스템 상태</Card.Title><Card.Description>주요 서비스 요약</Card.Description></Card.Header><Card.Content class="divide-y">{#each [['시스템 정보', connected ? '정상' : '—'], ['리소스', connected ? `${system?.cores} cores · ${formatBytes(system?.physmem)}` : '—'], ['스토리지 풀', healthy ? '정상' : pools.length ? '확인 필요' : '—'], ['실행 서비스', connected ? `${runningServices}개` : '—']] as row}<div class="flex items-center justify-between py-3 text-sm"><span>{row[0]}</span><span class="text-muted-foreground">{row[1]}</span></div>{/each}</Card.Content></Card.Root>
    </div>
</section>
