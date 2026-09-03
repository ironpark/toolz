<script lang="ts">
    import { Eye, EyeOff, HardDrive, LogIn, Plus, ShieldCheck, Trash2 } from '@lucide/svelte';
    import type { SavedServer } from '../../../bindings/github.com/ironpark/toolz/desktop/charmtrue';
    import * as Alert from '$lib/components/ui/alert';
    import { Badge } from '$lib/components/ui/badge';
    import { Button } from '$lib/components/ui/button';
    import { Checkbox } from '$lib/components/ui/checkbox';
    import * as Dialog from '$lib/components/ui/dialog';
    import { Input } from '$lib/components/ui/input';
    import { Label } from '$lib/components/ui/label';
    import { Separator } from '$lib/components/ui/separator';

    let {
        loading,
        message,
        savedServers,
        savedServersError,
        onclose,
        onconnect,
        onconnectsaved,
        ondelete,
    }: {
        loading: boolean;
        message: string;
        savedServers: SavedServer[];
        savedServersError: string;
        onclose: () => void;
        onconnect: (
            endpoint: string,
            username: string,
            secret: string,
            allowPrivateCertificate: boolean,
            saveServer: boolean,
            saveCredential: boolean,
        ) => Promise<void>;
        onconnectsaved: (id: string) => Promise<void>;
        ondelete: (id: string) => Promise<void>;
    } = $props();

    let endpoint = $state('');
    let username = $state('');
    let secret = $state('');
    let allowPrivateCertificate = $state(true);
    let saveServer = $state(true);
    let saveCredential = $state(false);
    let revealSecret = $state(false);
    let selectedServerID = $state('');
    let deletingServerID = $state('');
    const passwordServers = $derived(savedServers.filter((server) => server.authenticationMethod === 'password'));
    const selectedServer = $derived(passwordServers.find((server) => server.id === selectedServerID));
    const canUseStoredCredential = $derived(Boolean(
        selectedServer?.credentialStored
        && saveServer
        && saveCredential
        && !secret
        && endpoint === selectedServer.endpoint
        && username === selectedServer.username
        && allowPrivateCertificate === selectedServer.allowPrivateCertificate,
    ));

    function selectServer(server: SavedServer): void {
        selectedServerID = server.id;
        endpoint = server.endpoint;
        username = server.username;
        allowPrivateCertificate = server.allowPrivateCertificate;
        saveServer = true;
        saveCredential = server.credentialStored;
        secret = '';
        revealSecret = false;
    }

    function newServer(): void {
        selectedServerID = '';
        endpoint = '';
        username = '';
        secret = '';
        allowPrivateCertificate = true;
        saveServer = true;
        saveCredential = false;
        revealSecret = false;
    }

    async function deleteServer(id: string): Promise<void> {
        deletingServerID = id;
        await ondelete(id);
        deletingServerID = '';
        if (selectedServerID === id) newServer();
    }

    async function submit(event: SubmitEvent): Promise<void> {
        event.preventDefault();
        const form = event.currentTarget as HTMLFormElement;
        if (form.reportValidity()) {
            if (canUseStoredCredential && selectedServerID) {
                await onconnectsaved(selectedServerID);
                return;
            }
            await onconnect(endpoint, username, secret, allowPrivateCertificate, saveServer, saveCredential);
        }
    }
</script>

<Dialog.Root open onOpenChange={(open) => { if (!open) onclose(); }}>
    <Dialog.Content class="max-h-[calc(100vh-2rem)] overflow-y-auto sm:max-w-lg">
        <Dialog.Header>
            <Dialog.Title>TrueNAS 로그인</Dialog.Title>
            <Dialog.Description>저장된 서버를 선택하거나 새 서버의 로그인 정보를 입력하세요.</Dialog.Description>
        </Dialog.Header>

        {#if passwordServers.length > 0 || savedServersError}
            <section class="grid gap-3" aria-labelledby="saved-servers-title">
                <div class="flex items-center justify-between">
                    <div>
                        <p id="saved-servers-title" class="text-sm font-medium">저장된 서버</p>
                        <p class="text-xs text-muted-foreground">키체인 저장 서버는 바로 로그인할 수 있습니다.</p>
                    </div>
                    <Button type="button" variant="ghost" size="sm" onclick={newServer}>
                        <Plus /> 새 서버
                    </Button>
                </div>

                {#if savedServersError}
                    <Alert.Root variant="destructive">
                        <Alert.Title>서버 목록 조회 실패</Alert.Title>
                        <Alert.Description>{savedServersError}</Alert.Description>
                    </Alert.Root>
                {/if}

                {#if passwordServers.length > 0}
                    <div class="grid max-h-48 gap-1 overflow-y-auto rounded-lg bg-muted/40 p-1">
                        {#each passwordServers as server (server.id)}
                            <div class="flex items-center gap-1 rounded-md {selectedServerID === server.id ? 'bg-background shadow-sm' : ''}">
                                <Button
                                    type="button"
                                    variant="ghost"
                                    class="h-auto min-w-0 flex-1 justify-start px-3 py-2 text-left"
                                    aria-pressed={selectedServerID === server.id}
                                    onclick={() => selectServer(server)}
                                >
                                    <span class="flex size-9 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
                                        <HardDrive />
                                    </span>
                                    <span class="min-w-0 flex-1">
                                        <span class="flex items-center gap-2">
                                            <span class="truncate font-medium">{server.name || server.endpoint}</span>
                                            <Badge variant="secondary">비밀번호</Badge>
                                            {#if server.credentialStored}<Badge variant="outline"><ShieldCheck /> 키체인</Badge>{/if}
                                        </span>
                                        <span class="block truncate text-xs font-normal text-muted-foreground">{server.username} · {server.endpoint}</span>
                                    </span>
                                </Button>
                                <Button
                                    type="button"
                                    variant="ghost"
                                    size="icon-sm"
                                    class="mr-1 text-muted-foreground hover:text-destructive"
                                    aria-label={`${server.name || server.endpoint} 저장 정보 삭제`}
                                    disabled={deletingServerID === server.id}
                                    onclick={() => deleteServer(server.id)}
                                >
                                    <Trash2 />
                                </Button>
                            </div>
                        {/each}
                    </div>
                {/if}
            </section>
            <Separator />
        {/if}

        <form class="grid gap-5" onsubmit={submit}>
            <div class="grid gap-2">
                <Label for="endpoint">서버 주소</Label>
                <Input id="endpoint" name="endpoint" bind:value={endpoint} placeholder="truenas.local" autocomplete="url" required />
                <p class="text-xs text-muted-foreground">경로를 생략하면 <code>/api/current</code>를 사용합니다.</p>
            </div>

            <div class="grid gap-2">
                <Label for="username">사용자명</Label>
                <Input id="username" name="username" bind:value={username} placeholder="admin" autocomplete="username" required />
            </div>

            <div class="grid gap-2">
                <Label for="secret">비밀번호</Label>
                <div class="relative">
                    <Input
                        id="secret"
                        name="secret"
                        bind:value={secret}
                        type={revealSecret ? 'text' : 'password'}
                        class="pr-10"
                        autocomplete="current-password"
                        required={!canUseStoredCredential}
                        autofocus={Boolean(selectedServerID)}
                        placeholder={canUseStoredCredential ? '키체인에 저장된 로그인 정보 사용' : ''}
                    />
                    <Button
                        type="button"
                        variant="ghost"
                        size="icon-sm"
                        class="absolute right-1 top-1"
                        aria-label={revealSecret ? '인증 정보 숨기기' : '인증 정보 보기'}
                        onclick={() => revealSecret = !revealSecret}
                    >
                        {#if revealSecret}<EyeOff />{:else}<Eye />{/if}
                    </Button>
                </div>
            </div>

            <div class="grid gap-3 rounded-lg bg-muted/50 p-3">
                <div class="flex items-start gap-3">
                    <Checkbox
                        id="save-server"
                        checked={saveServer}
                        onCheckedChange={(checked) => {
                            saveServer = checked === true;
                            if (!saveServer) saveCredential = false;
                        }}
                    />
                    <div class="grid gap-1">
                        <Label for="save-server">이 서버 저장</Label>
                        <p class="text-xs text-muted-foreground">주소와 사용자명만 저장하며 비밀번호는 저장하지 않습니다.</p>
                    </div>
                </div>
                <div class="flex items-start gap-3">
                    <Checkbox id="save-credential" bind:checked={saveCredential} disabled={!saveServer} />
                    <div class="grid gap-1">
                        <Label for="save-credential">로그인 정보도 키체인에 저장</Label>
                        <p class="text-xs text-muted-foreground">운영체제의 보안 저장소에 암호화하여 보관합니다.</p>
                    </div>
                </div>
                <div class="flex items-start gap-3">
                    <Checkbox id="private-certificate" bind:checked={allowPrivateCertificate} />
                    <div class="grid gap-1">
                        <Label for="private-certificate">사설 인증서 허용</Label>
                        <p class="text-xs text-muted-foreground">신뢰하는 내부 TrueNAS에서만 사용하세요.</p>
                    </div>
                </div>
            </div>

            <div class="flex items-center gap-2 text-xs text-muted-foreground">
                <ShieldCheck class="size-4" />
                비밀번호는 프로필 파일이나 프런트엔드 저장소에 기록되지 않습니다.
            </div>

            {#if message}
                <Alert.Root variant="destructive">
                    <Alert.Title>로그인 실패</Alert.Title>
                    <Alert.Description>{message}</Alert.Description>
                </Alert.Root>
            {/if}

            <Dialog.Footer>
                <Button type="button" variant="outline" onclick={onclose}>취소</Button>
                <Button type="submit" disabled={loading}>
                    <LogIn />
                    {loading ? '로그인 중…' : canUseStoredCredential ? '저장된 정보로 로그인' : '로그인 및 연결'}
                </Button>
            </Dialog.Footer>
        </form>
    </Dialog.Content>
</Dialog.Root>
