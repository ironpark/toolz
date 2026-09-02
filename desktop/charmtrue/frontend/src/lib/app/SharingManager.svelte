<script lang="ts">
    import { onMount } from 'svelte';
    import { Ellipsis, FolderX, Pencil, Play, RefreshCw, Trash2 } from '@lucide/svelte';
    import * as Alert from '$lib/components/ui/alert';
    import { Badge } from '$lib/components/ui/badge';
    import { Button } from '$lib/components/ui/button';
    import * as Card from '$lib/components/ui/card';
    import * as Dialog from '$lib/components/ui/dialog';
    import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
    import * as Empty from '$lib/components/ui/empty';
    import { Input } from '$lib/components/ui/input';
    import * as NativeSelect from '$lib/components/ui/native-select';
    import { Spinner } from '$lib/components/ui/spinner';
    import * as Table from '$lib/components/ui/table';
    import { Textarea } from '$lib/components/ui/textarea';
    import ConfirmActionDialog from './ConfirmActionDialog.svelte';
    import { getAppContext } from './context.svelte';
    import type { RsyncTaskInfo, ShareInfo } from '../../../bindings/github.com/ironpark/toolz/desktop/charmtrue';

    type EditorKind = 'share' | 'rsync';
    type ShareForm = {
        id: number; protocol: string; name: string; path: string; purpose: string; comment: string;
        enabled: boolean; readOnly: boolean; browsable: boolean; accessBasedShareEnumeration: boolean;
        recycleBin: boolean; pathSuffix: string; hostsAllow: string; hostsDeny: string; home: boolean;
        networks: string; hosts: string; mapRootUser: string; mapRootGroup: string; mapAllUser: string;
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

    onMount(() => { if (!app.sharing) void app.refreshSharing(); });
    const shares = $derived((app.sharing?.shares ?? []).filter((share) => !query || `${share.name} ${share.path} ${share.protocol}`.toLowerCase().includes(query.toLowerCase())));
    const tasks = $derived(app.sharing?.rsyncTasks ?? []);

    function emptyShareForm(): ShareForm {
        return { id: 0, protocol: 'SMB', name: '', path: '', purpose: 'DEFAULT_SHARE', comment: '', enabled: true, readOnly: false, browsable: true, accessBasedShareEnumeration: false, recycleBin: false, pathSuffix: '', hostsAllow: '', hostsDeny: '', home: false, networks: '', hosts: '', mapRootUser: '', mapRootGroup: '', mapAllUser: '', mapAllGroup: '', security: ['SYS'], exposeSnapshots: false };
    }

    function emptyRsyncForm(): RsyncForm {
        return { id: 0, path: '', user: '', mode: 'MODULE', remoteHost: '', remotePort: '', remoteModule: '', remotePath: '', sshCredentialId: '', direction: 'PUSH', description: '', scheduleMinute: '00', scheduleHour: '*', scheduleDayOfMonth: '*', scheduleMonth: '*', scheduleDayOfWeek: '*', recursive: true, times: true, compress: false, archive: false, delete: false, quiet: false, preservePermissions: false, preserveAttributes: false, delayUpdates: true, extra: '', enabled: true, validateRemotePath: true, sshKeyScan: false };
    }

    function lines(value: string): string[] {
        return value.split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
    }

    function editShare(share: ShareInfo): void {
        editorKind = 'share'; editorError = '';
        shareForm = {
            id: share.id, protocol: share.protocol, name: share.name, path: share.path,
            purpose: share.purpose || 'DEFAULT_SHARE', comment: share.comment, enabled: share.enabled,
            readOnly: share.readOnly, browsable: share.browsable, accessBasedShareEnumeration: share.accessBasedShareEnumeration,
            recycleBin: share.recycleBin, pathSuffix: share.pathSuffix, hostsAllow: (share.hostsAllow ?? []).join('\n'),
            hostsDeny: (share.hostsDeny ?? []).join('\n'), home: share.home, networks: (share.networks ?? []).join('\n'),
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
                await app.saveShare({ ...shareForm, hostsAllow: lines(shareForm.hostsAllow), hostsDeny: lines(shareForm.hostsDeny), networks: lines(shareForm.networks), hosts: lines(shareForm.hosts) });
            } else {
                await app.saveRsyncTask({ ...rsyncForm, remotePort: Number(rsyncForm.remotePort || 0), sshCredentialId: Number(rsyncForm.sshCredentialId || 0), extra: lines(rsyncForm.extra) });
            }
            editorOpen = false;
        } catch (error) { editorError = error instanceof Error ? error.message : String(error); }
        finally { busy = ''; }
    }

    async function toggle(protocol: string, id: number, enabled: boolean): Promise<void> { busy = `${protocol}-${id}`; await app.setShareEnabled(protocol, id, enabled); busy = ''; }
    function askRemove(protocol: string, id: number, name: string): void { deleteTarget = { protocol, id, name }; deleteDialogOpen = true; }
    async function remove(): Promise<void> { if (!deleteTarget) return; busy = `${deleteTarget.protocol}-${deleteTarget.id}`; try { await app.deleteShare(deleteTarget.protocol, deleteTarget.id); } finally { busy = ''; deleteTarget = null; } }
    async function run(id: number): Promise<void> { busy = `rsync-${id}`; await app.runRsyncTask(id); busy = ''; }
</script>

<section class="space-y-6">
    <header class="flex items-end justify-between"><div><h2 class="text-3xl font-semibold tracking-tight">공유 서비스</h2><p class="mt-1 text-sm text-muted-foreground">SMB, NFS 공유와 Rsync 작업을 제어합니다.</p></div><Button variant="outline" disabled={app.sharingLoading} onclick={() => app.refreshSharing()}>{#if app.sharingLoading}<Spinner aria-label="공유 새로고침 중" />{:else}<RefreshCw />{/if}새로고침</Button></header>
    {#if app.sharingError}<Alert.Root variant="destructive"><Alert.Title>조회 실패</Alert.Title><Alert.Description>{app.sharingError}</Alert.Description></Alert.Root>{/if}
    <Card.Root><Card.Header class="flex flex-row items-center justify-between"><div><Card.Title>파일 공유</Card.Title><Card.Description>{shares.length}개 항목</Card.Description></div><Input class="max-w-xs" bind:value={query} placeholder="이름, 경로, 프로토콜 검색" /></Card.Header><Card.Content>
        {#if shares.length}
            <Table.Root><Table.Header><Table.Row><Table.Head>이름</Table.Head><Table.Head>프로토콜</Table.Head><Table.Head>상태</Table.Head><Table.Head class="w-14 text-right"><span class="sr-only">작업</span></Table.Head></Table.Row></Table.Header><Table.Body>{#each shares as share (share.protocol + share.id)}<Table.Row><Table.Cell><p class="font-medium">{share.name || share.path}</p><p class="text-xs text-muted-foreground">{share.path}</p></Table.Cell><Table.Cell><Badge variant="outline">{share.protocol}</Badge></Table.Cell><Table.Cell>{share.locked ? '잠김' : share.enabled ? '활성' : '비활성'}</Table.Cell><Table.Cell class="text-right"><DropdownMenu.Root><DropdownMenu.Trigger>{#snippet child({ props })}<Button {...props} variant="ghost" size="icon-sm" aria-label={`${share.name || share.path} 작업 메뉴`}><Ellipsis /></Button>{/snippet}</DropdownMenu.Trigger><DropdownMenu.Content align="end" class="w-40"><DropdownMenu.Label>{share.protocol} 공유</DropdownMenu.Label><DropdownMenu.Item disabled={busy === `${share.protocol}-${share.id}` || share.locked} onclick={() => toggle(share.protocol, share.id, !share.enabled)}>{share.enabled ? '비활성화' : '활성화'}</DropdownMenu.Item><DropdownMenu.Item disabled={share.locked} onclick={() => editShare(share)}><Pencil />수정</DropdownMenu.Item><DropdownMenu.Separator /><DropdownMenu.Item variant="destructive" disabled={busy === `${share.protocol}-${share.id}`} onclick={() => askRemove(share.protocol, share.id, share.name || share.path)}><Trash2 />삭제</DropdownMenu.Item></DropdownMenu.Content></DropdownMenu.Root></Table.Cell></Table.Row>{/each}</Table.Body></Table.Root>
        {:else}
            <Empty.Root class="min-h-48 border-0 p-6"><Empty.Media variant="icon"><FolderX /></Empty.Media><Empty.Header><Empty.Title>{query ? '일치하는 공유가 없습니다' : '등록된 파일 공유가 없습니다'}</Empty.Title><Empty.Description>{query ? '검색어를 바꾸거나 지운 뒤 다시 확인하세요.' : 'TrueNAS에서 SMB 또는 NFS 공유를 추가하면 여기에 표시됩니다.'}</Empty.Description></Empty.Header>{#if query}<Empty.Content><Button variant="outline" size="sm" onclick={() => (query = '')}>검색 초기화</Button></Empty.Content>{/if}</Empty.Root>
        {/if}
    </Card.Content></Card.Root>
    <Card.Root><Card.Header><Card.Title>Rsync 작업</Card.Title></Card.Header><Card.Content class={tasks.length ? 'divide-y' : ''}>{#each tasks as task (task.id)}<div class="flex items-center justify-between gap-4 py-3"><div><p class="text-sm font-medium">{task.description || task.path}</p><p class="text-xs text-muted-foreground">{task.direction} · {task.path} → {task.destination}</p></div><div class="flex gap-1"><Button variant="ghost" size="icon-sm" aria-label={`${task.description || task.path} 수정`} onclick={() => editRsync(task)}><Pencil /></Button><Button variant="outline" size="sm" disabled={!task.enabled || busy === `rsync-${task.id}`} onclick={() => run(task.id)}>{#if busy === `rsync-${task.id}`}<Spinner />{:else}<Play />{/if}실행</Button></div></div>{:else}<Empty.Root class="min-h-40 border-0 p-6"><Empty.Media variant="icon"><RefreshCw /></Empty.Media><Empty.Header><Empty.Title>등록된 Rsync 작업이 없습니다</Empty.Title><Empty.Description>TrueNAS에서 만든 작업이 여기에 표시됩니다.</Empty.Description></Empty.Header></Empty.Root>{/each}</Card.Content></Card.Root>
</section>

<Dialog.Root bind:open={editorOpen}>
    <Dialog.Content class="max-h-[88dvh] overflow-y-auto sm:max-w-2xl">
        <form class="space-y-5" onsubmit={saveEditor}>
            <Dialog.Header><Dialog.Title>{editorKind === 'rsync' ? 'Rsync 작업 수정' : `${shareForm.protocol} 공유 수정`}</Dialog.Title><Dialog.Description>변경한 설정은 TrueNAS 서비스에 바로 반영됩니다.</Dialog.Description></Dialog.Header>
            {#if editorKind === 'share' && shareForm.protocol === 'SMB'}
                <div class="grid gap-4 sm:grid-cols-2"><label class="grid gap-2 text-sm font-medium">공유 이름<Input bind:value={shareForm.name} maxlength={80} required /></label><label class="grid gap-2 text-sm font-medium">용도<NativeSelect.Root class="w-full" bind:value={shareForm.purpose}><option value="DEFAULT_SHARE">일반 공유</option><option value="LEGACY_SHARE">레거시 호환 공유</option><option value="TIMEMACHINE_SHARE">Time Machine</option><option value="MULTIPROTOCOL_SHARE">다중 프로토콜</option><option value="TIME_LOCKED_SHARE">시간 잠금</option><option value="PRIVATE_DATASETS_SHARE">개인 데이터셋</option><option value="EXTERNAL_SHARE">외부 DFS</option><option value="VEEAM_REPOSITORY_SHARE">Veeam 저장소</option><option value="FCP_SHARE">Final Cut Pro</option></NativeSelect.Root></label><label class="grid gap-2 text-sm font-medium sm:col-span-2">경로<Input bind:value={shareForm.path} required /></label><label class="grid gap-2 text-sm font-medium sm:col-span-2">설명<Input bind:value={shareForm.comment} /></label><label class="grid gap-2 text-sm font-medium">동적 경로 접미사<Input bind:value={shareForm.pathSuffix} placeholder="예: %D/%U" /></label></div>
                <div class="grid gap-3 rounded-lg border p-4 sm:grid-cols-2"><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.enabled} />공유 활성화</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.readOnly} />읽기 전용</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.browsable} />네트워크 탐색에 표시</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.accessBasedShareEnumeration} />권한 기반 열거</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.recycleBin} />휴지통 사용</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.home} />사용자 홈 공유</label></div>
                <div class="grid gap-4 sm:grid-cols-2"><label class="grid gap-2 text-sm font-medium">허용 호스트<Textarea class="min-h-20 resize-y" bind:value={shareForm.hostsAllow} autocorrect="off" autocapitalize="none" spellcheck={false} placeholder="한 줄에 하나씩 입력" /></label><label class="grid gap-2 text-sm font-medium">차단 호스트<Textarea class="min-h-20 resize-y" bind:value={shareForm.hostsDeny} autocorrect="off" autocapitalize="none" spellcheck={false} placeholder="한 줄에 하나씩 입력" /></label></div>
            {:else if editorKind === 'share'}
                <div class="grid gap-4 sm:grid-cols-2"><label class="grid gap-2 text-sm font-medium sm:col-span-2">내보낼 경로<Input bind:value={shareForm.path} required /></label><label class="grid gap-2 text-sm font-medium sm:col-span-2">설명<Input bind:value={shareForm.comment} /></label><label class="grid gap-2 text-sm font-medium">허용 네트워크<Textarea class="min-h-20 resize-y" bind:value={shareForm.networks} autocorrect="off" autocapitalize="none" spellcheck={false} placeholder="10.0.0.0/24" /></label><label class="grid gap-2 text-sm font-medium">허용 호스트<Textarea class="min-h-20 resize-y" bind:value={shareForm.hosts} autocorrect="off" autocapitalize="none" spellcheck={false} placeholder="nas-client.local" /></label><label class="grid gap-2 text-sm font-medium">Root 매핑 사용자<Input bind:value={shareForm.mapRootUser} /></label><label class="grid gap-2 text-sm font-medium">Root 매핑 그룹<Input bind:value={shareForm.mapRootGroup} /></label><label class="grid gap-2 text-sm font-medium">전체 매핑 사용자<Input bind:value={shareForm.mapAllUser} /></label><label class="grid gap-2 text-sm font-medium">전체 매핑 그룹<Input bind:value={shareForm.mapAllGroup} /></label></div>
                <fieldset class="grid gap-2"><legend class="text-sm font-medium">보안 방식</legend><div class="flex flex-wrap gap-3 rounded-lg border p-3">{#each ['SYS', 'KRB5', 'KRB5I', 'KRB5P'] as security}<label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" checked={shareForm.security.includes(security)} onchange={() => toggleSecurity(security)} />{security}</label>{/each}</div></fieldset>
                <div class="grid gap-3 rounded-lg border p-4 sm:grid-cols-3"><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.enabled} />공유 활성화</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.readOnly} />읽기 전용</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={shareForm.exposeSnapshots} />스냅샷 노출</label></div>
            {:else}
                <div class="grid gap-4 sm:grid-cols-2"><label class="grid gap-2 text-sm font-medium sm:col-span-2">로컬 경로<Input bind:value={rsyncForm.path} required /></label><label class="grid gap-2 text-sm font-medium">실행 사용자<Input bind:value={rsyncForm.user} required /></label><label class="grid gap-2 text-sm font-medium">전송 방향<NativeSelect.Root class="w-full" bind:value={rsyncForm.direction}><option value="PUSH">PUSH</option><option value="PULL">PULL</option></NativeSelect.Root></label><label class="grid gap-2 text-sm font-medium">연결 방식<NativeSelect.Root class="w-full" bind:value={rsyncForm.mode}><option value="MODULE">Rsync 모듈</option><option value="SSH">SSH</option></NativeSelect.Root></label><label class="grid gap-2 text-sm font-medium">원격 호스트<Input bind:value={rsyncForm.remoteHost} required /></label>{#if rsyncForm.mode === 'MODULE'}<label class="grid gap-2 text-sm font-medium sm:col-span-2">원격 모듈<Input bind:value={rsyncForm.remoteModule} required /></label>{:else}<label class="grid gap-2 text-sm font-medium">원격 경로<Input bind:value={rsyncForm.remotePath} required /></label><label class="grid gap-2 text-sm font-medium">SSH 포트<Input type="number" min="1" max="65535" bind:value={rsyncForm.remotePort} placeholder="22" /></label><label class="grid gap-2 text-sm font-medium sm:col-span-2">SSH 자격 증명 ID<Input type="number" min="1" bind:value={rsyncForm.sshCredentialId} placeholder="사용자 SSH 키 사용 시 비워 둠" /></label>{/if}<label class="grid gap-2 text-sm font-medium sm:col-span-2">설명<Input bind:value={rsyncForm.description} /></label></div>
                <fieldset class="space-y-2"><legend class="text-sm font-medium">실행 스케줄</legend><div class="grid grid-cols-5 gap-2"><label class="grid gap-1 text-xs">분<Input bind:value={rsyncForm.scheduleMinute} /></label><label class="grid gap-1 text-xs">시<Input bind:value={rsyncForm.scheduleHour} /></label><label class="grid gap-1 text-xs">일<Input bind:value={rsyncForm.scheduleDayOfMonth} /></label><label class="grid gap-1 text-xs">월<Input bind:value={rsyncForm.scheduleMonth} /></label><label class="grid gap-1 text-xs">요일<Input bind:value={rsyncForm.scheduleDayOfWeek} /></label></div></fieldset>
                <div class="grid gap-3 rounded-lg border p-4 sm:grid-cols-3"><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={rsyncForm.enabled} />작업 활성화</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={rsyncForm.recursive} />하위 폴더 포함</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={rsyncForm.times} />수정 시간 보존</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={rsyncForm.compress} />전송 압축</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={rsyncForm.archive} />아카이브 모드</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={rsyncForm.delete} />대상 잉여 파일 삭제</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={rsyncForm.quiet} />정보 메시지 숨김</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={rsyncForm.preservePermissions} />권한 보존</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={rsyncForm.preserveAttributes} />확장 속성 보존</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={rsyncForm.delayUpdates} />완료 후 대상 갱신</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={rsyncForm.validateRemotePath} />원격 경로 검증</label><label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={rsyncForm.sshKeyScan} />SSH 호스트 키 등록</label></div>
                <label class="grid gap-2 text-sm font-medium">추가 Rsync 옵션<Textarea class="min-h-20 resize-y" bind:value={rsyncForm.extra} autocorrect="off" autocapitalize="none" spellcheck={false} placeholder="한 줄에 하나씩 입력" /></label>
            {/if}
            {#if editorError}<Alert.Root variant="destructive"><Alert.Title>수정하지 못했습니다</Alert.Title><Alert.Description>{editorError}</Alert.Description></Alert.Root>{/if}
            <Dialog.Footer><Button type="button" variant="outline" disabled={busy === 'save'} onclick={() => (editorOpen = false)}>취소</Button><Button type="submit" disabled={busy === 'save'}>{#if busy === 'save'}<Spinner />{/if}{busy === 'save' ? '저장하는 중' : '변경 저장'}</Button></Dialog.Footer>
        </form>
    </Dialog.Content>
</Dialog.Root>

<ConfirmActionDialog
    bind:open={deleteDialogOpen}
    title={`“${deleteTarget?.name ?? ''}” 공유를 삭제할까요?`}
    description="클라이언트의 공유 연결이 끊어집니다. 저장된 데이터 자체는 삭제되지 않습니다."
    confirmLabel="공유 삭제"
    busy={deleteTarget ? busy === `${deleteTarget.protocol}-${deleteTarget.id}` : false}
    onconfirm={remove}
/>
