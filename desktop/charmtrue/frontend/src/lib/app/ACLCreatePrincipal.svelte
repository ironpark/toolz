<script lang="ts">
    import { Button } from '$lib/components/ui/button';
    import { Checkbox } from '$lib/components/ui/checkbox';
    import { Input } from '$lib/components/ui/input';
    import { Spinner } from '$lib/components/ui/spinner';
    import { getAppContext } from './context.svelte';
    import type { ACLPrincipal } from './acl-editor';

    let { kind, busy = $bindable(false), oncreated, oncancel }: {
        kind: 'USER' | 'GROUP'; busy?: boolean; oncreated: (principal: ACLPrincipal) => void; oncancel: () => void;
    } = $props();
    const app = getAppContext();
    let name = $state(''), fullName = $state(''), password = $state(''), confirmation = $state('');
    let smb = $state(true), error = $state(''), createdName = $state('');
    let createdID = $state<number | null>(null);

    function selectCreated(): void {
        const user = kind === 'USER' ? app.identity?.users?.find(item => item.local && (createdID !== null ? item.id === createdID : item.username === createdName)) : undefined;
        const group = kind === 'GROUP' ? app.identity?.groups?.find(item => item.local && item.name === createdName) : undefined;
        if (user) oncreated({ tag: 'USER', name: user.username, id: user.uid });
        else if (group) oncreated({ tag: 'GROUP', name: group.name, id: group.gid });
        else error = '생성은 완료됐지만 새 대상의 UID/GID를 조회하지 못했습니다. 목록을 다시 불러오세요. 다시 생성하지 않습니다.';
    }

    async function create(): Promise<void> {
        if (busy || createdName) return;
        error = '';
        const username = name.trim();
        if (!username) { error = '이름을 입력하세요.'; return; }
        if (kind === 'USER' && (!password || password !== confirmation)) { error = !password ? '비밀번호를 입력하세요.' : '비밀번호 확인이 일치하지 않습니다.'; return; }
        if ((kind === 'USER' ? app.identity?.users?.some(item => item.username === username) : app.identity?.groups?.some(item => item.name === username))) { error = '같은 이름이 이미 있습니다. 기존 대상을 선택하세요.'; return; }
        busy = true;
        try {
            if (kind === 'USER') {
                const result = await app.saveUser({ id: 0, uid: 0, setUid: false, username, fullName: fullName.trim() || username,
                    email: '', home: '/var/empty', shell: '/usr/sbin/nologin', password, randomPassword: false,
                    smb, locked: false, passwordDisabled: false, sshPasswordEnabled: false, sshPublicKey: '',
                    groupCreate: true, primaryGroupId: 0, groups: [], homeCreate: false, homeMode: '700', usernsIdmap: '',
                    sudoCommands: [], sudoCommandsNoPassword: [] });
                createdID = result.id;
            } else await app.saveGroup({ id: 0, name: username, smb });
            createdName = username; password = ''; confirmation = '';
            selectCreated();
        } catch (e) { error = e instanceof Error ? e.message : String(e); }
        finally { busy = false; }
    }

    async function retryLookup(): Promise<void> {
        busy = true; error = '';
        try { await app.refreshIdentity(); selectCreated(); }
        catch (e) { error = e instanceof Error ? e.message : String(e); }
        finally { busy = false; }
    }
</script>

<section class="space-y-3 rounded-lg border bg-background p-4" aria-label={`새 ${kind === 'USER' ? '사용자' : '그룹'} 만들기`}>
    <h4 class="text-sm font-semibold">새 {kind === 'USER' ? '사용자' : '그룹'} 만들기</h4>
    <p class="text-xs text-muted-foreground">생성 버튼을 누르면 서버에 즉시 생성됩니다. ACL 변경은 별도로 적용하며, ACL 창을 취소해도 생성한 대상은 유지됩니다.</p>
    {#if !createdName}
        <fieldset disabled={busy} class="min-w-0 space-y-3">
            <label class="grid gap-2 text-sm">{kind === 'USER' ? '사용자 이름' : '그룹 이름'}<Input bind:value={name} autocomplete="off" placeholder={kind === 'USER' ? '예: alex' : '예: family'} /></label>
            {#if kind === 'USER'}
                <label class="grid gap-2 text-sm">표시 이름 (선택)<Input bind:value={fullName} autocomplete="off" placeholder="비워두면 사용자 이름 사용" /></label>
                <div class="grid gap-3 sm:grid-cols-2"><label class="grid gap-2 text-sm">비밀번호<Input type="password" bind:value={password} autocomplete="new-password" /></label><label class="grid gap-2 text-sm">비밀번호 확인<Input type="password" bind:value={confirmation} autocomplete="new-password" /></label></div>
                <p class="text-xs text-muted-foreground">같은 이름의 기본 그룹을 함께 만듭니다. 관리자 권한·SSH 로그인·홈 폴더 생성은 사용하지 않습니다.</p>
            {:else}<p class="text-xs text-muted-foreground">빈 그룹으로 생성됩니다. 구성원은 사용자·그룹 관리에서 추가할 수 있습니다.</p>{/if}
            <label class="flex items-center gap-2 text-sm"><Checkbox bind:checked={smb} disabled={busy} />SMB 공유에 사용</label>
        </fieldset>
    {/if}
    {#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}
    <div class="flex justify-end gap-2"><Button variant="ghost" size="sm" disabled={busy} onclick={oncancel}>{createdName ? '목록으로 돌아가기' : '취소'}</Button>{#if createdName}<Button size="sm" disabled={busy} onclick={retryLookup}>{#if busy}<Spinner />{/if}목록 다시 불러오기</Button>{:else}<Button size="sm" disabled={busy} onclick={create}>{#if busy}<Spinner />{/if}{busy ? '생성 중…' : '생성하고 선택'}</Button>{/if}</div>
</section>
