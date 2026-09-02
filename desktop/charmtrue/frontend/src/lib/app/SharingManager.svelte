<script lang="ts">
    import { onMount } from 'svelte';
    import { FolderX, Pencil, Play, Plus, RefreshCw, ShieldCheck, Trash2 } from '@lucide/svelte';
    import * as Alert from '$lib/components/ui/alert';
    import { Badge } from '$lib/components/ui/badge';
    import { Button } from '$lib/components/ui/button';
    import * as Card from '$lib/components/ui/card';
    import * as Dialog from '$lib/components/ui/dialog';
    import * as Empty from '$lib/components/ui/empty';
    import { Input } from '$lib/components/ui/input';
    import * as NativeSelect from '$lib/components/ui/native-select';
    import { Spinner } from '$lib/components/ui/spinner';
    import { Switch } from '$lib/components/ui/switch';
    import * as Table from '$lib/components/ui/table';
    import { Textarea } from '$lib/components/ui/textarea';
    import ConfirmActionDialog from './ConfirmActionDialog.svelte';
    import { getAppContext } from './context.svelte';
    import type { RsyncTaskInfo, ShareInfo, SMBShareACLEntry } from '../../../bindings/github.com/ironpark/toolz/desktop/charmtrue';

    type EditorKind = 'share' | 'rsync';
    type ShareForm = {
        id: number; protocol: string; name: string; path: string; purpose: string; comment: string;
        enabled: boolean; readOnly: boolean; browsable: boolean; accessBasedShareEnumeration: boolean;
        auditEnabled: boolean; auditWatchList: string; auditIgnoreList: string;
        recycleBin: boolean; pathSuffix: string; hostsAllow: string; hostsDeny: string; guestOk: boolean;
        streams: boolean; durableHandle: boolean; shadowCopy: boolean; fsrvp: boolean; home: boolean;
        acl: boolean; afp: boolean; timeMachine: boolean; timeMachineQuota: number; aaplNameMangling: boolean;
        vuid: string; auxSmbConf: string; autoSnapshot: boolean; autoDatasetCreation: boolean;
        datasetNamingSchema: string; gracePeriod: number; autoQuota: number; remotePath: string;
        networks: string; aliases: string; hosts: string; mapRootUser: string; mapRootGroup: string; mapAllUser: string;
        mapAllGroup: string; security: string[]; exposeSnapshots: boolean;
    };
    type RsyncForm = {
        id: number; path: string; user: string; mode: string; remoteHost: string; remotePort: string;
        remoteModule: string; remotePath: string; sshCredentialId: string; direction: string; description: string;
        scheduleMinute: string; scheduleHour: string; scheduleDayOfMonth: string; scheduleMonth: string;
        scheduleDayOfWeek: string; recursive: boolean; times: boolean; compress: boolean; archive: boolean;
        delete: boolean; quiet: boolean; preservePermissions: boolean; preserveAttributes: boolean;
        delayUpdates: boolean; extra: string; enabled: boolean; validateRemotePath: boolean; sshKeyScan: boolean;
    };

    const app = getAppContext();
    let query = $state('');
    let busy = $state('');
    let editorOpen = $state(false);
    let editorKind = $state<EditorKind>('share');
    let editorError = $state('');
    let shareForm = $state<ShareForm>(emptyShareForm());
    let rsyncForm = $state<RsyncForm>(emptyRsyncForm());
    let deleteDialogOpen = $state(false);
    let deleteTarget = $state<{ protocol: string; id: number; name: string } | null>(null);
    let aclOpen = $state(false);
    let aclLoading = $state(false);
    let aclError = $state('');
    let aclShareName = $state('');
    let aclEntries = $state<SMBShareACLEntry[]>([]);

    onMount(() => { if (!app.sharing) void app.refreshSharing(); });
    const shares = $derived((app.sharing?.shares ?? []).filter((share) => !query || `${share.name} ${share.path} ${share.protocol}`.toLowerCase().includes(query.toLowerCase())));
    const tasks = $derived(app.sharing?.rsyncTasks ?? []);

    function emptyShareForm(protocol = 'SMB'): ShareForm {
        return { id: 0, protocol, name: '', path: '', purpose: 'DEFAULT_SHARE', comment: '', enabled: true, readOnly: false, browsable: true, accessBasedShareEnumeration: false, auditEnabled: false, auditWatchList: '', auditIgnoreList: '', recycleBin: false, pathSuffix: '', hostsAllow: '', hostsDeny: '', guestOk: false, streams: true, durableHandle: true, shadowCopy: true, fsrvp: false, home: false, acl: true, afp: false, timeMachine: false, timeMachineQuota: 0, aaplNameMangling: false, vuid: '', auxSmbConf: '', autoSnapshot: false, autoDatasetCreation: false, datasetNamingSchema: '', gracePeriod: 900, autoQuota: 0, remotePath: '', networks: '', aliases: '', hosts: '', mapRootUser: '', mapRootGroup: '', mapAllUser: '', mapAllGroup: '', security: ['SYS'], exposeSnapshots: false };
    }

    function emptyRsyncForm(): RsyncForm {
        return { id: 0, path: '', user: '', mode: 'MODULE', remoteHost: '', remotePort: '', remoteModule: '', remotePath: '', sshCredentialId: '', direction: 'PUSH', description: '', scheduleMinute: '00', scheduleHour: '*', scheduleDayOfMonth: '*', scheduleMonth: '*', scheduleDayOfWeek: '*', recursive: true, times: true, compress: false, archive: false, delete: false, quiet: false, preservePermissions: false, preserveAttributes: false, delayUpdates: true, extra: '', enabled: true, validateRemotePath: true, sshKeyScan: false };
    }

    function lines(value: string): string[] {
        return value.split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
    }

    function createShare(protocol: 'SMB' | 'NFS'): void {
        editorKind = 'share'; editorError = ''; shareForm = emptyShareForm(protocol); editorOpen = true;
    }

    function createRsync(): void {
        editorKind = 'rsync'; editorError = ''; rsyncForm = emptyRsyncForm(); editorOpen = true;
    }

    function editShare(share: ShareInfo): void {
        editorKind = 'share'; editorError = '';
        shareForm = {
            id: share.id, protocol: share.protocol, name: share.name, path: share.path,
            purpose: share.purpose || 'DEFAULT_SHARE', comment: share.comment, enabled: share.enabled,
            readOnly: share.readOnly, browsable: share.browsable, accessBasedShareEnumeration: share.accessBasedShareEnumeration,
            auditEnabled: share.auditEnabled, auditWatchList: (share.auditWatchList ?? []).join('\n'), auditIgnoreList: (share.auditIgnoreList ?? []).join('\n'),
            recycleBin: share.recycleBin, pathSuffix: share.pathSuffix, hostsAllow: (share.hostsAllow ?? []).join('\n'),
            hostsDeny: (share.hostsDeny ?? []).join('\n'), guestOk: share.guestOk, streams: share.streams,
            durableHandle: share.durableHandle, shadowCopy: share.shadowCopy, fsrvp: share.fsrvp,
            home: share.home, acl: share.acl, afp: share.afp, timeMachine: share.timeMachine,
            timeMachineQuota: share.timeMachineQuota, aaplNameMangling: share.aaplNameMangling,
            vuid: share.vuid, auxSmbConf: share.auxSmbConf, autoSnapshot: share.autoSnapshot,
            autoDatasetCreation: share.autoDatasetCreation, datasetNamingSchema: share.datasetNamingSchema,
            gracePeriod: share.gracePeriod || 900, autoQuota: share.autoQuota, remotePath: (share.remotePath ?? []).join('\n'),
            networks: (share.networks ?? []).join('\n'), aliases: (share.aliases ?? []).join('\n'),
            hosts: (share.hosts ?? []).join('\n'), mapRootUser: share.mapRootUser, mapRootGroup: share.mapRootGroup,
            mapAllUser: share.mapAllUser, mapAllGroup: share.mapAllGroup,
            security: share.security?.length ? [...share.security] : ['SYS'], exposeSnapshots: share.exposeSnapshots,
        };
        editorOpen = true;
    }

    function editRsync(task: RsyncTaskInfo): void {
        editorKind = 'rsync'; editorError = '';
        rsyncForm = {
            id: task.id, path: task.path, user: task.user, mode: task.mode || 'MODULE', remoteHost: task.remoteHost,
            remotePort: task.remotePort ? String(task.remotePort) : '', remoteModule: task.remoteModule,
            remotePath: task.remotePath, sshCredentialId: task.sshCredentialId ? String(task.sshCredentialId) : '',
            direction: task.direction, description: task.description, scheduleMinute: task.scheduleMinute || '00',
            scheduleHour: task.scheduleHour || '*', scheduleDayOfMonth: task.scheduleDayOfMonth || '*',
            scheduleMonth: task.scheduleMonth || '*', scheduleDayOfWeek: task.scheduleDayOfWeek || '*',
            recursive: task.recursive, times: task.times, compress: task.compress, archive: task.archive,
            delete: task.delete, quiet: task.quiet, preservePermissions: task.preservePermissions,
            preserveAttributes: task.preserveAttributes, delayUpdates: task.delayUpdates,
            extra: (task.extra ?? []).join('\n'), enabled: task.enabled, validateRemotePath: task.validateRemotePath,
            sshKeyScan: task.sshKeyScan,
        };
        editorOpen = true;
    }

    function toggleSecurity(value: string): void {
        shareForm.security = shareForm.security.includes(value) ? shareForm.security.filter((item) => item !== value) : [...shareForm.security, value];
    }

    async function saveEditor(event: SubmitEvent): Promise<void> {
        event.preventDefault(); busy = 'save'; editorError = '';
        try {
            if (editorKind === 'share') {
                await app.saveShare({ ...shareForm, path: shareForm.purpose === 'EXTERNAL_SHARE' ? 'EXTERNAL' : shareForm.path, remotePath: lines(shareForm.remotePath), auditWatchList: lines(shareForm.auditWatchList), auditIgnoreList: lines(shareForm.auditIgnoreList), hostsAllow: lines(shareForm.hostsAllow), hostsDeny: lines(shareForm.hostsDeny), networks: lines(shareForm.networks), aliases: lines(shareForm.aliases), hosts: lines(shareForm.hosts) });
            } else {
                await app.saveRsyncTask({ ...rsyncForm, remotePort: Number(rsyncForm.remotePort || 0), sshCredentialId: Number(rsyncForm.sshCredentialId || 0), extra: lines(rsyncForm.extra) });
            }
            editorOpen = false;
        } catch (error) { editorError = error instanceof Error ? error.message : String(error); }
        finally { busy = ''; }
    }

    async function toggle(protocol: string, id: number, enabled: boolean): Promise<void> { busy = `${protocol}-${id}`; await app.setShareEnabled(protocol, id, enabled); busy = ''; }
    function askRemove(protocol: string, id: number, name: string): void { deleteTarget = { protocol, id, name }; deleteDialogOpen = true; }
    async function remove(): Promise<void> { if (!deleteTarget) return; busy = `${deleteTarget.protocol}-${deleteTarget.id}`; try { if (deleteTarget.protocol === 'RSYNC') await app.deleteRsyncTask(deleteTarget.id); else await app.deleteShare(deleteTarget.protocol, deleteTarget.id); } finally { busy = ''; deleteTarget = null; } }
    async function run(id: number): Promise<void> { busy = `rsync-${id}`; await app.runRsyncTask(id); busy = ''; }

    async function editACL(share: ShareInfo): Promise<void> {
        aclOpen = true; aclLoading = true; aclError = ''; aclShareName = share.name; aclEntries = [];
        try {
            const acl = await app.getSMBShareACL(share.name);
            aclEntries = [...(acl.entries ?? [])];
        }
        catch (error) { aclError = error instanceof Error ? error.message : String(error); }
        finally { aclLoading = false; }
    }

    function addACLEntry(): void {
        aclEntries = [...aclEntries, { permission: 'READ', entryType: 'ALLOWED', sid: '', idType: 'USER', id: 0, hasId: false, name: '' }];
    }

    function aclIdentityKind(entry: SMBShareACLEntry): 'NAME' | 'SID' | 'USER_ID' | 'GROUP_ID' {
        if (entry.sid) return 'SID';
        if (entry.hasId) return entry.idType === 'GROUP' ? 'GROUP_ID' : 'USER_ID';
        return 'NAME';
    }

    function setACLIdentityKind(index: number, kind: string): void {
        const entry = { ...aclEntries[index], sid: '', name: '', hasId: false, id: 0 };
        if (kind === 'USER_ID' || kind === 'GROUP_ID') {
            entry.hasId = true;
            entry.idType = kind === 'GROUP_ID' ? 'GROUP' : 'USER';
        }
        aclEntries[index] = entry;
    }

    function setACLIdentityValue(index: number, value: string): void {
        const entry = { ...aclEntries[index] };
        switch (aclIdentityKind(entry)) {
            case 'SID': entry.sid = value; break;
            case 'USER_ID':
            case 'GROUP_ID': entry.id = Number(value || 0); break;
            default: entry.name = value;
        }
        aclEntries[index] = entry;
    }

    function aclIdentityValue(entry: SMBShareACLEntry): string {
        if (entry.sid) return entry.sid;
        if (entry.hasId) return String(entry.id);
        return entry.name;
    }

    function removeACLEntry(index: number): void {
        aclEntries = aclEntries.filter((_, entryIndex) => entryIndex !== index);
    }

    async function saveACL(event: SubmitEvent): Promise<void> {
        event.preventDefault(); aclLoading = true; aclError = '';
        try { await app.saveSMBShareACL({ shareName: aclShareName, entries: aclEntries }); aclOpen = false; }
        catch (error) { aclError = error instanceof Error ? error.message : String(error); }
        finally { aclLoading = false; }
    }
</script>

<section class="space-y-6">
    {#if app.sharingError}<Alert.Root variant="destructive"><Alert.Title>조회 실패</Alert.Title><Alert.Description>{app.sharingError}</Alert.Description></Alert.Root>{/if}
    <Card.Root><Card.Header class="gap-4 lg:flex-row lg:items-center lg:justify-between"><div><Card.Title>파일 공유</Card.Title><Card.Description>{shares.length}개 항목</Card.Description></div><div class="flex flex-wrap items-center gap-2"><Input class="min-w-48 flex-1 lg:w-64" bind:value={query} placeholder="이름, 경로, 프로토콜 검색" /><Button variant="outline" size="sm" onclick={() => createShare('NFS')}><Plus />NFS 추가</Button><Button size="sm" onclick={() => createShare('SMB')}><Plus />SMB 추가</Button></div></Card.Header><Card.Content>
        {#if shares.length}
            <Table.Root><Table.Header><Table.Row><Table.Head>이름</Table.Head><Table.Head>프로토콜</Table.Head><Table.Head>상태</Table.Head><Table.Head class="w-32 text-right">작업</Table.Head></Table.Row></Table.Header><Table.Body>{#each shares as share (share.protocol + share.id)}<Table.Row><Table.Cell><p class="font-medium">{share.name || share.path}</p><p class="text-xs text-muted-foreground">{share.path}</p></Table.Cell><Table.Cell><Badge variant="outline">{share.protocol}</Badge></Table.Cell><Table.Cell><div class="flex items-center gap-2"><Switch checked={share.enabled} disabled={share.locked || busy === `${share.protocol}-${share.id}`} onCheckedChange={(checked) => toggle(share.protocol, share.id, checked)} aria-label={`${share.name || share.path} 공유 ${share.enabled ? '비활성화' : '활성화'}`} /><span class="text-sm text-muted-foreground">{share.locked ? '잠김' : share.enabled ? '활성' : '비활성'}</span></div></Table.Cell><Table.Cell><div class="flex justify-end gap-1">{#if share.protocol === 'SMB'}<Button variant="ghost" size="icon-sm" disabled={share.locked} aria-label={`${share.name} ACL 편집`} title="공유 ACL" onclick={() => editACL(share)}><ShieldCheck /></Button>{/if}<Button variant="ghost" size="icon-sm" disabled={share.locked} aria-label={`${share.name || share.path} 수정`} onclick={() => editShare(share)}><Pencil /></Button><Button variant="ghost" size="icon-sm" class="text-destructive hover:bg-destructive/10 hover:text-destructive" disabled={share.locked || busy === `${share.protocol}-${share.id}`} aria-label={`${share.name || share.path} 삭제`} onclick={() => askRemove(share.protocol, share.id, share.name || share.path)}><Trash2 /></Button></div></Table.Cell></Table.Row>{/each}</Table.Body></Table.Root>
        {:else}
            <Empty.Root class="min-h-48 border-0 p-6"><Empty.Media variant="icon"><FolderX /></Empty.Media><Empty.Header><Empty.Title>{query ? '일치하는 공유가 없습니다' : '등록된 파일 공유가 없습니다'}</Empty.Title><Empty.Description>{query ? '검색어를 바꾸거나 지운 뒤 다시 확인하세요.' : 'TrueNAS에서 SMB 또는 NFS 공유를 추가하면 여기에 표시됩니다.'}</Empty.Description></Empty.Header>{#if query}<Empty.Content><Button variant="outline" size="sm" onclick={() => (query = '')}>검색 초기화</Button></Empty.Content>{/if}</Empty.Root>
        {/if}
    </Card.Content></Card.Root>
    <Card.Root><Card.Header class="flex-row items-center justify-between"><div><Card.Title>Rsync 작업</Card.Title><Card.Description>{tasks.length}개 작업</Card.Description></div><Button size="sm" onclick={createRsync}><Plus />작업 추가</Button></Card.Header><Card.Content class={tasks.length ? 'divide-y' : ''}>{#each tasks as task (task.id)}<div class="flex items-center justify-between gap-4 py-3"><div class="min-w-0"><div class="flex items-center gap-2"><p class="truncate text-sm font-medium">{task.description || task.path}</p>{#if task.locked}<Badge variant="secondary">잠김</Badge>{:else if !task.enabled}<Badge variant="secondary">비활성</Badge>{/if}</div><p class="truncate text-xs text-muted-foreground">{task.direction} · {task.path} → {task.destination}</p></div><div class="flex shrink-0 gap-1"><Button variant="ghost" size="icon-sm" disabled={task.locked} aria-label={`${task.description || task.path} 수정`} onclick={() => editRsync(task)}><Pencil /></Button><Button variant="outline" size="sm" disabled={task.locked || !task.enabled || busy === `rsync-${task.id}`} onclick={() => run(task.id)}>{#if busy === `rsync-${task.id}`}<Spinner />{:else}<Play />{/if}실행</Button><Button variant="ghost" size="icon-sm" class="text-destructive hover:bg-destructive/10 hover:text-destructive" disabled={task.locked || busy === `RSYNC-${task.id}`} aria-label={`${task.description || task.path} 삭제`} onclick={() => askRemove('RSYNC', task.id, task.description || task.path)}><Trash2 /></Button></div></div>{:else}<Empty.Root class="min-h-40 border-0 p-6"><Empty.Media variant="icon"><RefreshCw /></Empty.Media><Empty.Header><Empty.Title>등록된 Rsync 작업이 없습니다</Empty.Title><Empty.Description>반복 전송 작업을 추가해 원격 시스템과 데이터를 동기화하세요.</Empty.Description></Empty.Header><Empty.Content><Button size="sm" onclick={createRsync}><Plus />첫 작업 추가</Button></Empty.Content></Empty.Root>{/each}</Card.Content></Card.Root>
</section>

<Dialog.Root bind:open={editorOpen}>
    <Dialog.Content class="max-h-[88dvh] overflow-y-auto sm:max-w-3xl">
        <form class="space-y-5" onsubmit={saveEditor}>
            <Dialog.Header>
                <Dialog.Title>{editorKind === 'rsync' ? `Rsync 작업 ${rsyncForm.id ? '수정' : '추가'}` : `${shareForm.protocol} 공유 ${shareForm.id ? '수정' : '추가'}`}</Dialog.Title>
                <Dialog.Description>저장하면 설정이 TrueNAS에 바로 반영됩니다.</Dialog.Description>
            </Dialog.Header>
            {#if editorKind === 'share' && shareForm.protocol === 'SMB'}
                <div class="grid gap-4 sm:grid-cols-2">
                    <label class="grid gap-2 text-sm font-medium">공유 이름<Input bind:value={shareForm.name} maxlength={80} required /></label>
                    <label class="grid gap-2 text-sm font-medium">용도<NativeSelect.Root class="w-full" bind:value={shareForm.purpose}><option value="DEFAULT_SHARE">일반 공유</option><option value="LEGACY_SHARE">레거시 호환 공유</option><option value="TIMEMACHINE_SHARE">Time Machine</option><option value="MULTIPROTOCOL_SHARE">다중 프로토콜</option><option value="TIME_LOCKED_SHARE">시간 잠금</option><option value="PRIVATE_DATASETS_SHARE">개인 데이터셋</option><option value="EXTERNAL_SHARE">외부 DFS</option><option value="VEEAM_REPOSITORY_SHARE">Veeam 저장소</option><option value="FCP_SHARE">Final Cut Pro</option></NativeSelect.Root></label>
                    {#if shareForm.purpose === 'EXTERNAL_SHARE'}
                        <label class="grid gap-2 text-sm font-medium sm:col-span-2">원격 공유 경로<Textarea class="min-h-20 resize-y" bind:value={shareForm.remotePath} required placeholder="서버별 경로를 한 줄에 하나씩 입력 (예: server.example.com\\share)" /></label>
                    {:else}
                        <label class="grid gap-2 text-sm font-medium sm:col-span-2">데이터셋 경로<Input bind:value={shareForm.path} required placeholder="/mnt/pool/dataset" /></label>
                    {/if}
                    <label class="grid gap-2 text-sm font-medium sm:col-span-2">설명<Input bind:value={shareForm.comment} /></label>
                </div>

                <fieldset class="grid gap-3 rounded-xl bg-muted/50 p-4 sm:grid-cols-2">
                    <legend class="px-1 text-sm font-semibold">일반 설정</legend>
                    <label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.enabled} />공유 활성화</label>
                    <label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.readOnly} />읽기 전용</label>
                    <label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.browsable} />네트워크 탐색에 표시</label>
                    <label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.accessBasedShareEnumeration} />권한 기반 열거</label>
                </fieldset>

                <fieldset class="space-y-3 rounded-xl bg-muted/50 p-4">
                    <legend class="px-1 text-sm font-semibold">감사 로그</legend>
                    <label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.auditEnabled} />파일 작업 감사 기록</label>
                    {#if shareForm.auditEnabled}
                        <div class="grid gap-4 sm:grid-cols-2"><label class="grid gap-2 text-sm font-medium">감사 대상<Textarea class="min-h-20 resize-y bg-background" bind:value={shareForm.auditWatchList} placeholder="한 줄에 하나씩 입력" /></label><label class="grid gap-2 text-sm font-medium">감사 제외 대상<Textarea class="min-h-20 resize-y bg-background" bind:value={shareForm.auditIgnoreList} placeholder="한 줄에 하나씩 입력" /></label></div>
                    {/if}
                </fieldset>

                {#if shareForm.purpose !== 'EXTERNAL_SHARE'}
                    <div class="grid gap-4 sm:grid-cols-2"><label class="grid gap-2 text-sm font-medium">허용 호스트<Textarea class="min-h-20 resize-y" bind:value={shareForm.hostsAllow} placeholder="한 줄에 하나씩 입력" /></label><label class="grid gap-2 text-sm font-medium">차단 호스트<Textarea class="min-h-20 resize-y" bind:value={shareForm.hostsDeny} placeholder="한 줄에 하나씩 입력" /></label></div>
                {/if}

                {#if ['DEFAULT_SHARE', 'MULTIPROTOCOL_SHARE'].includes(shareForm.purpose)}
                    <fieldset class="rounded-xl bg-muted/50 p-4"><legend class="px-1 text-sm font-semibold">호환성</legend><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.aaplNameMangling} />macOS 호환 이름 변환</label></fieldset>
                {:else if shareForm.purpose === 'FCP_SHARE'}
                    <div class="rounded-xl bg-muted/50 p-4 text-sm text-muted-foreground">Final Cut Pro 호환 이름 변환이 자동으로 적용됩니다.</div>
                {:else if shareForm.purpose === 'LEGACY_SHARE'}
                    <fieldset class="space-y-4 rounded-xl bg-muted/50 p-4">
                        <legend class="px-1 text-sm font-semibold">레거시 고급 설정</legend>
                        <div class="grid gap-4 sm:grid-cols-2"><label class="grid gap-2 text-sm font-medium">동적 경로 접미사<Input class="bg-background" bind:value={shareForm.pathSuffix} placeholder="예: %D/%U" /></label><label class="grid gap-2 text-sm font-medium">Time Machine 할당량 (바이트)<Input class="bg-background" type="number" min="0" bind:value={shareForm.timeMachineQuota} /></label><label class="grid gap-2 text-sm font-medium">VUID<Input class="bg-background" bind:value={shareForm.vuid} /></label><label class="grid gap-2 text-sm font-medium sm:col-span-2">추가 smb.conf 설정<Textarea class="min-h-20 resize-y bg-background" bind:value={shareForm.auxSmbConf} /></label></div>
                        <div class="grid gap-3 sm:grid-cols-3"><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.recycleBin} />휴지통</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.guestOk} />게스트 접근</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.streams} />스트림</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.durableHandle} />Durable handle</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.shadowCopy} />Shadow copy</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.fsrvp} />FSRVP</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.home} />사용자 홈</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.acl} />ACL</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.afp} />AFP 호환</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.timeMachine} />Time Machine</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.aaplNameMangling} />macOS 이름 변환</label></div>
                    </fieldset>
                {:else if shareForm.purpose === 'TIMEMACHINE_SHARE'}
                    <fieldset class="space-y-4 rounded-xl bg-muted/50 p-4"><legend class="px-1 text-sm font-semibold">Time Machine 설정</legend><div class="grid gap-4 sm:grid-cols-2"><label class="grid gap-2 text-sm font-medium">할당량 (바이트)<Input class="bg-background" type="number" min="0" bind:value={shareForm.timeMachineQuota} /></label><label class="grid gap-2 text-sm font-medium">VUID<Input class="bg-background" bind:value={shareForm.vuid} /></label><label class="grid gap-2 text-sm font-medium sm:col-span-2">데이터셋 이름 형식<Input class="bg-background" bind:value={shareForm.datasetNamingSchema} placeholder="예: %U" /></label></div><div class="grid gap-3 sm:grid-cols-2"><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.autoSnapshot} />자동 스냅샷</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.autoDatasetCreation} />데이터셋 자동 생성</label></div></fieldset>
                {:else if shareForm.purpose === 'TIME_LOCKED_SHARE'}
                    <fieldset class="grid gap-4 rounded-xl bg-muted/50 p-4 sm:grid-cols-2"><legend class="px-1 text-sm font-semibold">시간 잠금 설정</legend><label class="grid gap-2 text-sm font-medium">유예 시간 (초)<Input class="bg-background" type="number" min="60" max="15552000" bind:value={shareForm.gracePeriod} required /></label><label class="flex items-end gap-2 pb-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.aaplNameMangling} />macOS 호환 이름 변환</label></fieldset>
                {:else if shareForm.purpose === 'PRIVATE_DATASETS_SHARE'}
                    <fieldset class="grid gap-4 rounded-xl bg-muted/50 p-4 sm:grid-cols-2"><legend class="px-1 text-sm font-semibold">개인 데이터셋 설정</legend><label class="grid gap-2 text-sm font-medium">데이터셋 이름 형식<Input class="bg-background" bind:value={shareForm.datasetNamingSchema} /></label><label class="grid gap-2 text-sm font-medium">자동 할당량 (GiB)<Input class="bg-background" type="number" min="0" bind:value={shareForm.autoQuota} /></label><label class="flex items-center gap-2 text-sm sm:col-span-2"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.aaplNameMangling} />macOS 호환 이름 변환</label></fieldset>
                {/if}
            {:else if editorKind === 'share'}
                <div class="grid gap-4 sm:grid-cols-2"><label class="grid gap-2 text-sm font-medium sm:col-span-2">내보낼 경로<Input bind:value={shareForm.path} required placeholder="/mnt/pool/dataset" /></label><label class="grid gap-2 text-sm font-medium sm:col-span-2">별칭<Textarea class="min-h-20 resize-y" bind:value={shareForm.aliases} placeholder="한 줄에 하나씩 입력" /></label><label class="grid gap-2 text-sm font-medium sm:col-span-2">설명<Input bind:value={shareForm.comment} /></label><label class="grid gap-2 text-sm font-medium">허용 네트워크<Textarea class="min-h-20 resize-y" bind:value={shareForm.networks} placeholder="10.0.0.0/24" /></label><label class="grid gap-2 text-sm font-medium">허용 호스트<Textarea class="min-h-20 resize-y" bind:value={shareForm.hosts} placeholder="nas-client.local" /></label><label class="grid gap-2 text-sm font-medium">Root 매핑 사용자<Input bind:value={shareForm.mapRootUser} /></label><label class="grid gap-2 text-sm font-medium">Root 매핑 그룹<Input bind:value={shareForm.mapRootGroup} /></label><label class="grid gap-2 text-sm font-medium">전체 매핑 사용자<Input bind:value={shareForm.mapAllUser} /></label><label class="grid gap-2 text-sm font-medium">전체 매핑 그룹<Input bind:value={shareForm.mapAllGroup} /></label></div>
                <fieldset class="space-y-3 rounded-xl bg-muted/50 p-4"><legend class="px-1 text-sm font-semibold">보안 방식</legend><div class="flex flex-wrap gap-4">{#each ['SYS', 'KRB5', 'KRB5I', 'KRB5P'] as security}<label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" checked={shareForm.security.includes(security)} onchange={() => toggleSecurity(security)} />{security}</label>{/each}</div></fieldset>
                <div class="grid gap-3 rounded-xl bg-muted/50 p-4 sm:grid-cols-3"><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.enabled} />공유 활성화</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.readOnly} />읽기 전용</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.exposeSnapshots} />스냅샷 노출</label></div>
            {:else}
                <div class="grid gap-4 sm:grid-cols-2"><label class="grid gap-2 text-sm font-medium sm:col-span-2">로컬 경로<Input bind:value={rsyncForm.path} required /></label><label class="grid gap-2 text-sm font-medium">실행 사용자<Input bind:value={rsyncForm.user} required /></label><label class="grid gap-2 text-sm font-medium">전송 방향<NativeSelect.Root class="w-full" bind:value={rsyncForm.direction}><option value="PUSH">PUSH</option><option value="PULL">PULL</option></NativeSelect.Root></label><label class="grid gap-2 text-sm font-medium">연결 방식<NativeSelect.Root class="w-full" bind:value={rsyncForm.mode}><option value="MODULE">Rsync 모듈</option><option value="SSH">SSH</option></NativeSelect.Root></label><label class="grid gap-2 text-sm font-medium">원격 호스트<Input bind:value={rsyncForm.remoteHost} required /></label>{#if rsyncForm.mode === 'MODULE'}<label class="grid gap-2 text-sm font-medium sm:col-span-2">원격 모듈<Input bind:value={rsyncForm.remoteModule} required /></label>{:else}<label class="grid gap-2 text-sm font-medium">원격 경로<Input bind:value={rsyncForm.remotePath} required /></label><label class="grid gap-2 text-sm font-medium">SSH 포트<Input type="number" min="1" max="65535" bind:value={rsyncForm.remotePort} placeholder="22" /></label><label class="grid gap-2 text-sm font-medium sm:col-span-2">SSH 자격 증명 ID<Input type="number" min="1" bind:value={rsyncForm.sshCredentialId} placeholder="사용자 SSH 키 사용 시 비워 둠" /></label>{/if}<label class="grid gap-2 text-sm font-medium sm:col-span-2">설명<Input bind:value={rsyncForm.description} /></label></div>
                <fieldset class="space-y-2"><legend class="text-sm font-medium">실행 스케줄</legend><div class="grid grid-cols-5 gap-2"><label class="grid gap-1 text-xs">분<Input bind:value={rsyncForm.scheduleMinute} /></label><label class="grid gap-1 text-xs">시<Input bind:value={rsyncForm.scheduleHour} /></label><label class="grid gap-1 text-xs">일<Input bind:value={rsyncForm.scheduleDayOfMonth} /></label><label class="grid gap-1 text-xs">월<Input bind:value={rsyncForm.scheduleMonth} /></label><label class="grid gap-1 text-xs">요일<Input bind:value={rsyncForm.scheduleDayOfWeek} /></label></div></fieldset>
                <div class="grid gap-3 rounded-xl bg-muted/50 p-4 sm:grid-cols-3"><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={rsyncForm.enabled} />작업 활성화</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={rsyncForm.recursive} />하위 폴더 포함</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={rsyncForm.times} />수정 시간 보존</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={rsyncForm.compress} />전송 압축</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={rsyncForm.archive} />아카이브 모드</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={rsyncForm.delete} />대상 잉여 파일 삭제</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={rsyncForm.quiet} />정보 메시지 숨김</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={rsyncForm.preservePermissions} />권한 보존</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={rsyncForm.preserveAttributes} />확장 속성 보존</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={rsyncForm.delayUpdates} />완료 후 대상 갱신</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={rsyncForm.validateRemotePath} />원격 경로 검증</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={rsyncForm.sshKeyScan} />SSH 호스트 키 등록</label></div>
                <label class="grid gap-2 text-sm font-medium">추가 Rsync 옵션<Textarea class="min-h-20 resize-y" bind:value={rsyncForm.extra} autocorrect="off" autocapitalize="none" spellcheck={false} placeholder="한 줄에 하나씩 입력" /></label>
            {/if}
            {#if editorError}<Alert.Root variant="destructive"><Alert.Title>저장하지 못했습니다</Alert.Title><Alert.Description>{editorError}</Alert.Description></Alert.Root>{/if}
            <Dialog.Footer><Button type="button" variant="outline" disabled={busy === 'save'} onclick={() => (editorOpen = false)}>취소</Button><Button type="submit" disabled={busy === 'save'}>{#if busy === 'save'}<Spinner />{/if}{busy === 'save' ? '저장하는 중' : '저장'}</Button></Dialog.Footer>
        </form>
    </Dialog.Content>
</Dialog.Root>

<Dialog.Root bind:open={aclOpen}>
    <Dialog.Content class="max-h-[88dvh] overflow-y-auto sm:max-w-3xl">
        <form class="space-y-5" onsubmit={saveACL}>
            <Dialog.Header><Dialog.Title>{aclShareName} 공유 ACL</Dialog.Title><Dialog.Description>공유에 접근할 사용자와 그룹의 SMB 권한을 설정합니다.</Dialog.Description></Dialog.Header>
            {#if aclLoading && !aclEntries.length}<div class="flex min-h-32 items-center justify-center"><Spinner aria-label="ACL 불러오는 중" /></div>{:else}<div class="space-y-3">{#each aclEntries as entry, index (index)}<div class="grid gap-3 rounded-xl bg-muted/50 p-4 sm:grid-cols-[1fr_1fr_1fr_2fr_auto]"><label class="grid gap-1.5 text-xs font-medium">권한<NativeSelect.Root class="w-full bg-background" bind:value={entry.permission}><option value="FULL">모든 권한</option><option value="CHANGE">변경</option><option value="READ">읽기</option></NativeSelect.Root></label><label class="grid gap-1.5 text-xs font-medium">규칙<NativeSelect.Root class="w-full bg-background" bind:value={entry.entryType}><option value="ALLOWED">허용</option><option value="DENIED">거부</option></NativeSelect.Root></label><label class="grid gap-1.5 text-xs font-medium">대상 유형<NativeSelect.Root class="w-full bg-background" value={aclIdentityKind(entry)} onchange={(event) => setACLIdentityKind(index, event.currentTarget.value)}><option value="NAME">이름</option><option value="SID">SID</option><option value="USER_ID">사용자 ID</option><option value="GROUP_ID">그룹 ID</option></NativeSelect.Root></label><label class="grid gap-1.5 text-xs font-medium">사용자 또는 그룹<Input class="bg-background" type={entry.hasId ? 'number' : 'text'} min={entry.hasId ? 0 : undefined} value={aclIdentityValue(entry)} oninput={(event) => setACLIdentityValue(index, event.currentTarget.value)} required /></label><div class="flex items-end"><Button type="button" variant="ghost" size="icon-sm" class="text-destructive hover:bg-destructive/10 hover:text-destructive" aria-label="ACL 항목 삭제" onclick={() => removeACLEntry(index)}><Trash2 /></Button></div></div>{/each}</div>{/if}
            <Button type="button" variant="outline" size="sm" onclick={addACLEntry}><Plus />ACL 항목 추가</Button>
            {#if aclError}<Alert.Root variant="destructive"><Alert.Title>ACL을 처리하지 못했습니다</Alert.Title><Alert.Description>{aclError}</Alert.Description></Alert.Root>{/if}
            <Dialog.Footer><Button type="button" variant="outline" disabled={aclLoading} onclick={() => (aclOpen = false)}>취소</Button><Button type="submit" disabled={aclLoading || !aclEntries.length}>{#if aclLoading}<Spinner />{/if}ACL 저장</Button></Dialog.Footer>
        </form>
    </Dialog.Content>
</Dialog.Root>

<ConfirmActionDialog
    bind:open={deleteDialogOpen}
    title={`“${deleteTarget?.name ?? ''}” ${deleteTarget?.protocol === 'RSYNC' ? '작업을' : '공유를'} 삭제할까요?`}
    description={deleteTarget?.protocol === 'RSYNC' ? '예약된 동기화 작업이 삭제됩니다. 저장된 데이터 자체는 삭제되지 않습니다.' : '클라이언트의 공유 연결이 끊어집니다. 저장된 데이터 자체는 삭제되지 않습니다.'}
    confirmLabel={deleteTarget?.protocol === 'RSYNC' ? '작업 삭제' : '공유 삭제'}
    busy={deleteTarget ? busy === `${deleteTarget.protocol}-${deleteTarget.id}` : false}
    onconfirm={remove}
/>
