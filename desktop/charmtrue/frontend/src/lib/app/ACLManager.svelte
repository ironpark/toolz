<script lang="ts">
    import { onMount, tick } from 'svelte';
    import { ArrowLeft, CircleHelp, Plus, Search, ShieldCheck, Trash2 } from '@lucide/svelte';
    import * as Tooltip from '$lib/components/ui/tooltip';
    import * as Alert from '$lib/components/ui/alert';
    import { Badge } from '$lib/components/ui/badge';
    import { Button } from '$lib/components/ui/button';
    import { Checkbox } from '$lib/components/ui/checkbox';
    import * as Dialog from '$lib/components/ui/dialog';
    import { Input } from '$lib/components/ui/input';
    import * as NativeSelect from '$lib/components/ui/native-select';
    import { Spinner } from '$lib/components/ui/spinner';
    import { Textarea } from '$lib/components/ui/textarea';
    import ACLAdvancedEditor from './ACLAdvancedEditor.svelte';
    import ACLCreatePrincipal from './ACLCreatePrincipal.svelte';
    import { filterPrincipals, principalCandidates, specialPrincipalHelp } from './acl-principals';
    import type { ACLTemplateInfo, FilesystemACLMutation } from '../../../bindings/github.com/ironpark/toolz/desktop/charmtrue';
    import { getAppContext } from './context.svelte';
    import { aclChanges, aclRole, aclWarnings, addACLGrant, cloneACL, editableACL, entryDescription, principalLabel, stableACL, withACLRole, type EditableACL, type ACLPrincipal, type ACLRole, type ACLScope } from './acl-editor';

    let { path, onclose }: { path: string; onclose: () => void } = $props();
    const app = getAppContext();
    let open = $state(true), loading = $state(true), saving = $state(false);
    let original = $state<EditableACL | null>(null), draft = $state<EditableACL | null>(null);
    let templates = $state<ACLTemplateInfo[]>([]), templateID = $state(''), templateName = $state('');
    let error = $state(''), templateError = $state('');
    let mode = $state<'basic' | 'advanced'>('basic'), review = $state(false), acknowledged = $state(false);
    let principalType = $state<'GROUP' | 'USER'>('GROUP'), query = $state(''), principal = $state<ACLPrincipal | null>(null);
    let role = $state<ACLRole>('READ'), scope = $state<ACLScope>('folder');
    let recursive = $state(false), traverse = $state(false), strip = $state(false), validate = $state(true);
    let advancedText = $state('');
    let jsonEditing = $state(false);
    let showSystem = $state(false);
    let createKind = $state<'USER' | 'GROUP' | null>(null), creating = $state(false);
    let approved = $state<FilesystemACLMutation | null>(null);
    let alive = true;
    const changes = $derived(original && draft ? aclChanges(original, draft) : { added: [], removed: [] });
    const warnings = $derived(draft ? aclWarnings(draft) : []);
    const supported = $derived(draft && ['NFS4', 'POSIX1E'].includes(draft.aclType));
    const risky = $derived(recursive || strip || !!templateName);
    const hasChanges = $derived(original && draft && (stableACL(original) !== stableACL(draft) || recursive || strip));
    const allCandidates = $derived(principalCandidates(app.identity?.users ?? [], app.identity?.groups ?? [], principalType));
    const matchingCandidates = $derived(filterPrincipals(allCandidates, query, showSystem));
    const candidates = $derived(matchingCandidates.slice(0, 20));
    const hiddenSystemCount = $derived(filterPrincipals(allCandidates, query, true).filter(item => item.system).length);

    onMount(() => { void load(); void app.refreshIdentity(); return () => { alive = false; }; });

    async function load(): Promise<void> {
        loading = true; error = ''; templateError = '';
        try {
            const value = await app.getFilesystemACL(path);
            if (!alive) return;
            original = editableACL(value); draft = editableACL(value);
            recursive = false; traverse = false; strip = false; validate = true;
            review = false; approved = null; templateName = ''; templateID = '';
            mode = 'basic'; jsonEditing = false; syncAdvanced();
            try { const values = await app.getACLTemplates(path); if (alive) templates = values.filter(item => item.aclType === value.aclType); }
            catch (e) { if (alive) templateError = e instanceof Error ? e.message : String(e); }
        } catch (e) { if (alive) error = e instanceof Error ? e.message : String(e); }
        finally { if (alive) loading = false; }
    }

    function syncAdvanced(): void {
        if (draft) advancedText = JSON.stringify({ entries: draft.entries, nfs41Flags: draft.nfs41Flags ?? {} }, null, 2);
    }

    function acceptAdvanced(): boolean {
        if (!draft) return false;
        try {
            const value = JSON.parse(advancedText);
            if (!Array.isArray(value.entries) || !value.nfs41Flags || typeof value.nfs41Flags !== 'object' || Array.isArray(value.nfs41Flags)) throw new Error('entries 배열과 nfs41Flags 객체가 필요합니다.');
            for (const entry of value.entries) {
                if (!entry || typeof entry.tag !== 'string' || typeof entry.type !== 'string' || typeof entry.who !== 'string' || typeof entry.hasId !== 'boolean' || typeof entry.default !== 'boolean' || typeof entry.basicPerms !== 'string' || typeof entry.basicFlags !== 'string' || !Number.isInteger(entry.id)) throw new Error('ACL 항목의 대상·ID·권한 필드를 확인하세요.');
                for (const key of ['permissions', 'flags']) if (!entry[key] || typeof entry[key] !== 'object' || Array.isArray(entry[key]) || Object.values(entry[key]).some(value => typeof value !== 'boolean')) throw new Error('permissions와 flags는 true/false 값의 객체여야 합니다.');
            }
            if (Object.values(value.nfs41Flags).some(value => typeof value !== 'boolean')) throw new Error('nfs41Flags는 true/false 값의 객체여야 합니다.');
            draft.entries = value.entries; draft.nfs41Flags = value.nfs41Flags;
            error = ''; return true;
        } catch (e) { error = e instanceof Error ? e.message : String(e); return false; }
    }

    function switchMode(next: 'basic' | 'advanced'): void {
        if (createKind) return;
        if (jsonEditing && !acceptAdvanced()) return;
        jsonEditing = false; mode = next;
    }

    function toggleJSON(): void {
        if (jsonEditing) {
            if (acceptAdvanced()) jsonEditing = false;
        } else { syncAdvanced(); jsonEditing = true; }
    }

    function addGrant(): void {
        if (!draft || !principal) return;
        try { draft = addACLGrant(draft, principal, role, scope); principal = null; query = ''; error = ''; }
        catch (e) { error = e instanceof Error ? e.message : String(e); }
    }

    function changeRole(index: number, role: string): void {
        if (!draft || aclRole(draft.entries[index], draft.aclType) === 'CUSTOM') return;
        draft.entries[index] = withACLRole(draft.entries[index], draft.aclType, role as ACLRole);
    }

    function removeEntry(index: number): void {
        if (draft) draft.entries = draft.entries.filter((_, i) => i !== index);
    }

    function applyTemplate(): void {
        const template = templates.find(item => String(item.id) === templateID);
        if (!draft || !template) return;
        draft.entries = cloneACL(template.entries ?? []);
        templateName = template.name;
        syncAdvanced();
    }

    function preview(event: SubmitEvent): void {
        event.preventDefault();
        if (createKind || creating) return;
        if (!draft || !original || !supported || (jsonEditing && !acceptAdvanced())) return;
        error = '';
        if (stableACL(original) === stableACL(draft) && !recursive && !strip) { error = '변경한 권한이나 적용 옵션이 없습니다.'; return; }
        if (!strip && !draft.entries.length) { error = '하나 이상의 접근 규칙이 필요합니다.'; return; }
        if (draft.user !== original.user && !draft.user.trim() || draft.group !== original.group && !draft.group.trim()) { error = '소유 사용자와 그룹을 비워 둘 수 없습니다.'; return; }
        for (const entry of strip ? [] : draft.entries) {
            if (['USER', 'GROUP'].includes(entry.tag) && entry.hasId && (!Number.isInteger(entry.id) || entry.id < 0)) { error = 'UID/GID는 0 이상의 정수로 입력하세요.'; return; }
            if (['USER', 'GROUP'].includes(entry.tag) && (!entry.hasId || entry.id < 0) && !entry.who.trim()) { error = '사용자·그룹 항목에 이름 또는 유효한 ID가 필요합니다.'; return; }
        }
        approved = cloneACL({ path: draft.path, aclType: draft.aclType, user: draft.user === original.user ? '' : draft.user, group: draft.group === original.group ? '' : draft.group, entries: strip ? [] : draft.entries, nfs41Flags: draft.nfs41Flags ?? {}, stripAcl: strip, recursive, traverse: recursive && traverse, canonicalize: true, validateEffectiveAcl: validate });
        acknowledged = false; review = true;
    }

    async function save(): Promise<void> {
        if (!approved || !original || saving || risky && !acknowledged) return;
        saving = true; error = '';
        try {
            const current = await app.getFilesystemACL(path);
            if (stableACL(editableACL(current)) !== stableACL(original)) throw new Error('다른 작업에서 이 폴더의 권한을 변경했습니다. 다시 불러온 뒤 수정해 주세요.');
            await app.saveFilesystemACL(approved);
            open = false; await tick(); onclose();
        } catch (e) { error = e instanceof Error ? e.message : String(e); }
        finally { saving = false; }
    }
</script>

<Dialog.Root {open} onOpenChange={(value) => { if (!saving && !creating) { open = value; if (!value) onclose(); } }}>
    <Dialog.Content class="max-h-[90dvh] overflow-y-auto sm:max-w-4xl" showCloseButton={!saving && !creating}>
        <Dialog.Header><Dialog.Title>{review ? '권한 변경 미리보기' : '폴더 접근 권한'}</Dialog.Title><Dialog.Description class="break-all">{path}</Dialog.Description></Dialog.Header>
        {#if loading}<div class="grid min-h-40 place-items-center"><Spinner aria-label="권한 불러오는 중" /></div>
        {:else if draft && original}
            {#if review}
                <div class="space-y-4">
                    <p class="text-sm">아래 변경을 저장하면 선택한 폴더의 권한에 반영됩니다.</p>
                    {#if templateName}<Alert.Root><Alert.Title>템플릿으로 규칙 교체</Alert.Title><Alert.Description>“{templateName}”의 접근 규칙으로 교체합니다. 아래 삭제·추가 항목을 확인하세요.</Alert.Description></Alert.Root>{/if}
                    {#if strip}<Alert.Root variant="destructive"><Alert.Title>확장 ACL 제거</Alert.Title><Alert.Description>이 폴더의 확장 접근 규칙을 제거합니다.</Alert.Description></Alert.Root>
                    {:else}<div class="space-y-2 rounded-lg border p-4">{#each changes.removed as entry}<p class="text-sm text-destructive">삭제: {entryDescription(entry, draft.aclType)}</p>{/each}{#each changes.added as entry}<p class="text-sm">추가: {entryDescription(entry, draft.aclType)}</p>{/each}{#if !changes.removed.length && !changes.added.length}<p class="text-sm text-muted-foreground">접근 규칙의 대상과 권한은 유지됩니다.</p>{/if}</div>{/if}
                    {#if original.user !== draft.user}<p class="text-sm">소유자: {original.user || original.uid} → {draft.user}</p>{/if}
                    {#if original.group !== draft.group}<p class="text-sm">소유 그룹: {original.group || original.gid} → {draft.group}</p>{/if}
                    {#if stableACL(original.nfs41Flags) !== stableACL(draft.nfs41Flags)}<p class="text-sm">NFSv4 ACL 상속·보호 플래그가 변경됩니다.</p>{/if}
                    <details class="rounded-lg border p-3"><summary class="cursor-pointer text-sm">정확한 변경 규칙과 순서 확인</summary><div class="mt-3 grid gap-3 sm:grid-cols-2"><div><p class="mb-2 text-xs font-medium">변경 전</p><pre class="max-h-64 overflow-auto whitespace-pre-wrap break-all text-xs">{JSON.stringify({ entries: original.entries, nfs41Flags: original.nfs41Flags }, null, 2)}</pre></div><div><p class="mb-2 text-xs font-medium">변경 후</p><pre class="max-h-64 overflow-auto whitespace-pre-wrap break-all text-xs">{JSON.stringify({ entries: strip ? [] : draft.entries, nfs41Flags: draft.nfs41Flags }, null, 2)}</pre></div></div></details>
                    <div class="rounded-lg bg-muted/50 p-4 text-sm"><p>{recursive ? '기존 하위 파일·폴더에도 현재 ACL 전체를 재적용합니다. 하위의 개별 권한과 변경한 소유권에도 영향을 줄 수 있습니다.' : '기존 하위 파일·폴더에는 일괄 적용하지 않습니다. 새 항목에는 각 규칙의 상속 설정이 적용됩니다.'}</p><p class="mt-2">{recursive && traverse ? '하위 데이터셋 경계도 포함합니다.' : '하위 데이터셋 경계를 넘어 일괄 적용하지 않습니다.'}</p><p class="mt-2">{validate ? '서버의 경로 접근 검증을 사용합니다.' : '경로 접근 검증이 꺼져 있습니다.'}</p></div>
                    {#each warnings as warning}<p class="text-sm text-amber-700 dark:text-amber-400">{warning}</p>{/each}
                    {#if risky}<label class="flex items-start gap-2 text-sm"><Checkbox bind:checked={acknowledged} disabled={saving} />변경 범위를 확인했습니다. 하위 전체 변경은 이 폴더 ACL만 복원해도 원상복구되지 않을 수 있습니다.</label>{/if}
                </div>
                <Dialog.Footer><Button variant="outline" disabled={saving} onclick={() => review = false}><ArrowLeft />수정으로 돌아가기</Button><Button disabled={saving || risky && !acknowledged} onclick={save}>{#if saving}<Spinner />{:else}<ShieldCheck />{/if}{saving ? '적용 중…' : '권한 적용'}</Button></Dialog.Footer>
            {:else}
                <form class="space-y-5" onsubmit={preview}>
                    <div class="flex flex-wrap items-center gap-2"><Button size="sm" variant={mode === 'basic' ? 'default' : 'outline'} aria-pressed={mode === 'basic'} onclick={() => switchMode('basic')}>기본</Button><Button size="sm" variant={mode === 'advanced' ? 'default' : 'outline'} aria-pressed={mode === 'advanced'} onclick={() => switchMode('advanced')}>고급</Button><Badge variant="outline">{draft.aclType}</Badge></div>
                    {#if !supported}<Alert.Root variant="destructive"><Alert.Title>지원하지 않는 ACL 유형</Alert.Title><Alert.Description>현재 ACL 유형은 이 편집기에서 저장할 수 없습니다.</Alert.Description></Alert.Root>{/if}
                    {#if mode === 'basic'}
                        <section class="space-y-3"><h3 class="text-sm font-semibold">현재 접근 규칙</h3><p class="text-xs text-muted-foreground">사용자 지정 규칙은 그대로 보존됩니다. 편집하려면 고급 모드를 사용하세요.</p>
                            <Tooltip.Provider delayDuration={200}><div class="divide-y rounded-lg border">
                            {#each draft.entries as entry, index}
                                {@const currentRole = aclRole(entry, draft.aclType)}
                                <div class="flex flex-wrap items-center gap-x-3 gap-y-2 px-3 py-2"><div class="flex min-w-0 flex-1 items-center gap-1.5"><span class="break-all text-sm font-medium">{principalLabel(entry)}</span>
                                    {#if specialPrincipalHelp[entry.tag]}<Tooltip.Root><Tooltip.Trigger type="button" class="inline-flex size-6 shrink-0 items-center justify-center rounded-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" aria-label={`${principalLabel(entry)} 설명`}><CircleHelp class="size-3.5" /></Tooltip.Trigger><Tooltip.Content class="max-w-72 whitespace-normal leading-relaxed">{specialPrincipalHelp[entry.tag]}</Tooltip.Content></Tooltip.Root>{/if}
                                    {#if ['owner@', 'USER_OBJ'].includes(entry.tag)}<span class="truncate text-xs text-muted-foreground">{draft.user || draft.uid}</span>{:else if ['group@', 'GROUP_OBJ'].includes(entry.tag)}<span class="truncate text-xs text-muted-foreground">{draft.group || draft.gid}</span>{/if}
                                    {#if entry.default}<Badge variant="outline">상속 기본값</Badge>{/if}
                                    {#if entry.type === 'DENY'}<Badge variant="destructive">거부</Badge>{/if}
                                </div>
                                    {#if currentRole === 'CUSTOM'}<Button variant="ghost" size="sm" onclick={() => switchMode('advanced')}>사용자 지정 · 상세 편집</Button>{:else}<NativeSelect.Root size="sm" aria-label={`${principalLabel(entry)} 권한`} value={currentRole} onchange={event => changeRole(index, event.currentTarget.value)}><option value="NONE">접근 없음</option><option value="READ">읽기 전용</option><option value="MODIFY">읽기·쓰기</option>{#if draft.aclType === 'NFS4'}<option value="FULL_CONTROL">전체 관리</option>{/if}</NativeSelect.Root>{/if}
                                    {#if currentRole !== 'CUSTOM' && draft.aclType === 'NFS4'}<NativeSelect.Root size="sm" aria-label={`${principalLabel(entry)} 적용 범위`} bind:value={entry.basicFlags}><option value="NOINHERIT">현재 폴더</option><option value="INHERIT">새 하위 항목에도 상속</option></NativeSelect.Root>{/if}
                                    {#if currentRole !== 'CUSTOM' && ['USER', 'GROUP'].includes(entry.tag)}<Button variant="ghost" size="icon-sm" aria-label={`${principalLabel(entry)} 규칙 삭제`} onclick={() => removeEntry(index)}><Trash2 /></Button>{/if}
                                </div>
                            {/each}
                            </div></Tooltip.Provider>
                        </section>
                        <section class="space-y-4 rounded-xl bg-muted/40 p-4"><div class="flex flex-wrap items-center justify-between gap-2"><h3 class="text-sm font-semibold">접근할 사용자·그룹 추가</h3><div class="flex gap-1"><Button variant="outline" size="sm" disabled={!!createKind} onclick={() => createKind = 'USER'}><Plus />새 사용자</Button><Button variant="outline" size="sm" disabled={!!createKind} onclick={() => createKind = 'GROUP'}><Plus />새 그룹</Button></div></div>
                            {#if createKind}<ACLCreatePrincipal kind={createKind} bind:busy={creating} oncancel={() => createKind = null} oncreated={value => { principalType = value.tag; principal = value; query = value.name; createKind = null; }} />{:else}
                            <div class="grid gap-3 sm:grid-cols-[140px_1fr]"><NativeSelect.Root aria-label="대상 종류" bind:value={principalType} onchange={() => principal = null}><option value="GROUP">그룹</option><option value="USER">사용자</option></NativeSelect.Root><div class="relative"><Search class="absolute left-3 top-2.5 size-4 text-muted-foreground" /><Input class="pl-9" aria-label="사용자 또는 그룹 검색" bind:value={query} placeholder="이름 또는 UID/GID 검색" oninput={() => principal = null} /></div></div>
                            <label class="flex items-center gap-2 text-xs text-muted-foreground"><Checkbox bind:checked={showSystem} />시스템 사용자·그룹도 표시{#if !showSystem && hiddenSystemCount > 0}<span>({hiddenSystemCount}개 숨김)</span>{/if}</label>
                            {#if app.identityError}<p class="text-sm text-destructive">{app.identityError}</p><Button size="sm" variant="outline" onclick={() => app.refreshIdentity()}>사용자·그룹 다시 불러오기</Button>{/if}
                            {#if principal}<div class="flex items-center justify-between gap-2 rounded-lg border bg-background p-3"><span class="text-sm">{principal.tag === 'GROUP' ? '그룹' : '사용자'} {principal.name} · {principal.id}</span><Button variant="ghost" size="sm" onclick={() => principal = null}>다시 선택</Button></div>
                            {:else}<div class="grid max-h-48 gap-1 overflow-y-auto rounded-lg border bg-background p-1">{#each candidates as candidate}<Button variant="ghost" class="h-auto min-h-9 justify-start whitespace-normal py-2 text-left" onclick={() => principal = candidate}><span class="min-w-0 flex-1"><span class="flex flex-wrap items-center gap-2"><span class="break-all">{candidate.name}</span><span class="text-xs text-muted-foreground">{candidate.id}</span>{#if candidate.system}<Badge variant="outline">시스템</Badge>{/if}</span>{#if candidate.description}<span class="mt-0.5 block text-xs font-normal text-muted-foreground">{candidate.description}</span>{/if}</span></Button>{:else}<p class="p-3 text-sm text-muted-foreground">{app.identityLoading ? '불러오는 중…' : !showSystem && hiddenSystemCount ? '일반 대상이 없습니다. 시스템 사용자·그룹 표시를 체크해 보세요.' : '검색 결과가 없습니다.'}</p>{/each}</div>{#if matchingCandidates.length > candidates.length}<p class="text-xs text-muted-foreground">{matchingCandidates.length}개 중 {candidates.length}개 표시 · 이름이나 ID로 검색 범위를 좁히세요.</p>{/if}{/if}
                            <div class="grid gap-3 sm:grid-cols-2"><label class="grid gap-2 text-sm">역할<NativeSelect.Root bind:value={role}><option value="NONE">접근 없음</option><option value="READ">읽기 전용</option><option value="MODIFY">읽기·쓰기</option>{#if draft.aclType === 'NFS4'}<option value="FULL_CONTROL">전체 관리</option>{/if}</NativeSelect.Root></label><label class="grid gap-2 text-sm">새 규칙 적용 범위<NativeSelect.Root bind:value={scope}><option value="folder">현재 폴더</option><option value="inherit">현재 폴더 + 새 하위 항목</option></NativeSelect.Root></label></div>
                            <p class="text-xs text-muted-foreground">접근 없음: 이 규칙의 권한을 모두 끔 / 읽기 전용: 탐색·열기·다운로드 / 읽기·쓰기: 생성·수정·삭제{draft.aclType === 'NFS4' ? ' / 전체 관리: 권한 관리 포함' : ' · POSIX는 읽기 역할에 폴더 탐색 권한을 함께 부여하며 전체 관리 역할을 제공하지 않습니다.'}</p>
                            <Button variant="outline" disabled={!principal || !supported || strip} onclick={addGrant}><Plus />변경 목록에 추가</Button>
                            {/if}
                        </section>
                    {:else}
                        <div class="grid gap-3 sm:grid-cols-2"><label class="grid gap-2 text-sm">소유 사용자<Input bind:value={draft.user} placeholder={String(draft.uid)} /></label><label class="grid gap-2 text-sm">소유 그룹<Input bind:value={draft.group} placeholder={String(draft.gid)} /></label></div>
                        {#if templates.length}<details class="space-y-2 rounded-lg bg-muted/40 p-3"><summary class="cursor-pointer text-sm font-medium">템플릿으로 규칙 교체</summary><div class="flex flex-wrap gap-2"><NativeSelect.Root aria-label="권한 템플릿" bind:value={templateID}><option value="">템플릿 선택</option>{#each templates as template}<option value={String(template.id)}>{template.name}</option>{/each}</NativeSelect.Root><Button variant="outline" disabled={!templateID || jsonEditing} onclick={applyTemplate}>편집 내용에 교체 반영</Button></div><p class="text-xs text-muted-foreground">현재 접근 규칙을 교체합니다. 저장 전 미리보기에서 삭제·추가 항목을 확인합니다. JSON 편집 중에는 폼으로 돌아온 뒤 사용하세요.</p></details>{/if}
                        {#if templateError}<p class="text-xs text-muted-foreground">템플릿 조회 실패: {templateError}</p>{/if}
                        {#if jsonEditing}
                            <section class="space-y-3 rounded-lg border p-4"><div class="flex flex-wrap items-center justify-between gap-2"><h3 class="text-sm font-semibold">JSON 직접 편집 (선택 사항)</h3><Button variant="outline" size="sm" onclick={toggleJSON}>폼으로 돌아가기</Button></div><p class="text-xs text-muted-foreground">폼으로 돌아가거나 미리보기를 열 때 입력값을 검사합니다. 아직 서버에는 저장하지 않습니다.</p><label class="grid gap-2 text-sm">규칙과 상속 플래그<Textarea class="min-h-64 font-mono text-xs" bind:value={advancedText} spellcheck={false} /></label><Button variant="ghost" size="sm" onclick={() => { jsonEditing = false; error = ''; }}>JSON 수정 취소 · 폼으로 돌아가기</Button></section>
                        {:else}
                            <ACLAdvancedEditor bind:draft />
                            <div class="flex justify-end"><Button variant="ghost" size="sm" onclick={toggleJSON}>JSON 직접 편집…</Button></div>
                        {/if}
                        <div class="grid gap-3"><label class="flex items-center gap-2 text-sm"><Checkbox bind:checked={validate} />서버의 경로 접근 검증 사용</label><label class="flex items-center gap-2 text-sm"><Checkbox bind:checked={traverse} disabled={!recursive} />기존 하위 항목 적용 시 다른 데이터셋 경계도 포함</label><label class="flex items-center gap-2 text-sm text-destructive"><Checkbox bind:checked={strip} />확장 ACL 제거</label></div>
                    {/if}
                    <section class="space-y-2 rounded-lg border p-4"><h3 class="text-sm font-semibold">기존 파일·폴더에 적용</h3><label class="flex items-start gap-2 text-sm"><Checkbox bind:checked={recursive} />기존 하위 파일·폴더에도 현재 ACL 전체 적용</label><p class="text-xs text-muted-foreground">기본은 꺼짐입니다. 새 항목에 대한 상속과 별개이며, 켜면 하위 항목의 개별 접근 규칙도 변경될 수 있습니다.</p></section>
                    {#each warnings as warning}<p class="text-sm text-amber-700 dark:text-amber-400">{warning}</p>{/each}
                    <details class="rounded-lg bg-muted/30 p-4"><summary class="cursor-pointer text-sm font-medium">접근이 안 될 때 확인할 사항</summary><div class="mt-3 space-y-2 text-sm text-muted-foreground"><p>사용자·그룹 소속 → 상위 폴더 탐색 권한 → 이 폴더 ACL → SMB 공유 권한을 확인하세요. 이 화면의 역할은 실제 접근 권한 전체를 판정한 결과가 아닙니다.</p><Button variant="outline" size="sm" href="/identity">사용자·그룹 관리</Button><Button variant="outline" size="sm" href="/services">공유 권한 관리</Button></div></details>
                    <Dialog.Footer><Button variant="outline" disabled={creating} onclick={onclose}>취소</Button><Button type="submit" disabled={!!createKind || !supported || (!jsonEditing && !hasChanges)}><ShieldCheck />변경 미리보기</Button></Dialog.Footer>
                </form>
            {/if}
        {/if}
        {#if error}<Alert.Root variant="destructive"><Alert.Title>권한 처리 실패</Alert.Title><Alert.Description>{error}</Alert.Description></Alert.Root><Button variant="outline" disabled={saving || loading} onclick={load}>편집 내용을 버리고 다시 불러오기</Button>{/if}
    </Dialog.Content>
</Dialog.Root>
