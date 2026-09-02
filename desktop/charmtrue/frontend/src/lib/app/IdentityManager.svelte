<script lang="ts">
    import { onMount } from 'svelte';
    import {
        KeyRound,
        Copy,
        Eye,
        EyeOff,
        Pencil,
        Plus,
        RefreshCw,
        Search,
        ShieldCheck,
        Trash2,
        UserRound,
        UsersRound,
        X,
    } from '@lucide/svelte';
    import * as Alert from '$lib/components/ui/alert';
    import { Badge } from '$lib/components/ui/badge';
    import { Button } from '$lib/components/ui/button';
    import * as Card from '$lib/components/ui/card';
    import * as Dialog from '$lib/components/ui/dialog';
    import { Input } from '$lib/components/ui/input';
    import { Skeleton } from '$lib/components/ui/skeleton';
    import * as Table from '$lib/components/ui/table';
    import * as Tabs from '$lib/components/ui/tabs';
    import { getAppContext } from './context.svelte';
    import type { APIKeyInfo, GroupInfo, UserInfo } from '../../../bindings/github.com/ironpark/toolz/desktop/charmtrue';

    type IdentityTab = 'users' | 'groups' | 'keys';
    type IdentityKind = 'user' | 'group' | 'api_key';
    type DeleteTarget = {
        kind: IdentityKind;
        id: number;
        name: string;
        description: string;
    };

    const app = getAppContext();

    let tab = $state<IdentityTab>('users');
    let query = $state('');
    let busy = $state('');
    let deleteTarget = $state<DeleteTarget | null>(null);
    let deleteDialogOpen = $state(false);
    let showSystem = $state(false);
    let editorOpen = $state(false);
    let editorKind = $state<IdentityKind>('user');
    let editID = $state(0);
    let formName = $state('');
    let formUsername = $state('');
    let formFullName = $state('');
    let formEmail = $state('');
    let formUID = $state('');
    let formSetUID = $state(false);
    let formHome = $state('/var/empty');
    let formShell = $state('/usr/bin/zsh');
    let formPassword = $state('');
    let formRandomPassword = $state(false);
    let formSMB = $state(true);
    let formLocked = $state(false);
    let formPasswordDisabled = $state(false);
    let formSSHPasswordEnabled = $state(false);
    let formSSHPublicKey = $state('');
    let formGroupCreate = $state(true);
    let formPrimaryGroupID = $state(0);
    let formGroups = $state<number[]>([]);
    let formHomeCreate = $state(false);
    let formHomeMode = $state('700');
    let formUserNSIDMap = $state('');
    let formSudoCommands = $state('');
    let formSudoCommandsNoPassword = $state('');
    let formExpiresAt = $state('');
    let formReset = $state(false);
    let editorError = $state('');
    let generatedKey = $state('');
    let generatedPassword = $state('');

    onMount(() => {
        if (!app.identity) void app.refreshIdentity();
    });

    const normalizedQuery = $derived(query.trim().toLocaleLowerCase());
    const users = $derived(
        (app.identity?.users ?? []).filter((user) =>
            (showSystem || !user.builtin) && (!normalizedQuery
            || `${user.username} ${user.fullName} ${user.email} ${user.uid}`
                .toLocaleLowerCase()
                .includes(normalizedQuery)),
        ),
    );
    const groups = $derived(
        (app.identity?.groups ?? []).filter((group) =>
            (showSystem || !group.builtin) && (!normalizedQuery
            || `${group.name} ${group.gid}`.toLocaleLowerCase().includes(normalizedQuery)),
        ),
    );
    const keys = $derived(
        (app.identity?.apiKeys ?? []).filter((key) =>
            !normalizedQuery
            || `${key.name} ${key.username}`.toLocaleLowerCase().includes(normalizedQuery),
        ),
    );
    const currentCount = $derived(tab === 'users' ? users.length : tab === 'groups' ? groups.length : keys.length);
    const currentTotal = $derived(
        tab === 'users'
            ? (app.identity?.users?.filter((item) => showSystem || !item.builtin).length ?? 0)
            : tab === 'groups'
                ? (app.identity?.groups?.filter((item) => showSystem || !item.builtin).length ?? 0)
                : (app.identity?.apiKeys?.length ?? 0),
    );
    const searchPlaceholder = $derived(
        tab === 'users'
            ? '이름, 이메일 또는 UID 검색'
            : tab === 'groups'
                ? '그룹 이름 또는 GID 검색'
                : '키 이름 또는 사용자 검색',
    );
    const editableGroups = $derived((app.identity?.groups ?? []).filter((group) => group.local));
    const shellChoices = $derived(Object.entries(app.identity?.shellChoices ?? {}));
    const generatedSecret = $derived(generatedKey || generatedPassword);
    const selectClass = 'h-9 w-full rounded-md border border-input bg-transparent px-2.5 py-1 text-sm shadow-xs outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-input/30';
    const textareaClass = 'min-h-24 w-full resize-y rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-xs outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30';

    function requestDelete(target: DeleteTarget, builtin = false): void {
        if (builtin) return;
        deleteTarget = target;
        deleteDialogOpen = true;
    }

    async function confirmDelete(): Promise<void> {
        if (!deleteTarget) return;
        busy = `${deleteTarget.kind}-${deleteTarget.id}`;
        await app.deleteIdentity(deleteTarget.kind, deleteTarget.id);
        busy = '';
        deleteDialogOpen = false;
        deleteTarget = null;
    }

    function resetEditor(kind: IdentityKind, id = 0): void {
        editorKind = kind; editID = id; formName = ''; formUsername = ''; formFullName = '';
        formEmail = ''; formUID = ''; formSetUID = false; formHome = '/var/empty'; formShell = '/usr/bin/zsh';
        formPassword = ''; formRandomPassword = false; formSMB = true; formLocked = false;
        formPasswordDisabled = false; formSSHPasswordEnabled = false; formSSHPublicKey = '';
        formGroupCreate = true; formPrimaryGroupID = 0; formGroups = []; formHomeCreate = false;
        formHomeMode = '700'; formUserNSIDMap = ''; formSudoCommands = ''; formSudoCommandsNoPassword = '';
        formExpiresAt = ''; formReset = false; editorError = ''; generatedKey = ''; generatedPassword = '';
    }

    function openCreate(): void {
        resetEditor(tab === 'users' ? 'user' : tab === 'groups' ? 'group' : 'api_key');
        editorOpen = true;
    }

    function editUser(user: UserInfo): void {
        resetEditor('user', user.id); formUsername = user.username; formFullName = user.fullName;
        formEmail = user.email; formSMB = user.smb; formLocked = user.locked;
        formUID = String(user.uid); formHome = user.home || '/var/empty'; formShell = user.shell || '/usr/bin/zsh';
        formPasswordDisabled = user.passwordDisabled; formSSHPasswordEnabled = user.sshPasswordEnabled;
        formSSHPublicKey = user.sshPublicKey; formGroupCreate = false; formPrimaryGroupID = user.primaryGroupId;
        formGroups = [...(user.groups ?? [])]; formUserNSIDMap = user.usernsIdmap;
        formSudoCommands = (user.sudoCommands ?? []).join('\n');
        formSudoCommandsNoPassword = (user.sudoCommandsNoPassword ?? []).join('\n'); editorOpen = true;
    }

    function editGroup(group: GroupInfo): void {
        resetEditor('group', group.id); formName = group.name; formSMB = group.smb; editorOpen = true;
    }

    function editKey(key: APIKeyInfo): void {
        resetEditor('api_key', key.id); formName = key.name; formUsername = key.username;
        formExpiresAt = key.expiresAt ? key.expiresAt.slice(0, 16) : ''; editorOpen = true;
    }

    async function saveEditor(event: SubmitEvent): Promise<void> {
        event.preventDefault(); busy = 'save'; editorError = '';
        try {
            if (editorKind === 'user') {
                const result = await app.saveUser({
                    id: editID, uid: Number(formUID || 0), setUid: !editID && formSetUID,
                    username: formUsername, fullName: formFullName, email: formEmail,
                    home: formHome, shell: formShell, password: formPassword, randomPassword: formRandomPassword,
                    smb: formSMB, locked: formLocked, passwordDisabled: formPasswordDisabled,
                    sshPasswordEnabled: formSSHPasswordEnabled, sshPublicKey: formSSHPublicKey,
                    groupCreate: !editID && formGroupCreate, primaryGroupId: formPrimaryGroupID,
                    groups: formGroups, homeCreate: formHomeCreate, homeMode: formHomeMode,
                    usernsIdmap: formUserNSIDMap, sudoCommands: splitLines(formSudoCommands),
                    sudoCommandsNoPassword: splitLines(formSudoCommandsNoPassword),
                });
                if (formRandomPassword && result.password) {
                    generatedPassword = result.password;
                    return;
                }
            } else if (editorKind === 'group') {
                await app.saveGroup({ id: editID, name: formName, smb: formSMB });
            } else {
                const result = await app.saveAPIKey({ id: editID, name: formName, username: formUsername, expiresAt: formExpiresAt ? new Date(formExpiresAt).toISOString() : '', reset: formReset });
                generatedKey = result.key;
                if (generatedKey) return;
            }
            editorOpen = false;
        } catch (error) { editorError = error instanceof Error ? error.message : String(error); }
        finally { busy = ''; }
    }

    function splitLines(value: string): string[] {
        return value.split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
    }

    function toggleSupplementaryGroup(id: number): void {
        formGroups = formGroups.includes(id) ? formGroups.filter((groupID) => groupID !== id) : [...formGroups, id];
    }

    function disableCorrections(node: HTMLTextAreaElement): void {
        node.setAttribute('autocorrect', 'off');
        node.setAttribute('autocapitalize', 'none');
        node.spellcheck = false;
    }
</script>

<section class="space-y-6">
    <header class="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
            <h2 class="text-3xl font-semibold tracking-tight">계정 및 권한</h2>
            <p class="mt-1 text-sm text-muted-foreground">TrueNAS에 접근할 사용자, 그룹과 API 키를 관리합니다.</p>
        </div>
        <div class="flex gap-2">
            <Button variant="outline" onclick={() => app.refreshIdentity()} disabled={app.identityLoading}>
                <RefreshCw class={app.identityLoading ? 'animate-spin' : ''} />
                {app.identityLoading ? '불러오는 중' : '새로고침'}
            </Button>
            <Button onclick={openCreate}><Plus />{tab === 'users' ? '사용자 추가' : tab === 'groups' ? '그룹 추가' : 'API 키 추가'}</Button>
        </div>
    </header>

    {#if app.identityError}
        <Alert.Root variant="destructive">
            <Alert.Title>계정 정보를 불러오지 못했습니다</Alert.Title>
            <Alert.Description>{app.identityError}</Alert.Description>
        </Alert.Root>
    {/if}

    <Card.Root>
        <Card.Header class="gap-4 border-b">
            <div>
                <Card.Title>접근 주체</Card.Title>
                <Card.Description>시스템 계정은 기본적으로 숨겨지며 일반 계정만 편집할 수 있습니다.</Card.Description>
            </div>
            <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                <Tabs.Root bind:value={tab} class="w-full lg:w-auto">
                    <Tabs.List class="grid w-full grid-cols-3 lg:w-auto">
                        <Tabs.Trigger value="users">
                            사용자
                            <Badge variant="secondary">{app.identity?.users?.filter((item) => showSystem || !item.builtin).length ?? 0}</Badge>
                        </Tabs.Trigger>
                        <Tabs.Trigger value="groups">
                            그룹
                            <Badge variant="secondary">{app.identity?.groups?.filter((item) => showSystem || !item.builtin).length ?? 0}</Badge>
                        </Tabs.Trigger>
                        <Tabs.Trigger value="keys">
                            API 키
                            <Badge variant="secondary">{app.identity?.apiKeys?.length ?? 0}</Badge>
                        </Tabs.Trigger>
                    </Tabs.List>
                </Tabs.Root>

                <div class="flex w-full flex-col gap-2 sm:flex-row lg:w-auto">
                    {#if tab !== 'keys'}
                        <Button variant="outline" class="shrink-0" onclick={() => (showSystem = !showSystem)}>
                            {#if showSystem}<EyeOff />시스템 숨기기{:else}<Eye />시스템 표시{/if}
                        </Button>
                    {/if}
                    <div class="relative w-full lg:w-80">
                    <Search class="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                    <Input
                        class="pl-9 pr-9"
                        bind:value={query}
                        placeholder={searchPlaceholder}
                        aria-label={searchPlaceholder}
                    />
                    {#if query}
                        <Button
                            class="absolute right-1 top-1/2 -translate-y-1/2"
                            variant="ghost"
                            size="icon-xs"
                            aria-label="검색어 지우기"
                            onclick={() => (query = '')}
                        >
                            <X />
                        </Button>
                    {/if}
                    </div>
                </div>
            </div>
            <p class="text-xs text-muted-foreground" aria-live="polite">
                {#if normalizedQuery}
                    전체 {currentTotal}개 중 {currentCount}개 검색됨
                {:else}
                    {currentTotal}개 항목
                {/if}
            </p>
        </Card.Header>

        <Card.Content class="p-0">
            {#if app.identityLoading && !app.identity}
                <div class="space-y-3 p-6" aria-label="계정 정보 불러오는 중">
                    {#each Array(5) as _}
                        <div class="flex items-center gap-4">
                            <Skeleton class="size-9 rounded-full" />
                            <div class="flex-1 space-y-2">
                                <Skeleton class="h-4 w-40" />
                                <Skeleton class="h-3 w-64 max-w-full" />
                            </div>
                            <Skeleton class="h-8 w-16" />
                        </div>
                    {/each}
                </div>
            {:else if currentCount === 0}
                <div class="flex min-h-64 flex-col items-center justify-center px-6 py-12 text-center">
                    <div class="mb-4 rounded-full bg-muted p-3 text-muted-foreground">
                        {#if normalizedQuery}
                            <Search class="size-6" />
                        {:else if tab === 'users'}
                            <UserRound class="size-6" />
                        {:else if tab === 'groups'}
                            <UsersRound class="size-6" />
                        {:else}
                            <KeyRound class="size-6" />
                        {/if}
                    </div>
                    <p class="font-medium">{normalizedQuery ? '검색 결과가 없습니다' : '등록된 항목이 없습니다'}</p>
                    <p class="mt-1 max-w-sm text-sm text-muted-foreground">
                        {normalizedQuery ? '다른 이름이나 식별자로 다시 검색해 보세요.' : 'TrueNAS에서 항목을 추가하면 여기에 표시됩니다.'}
                    </p>
                    {#if normalizedQuery}
                        <Button class="mt-4" variant="outline" size="sm" onclick={() => (query = '')}>검색 초기화</Button>
                    {/if}
                </div>
            {:else}
                <Table.Root>
                    <Table.Header>
                        <Table.Row>
                            <Table.Head>이름</Table.Head>
                            <Table.Head class="hidden md:table-cell">식별자</Table.Head>
                            <Table.Head>상태</Table.Head>
                            <Table.Head class="hidden lg:table-cell">세부 정보</Table.Head>
                            <Table.Head class="w-36 text-right">작업</Table.Head>
                        </Table.Row>
                    </Table.Header>
                    <Table.Body>
                        {#if tab === 'users'}
                            {#each users as user (user.id)}
                                <Table.Row>
                                    <Table.Cell class="min-w-52 whitespace-normal">
                                        <div class="flex items-center gap-3">
                                            <span class="flex size-9 shrink-0 items-center justify-center rounded-full bg-muted text-sm font-semibold uppercase text-muted-foreground">
                                                {user.username.slice(0, 1)}
                                            </span>
                                            <div class="min-w-0">
                                                <p class="truncate font-medium">{user.username}</p>
                                                <p class="truncate text-xs text-muted-foreground">{user.fullName || user.email || '추가 정보 없음'}</p>
                                            </div>
                                        </div>
                                    </Table.Cell>
                                    <Table.Cell class="hidden text-muted-foreground md:table-cell">UID {user.uid}</Table.Cell>
                                    <Table.Cell>
                                        {#if user.locked}
                                            <Badge variant="destructive">잠김</Badge>
                                        {:else if user.passwordDisabled}
                                            <Badge variant="secondary">비밀번호 꺼짐</Badge>
                                        {:else}
                                            <Badge variant="outline">사용 가능</Badge>
                                        {/if}
                                    </Table.Cell>
                                    <Table.Cell class="hidden lg:table-cell">
                                        <div class="flex flex-wrap gap-1.5">
                                            <Badge variant="secondary">{user.builtin ? '시스템' : user.local ? '로컬' : '디렉터리'}</Badge>
                                            {#if user.email}<span class="max-w-52 truncate text-xs text-muted-foreground">{user.email}</span>{/if}
                                        </div>
                                    </Table.Cell>
                                    <Table.Cell class="text-right">
                                        {#if user.builtin || user.immutable}
                                            <span class="inline-flex items-center gap-1 text-xs text-muted-foreground" title="시스템 사용자는 삭제할 수 없습니다">
                                                <ShieldCheck class="size-4" /> 보호됨
                                            </span>
                                        {:else if user.local}
                                            <div class="flex justify-end gap-1">
                                            <Button variant="ghost" size="icon-sm" aria-label={`${user.username} 수정`} onclick={() => editUser(user)}><Pencil /></Button>
                                            <Button
                                                variant="ghost"
                                                size="icon-sm"
                                                class="text-destructive hover:text-destructive"
                                                aria-label={`${user.username} 삭제`}
                                                disabled={busy === `user-${user.id}`}
                                                onclick={() => requestDelete({ kind: 'user', id: user.id, name: user.username, description: `UID ${user.uid}` })}
                                            >
                                                <Trash2 />
                                            </Button>
                                            </div>
                                        {:else}
                                            <span class="text-xs text-muted-foreground">읽기 전용</span>
                                        {/if}
                                    </Table.Cell>
                                </Table.Row>
                            {/each}
                        {:else if tab === 'groups'}
                            {#each groups as group (group.id)}
                                <Table.Row>
                                    <Table.Cell class="min-w-52 whitespace-normal">
                                        <div class="flex items-center gap-3">
                                            <span class="flex size-9 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground"><UsersRound class="size-4" /></span>
                                            <div>
                                                <p class="font-medium">{group.name}</p>
                                                <p class="text-xs text-muted-foreground">구성원 {group.userCount}명</p>
                                            </div>
                                        </div>
                                    </Table.Cell>
                                    <Table.Cell class="hidden text-muted-foreground md:table-cell">GID {group.gid}</Table.Cell>
                                    <Table.Cell><Badge variant="outline">{group.builtin ? '시스템' : group.local ? '로컬' : '디렉터리'}</Badge></Table.Cell>
                                    <Table.Cell class="hidden text-sm text-muted-foreground lg:table-cell">사용자 {group.userCount}명</Table.Cell>
                                    <Table.Cell class="text-right">
                                        {#if group.builtin}
                                            <span class="inline-flex items-center gap-1 text-xs text-muted-foreground" title="시스템 그룹은 삭제할 수 없습니다">
                                                <ShieldCheck class="size-4" /> 보호됨
                                            </span>
                                        {:else if group.local}
                                            <div class="flex justify-end gap-1">
                                            <Button variant="ghost" size="icon-sm" aria-label={`${group.name} 수정`} onclick={() => editGroup(group)}><Pencil /></Button>
                                            <Button
                                                variant="ghost"
                                                size="icon-sm"
                                                class="text-destructive hover:text-destructive"
                                                aria-label={`${group.name} 삭제`}
                                                disabled={busy === `group-${group.id}`}
                                                onclick={() => requestDelete({ kind: 'group', id: group.id, name: group.name, description: `GID ${group.gid} · 구성원 ${group.userCount}명` })}
                                            >
                                                <Trash2 />
                                            </Button>
                                            </div>
                                        {:else}
                                            <span class="text-xs text-muted-foreground">읽기 전용</span>
                                        {/if}
                                    </Table.Cell>
                                </Table.Row>
                            {/each}
                        {:else}
                            {#each keys as key (key.id)}
                                <Table.Row>
                                    <Table.Cell class="min-w-52 whitespace-normal">
                                        <div class="flex items-center gap-3">
                                            <span class="flex size-9 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground"><KeyRound class="size-4" /></span>
                                            <div>
                                                <p class="font-medium">{key.name}</p>
                                                <p class="text-xs text-muted-foreground">{key.username}</p>
                                            </div>
                                        </div>
                                    </Table.Cell>
                                    <Table.Cell class="hidden text-muted-foreground md:table-cell">ID {key.id}</Table.Cell>
                                    <Table.Cell><Badge variant={key.revoked ? 'destructive' : 'outline'}>{key.revoked ? '폐기됨' : '활성'}</Badge></Table.Cell>
                                    <Table.Cell class="hidden text-sm text-muted-foreground lg:table-cell">{key.username} 소유</Table.Cell>
                                    <Table.Cell class="text-right">
                                        <div class="flex justify-end gap-1">
                                        <Button variant="ghost" size="icon-sm" aria-label={`${key.name} 수정`} onclick={() => editKey(key)}><Pencil /></Button>
                                        <Button
                                            variant="ghost"
                                            size="icon-sm"
                                            class="text-destructive hover:text-destructive"
                                            aria-label={`${key.name} 삭제`}
                                            disabled={busy === `api_key-${key.id}`}
                                            onclick={() => requestDelete({ kind: 'api_key', id: key.id, name: key.name, description: `${key.username} 사용자의 API 키` })}
                                        >
                                            <Trash2 />
                                        </Button>
                                        </div>
                                    </Table.Cell>
                                </Table.Row>
                            {/each}
                        {/if}
                    </Table.Body>
                </Table.Root>
            {/if}
        </Card.Content>
    </Card.Root>
</section>

<Dialog.Root bind:open={editorOpen}>
    <Dialog.Content class="max-h-[88dvh] overflow-y-auto sm:max-w-2xl">
        {#if generatedSecret}
            <Dialog.Header>
                <Dialog.Title>{generatedPassword ? '임의 비밀번호가 생성되었습니다' : 'API 키가 생성되었습니다'}</Dialog.Title>
                <Dialog.Description>이 값은 다시 표시되지 않습니다. 지금 안전한 곳에 복사하세요.</Dialog.Description>
            </Dialog.Header>
            <div class="rounded-lg border bg-muted/40 p-4">
                <code class="block break-all text-sm select-all">{generatedSecret}</code>
            </div>
            <Dialog.Footer>
                <Button variant="outline" onclick={() => navigator.clipboard.writeText(generatedSecret)}><Copy />복사</Button>
                <Button onclick={() => (editorOpen = false)}>확인</Button>
            </Dialog.Footer>
        {:else}
            <form onsubmit={saveEditor} class="space-y-5">
                <Dialog.Header>
                    <Dialog.Title>{editID ? '수정' : '추가'} · {editorKind === 'user' ? '사용자' : editorKind === 'group' ? '그룹' : 'API 키'}</Dialog.Title>
                    <Dialog.Description>{editID ? '변경할 항목만 확인한 뒤 저장하세요.' : '필수 정보를 입력하면 TrueNAS에 바로 생성됩니다.'}</Dialog.Description>
                </Dialog.Header>

                {#if editorKind === 'user'}
                    <div class="space-y-5">
                        <section class="space-y-3">
                            <div><h3 class="text-sm font-semibold">기본 정보</h3><p class="text-xs text-muted-foreground">계정의 이름과 로그인 정보를 설정합니다.</p></div>
                            <div class="grid gap-4 sm:grid-cols-2">
                                <label class="grid gap-2 text-sm font-medium" for="identity-username">사용자명<Input id="identity-username" bind:value={formUsername} required /></label>
                                <label class="grid gap-2 text-sm font-medium" for="identity-full-name">표시 이름<Input id="identity-full-name" bind:value={formFullName} required /></label>
                                <label class="grid gap-2 text-sm font-medium sm:col-span-2" for="identity-email">이메일<Input id="identity-email" type="email" bind:value={formEmail} placeholder="선택 사항" /></label>
                                {#if !editID}
                                    <div class="grid gap-2 sm:col-span-2">
                                        <label class="flex items-center gap-2 text-sm"><input class="size-4 accent-primary" type="checkbox" bind:checked={formSetUID} />UID 직접 지정</label>
                                        {#if formSetUID}<Input type="number" min="0" max="90000000" bind:value={formUID} placeholder="예: 3000" required />{/if}
                                    </div>
                                {/if}
                                <label class="grid gap-2 text-sm font-medium sm:col-span-2" for="identity-password">
                                    {editID ? '새 비밀번호' : '비밀번호'}
                                    <Input id="identity-password" type="password" bind:value={formPassword} autocomplete="new-password" disabled={formRandomPassword} required={!editID && !formPasswordDisabled && !formRandomPassword} />
                                    <span class="text-xs font-normal text-muted-foreground">{editID ? '변경하지 않으려면 비워 두세요.' : '직접 입력하거나 안전한 임의 비밀번호를 생성할 수 있습니다.'}</span>
                                </label>
                                <label class="flex items-start gap-2 rounded-lg border p-3 text-sm sm:col-span-2"><input class="mt-0.5 size-4 accent-primary" type="checkbox" bind:checked={formRandomPassword} onchange={() => { if (formRandomPassword) formPassword = ''; }} /><span><strong class="block font-medium">임의 비밀번호 생성</strong><span class="text-xs text-muted-foreground">TrueNAS가 20자 비밀번호를 생성하며 저장 후 한 번만 표시합니다.</span></span></label>
                            </div>
                        </section>

                        <section class="space-y-3 border-t pt-5">
                            <div><h3 class="text-sm font-semibold">그룹 및 접근</h3><p class="text-xs text-muted-foreground">기본 그룹과 프로토콜별 로그인 허용 여부를 지정합니다.</p></div>
                            {#if !editID}
                                <label class="flex items-center gap-2 text-sm"><input class="size-4 accent-primary" type="checkbox" bind:checked={formGroupCreate} />사용자명과 같은 기본 그룹 자동 생성</label>
                            {/if}
                            {#if editID || !formGroupCreate}
                                <label class="grid gap-2 text-sm font-medium" for="identity-primary-group">기본 그룹
                                    <select id="identity-primary-group" class={selectClass} bind:value={formPrimaryGroupID} required>
                                        <option value={0} disabled>기본 그룹 선택</option>
                                        {#each editableGroups as group (group.id)}<option value={group.id}>{group.name} · GID {group.gid}</option>{/each}
                                    </select>
                                </label>
                            {/if}
                            <fieldset class="grid gap-2">
                                <legend class="text-sm font-medium">보조 그룹</legend>
                                <div class="grid max-h-32 gap-2 overflow-y-auto rounded-lg border p-3 sm:grid-cols-2">
                                    {#if editableGroups.length === 0}<p class="text-xs text-muted-foreground">선택 가능한 로컬 그룹이 없습니다.</p>{/if}
                                    {#each editableGroups.filter((group) => group.id !== formPrimaryGroupID) as group (group.id)}
                                        <label class="flex items-center gap-2 text-sm"><input class="size-4 accent-primary" type="checkbox" checked={formGroups.includes(group.id)} onchange={() => toggleSupplementaryGroup(group.id)} />{group.name}</label>
                                    {/each}
                                </div>
                            </fieldset>
                            <div class="grid gap-3 rounded-lg border p-4 sm:grid-cols-2">
                                <label class="flex items-center gap-2 text-sm"><input class="size-4 accent-primary" type="checkbox" bind:checked={formSMB} onchange={() => { if (formSMB) formPasswordDisabled = false; }} />SMB 접근 허용</label>
                                <label class="flex items-center gap-2 text-sm"><input class="size-4 accent-primary" type="checkbox" bind:checked={formPasswordDisabled} disabled={formSMB} onchange={() => { if (formPasswordDisabled) formSSHPasswordEnabled = false; }} />비밀번호 로그인 끄기</label>
                                <label class="flex items-center gap-2 text-sm"><input class="size-4 accent-primary" type="checkbox" bind:checked={formSSHPasswordEnabled} disabled={formPasswordDisabled} />SSH 비밀번호 로그인</label>
                                <label class="flex items-center gap-2 text-sm"><input class="size-4 accent-primary" type="checkbox" bind:checked={formLocked} />계정 잠금</label>
                            </div>
                        </section>

                        <details class="group rounded-lg border">
                            <summary class="cursor-pointer list-none px-4 py-3 text-sm font-semibold">고급 옵션 <span class="float-right text-xs font-normal text-muted-foreground group-open:hidden">펼치기</span><span class="float-right hidden text-xs font-normal text-muted-foreground group-open:inline">접기</span></summary>
                            <div class="space-y-4 border-t p-4">
                                <div class="grid gap-4 sm:grid-cols-[1fr_8rem]">
                                    <label class="grid gap-2 text-sm font-medium" for="identity-home">홈 디렉터리<Input id="identity-home" bind:value={formHome} required /></label>
                                    <label class="grid gap-2 text-sm font-medium" for="identity-home-mode">홈 권한<Input id="identity-home-mode" bind:value={formHomeMode} pattern={'[0-7]{3,4}'} placeholder="700" required /></label>
                                </div>
                                <label class="flex items-center gap-2 text-sm"><input class="size-4 accent-primary" type="checkbox" bind:checked={formHomeCreate} />지정한 홈 디렉터리 생성</label>
                                <label class="grid gap-2 text-sm font-medium" for="identity-shell">로그인 셸
                                    <select id="identity-shell" class={selectClass} bind:value={formShell} required>
                                        {#if formShell && !shellChoices.some(([path]) => path === formShell)}<option value={formShell}>{formShell}</option>{/if}
                                        {#if shellChoices.length === 0}<option value="/usr/bin/zsh">/usr/bin/zsh</option>{/if}
                                        {#each shellChoices as [path, name]}<option value={path}>{name} · {path}</option>{/each}
                                    </select>
                                </label>
                                <label class="grid gap-2 text-sm font-medium" for="identity-ssh-key">SSH 공개키
                                    <textarea id="identity-ssh-key" class={textareaClass} bind:value={formSSHPublicKey} use:disableCorrections placeholder="ssh-ed25519 AAAA…"></textarea>
                                </label>
                                <label class="grid gap-2 text-sm font-medium" for="identity-userns">컨테이너 UID 매핑<Input id="identity-userns" bind:value={formUserNSIDMap} placeholder="비워 둠, DIRECT 또는 숫자" /><span class="text-xs font-normal text-muted-foreground">컨테이너에 직접 매핑하려면 DIRECT, 특정 UID에는 숫자를 입력합니다.</span></label>
                                <div class="grid gap-4 sm:grid-cols-2">
                                    <label class="grid gap-2 text-sm font-medium" for="identity-sudo">비밀번호 필요 sudo 명령<textarea id="identity-sudo" class={textareaClass} bind:value={formSudoCommands} use:disableCorrections placeholder={'/usr/bin/ls\n/usr/bin/systemctl status'}></textarea><span class="text-xs font-normal text-muted-foreground">한 줄에 하나씩 입력합니다.</span></label>
                                    <label class="grid gap-2 text-sm font-medium" for="identity-sudo-nopasswd">비밀번호 없는 sudo 명령<textarea id="identity-sudo-nopasswd" class={textareaClass} bind:value={formSudoCommandsNoPassword} use:disableCorrections placeholder="/usr/bin/zfs list"></textarea><span class="text-xs font-normal text-muted-foreground">권한 범위를 최소화하세요.</span></label>
                                </div>
                            </div>
                        </details>
                    </div>
                {:else if editorKind === 'group'}
                    <label class="grid gap-2 text-sm font-medium" for="identity-group-name">그룹 이름<Input id="identity-group-name" bind:value={formName} required /></label>
                    <label class="flex items-center gap-2 rounded-lg border p-4 text-sm"><input class="size-4 accent-primary" type="checkbox" bind:checked={formSMB} />SMB ACL에서 이 그룹 사용</label>
                {:else}
                    <label class="grid gap-2 text-sm font-medium" for="identity-key-name">키 이름<Input id="identity-key-name" bind:value={formName} maxlength={200} required /></label>
                    {#if !editID}<label class="grid gap-2 text-sm font-medium" for="identity-key-owner">소유 사용자<Input id="identity-key-owner" bind:value={formUsername} placeholder="예: admin" required /></label>{/if}
                    <label class="grid gap-2 text-sm font-medium" for="identity-key-expiry">만료 시간<Input id="identity-key-expiry" type="datetime-local" bind:value={formExpiresAt} /><span class="text-xs font-normal text-muted-foreground">비워 두면 만료되지 않습니다.</span></label>
                    {#if editID}<label class="flex items-start gap-2 rounded-lg border border-destructive/30 bg-destructive/5 p-4 text-sm"><input class="mt-0.5 size-4 accent-destructive" type="checkbox" bind:checked={formReset} /><span><strong class="block">키 재발급</strong><span class="text-xs text-muted-foreground">기존 키는 즉시 사용할 수 없게 됩니다.</span></span></label>{/if}
                {/if}

                {#if editorError}<Alert.Root variant="destructive"><Alert.Title>저장하지 못했습니다</Alert.Title><Alert.Description>{editorError}</Alert.Description></Alert.Root>{/if}
                <Dialog.Footer>
                    <Button type="button" variant="outline" disabled={busy === 'save'} onclick={() => (editorOpen = false)}>취소</Button>
                    <Button type="submit" disabled={busy === 'save'}>{#if busy === 'save'}<RefreshCw class="animate-spin" />{/if}{busy === 'save' ? '저장하는 중' : '저장'}</Button>
                </Dialog.Footer>
            </form>
        {/if}
    </Dialog.Content>
</Dialog.Root>

<Dialog.Root bind:open={deleteDialogOpen}>
    <Dialog.Content>
        <Dialog.Header>
            <Dialog.Title>“{deleteTarget?.name}”을 삭제할까요?</Dialog.Title>
            <Dialog.Description>
                {deleteTarget?.description}. 이 작업은 되돌릴 수 없으며 연결된 서비스의 접근이 중단될 수 있습니다.
            </Dialog.Description>
        </Dialog.Header>
        <Dialog.Footer>
            <Button variant="outline" disabled={Boolean(busy)} onclick={() => (deleteDialogOpen = false)}>취소</Button>
            <Button variant="destructive" disabled={Boolean(busy)} onclick={confirmDelete}>
                {#if busy}<RefreshCw class="animate-spin" />{/if}
                {busy ? '삭제하는 중' : '삭제'}
            </Button>
        </Dialog.Footer>
    </Dialog.Content>
</Dialog.Root>
