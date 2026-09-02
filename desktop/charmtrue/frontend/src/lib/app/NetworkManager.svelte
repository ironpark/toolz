<script lang="ts">
    import { onMount } from 'svelte';
    import { Check, Ellipsis, Pencil, Plus, RefreshCw, Route, Trash2, Undo2 } from '@lucide/svelte';
    import * as Alert from '$lib/components/ui/alert';
    import { Badge } from '$lib/components/ui/badge';
    import { Button } from '$lib/components/ui/button';
    import * as Card from '$lib/components/ui/card';
    import * as Dialog from '$lib/components/ui/dialog';
    import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
    import * as Empty from '$lib/components/ui/empty';
    import { Input } from '$lib/components/ui/input';
    import { Spinner } from '$lib/components/ui/spinner';
    import * as Table from '$lib/components/ui/table';
    import type { NetworkInterfaceInfo, StaticRouteInfo } from '../../../bindings/github.com/ironpark/toolz/desktop/charmtrue';
    import ConfirmActionDialog from './ConfirmActionDialog.svelte';
    import { getAppContext } from './context.svelte';

    type Editor = 'configuration' | 'interface' | 'route' | null;
    type ConfigurationForm = {
        hostname: string; domain: string; ipv4Gateway: string; ipv6Gateway: string; nameServers: string;
        httpProxy: string; hosts: string; searchDomains: string; announceNetbios: boolean; announceMdns: boolean; announceWsd: boolean;
    };
    type InterfaceForm = {
        id: string; name: string; type: string; description: string; ipv4Dhcp: boolean; ipv6Auto: boolean;
        aliases: string; mtu: number; lagProtocol: string; lagPorts: string; bridgeMembers: string;
        vlanParent: string; vlanTag: number; vlanPriority: number; enableLearning: boolean;
    };
    type RouteForm = { id: number; destination: string; gateway: string; description: string };

    const app = getAppContext();
    const selectClass = 'h-9 w-full rounded-md border border-input bg-transparent px-2.5 py-1 text-sm shadow-xs outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30';
    const textareaClass = 'min-h-24 w-full resize-y rounded-md border border-input bg-transparent px-3 py-2 font-mono text-sm shadow-xs outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30';
    let editor = $state<Editor>(null);
    let busy = $state('');
    let editorError = $state('');
    let configurationForm = $state<ConfigurationForm>(emptyConfiguration());
    let interfaceForm = $state<InterfaceForm>(emptyInterface());
    let routeForm = $state<RouteForm>({ id: 0, destination: '', gateway: '', description: '' });
    let confirmOpen = $state(false);
    let confirmation = $state<{ kind: 'commit' | 'rollback' | 'interface' | 'route'; title: string; description: string; label: string; target?: NetworkInterfaceInfo | StaticRouteInfo }>({ kind: 'commit', title: '', description: '', label: '' });
    const interfaces = $derived(app.network?.interfaces ?? []);
    const staticRoutes = $derived(app.network?.staticRoutes ?? []);

    onMount(() => { if (!app.network) void app.refreshNetwork(); });

    function lines(value: string): string[] {
        return value.split(/\r?\n/).map((item) => item.trim()).filter(Boolean);
    }

    function disableCorrections(node: HTMLTextAreaElement): void {
        node.setAttribute('autocorrect', 'off');
        node.setAttribute('autocapitalize', 'none');
        node.spellcheck = false;
    }

    function emptyConfiguration(): ConfigurationForm {
        return { hostname: '', domain: '', ipv4Gateway: '', ipv6Gateway: '', nameServers: '', httpProxy: '', hosts: '', searchDomains: '', announceNetbios: false, announceMdns: true, announceWsd: true };
    }

    function emptyInterface(): InterfaceForm {
        return { id: '', name: '', type: 'BRIDGE', description: '', ipv4Dhcp: false, ipv6Auto: false, aliases: '', mtu: 1500, lagProtocol: 'LACP', lagPorts: '', bridgeMembers: '', vlanParent: '', vlanTag: 1, vlanPriority: 0, enableLearning: true };
    }

    function editConfiguration(): void {
        const value = app.network?.configuration;
        if (!value) return;
        configurationForm = {
            hostname: value.hostname, domain: value.domain, ipv4Gateway: value.ipv4Gateway, ipv6Gateway: value.ipv6Gateway,
            nameServers: (value.nameServers ?? []).join('\n'), httpProxy: value.httpProxy, hosts: (value.hosts ?? []).join('\n'),
            searchDomains: (value.searchDomains ?? []).join('\n'), announceNetbios: value.announceNetbios,
            announceMdns: value.announceMdns, announceWsd: value.announceWsd,
        };
        editorError = '';
        editor = 'configuration';
    }

    function editInterface(value?: NetworkInterfaceInfo): void {
        interfaceForm = value ? {
            id: value.id, name: value.name, type: value.type, description: value.description,
            ipv4Dhcp: value.ipv4Dhcp, ipv6Auto: value.ipv6Auto,
            aliases: (value.aliases ?? []).map((alias) => `${alias.address}/${alias.netmask}`).join('\n'),
            mtu: value.mtu || 1500, lagProtocol: value.lagProtocol || 'LACP', lagPorts: (value.lagPorts ?? []).join('\n'),
            bridgeMembers: (value.bridgeMembers ?? []).join('\n'), vlanParent: value.vlanParent,
            vlanTag: value.vlanTag || 1, vlanPriority: value.vlanPriority || 0, enableLearning: value.enableLearning,
        } : emptyInterface();
        editorError = '';
        editor = 'interface';
    }

    function editRoute(value?: StaticRouteInfo): void {
        routeForm = value ? { id: value.id, destination: value.destination, gateway: value.gateway, description: value.description } : { id: 0, destination: '', gateway: '', description: '' };
        editorError = '';
        editor = 'route';
    }

    function parseAliases(value: string): Array<{ type: string; address: string; netmask: number }> {
        return lines(value).map((item) => {
            const separator = item.lastIndexOf('/');
            if (separator < 1) throw new Error(`CIDR 형식으로 입력하세요: ${item}`);
            const address = item.slice(0, separator).trim();
            const netmask = Number(item.slice(separator + 1));
            if (!Number.isInteger(netmask)) throw new Error(`네트워크 마스크가 올바르지 않습니다: ${item}`);
            return { type: address.includes(':') ? 'INET6' : 'INET', address, netmask };
        });
    }

    async function saveEditor(event: SubmitEvent): Promise<void> {
        event.preventDefault();
        busy = 'save';
        editorError = '';
        try {
            if (editor === 'configuration') {
                await app.saveNetworkConfiguration({ ...configurationForm, nameServers: lines(configurationForm.nameServers), hosts: lines(configurationForm.hosts), searchDomains: lines(configurationForm.searchDomains) });
            } else if (editor === 'interface') {
                await app.saveNetworkInterface({
                    ...interfaceForm, aliases: parseAliases(interfaceForm.aliases),
                    lagPorts: lines(interfaceForm.lagPorts), bridgeMembers: lines(interfaceForm.bridgeMembers),
                });
            } else if (editor === 'route') {
                await app.saveStaticRoute(routeForm);
            }
            editor = null;
        } catch (error) {
            editorError = error instanceof Error ? error.message : String(error);
        } finally {
            busy = '';
        }
    }

    async function commit(): Promise<void> {
        busy = 'commit';
        try { await app.commitNetworkChanges(60); } catch {} finally { busy = ''; }
    }

    async function checkin(): Promise<void> {
        busy = 'checkin';
        try { await app.checkinNetworkChanges(); } catch {} finally { busy = ''; }
    }

    async function rollback(): Promise<void> {
        busy = 'rollback';
        try { await app.rollbackNetworkChanges(); } catch {} finally { busy = ''; }
    }

    async function removeInterface(value: NetworkInterfaceInfo): Promise<void> {
        busy = `interface-${value.id}`;
        try { await app.deleteNetworkInterface(value.id); } catch {} finally { busy = ''; }
    }

    async function removeRoute(value: StaticRouteInfo): Promise<void> {
        busy = `route-${value.id}`;
        try { await app.deleteStaticRoute(value.id); } catch {} finally { busy = ''; }
    }

    function ask(kind: 'commit' | 'rollback' | 'interface' | 'route', target?: NetworkInterfaceInfo | StaticRouteInfo): void {
        confirmation = kind === 'commit'
            ? { kind, title: '네트워크 변경을 안전 적용할까요?', description: '적용 후 60초 안에 연결을 확정하지 않으면 TrueNAS가 이전 설정으로 자동 롤백합니다.', label: '60초 안전 적용' }
            : kind === 'rollback'
                ? { kind, title: '대기 중인 변경을 취소할까요?', description: '아직 확정하지 않은 네트워크 설정을 버리고 이전 구성으로 되돌립니다.', label: '변경 취소' }
                : kind === 'interface'
                    ? { kind, target, title: `“${(target as NetworkInterfaceInfo).name}” 인터페이스를 삭제할까요?`, description: '이 인터페이스를 사용하는 연결과 서비스가 즉시 영향을 받을 수 있습니다.', label: '인터페이스 삭제' }
                    : { kind, target, title: `“${(target as StaticRouteInfo).destination}” 라우트를 삭제할까요?`, description: '이 경로를 사용하는 네트워크 통신이 즉시 중단될 수 있습니다.', label: '라우트 삭제' };
        confirmOpen = true;
    }

    async function confirmAction(): Promise<void> {
        if (confirmation.kind === 'commit') await commit();
        else if (confirmation.kind === 'rollback') await rollback();
        else if (confirmation.kind === 'interface') await removeInterface(confirmation.target as NetworkInterfaceInfo);
        else await removeRoute(confirmation.target as StaticRouteInfo);
    }
</script>

<section class="space-y-6">
    <header class="flex flex-wrap items-end justify-between gap-3">
        <div><h2 class="text-3xl font-semibold tracking-tight">네트워크</h2><p class="mt-1 text-sm text-muted-foreground">인터페이스, 게이트웨이, DNS와 정적 라우트를 관리합니다.</p></div>
        <Button variant="outline" disabled={app.networkLoading} onclick={() => app.refreshNetwork()}>{#if app.networkLoading}<Spinner aria-label="네트워크 새로고침 중" />{:else}<RefreshCw />{/if}새로고침</Button>
    </header>

    {#if app.networkError}<Alert.Root variant="destructive"><Alert.Title>네트워크 작업 실패</Alert.Title><Alert.Description>{app.networkError}</Alert.Description></Alert.Root>{/if}
    {#if app.network?.checkinRemaining}
        <Alert.Root><Alert.Title>네트워크 연결 확인 필요</Alert.Title><Alert.Description><div class="flex flex-wrap items-center justify-between gap-3"><span>{app.network.checkinRemaining}초 안에 현재 연결을 확정하지 않으면 자동으로 롤백됩니다.</span><div class="flex gap-2"><Button size="sm" onclick={checkin} disabled={busy !== ''}>{#if busy === 'checkin'}<Spinner />{:else}<Check />{/if}연결 확정</Button><Button size="sm" variant="outline" onclick={() => ask('rollback')} disabled={busy !== ''}><Undo2 />즉시 롤백</Button></div></div></Alert.Description></Alert.Root>
    {:else if app.network?.pendingChanges}
        <Alert.Root><Alert.Title>적용 대기 중인 변경</Alert.Title><Alert.Description><div class="flex flex-wrap items-center justify-between gap-3"><span>인터페이스 변경을 검토한 후 안전 적용하세요.</span><div class="flex gap-2"><Button size="sm" onclick={() => ask('commit')} disabled={busy !== ''}>60초 안전 적용</Button><Button size="sm" variant="outline" onclick={() => ask('rollback')} disabled={busy !== ''}><Undo2 />변경 취소</Button></div></div></Alert.Description></Alert.Root>
    {/if}

    <div class="grid gap-4 md:grid-cols-3">
        <Card.Root><Card.Header><Card.Description>활성 인터페이스</Card.Description><Card.Title class="text-2xl">{interfaces.filter((item) => item.linkState.toUpperCase().includes('UP')).length} / {interfaces.length}</Card.Title></Card.Header></Card.Root>
        <Card.Root><Card.Header><Card.Description>기본 경로</Card.Description><Card.Title class="truncate text-lg">{app.network?.summary.defaultRoutes?.[0] || '없음'}</Card.Title></Card.Header></Card.Root>
        <Card.Root><Card.Header><Card.Description>DNS 서버</Card.Description><Card.Title class="truncate text-lg">{app.network?.summary.nameServers?.join(', ') || '없음'}</Card.Title></Card.Header></Card.Root>
    </div>

    <Card.Root>
        <Card.Header class="flex flex-row items-start justify-between gap-4"><div><Card.Title>전역 네트워크 설정</Card.Title><Card.Description>{app.network?.configuration.hostname || '—'}{app.network?.configuration.domain ? `.${app.network.configuration.domain}` : ''}</Card.Description></div><Button size="sm" variant="outline" onclick={editConfiguration}><Pencil />수정</Button></Card.Header>
        <Card.Content class="grid gap-4 text-sm sm:grid-cols-2 lg:grid-cols-4"><div><p class="text-muted-foreground">IPv4 게이트웨이</p><p class="mt-1 font-medium">{app.network?.configuration.ipv4Gateway || '자동'}</p></div><div><p class="text-muted-foreground">IPv6 게이트웨이</p><p class="mt-1 font-medium">{app.network?.configuration.ipv6Gateway || '자동'}</p></div><div><p class="text-muted-foreground">검색 도메인</p><p class="mt-1 font-medium">{app.network?.configuration.searchDomains?.join(', ') || '없음'}</p></div><div><p class="text-muted-foreground">서비스 검색</p><p class="mt-1 font-medium">{[app.network?.configuration.announceMdns && 'mDNS', app.network?.configuration.announceWsd && 'WSD', app.network?.configuration.announceNetbios && 'NetBIOS'].filter(Boolean).join(', ') || '꺼짐'}</p></div></Card.Content>
    </Card.Root>

    <Card.Root>
        <Card.Header class="flex flex-row items-start justify-between gap-4"><div><Card.Title>네트워크 인터페이스</Card.Title><Card.Description>물리 및 가상 인터페이스 {interfaces.length}개</Card.Description></div><Button size="sm" onclick={() => editInterface()}><Plus />가상 인터페이스</Button></Card.Header>
        <Card.Content><Table.Root><Table.Header><Table.Row><Table.Head>인터페이스</Table.Head><Table.Head>주소</Table.Head><Table.Head>링크</Table.Head><Table.Head>MTU</Table.Head><Table.Head class="text-right">작업</Table.Head></Table.Row></Table.Header><Table.Body>
            {#each interfaces as item (item.id)}<Table.Row><Table.Cell><div class="flex items-center gap-2"><span class={`size-2 rounded-full ${item.linkState.toUpperCase().includes('UP') ? 'bg-emerald-500' : 'bg-muted-foreground'}`}></span><div><p class="font-medium">{item.name}</p><p class="text-xs text-muted-foreground">{item.description || item.type} · {item.macAddress || 'MAC 없음'}</p></div></div></Table.Cell><Table.Cell><div class="space-y-1">{#each item.aliases ?? [] as alias}<p class="font-mono text-xs">{alias.address}/{alias.netmask}</p>{/each}{#if item.ipv4Dhcp}<Badge variant="outline">DHCP</Badge>{/if}</div></Table.Cell><Table.Cell><p>{item.mediaSubtype || item.linkState || '—'}</p><p class="text-xs text-muted-foreground">{item.mediaType}</p></Table.Cell><Table.Cell>{item.mtu || '—'}</Table.Cell><Table.Cell><div class="flex justify-end"><DropdownMenu.Root><DropdownMenu.Trigger>{#snippet child({ props })}<Button {...props} size="icon-sm" variant="ghost" aria-label={`${item.name} 작업 메뉴`}><Ellipsis /></Button>{/snippet}</DropdownMenu.Trigger><DropdownMenu.Content align="end"><DropdownMenu.Item onclick={() => editInterface(item)}><Pencil />수정</DropdownMenu.Item>{#if item.type !== 'PHYSICAL'}<DropdownMenu.Separator /><DropdownMenu.Item variant="destructive" disabled={busy === `interface-${item.id}`} onclick={() => ask('interface', item)}><Trash2 />삭제</DropdownMenu.Item>{/if}</DropdownMenu.Content></DropdownMenu.Root></div></Table.Cell></Table.Row>{/each}
        </Table.Body></Table.Root></Card.Content>
    </Card.Root>

    <Card.Root>
        <Card.Header class="flex flex-row items-start justify-between gap-4"><div><Card.Title>정적 라우트</Card.Title><Card.Description>특정 네트워크로 향하는 고정 경로</Card.Description></div><Button size="sm" variant="outline" onclick={() => editRoute()}><Plus />라우트 추가</Button></Card.Header>
        <Card.Content>{#if staticRoutes.length}<Table.Root><Table.Header><Table.Row><Table.Head>목적지</Table.Head><Table.Head>게이트웨이</Table.Head><Table.Head>설명</Table.Head><Table.Head class="w-14 text-right"><span class="sr-only">작업</span></Table.Head></Table.Row></Table.Header><Table.Body>{#each staticRoutes as item (item.id)}<Table.Row><Table.Cell class="font-mono text-xs">{item.destination}</Table.Cell><Table.Cell class="font-mono text-xs">{item.gateway}</Table.Cell><Table.Cell>{item.description || '—'}</Table.Cell><Table.Cell><div class="flex justify-end"><DropdownMenu.Root><DropdownMenu.Trigger>{#snippet child({ props })}<Button {...props} size="icon-sm" variant="ghost" aria-label={`${item.destination} 작업 메뉴`}><Ellipsis /></Button>{/snippet}</DropdownMenu.Trigger><DropdownMenu.Content align="end"><DropdownMenu.Item onclick={() => editRoute(item)}><Pencil />수정</DropdownMenu.Item><DropdownMenu.Separator /><DropdownMenu.Item variant="destructive" disabled={busy === `route-${item.id}`} onclick={() => ask('route', item)}><Trash2 />삭제</DropdownMenu.Item></DropdownMenu.Content></DropdownMenu.Root></div></Table.Cell></Table.Row>{/each}</Table.Body></Table.Root>{:else}<Empty.Root class="min-h-36 border-0 p-6"><Empty.Media variant="icon"><Route /></Empty.Media><Empty.Header><Empty.Title>정적 라우트가 없습니다</Empty.Title><Empty.Description>특정 네트워크에 고정 경로가 필요할 때 추가하세요.</Empty.Description></Empty.Header><Empty.Content><Button size="sm" variant="outline" onclick={() => editRoute()}><Plus />라우트 추가</Button></Empty.Content></Empty.Root>{/if}</Card.Content>
    </Card.Root>
</section>

<Dialog.Root open={editor !== null} onOpenChange={(open) => { if (!open) editor = null; }}>
    <Dialog.Content class="max-h-[88dvh] overflow-y-auto sm:max-w-2xl">
        <form class="space-y-5" onsubmit={saveEditor}>
            <Dialog.Header><Dialog.Title>{editor === 'configuration' ? '전역 네트워크 설정' : editor === 'route' ? (routeForm.id ? '정적 라우트 수정' : '정적 라우트 추가') : interfaceForm.id ? `${interfaceForm.name} 수정` : '가상 인터페이스 추가'}</Dialog.Title><Dialog.Description>인터페이스 변경은 저장 후 별도의 안전 적용과 연결 확정이 필요합니다.</Dialog.Description></Dialog.Header>
            {#if editor === 'configuration'}
                <div class="grid gap-4 sm:grid-cols-2"><label class="grid gap-2 text-sm font-medium">호스트 이름<Input bind:value={configurationForm.hostname} required /></label><label class="grid gap-2 text-sm font-medium">도메인<Input bind:value={configurationForm.domain} /></label><label class="grid gap-2 text-sm font-medium">IPv4 게이트웨이<Input bind:value={configurationForm.ipv4Gateway} placeholder="자동" /></label><label class="grid gap-2 text-sm font-medium">IPv6 게이트웨이<Input bind:value={configurationForm.ipv6Gateway} placeholder="자동" /></label><label class="grid gap-2 text-sm font-medium sm:col-span-2">HTTP 프록시<Input bind:value={configurationForm.httpProxy} placeholder="http://proxy.example:3128" /></label><label class="grid gap-2 text-sm font-medium">DNS 서버<textarea class={textareaClass} bind:value={configurationForm.nameServers} use:disableCorrections placeholder="한 줄에 하나, 최대 3개"></textarea></label><label class="grid gap-2 text-sm font-medium">검색 도메인<textarea class={textareaClass} bind:value={configurationForm.searchDomains} use:disableCorrections placeholder="example.internal"></textarea></label><label class="grid gap-2 text-sm font-medium sm:col-span-2">정적 hosts 항목<textarea class={textareaClass} bind:value={configurationForm.hosts} use:disableCorrections placeholder="192.168.1.20 backup.local"></textarea></label></div>
                <div class="grid gap-3 rounded-lg border p-4 sm:grid-cols-3"><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={configurationForm.announceMdns} />mDNS 알림</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={configurationForm.announceWsd} />WSD 알림</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={configurationForm.announceNetbios} />NetBIOS 알림</label></div>
            {:else if editor === 'interface'}
                <div class="grid gap-4 sm:grid-cols-2">{#if !interfaceForm.id}<label class="grid gap-2 text-sm font-medium">유형<select class={selectClass} bind:value={interfaceForm.type}><option value="BRIDGE">브리지</option><option value="LINK_AGGREGATION">링크 집계</option><option value="VLAN">VLAN</option></select></label>{/if}<label class="grid gap-2 text-sm font-medium">이름<Input bind:value={interfaceForm.name} placeholder={interfaceForm.id ? interfaceForm.id : '자동 생성'} /></label><label class="grid gap-2 text-sm font-medium sm:col-span-2">설명<Input bind:value={interfaceForm.description} /></label><label class="grid gap-2 text-sm font-medium">MTU<Input type="number" min="68" max="9216" bind:value={interfaceForm.mtu} /></label><label class="grid gap-2 text-sm font-medium sm:col-span-2">고정 IP 주소<textarea class={textareaClass} bind:value={interfaceForm.aliases} use:disableCorrections placeholder={'192.168.1.10/24\n2001:db8::10/64'}></textarea></label></div>
                <div class="grid gap-3 rounded-lg border p-4 sm:grid-cols-2"><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={interfaceForm.ipv4Dhcp} />IPv4 DHCP</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={interfaceForm.ipv6Auto} />IPv6 자동 구성</label></div>
                {#if interfaceForm.type === 'BRIDGE'}<div class="grid gap-4"><label class="grid gap-2 text-sm font-medium">구성 인터페이스<textarea class={textareaClass} bind:value={interfaceForm.bridgeMembers} use:disableCorrections placeholder="enp1s0"></textarea></label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={interfaceForm.enableLearning} />MAC 주소 학습</label></div>{:else if interfaceForm.type === 'LINK_AGGREGATION'}<div class="grid gap-4 sm:grid-cols-2"><label class="grid gap-2 text-sm font-medium">프로토콜<select class={selectClass} bind:value={interfaceForm.lagProtocol}><option value="LACP">LACP</option><option value="FAILOVER">Failover</option><option value="LOADBALANCE">Load balance</option></select></label><label class="grid gap-2 text-sm font-medium sm:col-span-2">구성 포트<textarea class={textareaClass} bind:value={interfaceForm.lagPorts} use:disableCorrections placeholder={'enp1s0\nenp2s0'}></textarea></label></div>{:else if interfaceForm.type === 'VLAN'}<div class="grid gap-4 sm:grid-cols-3"><label class="grid gap-2 text-sm font-medium">부모 인터페이스<Input bind:value={interfaceForm.vlanParent} required /></label><label class="grid gap-2 text-sm font-medium">VLAN 태그<Input type="number" min="1" max="4094" bind:value={interfaceForm.vlanTag} required /></label><label class="grid gap-2 text-sm font-medium">우선순위<Input type="number" min="0" max="7" bind:value={interfaceForm.vlanPriority} /></label></div>{/if}
            {:else if editor === 'route'}
                <div class="grid gap-4 sm:grid-cols-2"><label class="grid gap-2 text-sm font-medium">목적지 CIDR<Input bind:value={routeForm.destination} placeholder="10.20.0.0/16" required /></label><label class="grid gap-2 text-sm font-medium">게이트웨이<Input bind:value={routeForm.gateway} placeholder="192.168.1.1" required /></label><label class="grid gap-2 text-sm font-medium sm:col-span-2">설명<Input bind:value={routeForm.description} /></label></div>
            {/if}
            {#if editorError}<Alert.Root variant="destructive"><Alert.Title>저장하지 못했습니다</Alert.Title><Alert.Description>{editorError}</Alert.Description></Alert.Root>{/if}
            <Dialog.Footer><Button type="button" variant="outline" disabled={busy === 'save'} onclick={() => (editor = null)}>취소</Button><Button type="submit" disabled={busy === 'save'}>{#if busy === 'save'}<Spinner />{/if}{busy === 'save' ? '저장하는 중' : '저장'}</Button></Dialog.Footer>
        </form>
    </Dialog.Content>
</Dialog.Root>

<ConfirmActionDialog
    bind:open={confirmOpen}
    title={confirmation.title}
    description={confirmation.description}
    confirmLabel={confirmation.label}
    destructive={confirmation.kind !== 'commit'}
    busy={busy !== ''}
    onconfirm={confirmAction}
/>
