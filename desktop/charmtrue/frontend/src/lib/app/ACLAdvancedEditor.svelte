<script lang="ts">
    import { Plus, Trash2 } from '@lucide/svelte';
    import { Button } from '$lib/components/ui/button';
    import { Checkbox } from '$lib/components/ui/checkbox';
    import { Input } from '$lib/components/ui/input';
    import * as NativeSelect from '$lib/components/ui/native-select';
    import { getAppContext } from './context.svelte';
    import { principalLabel, type EditableACL } from './acl-editor';
    import { aclFlags, changeACLTarget, expandedFlags, expandedPermissions, inheritanceFlags, newAdvancedEntry, nfsPermissions, permissionPresets, posixPermissions } from './acl-advanced';

    let { draft = $bindable() }: { draft: EditableACL } = $props();
    const app = getAppContext();
    const id = $props.id();
    const nfs = $derived(draft.aclType === 'NFS4');
    const targets = $derived(nfs ? { 'owner@': '소유자', 'group@': '소유 그룹', 'everyone@': '모두', USER: '사용자', GROUP: '그룹' }
        : { USER_OBJ: '소유자', GROUP_OBJ: '소유 그룹', OTHER: '기타 사용자', MASK: '그룹 권한 상한 (마스크)', USER: '사용자', GROUP: '그룹' });
    let error = $state('');

    function permissions(index: number, preset: string): void {
        const entry = draft.entries[index];
        try {
            const values = preset ? {} : expandedPermissions(entry);
            draft.entries[index] = { ...entry, basicPerms: preset, permissions: values }; error = '';
        } catch (e) { error = String(e); }
    }
    function inheritance(index: number, preset: string): void {
        const entry = draft.entries[index];
        try {
            const values = preset ? {} : expandedFlags(entry);
            draft.entries[index] = { ...entry, basicFlags: preset, flags: values }; error = '';
        } catch (e) { error = String(e); }
    }
    function identity(index: number, value: string): void {
        const entry = draft.entries[index];
        const candidate = entry.tag === 'USER' ? app.identity?.users?.find(user => user.username === value)?.uid
            : app.identity?.groups?.find(group => group.name === value)?.gid;
        draft.entries[index] = { ...entry, who: value, hasId: candidate !== undefined, id: candidate ?? 0 };
    }
</script>

<datalist id={`${id}-users`}>{#each app.identity?.users ?? [] as user}<option value={user.username}>{user.uid}</option>{/each}</datalist>
<datalist id={`${id}-groups`}>{#each app.identity?.groups ?? [] as group}<option value={group.name}>{group.gid}</option>{/each}</datalist>
<section class="space-y-3">
    <div><h3 class="text-sm font-semibold">접근 규칙 상세 편집</h3><p class="mt-1 text-xs text-muted-foreground">대상, 권한, 상속을 항목별로 설정합니다. 선택하지 않은 기존 값은 유지됩니다.</p></div>
    {#each draft.entries as entry, index}
        <fieldset class="min-w-0 space-y-4 rounded-xl border p-4">
            <legend class="px-1 text-sm font-medium">{index + 1}. {principalLabel(entry)}</legend>
            <div class="grid gap-3 sm:grid-cols-2">
                <label class="grid gap-2 text-sm">대상 종류<NativeSelect.Root value={entry.tag} onchange={event => draft.entries[index] = changeACLTarget(entry, event.currentTarget.value)}>{#if !(entry.tag in targets)}<option value={entry.tag}>{entry.tag} (기존 값)</option>{/if}{#each Object.entries(targets) as [key, label]}<option value={key}>{label}</option>{/each}</NativeSelect.Root></label>
                {#if nfs}<label class="grid gap-2 text-sm">접근 처리<NativeSelect.Root bind:value={entry.type}><option value="ALLOW">허용</option><option value="DENY">거부</option></NativeSelect.Root></label>{/if}
                {#if ['USER', 'GROUP'].includes(entry.tag)}
                    <label class="grid gap-2 text-sm">{entry.tag === 'USER' ? '사용자' : '그룹'} 이름<Input value={entry.who} list={`${id}-${entry.tag === 'USER' ? 'users' : 'groups'}`} placeholder="이름 검색 또는 직접 입력" oninput={event => identity(index, event.currentTarget.value)} /></label>
                    <details class="sm:col-span-2"><summary class="cursor-pointer text-xs text-muted-foreground">숫자 ID로 지정{entry.hasId ? ` · ${entry.tag === 'USER' ? 'UID' : 'GID'} ${entry.id}` : ''}</summary><label class="mt-2 grid gap-2 text-sm">{entry.tag === 'USER' ? 'UID' : 'GID'}<Input type="number" min="0" step="1" value={entry.hasId ? entry.id : ''} oninput={event => { const value = event.currentTarget.value; entry.id = value ? Number(value) : 0; entry.hasId = !!value; entry.who = ''; }} /></label><p class="mt-1 text-xs text-muted-foreground">ID를 입력하면 이름 대신 사용합니다.</p></details>
                {/if}
            </div>
            {#if entry.type === 'DENY'}<p class="text-xs text-amber-700 dark:text-amber-400">아래에서 선택한 권한을 거부합니다. 다른 허용 규칙에도 영향을 줄 수 있습니다.</p>{/if}
            {#if nfs}
                <label class="grid gap-2 text-sm">권한<NativeSelect.Root value={entry.basicPerms} onchange={event => permissions(index, event.currentTarget.value)}>{#if entry.basicPerms && !(entry.basicPerms in permissionPresets)}<option value={entry.basicPerms}>{entry.basicPerms} (기존 값)</option>{/if}{#each Object.entries(permissionPresets) as [key, label]}<option value={key}>{label}</option>{/each}<option value="">세부 권한 직접 선택</option></NativeSelect.Root></label>
            {/if}
            {#if !nfs || !entry.basicPerms}
                <div class="grid gap-3 rounded-lg bg-muted/40 p-3 sm:grid-cols-2">
                    {#each Object.entries({ ...(nfs ? nfsPermissions : posixPermissions), ...Object.fromEntries(Object.keys(entry.permissions ?? {}).filter(key => !(key in (nfs ? nfsPermissions : posixPermissions))).map(key => [key, `${key} (추가 권한)`])) }) as [key, label]}
                        <label class="flex items-center gap-2 text-sm"><Checkbox checked={entry.permissions?.[key] ?? false} onCheckedChange={value => entry.permissions = { ...entry.permissions, [key]: value }} />{label}</label>
                    {/each}
                </div>
            {/if}
            {#if nfs}
                <label class="grid gap-2 text-sm">적용 범위<NativeSelect.Root value={entry.basicFlags} onchange={event => inheritance(index, event.currentTarget.value)}>{#if entry.basicFlags && !['NOINHERIT', 'INHERIT'].includes(entry.basicFlags)}<option value={entry.basicFlags}>{entry.basicFlags} (기존 값)</option>{/if}<option value="NOINHERIT">현재 폴더만</option><option value="INHERIT">현재 폴더 + 새 파일·폴더</option><option value="">상속 세부 설정</option></NativeSelect.Root></label>
                {#if !entry.basicFlags}<div class="grid gap-3 rounded-lg bg-muted/40 p-3 sm:grid-cols-2">{#each Object.entries({ ...inheritanceFlags, ...Object.fromEntries(Object.keys(entry.flags ?? {}).filter(key => !(key in inheritanceFlags)).map(key => [key, `${key} (추가 플래그)`])) }) as [key, label]}<label class="flex items-center gap-2 text-sm"><Checkbox checked={entry.flags?.[key] ?? false} onCheckedChange={value => entry.flags = { ...entry.flags, [key]: value }} />{label}</label>{/each}</div>{/if}
            {:else}
                <label class="flex items-center gap-2 text-sm"><Checkbox bind:checked={entry.default} />새 하위 항목을 위한 기본 권한 (현재 폴더 권한과 별도)</label>
                {#if entry.tag === 'MASK'}<p class="text-xs text-muted-foreground">지정 사용자·그룹과 소유 그룹이 가질 수 있는 최대 권한입니다.</p>{/if}
            {/if}
            <div class="flex justify-end"><Button variant="ghost" size="sm" class="text-destructive" onclick={() => draft.entries = draft.entries.filter((_, i) => i !== index)}><Trash2 />규칙 삭제</Button></div>
        </fieldset>
    {/each}
    <Button variant="outline" onclick={() => draft.entries = [...draft.entries, newAdvancedEntry(draft.aclType)]}><Plus />규칙 추가</Button>
    {#if app.identityError}<p class="text-xs text-destructive">사용자·그룹 목록 조회 실패: {app.identityError}</p><Button variant="outline" size="sm" onclick={() => app.refreshIdentity()}>목록 다시 불러오기</Button>{/if}
    {#if nfs}<details class="rounded-lg border p-3"><summary class="cursor-pointer text-sm">폴더 전체의 상속·보호 설정</summary><div class="mt-3 grid gap-3">{#each Object.entries({ ...aclFlags, ...Object.fromEntries(Object.keys(draft.nfs41Flags ?? {}).filter(key => !(key in aclFlags)).map(key => [key, `${key} (추가 플래그)`])) }) as [key, label]}<label class="flex items-center gap-2 text-sm"><Checkbox checked={draft.nfs41Flags?.[key] ?? false} onCheckedChange={value => draft.nfs41Flags = { ...draft.nfs41Flags, [key]: value }} />{label}</label>{/each}</div></details>{/if}
    {#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}
</section>
