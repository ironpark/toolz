<script lang="ts">
    import { onMount } from 'svelte';
    import { Power, RotateCw, SearchX } from '@lucide/svelte';
    import * as Alert from '$lib/components/ui/alert';
    import { Badge } from '$lib/components/ui/badge';
    import { Button } from '$lib/components/ui/button';
    import * as Card from '$lib/components/ui/card';
    import * as Empty from '$lib/components/ui/empty';
    import { Input } from '$lib/components/ui/input';
    import { Spinner } from '$lib/components/ui/spinner';
    import * as Table from '$lib/components/ui/table';
    import ConfirmActionDialog from './ConfirmActionDialog.svelte';
    import { getAppContext } from './context.svelte';

    const app = getAppContext();
    let query = $state('');
    let busy = $state('');
    let powerDialogOpen = $state(false);
    let powerTarget = $state<'reboot' | 'shutdown'>('reboot');

    onMount(() => { if (!app.systemManagement) void app.refreshSystem(); });
    const services = $derived((app.systemManagement?.services ?? []).filter((service) => !query || service.name.toLowerCase().includes(query.toLowerCase())));

    async function control(name: string, action: string): Promise<void> {
        busy = name;
        try { await app.controlSystemService(name, action); } finally { busy = ''; }
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
    <Card.Root>
        <Card.Header class="flex flex-row items-center justify-between gap-4"><div><Card.Title>시스템 서비스</Card.Title><Card.Description>{services.length}개 서비스</Card.Description></div><Input class="max-w-xs" bind:value={query} placeholder="서비스 검색" /></Card.Header>
        <Card.Content>
            {#if app.systemLoading && !app.systemManagement}
                <div class="grid min-h-48 place-items-center"><Spinner class="size-6" aria-label="서비스 불러오는 중" /></div>
            {:else if services.length}
                <Table.Root><Table.Header><Table.Row><Table.Head>서비스</Table.Head><Table.Head>시작 설정</Table.Head><Table.Head>상태</Table.Head><Table.Head class="text-right">작업</Table.Head></Table.Row></Table.Header><Table.Body>{#each services as service}<Table.Row><Table.Cell class="font-medium">{service.name}</Table.Cell><Table.Cell>{service.enabled ? '자동' : '수동'}</Table.Cell><Table.Cell><Badge variant={service.state === 'RUNNING' ? 'secondary' : 'outline'}>{service.state}</Badge></Table.Cell><Table.Cell class="space-x-2 text-right">{#if service.state === 'RUNNING'}<Button variant="outline" size="sm" disabled={busy === service.name} onclick={() => control(service.name, 'restart')}>{#if busy === service.name}<Spinner />{/if}재시작</Button><Button variant="ghost" size="sm" disabled={busy === service.name} onclick={() => control(service.name, 'stop')}>중지</Button>{:else}<Button size="sm" disabled={busy === service.name} onclick={() => control(service.name, 'start')}>{#if busy === service.name}<Spinner />{/if}시작</Button>{/if}</Table.Cell></Table.Row>{/each}</Table.Body></Table.Root>
            {:else}
                <Empty.Root class="min-h-48 border-0 p-6"><Empty.Media variant="icon"><SearchX /></Empty.Media><Empty.Header><Empty.Title>{query ? '일치하는 서비스가 없습니다' : '표시할 서비스가 없습니다'}</Empty.Title><Empty.Description>{query ? '검색어를 바꾸거나 지운 뒤 다시 확인하세요.' : 'TrueNAS에서 서비스 정보를 불러오면 여기에 표시됩니다.'}</Empty.Description></Empty.Header>{#if query}<Empty.Content><Button variant="outline" size="sm" onclick={() => (query = '')}>검색 초기화</Button></Empty.Content>{/if}</Empty.Root>
            {/if}
        </Card.Content>
    </Card.Root>
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
