<script lang="ts">
    import { tick } from 'svelte';
    import { ArrowLeft, ChevronRight, Eye, EyeOff, HardDrive, Plus, ShieldCheck, Trash2 } from '@lucide/svelte';
    import type { SavedServer } from '../../../bindings/github.com/ironpark/toolz/desktop/charmtrue';
    import * as Alert from '$lib/components/ui/alert';
    import { Badge } from '$lib/components/ui/badge';
    import { Button } from '$lib/components/ui/button';
    import { Checkbox } from '$lib/components/ui/checkbox';
    import * as Dialog from '$lib/components/ui/dialog';
    import { Input } from '$lib/components/ui/input';
    import { Label } from '$lib/components/ui/label';
    import { Spinner } from '$lib/components/ui/spinner';

    let { open, loading, message, savedServers, savedServersError, savedServersLoading = false, activeEndpoint = '', onclose, onconnect, onconnectsaved, ondelete, onclearmessage }: {
        open: boolean;
        loading: boolean;
        message: string;
        savedServers: SavedServer[];
        savedServersError: string;
        savedServersLoading?: boolean;
        activeEndpoint?: string;
        onclose: () => void;
        onconnect: (endpoint: string, username: string, secret: string, allowPrivateCertificate: boolean, saveServer: boolean, saveCredential: boolean, authenticationMethod: string) => Promise<boolean>;
        onconnectsaved: (id: string) => Promise<boolean>;
        ondelete: (id: string) => Promise<void>;
        onclearmessage: () => void;
    } = $props();

    let panel = $state<'list' | 'add' | 'login'>('list');
    let endpoint = $state('');
    let username = $state('');
    let secret = $state('');
    let authenticationMethod = $state('password');
    let allowPrivateCertificate = $state(true);
    let saveCredential = $state(true);
    let revealSecret = $state(false);
    let selectedServerID = $state('');
    let deletingServerID = $state('');
    let endpointInput = $state<HTMLInputElement | null>(null);
    let secretInput = $state<HTMLInputElement | null>(null);
    let addButton = $state<HTMLButtonElement | null>(null);
    const busy = $derived(loading || deletingServerID !== '');

    $effect(() => {
        if (open) {
            panel = 'list';
            selectedServerID = '';
        }
        secret = '';
        revealSecret = false;
    });

    async function showForm(mode: 'add' | 'login'): Promise<void> {
        panel = mode;
        await tick();
        (mode === 'add' ? endpointInput : secretInput)?.focus({ preventScroll: true });
    }

    async function selectServer(server: SavedServer): Promise<void> {
        if (busy) return;
        onclearmessage();
        selectedServerID = server.id;
        endpoint = server.endpoint;
        username = server.username;
        authenticationMethod = server.authenticationMethod;
        allowPrivateCertificate = server.allowPrivateCertificate;
        saveCredential = true;
        secret = '';
        revealSecret = false;
        if (server.credentialStored && await onconnectsaved(server.id)) return;
        await showForm('login');
    }

    async function newServer(): Promise<void> {
        if (busy) return;
        onclearmessage();
        selectedServerID = '';
        endpoint = '';
        username = '';
        secret = '';
        authenticationMethod = 'password';
        allowPrivateCertificate = true;
        saveCredential = true;
        revealSecret = false;
        await showForm('add');
    }

    async function back(): Promise<void> {
        if (busy) return;
        panel = 'list';
        secret = '';
        revealSecret = false;
        onclearmessage();
        await tick();
        addButton?.focus({ preventScroll: true });
    }

    async function deleteServer(id: string): Promise<void> {
        if (busy) return;
        deletingServerID = id;
        try { await ondelete(id); } finally { deletingServerID = ''; }
    }

    async function submit(event: SubmitEvent): Promise<void> {
        event.preventDefault();
        if (busy) return;
        if ((event.currentTarget as HTMLFormElement).reportValidity()) {
            await onconnect(endpoint, username, secret, allowPrivateCertificate, true, saveCredential, authenticationMethod);
        }
    }
</script>

<Dialog.Root {open} onOpenChange={(nextOpen) => { if (!nextOpen && !busy) onclose(); }}>
    <Dialog.Content class="gap-4 overflow-hidden sm:max-w-lg" showCloseButton={!busy}>
        <Dialog.Header class="pr-6">
            <Dialog.Title>{panel === 'list' ? '서버 선택' : panel === 'add' ? '서버 추가' : '서버 로그인'}</Dialog.Title>
            <Dialog.Description>{panel === 'list' ? '서버를 클릭하면 저장된 로그인 정보로 연결하거나 서버를 전환합니다.' : panel === 'add' ? '연결할 서버의 주소와 로그인 정보를 입력하세요.' : '이 서버에 연결할 로그인 정보를 입력하세요.'}</Dialog.Description>
        </Dialog.Header>
        <div class="min-w-0 w-full overflow-clip">
            <div class="flex h-[min(560px,calc(100dvh-12rem))] w-full min-w-0 transition-transform duration-300 ease-in-out motion-reduce:transition-none" style:transform={panel === 'list' ? 'translateX(0)' : 'translateX(-100%)'}>
                <section class="flex min-w-0 basis-full shrink-0 flex-col gap-4 overflow-x-hidden overflow-y-auto p-1" inert={panel !== 'list'} aria-hidden={panel !== 'list'} aria-label="저장된 서버">
                    <div class="flex items-center justify-between"><p class="text-sm font-medium">등록된 서버 {savedServers.length}개</p><Button bind:ref={addButton} size="sm" variant="outline" disabled={busy} onclick={newServer}><Plus />추가</Button></div>
                    {#if savedServersError}<Alert.Root variant="destructive"><Alert.Title>서버 목록 처리 실패</Alert.Title><Alert.Description>{savedServersError}</Alert.Description></Alert.Root>{/if}
                    {#if message}<Alert.Root variant="destructive"><Alert.Title>연결 실패</Alert.Title><Alert.Description>{message}</Alert.Description></Alert.Root>{/if}
                    {#if savedServersLoading && !savedServers.length}<div role="status" class="grid flex-1 place-items-center"><Spinner aria-label="서버 목록 불러오는 중" /></div>
                    {:else if savedServers.length}
                        <div class="space-y-2">{#each savedServers as server (server.id)}
                            <div class="flex items-center rounded-lg border bg-muted/20">
                                <Button variant="ghost" class="h-auto min-w-0 flex-1 justify-start gap-3 p-3 text-left" disabled={busy} onclick={() => selectServer(server)}>
                                    {#if loading && selectedServerID === server.id}<Spinner />{:else}<HardDrive class="size-5 shrink-0 text-muted-foreground" />{/if}
                                    <span class="min-w-0 flex-1"><span class="flex flex-wrap items-center gap-2"><span class="truncate font-medium">{server.name || server.endpoint}</span>{#if server.endpoint === activeEndpoint}<Badge variant="secondary">연결 중</Badge>{/if}</span><span class="mt-1 block truncate text-xs font-normal text-muted-foreground">{server.username} · {server.endpoint}</span><span class="mt-1 block text-xs font-normal text-muted-foreground">{server.credentialStored ? '클릭하여 바로 연결' : '로그인 정보 입력 필요'}</span></span>
                                    <ChevronRight class="size-4 shrink-0" />
                                </Button>
                                <Button variant="ghost" size="icon-sm" class="mr-2 shrink-0 text-muted-foreground hover:text-destructive" aria-label={server.name + ' 저장 정보 삭제'} disabled={busy} onclick={() => deleteServer(server.id)}>{#if deletingServerID === server.id}<Spinner />{:else}<Trash2 />{/if}</Button>
                            </div>
                        {/each}</div>
                    {:else}<div class="grid flex-1 content-center justify-items-center gap-3 py-10 text-center"><HardDrive class="size-8 text-muted-foreground" /><p class="text-sm font-medium">등록된 서버가 없습니다</p><p class="text-xs text-muted-foreground">서버를 추가하면 다음부터 목록에서 바로 연결할 수 있습니다.</p><Button onclick={newServer} disabled={busy}><Plus />서버 추가</Button></div>{/if}
                </section>
                <section class="min-w-0 basis-full shrink-0 overflow-x-hidden overflow-y-auto p-1" inert={panel === 'list'} aria-hidden={panel === 'list'} aria-label={panel === 'add' ? '서버 추가' : '서버 로그인'}>
                    <Button variant="ghost" size="sm" class="mb-4" disabled={busy} onclick={back}><ArrowLeft />서버 목록</Button>
                    <form class="grid gap-4" onsubmit={submit}>
                        <fieldset disabled={busy} class="grid min-w-0 gap-4">
                            {#if panel === 'login'}<div class="rounded-lg bg-muted/50 p-3 text-sm"><p class="font-medium">{username}</p><p class="break-all text-muted-foreground">{endpoint}</p></div>{/if}
                            <div class={panel === 'login' ? 'hidden' : 'grid gap-2'}><Label for="endpoint">서버 주소</Label><Input bind:ref={endpointInput} id="endpoint" bind:value={endpoint} placeholder="truenas.local" autocomplete="url" required /></div>
                            <div class={panel === 'login' ? 'hidden' : 'grid gap-2'}><Label for="username">사용자명</Label><Input id="username" bind:value={username} placeholder="admin" autocomplete="username" required /></div>
                            <div class="grid gap-2"><Label for="secret">{authenticationMethod === 'api_key' ? 'API 키' : '비밀번호'}</Label><div class="relative"><Input bind:ref={secretInput} id="secret" bind:value={secret} type={revealSecret ? 'text' : 'password'} class="pr-10" autocomplete={authenticationMethod === 'api_key' ? 'off' : 'current-password'} required /><Button type="button" variant="ghost" size="icon-sm" class="absolute right-1 top-1" aria-label={revealSecret ? '로그인 정보 숨기기' : '로그인 정보 보기'} onclick={() => revealSecret = !revealSecret}>{#if revealSecret}<EyeOff />{:else}<Eye />{/if}</Button></div></div>
                            <div class="grid gap-4 rounded-lg bg-muted/50 p-3">
                                <div class="flex items-start gap-3"><Checkbox id="save-credential" bind:checked={saveCredential} /><div class="grid gap-1"><Label for="save-credential">로그인 정보를 키체인에 저장</Label><p class="text-xs text-muted-foreground">다음부터 서버를 클릭하면 바로 연결합니다.</p></div></div>
                                <div class="flex items-start gap-3"><Checkbox id="private-certificate" bind:checked={allowPrivateCertificate} /><div class="grid gap-1"><Label for="private-certificate">사설 인증서 허용</Label><p class="text-xs text-muted-foreground">신뢰하는 내부 TrueNAS에서 사용하세요.</p></div></div>
                            </div>
                        </fieldset>
                        <p class="flex items-center gap-2 text-xs text-muted-foreground"><ShieldCheck class="size-4 shrink-0" />서버 주소와 사용자명은 앱에, 로그인 정보는 선택 시 운영체제 키체인에 저장합니다.</p>
                        {#if message}<Alert.Root variant="destructive"><Alert.Title>연결 실패</Alert.Title><Alert.Description>{message}</Alert.Description></Alert.Root>{/if}
                        <Button type="submit" disabled={busy}>{#if loading}<Spinner />{/if}{loading ? '연결 중…' : panel === 'add' ? '서버 추가 및 연결' : '연결'}</Button>
                    </form>
                </section>
            </div>
        </div>
    </Dialog.Content>
</Dialog.Root>
