<script lang="ts">
    import { onMount } from 'svelte';
    import { ArrowDown, ArrowUp } from '@lucide/svelte';
    import * as Card from '$lib/components/ui/card';
    import * as NativeSelect from '$lib/components/ui/native-select';
    import { TrueNASService } from '../../../bindings/github.com/ironpark/toolz/desktop/charmtrue';
    import { getAppContext } from './context.svelte';
    import { formatTraffic } from './format';
    import { appendTraffic, trafficPath, trafficSummary, type TrafficHistory } from './network-traffic';

    const app = getAppContext();
    let { history = $bindable({}), selected = $bindable('') }: { history?: TrafficHistory; selected?: string } = $props();
    let minutes = $state('5');
    let now = $state(Date.now() / 1000), error = $state(''), loading = $state(true);
    const names = $derived([...new Set([...(app.network?.interfaces ?? []).map(item => item.name || item.id), ...Object.keys(history)])].sort());
    const active = $derived(names.includes(selected) ? selected : names[0] ?? '');
    const points = $derived((history[active] ?? []).filter(point => point.time >= now - Number(minutes) * 60));
    const latest = $derived(points.at(-1));
    const fresh = $derived(!error && latest && now - latest.time <= 30);
    const rx = $derived(trafficSummary(points, 'rx')), tx = $derived(trafficSummary(points, 'tx'));
    const ceiling = $derived(Math.max(1, rx.peak ?? 0, tx.peak ?? 0) * 1.15);
    const start = $derived(now - Number(minutes) * 60);
    const timeLabel = (time: number) => new Date(time * 1000).toLocaleTimeString('ko-KR', { hour: '2-digit', minute: '2-digit', second: '2-digit' });

    onMount(() => { const timer = setInterval(() => now = Date.now() / 1000, 1000); return () => clearInterval(timer); });
    $effect(() => {
        const version = app.connectionVersion;
        const connected = app.connection?.connected;
        history = {}; selected = ''; error = ''; loading = !!connected;
        if (!connected) return;
        let disposed = false, pending = false;
        async function refresh(): Promise<void> {
            if (pending || document.hidden) return;
            pending = true;
            try {
                const stats = await TrueNASService.OverviewStats();
                if (disposed || version !== app.connectionVersion) return;
                now = Date.now() / 1000;
                error = stats.trafficError;
                if (!error) history = appendTraffic(history, stats.traffic ?? [], now);
            } catch (e) { if (!disposed && version === app.connectionVersion) error = e instanceof Error ? e.message : String(e); }
            finally { pending = false; if (!disposed) loading = false; }
        }
        void refresh();
        const timer = setInterval(() => void refresh(), 5000);
        const visible = () => { if (!document.hidden) void refresh(); };
        document.addEventListener('visibilitychange', visible);
        return () => { disposed = true; clearInterval(timer); document.removeEventListener('visibilitychange', visible); };
    });
</script>

<Card.Root>
    <Card.Header class="flex flex-wrap items-start justify-between gap-3">
        <div><Card.Title>송·수신 트래픽</Card.Title><Card.Description class="mt-2">5초마다 갱신 · 이 페이지를 연 뒤 수집한 최근 {minutes}분 기록</Card.Description></div>
        <div class="flex flex-wrap gap-2"><NativeSelect.Root aria-label="트래픽 인터페이스" value={active} onchange={event => selected = event.currentTarget.value}>{#each names as name}<option value={name}>{name}</option>{/each}</NativeSelect.Root><NativeSelect.Root aria-label="그래프 표시 기간" bind:value={minutes}><option value="5">최근 5분</option><option value="15">최근 15분</option></NativeSelect.Root></div>
    </Card.Header>
    <Card.Content class="space-y-4">
        {#if error}<p role="alert" class="text-sm text-destructive">트래픽 조회 실패: {error} · 자동으로 재시도합니다.</p>{/if}
        <div class="grid gap-3 sm:grid-cols-2">
            <div class="rounded-lg border p-4"><p class="flex items-center gap-2 text-sm text-muted-foreground"><ArrowDown class="size-4 text-sky-500" />수신 (서버로 들어옴)</p><p class="mt-2 text-2xl font-semibold tabular-nums">{formatTraffic(fresh ? latest?.rx : null)}</p><p class="mt-2 text-xs text-muted-foreground">평균 {formatTraffic(rx.average)} · 최대 {formatTraffic(rx.peak)}</p></div>
            <div class="rounded-lg border p-4"><p class="flex items-center gap-2 text-sm text-muted-foreground"><ArrowUp class="size-4 text-orange-500" />송신 (서버에서 나감)</p><p class="mt-2 text-2xl font-semibold tabular-nums">{formatTraffic(fresh ? latest?.tx : null)}</p><p class="mt-2 text-xs text-muted-foreground">평균 {formatTraffic(tx.average)} · 최대 {formatTraffic(tx.peak)}</p></div>
        </div>
        <div class="rounded-lg border p-3">
            <div class="flex flex-wrap justify-between gap-2 text-xs text-muted-foreground"><span>{active || '인터페이스 없음'}</span><span>실선: 수신 · 점선: 송신</span></div>
            {#if points.length}
                <svg viewBox="0 0 800 224" class="mt-2 w-full" role="img" aria-label={`${active} 최근 ${minutes}분 송수신 속도 그래프`}>
                    {#each [0, 0.5, 1] as fraction}
                        <line x1="64" x2="780" y1={190 - fraction * 166} y2={190 - fraction * 166} stroke="currentColor" class="text-border" />
                        <text x="58" y={194 - fraction * 166} text-anchor="end" fill="currentColor" class="text-muted-foreground" font-size="10">{formatTraffic(ceiling * fraction)}</text>
                    {/each}
                    <path d={trafficPath(points, 'rx', start, now, ceiling)} fill="none" stroke="currentColor" class="text-sky-500" stroke-width="2" />
                    <path d={trafficPath(points, 'tx', start, now, ceiling)} fill="none" stroke="currentColor" class="text-orange-500" stroke-width="2" stroke-dasharray="5 3" />
                    {#each points as point}<circle cx={64 + (point.time - start) / (now - start) * 716} cy={190 - point.rx / ceiling * 166} r="2" class="text-sky-500" fill="currentColor"><title>{timeLabel(point.time)} 수신 {formatTraffic(point.rx)} / 송신 {formatTraffic(point.tx)}</title></circle><circle cx={64 + (point.time - start) / (now - start) * 716} cy={190 - point.tx / ceiling * 166} r="2" class="text-orange-500" fill="currentColor"><title>{timeLabel(point.time)} 송신 {formatTraffic(point.tx)} / 수신 {formatTraffic(point.rx)}</title></circle>{/each}
                    <text x="64" y="216" fill="currentColor" class="text-muted-foreground" font-size="11">{timeLabel(start)}</text><text x="780" y="216" text-anchor="end" fill="currentColor" class="text-muted-foreground" font-size="11">{timeLabel(now)}</text>
                </svg>
                {#if !fresh}<p role="status" class="mt-2 text-xs text-muted-foreground">최신 샘플이 없습니다. 마지막 기록: {latest ? timeLabel(latest.time) : '—'}</p>{/if}
            {:else}<div class="grid min-h-48 place-items-center text-sm text-muted-foreground">{loading ? '트래픽을 불러오는 중…' : '아직 수집된 트래픽이 없습니다. 새 샘플을 기다리는 중입니다.'}</div>{/if}
        </div>
        <details class="text-xs text-muted-foreground"><summary class="cursor-pointer">통계 기준 안내</summary><p class="mt-2 leading-relaxed">평균·최대는 선택 기간에 수집한 샘플 기준입니다. 누락 구간은 0으로 계산하지 않습니다. 가상·물리 인터페이스의 중복 집계를 피하기 위해 전체 합산은 표시하지 않습니다.</p></details>
    </Card.Content>
</Card.Root>
