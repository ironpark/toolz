<script lang="ts">
    import { onMount } from 'svelte';
    import { Plus, Save, Play, Trash2 } from '@lucide/svelte';
    import { Button } from '$lib/components/ui/button';
    import { Checkbox } from '$lib/components/ui/checkbox';
    import { Input } from '$lib/components/ui/input';
    import { Spinner } from '$lib/components/ui/spinner';
    import * as Card from '$lib/components/ui/card';
    import * as NativeSelect from '$lib/components/ui/native-select';
    import * as Dialog from '$lib/components/ui/dialog';
    import { TrueNASService, type SetupPreset, type SetupPresetResult } from '../../../bindings/github.com/ironpark/toolz/desktop/charmtrue';
    import { getAppContext } from './context.svelte';
    import { cleanPreset, presetStorageKey, readPresets, serializePresets } from './setup-presets';
    const app = getAppContext();
    const empty = (): SetupPreset => ({ id: '', name: '', groups: [], users: [], enableSSH: false, adminUser: '', replaceAdminUser: '' });
    let presets = $state<SetupPreset[]>([]), draft = $state<SetupPreset>(empty()), groupText = $state('');
    let error = $state(''), message = $state(''), storageReady = $state(false), busy = $state(false);
    let confirmOpen = $state(false), pending = $state<SetupPreset | null>(null), endpoint = $state(''), acknowledged = $state(false);
    let passwords = $state<Record<string, string>>({}), confirmations = $state<Record<string, string>>({}), adminPassword = $state(''), adminConfirm = $state('');
    let result = $state<SetupPresetResult | null>(null);
    let replacementPassword = $state(''), replacementConfirm = $state('');
    const groupNames = $derived(groupText.split(/\r?\n/).map(name => name.trim()).filter(Boolean));
    onMount(() => {
        try { presets = readPresets(localStorage.getItem(presetStorageKey)); storageReady = true; }
        catch { error = '프리셋 저장소를 읽지 못했습니다. 기존 데이터를 보호하기 위해 저장을 비활성화했습니다.'; }
    });
    function selectPreset(id: string): void {
        const p = presets.find(p => p.id === id);
        draft = p ? cleanPreset(p) : empty(); groupText = (draft.groups ?? []).join('\n'); error = ''; message = ''; result = null;
    }
    function current(): SetupPreset {
        const p = cleanPreset({ ...draft, groups: groupNames });
        if (!p.name) throw new Error('프리셋 이름을 입력하세요.');
        if (!(p.groups?.length || p.users?.length || p.enableSSH || p.adminUser || p.replaceAdminUser)) throw new Error('하나 이상의 설정을 추가하세요.');
        if (p.replaceAdminUser && (p.adminUser || ['root', 'truenas_admin'].includes(p.replaceAdminUser) || p.users?.some(user => user.name === p.replaceAdminUser))) throw new Error('새 관리자 이름은 root·truenas_admin·일반 생성 사용자와 달라야 합니다. 기존 관리자 비밀번호 변경 항목도 비워두세요.');
        if (new Set(p.groups).size !== p.groups?.length || new Set(p.users?.map(u => u.name)).size !== p.users?.length) throw new Error('중복된 사용자·그룹 이름을 확인하세요.');
        if (p.users?.some(u => !u.name || !u.group)) throw new Error('사용자 이름과 기본 그룹을 입력하세요.');
        return p;
    }
    function save(): void {
        error = ''; message = '';
        try {
            const p = current(); if (!p.id) p.id = crypto.randomUUID();
            const next = [...presets.filter(item => item.id !== p.id), p];
            localStorage.setItem(presetStorageKey, serializePresets(next)); presets = next; draft = cleanPreset(p); message = '이 앱에 프리셋을 저장했습니다. 비밀번호는 저장하지 않습니다.';
        } catch (e) { error = e instanceof Error ? e.message : String(e); }
    }
    async function prepare(): Promise<void> {
        error = ''; result = null;
        try {
            pending = current(); endpoint = app.connection?.endpoint ?? '';
            if (!app.connection?.connected) throw new Error('적용할 서버를 먼저 연결하세요.');
            passwords = {}; confirmations = {}; adminPassword = ''; adminConfirm = ''; acknowledged = false;
            replacementPassword = ''; replacementConfirm = '';
            busy = true; await app.refreshIdentity();
            if (app.identityError || !app.identity || app.connection?.endpoint !== endpoint) throw new Error('대상 서버의 계정 목록을 확인하지 못했습니다. 연결과 조회 권한을 확인하세요.');
            confirmOpen = true;
        } catch (e) { error = e instanceof Error ? e.message : String(e); }
        finally { busy = false; }
    }
    async function apply(): Promise<void> {
        if (!pending || !acknowledged || busy) return;
        error = '';
        for (const user of pending.users ?? []) {
            if (app.identity?.users?.some(item => item.username === user.name)) continue;
            if (!passwords[user.name] || passwords[user.name] !== confirmations[user.name]) { error = `${user.name}의 비밀번호와 확인을 입력하세요.`; return; }
        }
        if (pending.adminUser && (!adminPassword || adminPassword !== adminConfirm)) { error = '관리자 새 비밀번호와 확인이 일치해야 합니다.'; return; }
        if (pending.replaceAdminUser && (!replacementPassword || replacementPassword !== replacementConfirm)) { error = '교체할 새 관리자 비밀번호와 확인이 일치해야 합니다.'; return; }
        busy = true;
        try {
            result = await TrueNASService.ApplySetupPreset({ preset: pending, expectedEndpoint: endpoint, passwords, adminPassword, replacementPassword });
            confirmOpen = false;
            void app.refreshIdentity(); void app.refreshSystem();
        } catch (e) { error = e instanceof Error ? e.message : String(e); }
        finally { busy = false; passwords = {}; confirmations = {}; adminPassword = ''; adminConfirm = ''; replacementPassword = ''; replacementConfirm = ''; }
    }
    function closeConfirmation(): void { if (!busy) { confirmOpen = false; passwords = {}; confirmations = {}; adminPassword = ''; adminConfirm = ''; replacementPassword = ''; replacementConfirm = ''; } }
</script>

<section class="space-y-5">
    <p class="text-sm text-muted-foreground">서버 초기 설정을 프리셋으로 저장하고 현재 연결된 서버에 적용합니다. 저장된 프리셋은 이 앱에서 다른 서버에도 재사용할 수 있습니다.</p>
    <div class="flex flex-wrap gap-2"><NativeSelect.Root aria-label="저장된 프리셋" value={draft.id} disabled={busy} onchange={event => selectPreset(event.currentTarget.value)}><option value="">새 프리셋</option>{#each presets as preset}<option value={preset.id}>{preset.name}</option>{/each}</NativeSelect.Root><Button variant="outline" disabled={busy} onclick={() => selectPreset('')}><Plus />새로 만들기</Button></div>
    <fieldset disabled={busy} class="min-w-0 space-y-5">
        <Card.Root class="border-amber-500/30"><Card.Header><Card.Title>시스템 관리자 교체</Card.Title><Card.Description>truenas_admin → 지정한 새 관리자 · 사용하지 않으면 비워두세요.</Card.Description></Card.Header><Card.Content class="space-y-3"><label class="grid gap-2 text-sm">새 관리자 아이디<Input bind:value={draft.replaceAdminUser} placeholder="예: nas_admin" autocomplete="off" /></label><p class="text-xs text-muted-foreground">새 계정 생성 → 그룹·sudo 권한 복제 및 FULL_ADMIN 부여 → 새 계정 로그인/API 검증 → truenas_admin 잠금 및 API 키 폐기 순서로 처리합니다. 기존 이름은 덮어쓰지 않습니다.</p><p class="text-xs text-muted-foreground">비밀번호는 적용할 때 입력합니다. 개인 SSH 키·MFA·파일 소유권은 복제하지 않으며 기존 로그인 세션은 강제 종료하지 않습니다. 작업 후 새 계정으로 다시 연결하세요.</p></Card.Content></Card.Root>
        <label class="grid gap-2 text-sm">프리셋 이름<Input bind:value={draft.name} placeholder="예: 파일 서버 기본 세팅" /></label>
        <div class="grid items-start gap-5 lg:grid-cols-2">
            <Card.Root><Card.Header><Card.Title>그룹 생성</Card.Title><Card.Description>한 줄에 하나 · 기존 그룹은 그대로 유지합니다.</Card.Description></Card.Header><Card.Content><label class="grid gap-2 text-sm">그룹 이름<textarea class="min-h-32 rounded-md border bg-transparent p-3 text-sm" bind:value={groupText} placeholder={'family\nmedia'}></textarea></label></Card.Content></Card.Root>
            <Card.Root><Card.Header><Card.Title>서버 설정</Card.Title></Card.Header><Card.Content class="space-y-4"><label class="flex items-center gap-2 text-sm"><Checkbox bind:checked={draft.enableSSH} disabled={busy} />SSH 실행 + 부팅 시 자동 시작</label><p class="text-xs text-muted-foreground">SSH 인증 방식·사용자 로그인 권한은 변경하지 않습니다.</p><label class="grid gap-2 text-sm">비밀번호를 변경할 관리자 계정<Input bind:value={draft.adminUser} placeholder="변경하지 않으면 비워두세요" /></label><p class="text-xs text-muted-foreground">기존 로컬 계정만 가능 · 새 비밀번호는 적용할 때 입력합니다.</p></Card.Content></Card.Root>
        </div>
        <Card.Root><Card.Header class="flex flex-wrap justify-between gap-2"><div><Card.Title>사용자 생성</Card.Title><Card.Description>기존 사용자는 비밀번호·그룹을 포함해 변경하지 않습니다.</Card.Description></div><Button variant="outline" size="sm" disabled={busy} onclick={() => draft.users = [...(draft.users ?? []), { name: '', fullName: '', group: groupNames[0] ?? '', smb: true }]}><Plus />사용자 추가</Button></Card.Header><Card.Content class="space-y-3">{#each draft.users ?? [] as user, index}<div class="grid items-end gap-3 rounded-lg border p-3 sm:grid-cols-2 lg:grid-cols-[1fr_1fr_1fr_auto_auto]"><label class="grid gap-2 text-sm">사용자 이름<Input bind:value={user.name} /></label><label class="grid gap-2 text-sm">표시 이름<Input bind:value={user.fullName} /></label><label class="grid gap-2 text-sm">기본 그룹<Input bind:value={user.group} placeholder="프리셋 또는 기존 일반 그룹" /></label><label class="flex h-9 items-center gap-2 text-sm"><Checkbox bind:checked={user.smb} disabled={busy} />SMB</label><Button variant="ghost" size="icon-sm" aria-label={`사용자 ${index + 1} 제거`} disabled={busy} onclick={() => draft.users = draft.users?.filter((_, i) => i !== index) ?? []}><Trash2 /></Button></div>{:else}<p class="py-4 text-sm text-muted-foreground">생성할 사용자가 없습니다.</p>{/each}<p class="text-xs text-muted-foreground">관리자·SSH 권한을 별도로 추가하거나 홈 폴더를 생성하지 않습니다. 기존 그룹을 지정하면 그 그룹의 권한을 물려받으므로 구성과 권한을 확인하세요.</p></Card.Content></Card.Root>
    </fieldset>
    <div class="flex flex-wrap justify-end gap-2"><Button variant="outline" disabled={busy || !storageReady} onclick={save}><Save />프리셋 저장</Button><Button disabled={busy || !app.connection?.connected} onclick={prepare}>{#if busy}<Spinner />{:else}<Play />{/if}현재 서버에 적용…</Button></div>
    {#if !app.connection?.connected}<Button variant="outline" onclick={() => app.openConnectModal()}>적용할 서버 연결</Button>{/if}
    {#if message}<p role="status" class="text-sm text-muted-foreground">{message}</p>{/if}
    {#if result}<Card.Root><Card.Header><Card.Title>{result.complete ? '프리셋 적용 완료' : '일부 적용 후 중단'}</Card.Title><Card.Description>대상: {endpoint} · 완료된 작업은 자동으로 되돌리지 않습니다.</Card.Description></Card.Header><Card.Content class="space-y-2">{#each result.steps ?? [] as step}<div class="rounded-lg border p-3 text-sm"><p class={step.status === 'failed' ? 'font-medium text-destructive' : 'font-medium'}>{step.status === 'done' ? '완료' : step.status === 'skipped' ? '유지' : '실패'} · {step.name}</p><p class="mt-1 text-xs text-muted-foreground">{step.message}</p></div>{/each}{#if pending?.adminUser}<p class="text-xs text-muted-foreground">비밀번호가 변경되면 기존 저장 로그인 정보로 연결하지 못할 수 있습니다. 새 비밀번호로 다시 연결해 저장하세요.</p>{/if}</Card.Content></Card.Root>{/if}
    {#if error && !confirmOpen}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}
</section>

<Dialog.Root open={confirmOpen} onOpenChange={value => { if (!value) closeConfirmation(); }}><Dialog.Content class="max-h-[88dvh] overflow-y-auto sm:max-w-2xl" showCloseButton={!busy}><Dialog.Header><Dialog.Title>프리셋 적용 확인</Dialog.Title><Dialog.Description class="break-all">{pending?.name} → {endpoint}</Dialog.Description></Dialog.Header>
    <p class="text-sm">그룹 생성 → 사용자 생성 → SSH 설정 → 관리자 비밀번호 변경 순서로 적용합니다. 실패 시 이후 작업을 중단합니다.</p>
    {#if pending?.replaceAdminUser}<div class="space-y-3 rounded-lg border border-amber-500/40 p-4"><p class="text-sm font-semibold">시스템 관리자 교체: truenas_admin → {pending.replaceAdminUser}</p><p class="text-xs text-muted-foreground">다른 설정 적용 후 새 계정 생성·권한·로그인을 검증하고 기존 관리자를 비활성화합니다. 검증 실패 시 기존 관리자는 유지되지만 새 계정은 남을 수 있습니다. 기존 로그인 세션과 개인 SSH 키·MFA·파일 소유권은 이전되지 않습니다.</p><div class="grid gap-2 sm:grid-cols-2"><Input type="password" aria-label="교체 관리자 비밀번호" placeholder="새 관리자 비밀번호" autocomplete="new-password" bind:value={replacementPassword} disabled={busy} /><Input type="password" aria-label="교체 관리자 비밀번호 확인" placeholder="비밀번호 확인" autocomplete="new-password" bind:value={replacementConfirm} disabled={busy} /></div></div>{/if}
    <p class="text-xs text-muted-foreground">그룹: {pending?.groups?.join(', ') || '없음'} · SSH: {pending?.enableSSH ? '실행 및 자동 시작' : '변경 없음'} · 관리자 비밀번호: {pending?.adminUser || '변경 없음'}</p>
    {#each pending?.users ?? [] as user}{@const exists = app.identity?.users?.some(item => item.username === user.name)}<div class="space-y-2 rounded-lg border p-3"><p class="text-sm font-medium">{user.name} · {exists ? '기존 사용자 유지' : `새로 생성 · 그룹 ${user.group} · SMB ${user.smb ? '사용' : '미사용'}`}</p>{#if !exists}<div class="grid gap-2 sm:grid-cols-2"><Input type="password" aria-label={`${user.name} 비밀번호`} placeholder="새 비밀번호" autocomplete="new-password" bind:value={passwords[user.name]} disabled={busy} /><Input type="password" aria-label={`${user.name} 비밀번호 확인`} placeholder="비밀번호 확인" autocomplete="new-password" bind:value={confirmations[user.name]} disabled={busy} /></div>{/if}</div>{/each}
    {#if pending?.adminUser}<div class="grid gap-2 sm:grid-cols-2"><Input type="password" aria-label="관리자 새 비밀번호" placeholder="관리자 새 비밀번호" autocomplete="new-password" bind:value={adminPassword} disabled={busy} /><Input type="password" aria-label="관리자 새 비밀번호 확인" placeholder="관리자 비밀번호 확인" autocomplete="new-password" bind:value={adminConfirm} disabled={busy} /></div>{/if}
    <label class="flex items-start gap-2 text-sm"><Checkbox bind:checked={acknowledged} disabled={busy} />대상 서버와 변경 내용을 확인했습니다. 즉시 적용되며 자동 복구되지 않는 것을 이해합니다.</label>
    {#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}
    <Dialog.Footer><Button variant="outline" disabled={busy} onclick={closeConfirmation}>취소</Button><Button disabled={busy || !acknowledged} onclick={apply}>{#if busy}<Spinner />{/if}{busy ? '서버에 적용 중…' : '지금 적용'}</Button></Dialog.Footer>
</Dialog.Content></Dialog.Root>
