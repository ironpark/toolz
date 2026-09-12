<script lang="ts">
    import { onMount } from 'svelte';
    import { Power, RotateCw, SearchX } from '@lucide/svelte';
    import * as Alert from '$lib/components/ui/alert';
    import { Badge } from '$lib/components/ui/badge';
    import { Button } from '$lib/components/ui/button';
    import * as Card from '$lib/components/ui/card';
    import * as Empty from '$lib/components/ui/empty';
    import { Input } from '$lib/components/ui/input';
    import * as NativeSelect from '$lib/components/ui/native-select';
    import { Spinner } from '$lib/components/ui/spinner';
    import { Switch } from '$lib/components/ui/switch';
    import * as Table from '$lib/components/ui/table';
    import CertificateManager from './CertificateManager.svelte';
    import HTTPSRedirectSettings from './HTTPSRedirectSettings.svelte';
    import ConfirmActionDialog from './ConfirmActionDialog.svelte';
    import { getAppContext } from './context.svelte';
    import { serviceMatches, servicePresentation } from './service-labels';

    const app = getAppContext();
    let query = $state('');
    let busy = $state('');
    let powerDialogOpen = $state(false);
    let powerTarget = $state<'reboot' | 'shutdown'>('reboot');

    onMount(() => { if (!app.systemManagement) void app.refreshSystem(); });
    const services = $derived((app.systemManagement?.services ?? []).filter((service) => serviceMatches(service.name, query)));

    async function control(name: string, action: string): Promise<void> {
        if (busy || app.systemLoading) return;
        busy = name;
        try { await app.controlSystemService(name, action); } finally { busy = ''; }
    }

    async function changeStartup(name: string, automatic: boolean): Promise<void> {
        if (busy || app.systemLoading) return;
        busy = `startup:${name}`;
        try { await app.setServiceStartup(name, automatic); } finally { busy = ''; }
    }

    function askPower(action: 'reboot' | 'shutdown'): void {
        powerTarget = action;
        powerDialogOpen = true;
    }

    async function confirmPower(): Promise<void> {
        busy = powerTarget;
        try { await app.powerAction(powerTarget); } finally { busy = ''; }
    }
</script>

<section class="space-y-6">
    {#if app.systemError}<Alert.Root variant="destructive"><Alert.Title>조회 실패</Alert.Title><Alert.Description>{app.systemError}</Alert.Description></Alert.Root>{/if}
    <section class="min-w-0 space-y-4">
        <header class="flex flex-row items-center justify-between gap-4"><div><h2 class="text-base font-semibold tracking-tight">시스템 서비스</h2><p class="mt-1 text-sm text-muted-foreground">{services.length}개 서비스</p></div><Input class="max-w-xs" bind:value={query} placeholder="서비스 검색" /></header>
        <div class="min-w-0">
            {#if app.systemLoading && !app.systemManagement}
                <div class="grid min-h-48 place-items-center"><Spinner class="size-6" aria-label="서비스 불러오는 중" /></div>
            {:else if services.length}
                <Table.Root>
                    <Table.Header><Table.Row><Table.Head>서비스</Table.Head><Table.Head>시작 설정</Table.Head><Table.Head>상태</Table.Head><Table.Head class="text-right">작업</Table.Head></Table.Row></Table.Header>
                    <Table.Body>
                        {#each services as service (service.name)}
                            {@const presentation = servicePresentation(service.name)}
                            {@const running = service.state === 'RUNNING'}
                            {@const pending = busy !== '' || app.systemLoading}
                            <Table.Row>
                                <Table.Cell><p class="font-medium">{presentation.label}</p><p class="mt-1 max-w-md whitespace-normal text-xs text-muted-foreground">{presentation.description}</p></Table.Cell>
                                <Table.Cell><div class="flex items-center gap-2">{#key `${service.enabled}-${busy}`}<NativeSelect.Root size="sm" aria-label={`${presentation.label} 시작 설정`} title="자동: 서버 부팅 시 시작 · 수동: 필요할 때 직접 시작" value={service.enabled ? 'automatic' : 'manual'} disabled={pending} onchange={event => { const automatic = event.currentTarget.value === 'automatic'; if (automatic !== service.enabled) void changeStartup(service.name, automatic); }}><option value="automatic">자동</option><option value="manual">수동</option></NativeSelect.Root>{/key}{#if busy === `startup:${service.name}`}<Spinner aria-label="시작 설정 저장 중" />{/if}</div></Table.Cell>
                                <Table.Cell><Badge variant={running ? 'secondary' : 'outline'}>{service.state}</Badge></Table.Cell>
                                <Table.Cell>
                                    <div class="flex items-center justify-end gap-4">
                                        <Button variant="ghost" size="icon-sm" aria-label={`${presentation.label} 재시작`} title={running ? '재시작' : '중지된 서비스는 재시작할 수 없습니다'} disabled={!running || pending} onclick={() => control(service.name, 'restart')}>
                                            {#if busy === service.name}<Spinner />{:else}<RotateCw />{/if}
                                        </Button>
                                        {#key `${service.state}-${busy === service.name}`}
                                            <Switch aria-label={`${presentation.label} 실행`} title={running ? '서비스 중지' : '서비스 시작'} checked={running} disabled={pending || !['RUNNING', 'STOPPED'].includes(service.state)} onCheckedChange={(checked) => { if (checked !== running) void control(service.name, checked ? 'start' : 'stop'); }} />
                                        {/key}
                                    </div>
                                </Table.Cell>
                            </Table.Row>
                        {/each}
                    </Table.Body>
                </Table.Root>
            {:else}
                <Empty.Root class="min-h-48 border-0 p-6"><Empty.Media variant="icon"><SearchX /></Empty.Media><Empty.Header><Empty.Title>{query ? '일치하는 서비스가 없습니다' : '표시할 서비스가 없습니다'}</Empty.Title><Empty.Description>{query ? '검색어를 바꾸거나 지운 뒤 다시 확인하세요.' : 'TrueNAS에서 서비스 정보를 불러오면 여기에 표시됩니다.'}</Empty.Description></Empty.Header>{#if query}<Empty.Content><Button variant="outline" size="sm" onclick={() => (query = '')}>검색 초기화</Button></Empty.Content>{/if}</Empty.Root>
            {/if}
        </div>
    </section>
    <CertificateManager />
    <HTTPSRedirectSettings />
    <Card.Root class="border-destructive/30"><Card.Header><Card.Title>전원 관리</Card.Title><Card.Description>실행 중인 작업과 연결을 확인한 후 진행하세요.</Card.Description></Card.Header><Card.Footer class="gap-2"><Button variant="outline" disabled={busy === 'reboot'} onclick={() => askPower('reboot')}><RotateCw />재부팅</Button><Button variant="destructive" disabled={busy === 'shutdown'} onclick={() => askPower('shutdown')}><Power />시스템 종료</Button></Card.Footer></Card.Root>
</section>

<ConfirmActionDialog
    bind:open={powerDialogOpen}
    title={powerTarget === 'reboot' ? 'TrueNAS를 재부팅할까요?' : 'TrueNAS를 종료할까요?'}
    description={powerTarget === 'reboot' ? '연결이 잠시 끊기며 실행 중인 서비스가 다시 시작됩니다.' : '종료 후에는 장치 전원을 직접 켜야 다시 연결할 수 있습니다.'}
    confirmLabel={powerTarget === 'reboot' ? '재부팅' : '시스템 종료'}
    busy={busy === powerTarget}
    onconfirm={confirmPower}
/>
