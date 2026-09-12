<script lang="ts">
    import { onMount } from 'svelte';
    import { Activity, RefreshCw, WandSparkles } from '@lucide/svelte';
    import * as Alert from '$lib/components/ui/alert';
    import { Badge } from '$lib/components/ui/badge';
    import { Button } from '$lib/components/ui/button';
    import * as Card from '$lib/components/ui/card';
    import { Spinner } from '$lib/components/ui/spinner';
    import ConfirmActionDialog from './ConfirmActionDialog.svelte';
    import { TrueNASService } from '../../../bindings/github.com/ironpark/toolz/desktop/charmtrue';
    import { buildDiagnosticChecks, type DiagnosticCheck, type CheckCategory, type CheckStatus } from './diagnostics';

    let { endpoint }: { endpoint: string } = $props();
    let active = true;

    let checks = $state<DiagnosticCheck[]>([]);
    let running = $state(false);
    let applying = $state(false);
    let checkedAt = $state('');
    let selected = $state<string[]>([]);
    let filter = $state<'all' | CheckCategory>('all');
    let confirmOpen = $state(false);
    let outcomes = $state<{ title: string; success: boolean; detail: string }[]>([]);
    const labels: Record<CheckStatus, string> = { pass: '정상', warning: '주의', critical: '위험', unknown: '확인 불가', info: '안내' };
    const categories = [{ id: 'all', label: '전체' }, { id: 'system', label: '시스템 점검' }, { id: 'tuning', label: '최적화 / 튜닝' }, { id: 'settings', label: '기타 설정 점검' }] as const;
    const visible = $derived(checks.filter(check => filter === 'all' || check.category === filter));
    const pending = $derived(checks.filter(check => selected.includes(check.id) && check.action));
    const problems = $derived(checks.filter(check => check.status === 'warning' || check.status === 'critical').length);
    const unknown = $derived(checks.filter(check => check.status === 'unknown').length);
    const confirmation = $derived(pending.map(check => `${check.target}: ${check.change}`).join('\n') + '\n선택한 속성을 변경합니다. 설정을 상속하는 하위 데이터셋에도 영향을 줄 수 있습니다. 항목별로 적용되며 일부가 실패해도 앞서 완료한 변경은 유지됩니다. 원래 값으로 되돌리려면 데이터셋 설정에서 수정하세요.');

    onMount(() => { void run(); return () => { active = false; }; });

    async function run(): Promise<void> {
        if (running) return;
        running = true;
        selected = [];
        try {
            const results = await Promise.allSettled([
                TrueNASService.StorageOverview(), TrueNASService.SystemManagementOverview(),
                TrueNASService.NetworkOverview(), TrueNASService.CertificateOverview(), TrueNASService.HTTPSRedirect(),
            ]);
            if (!active) return;
            checks = buildDiagnosticChecks(results);
            checkedAt = new Date().toLocaleString();
        } finally { running = false; }
    }

    function select(id: string, checked: boolean): void {
        selected = checked ? [...selected, id] : selected.filter(value => value !== id);
    }

    async function apply(): Promise<void> {
        if (applying || !pending.length) return;
        const jobs = [...pending];
        applying = true;
        outcomes = [];
        try {
            for (const check of jobs) {
                if (!active) break;
                try {
                    await TrueNASService.ApplyDiagnosticTuning(check.target!, check.action!, endpoint);
                    outcomes = [...outcomes, { title: check.target!, success: true, detail: `${check.change} 적용 완료` }];
                } catch (e) {
                    outcomes = [...outcomes, { title: check.target!, success: false, detail: e instanceof Error ? e.message : String(e) }];
                }
            }
            if (active) await run();
        } finally { applying = false; confirmOpen = false; }
    }
</script>

<section class="space-y-6">
    <div class="flex flex-wrap items-start justify-between gap-4">
        <div><h2 class="text-lg font-semibold">문제 진단 및 해결</h2><p class="mt-1 text-sm text-muted-foreground">서버 상태와 설정을 점검하고 필요한 조치를 선택하세요. 점검 자체는 설정을 변경하지 않습니다.</p><p class="mt-2 text-xs text-muted-foreground">{checkedAt ? `마지막 점검: ${checkedAt}` : '점검 준비 중'} · 하드웨어 정밀 검사와 실제 네트워크 통신 검사는 포함하지 않습니다.</p></div>
        <Button variant="outline" disabled={running || applying} onclick={() => { outcomes = []; void run(); }}>{#if running}<Spinner />{:else}<RefreshCw />{/if}다시 점검</Button>
    </div>
    <div class="grid gap-4 sm:grid-cols-3">
        {#each [['점검 항목', checks.length], ['확인 필요', problems], ['확인 불가', unknown]] as [label, count]}
            <Card.Root><Card.Header><Card.Description>{label}</Card.Description></Card.Header><Card.Content class="text-2xl font-semibold">{running ? '…' : count}</Card.Content></Card.Root>
        {/each}
    </div>
    {#if outcomes.length}<div class="space-y-2" aria-live="polite">{#each outcomes as result}<Alert.Root variant={result.success ? 'default' : 'destructive'}><Alert.Title>{result.title} · {result.success ? '적용 완료' : '적용 실패'}</Alert.Title><Alert.Description>{result.detail}</Alert.Description></Alert.Root>{/each}</div>{/if}
    <Card.Root>
        <Card.Header><Card.Title class="flex items-center gap-2"><WandSparkles class="size-4" />선택 항목 자동 최적화 / 복구</Card.Title><Card.Description>점검 결과의 체크박스로 대상을 선택하세요. 압축 OFF → LZ4, 동기 쓰기 DISABLED → STANDARD 조치를 지원합니다. 성능 개선 효과는 작업 특성에 따라 다릅니다.</Card.Description></Card.Header>
        <Card.Content class="space-y-2">{#each pending as check}<p class="break-all text-sm">{check.target} · {check.change}</p>{:else}<p class="text-sm text-muted-foreground">선택한 항목이 없습니다.</p>{/each}</Card.Content>
        <Card.Footer><Button disabled={running || applying || !pending.length} onclick={() => confirmOpen = true}>{#if applying}<Spinner />{/if}선택한 {pending.length}개 조치 검토 및 적용</Button></Card.Footer>
    </Card.Root>
    <div class="flex flex-wrap gap-2" aria-label="점검 분류">{#each categories as category}<Button size="sm" variant={filter === category.id ? 'default' : 'outline'} aria-pressed={filter === category.id} onclick={() => filter = category.id}>{category.label}</Button>{/each}</div>
    {#if running}<div role="status" class="flex items-center gap-2 py-8 text-sm text-muted-foreground"><Spinner />시스템과 설정을 점검하고 있습니다…</div>
    {:else}<div class="space-y-3">{#each visible as check (check.id)}
        <Card.Root><Card.Content class="flex flex-wrap items-start gap-4 pt-6">
            {#if check.action}<input type="checkbox" class="mt-1 size-4 accent-primary" aria-label={`${check.title} 선택`} checked={selected.includes(check.id)} disabled={applying} onchange={event => select(check.id, event.currentTarget.checked)} />{:else}<Activity class="mt-1 size-4 text-muted-foreground" />{/if}
            <div class="min-w-0 flex-1 space-y-2"><div class="flex flex-wrap items-center gap-2"><h3 class="break-all text-sm font-medium">{check.title}</h3><Badge variant={check.status === 'critical' ? 'destructive' : check.status === 'pass' ? 'secondary' : 'outline'}>{labels[check.status]}</Badge></div><p class="text-sm text-muted-foreground">{check.detail}</p>{#if check.change}<p class="font-mono text-xs">{check.change}</p>{/if}</div>
            <Button size="sm" variant="outline" href={check.href} disabled={applying}>설정 열기</Button>
        </Card.Content></Card.Root>
    {:else}<p class="py-8 text-sm text-muted-foreground">이 분류에 표시할 점검 결과나 자동 조치 제안이 없습니다.</p>{/each}</div>{/if}
</section>

<ConfirmActionDialog bind:open={confirmOpen} title="선택한 자동 조치를 적용할까요?" description={confirmation} confirmLabel="적용" destructive={false} busy={applying} onconfirm={apply} />
