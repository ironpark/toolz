<script lang="ts">
    import { onMount } from 'svelte';
    import * as Alert from '$lib/components/ui/alert';
    import { Button } from '$lib/components/ui/button';
    import * as Card from '$lib/components/ui/card';
    import { Spinner } from '$lib/components/ui/spinner';
    import { Switch } from '$lib/components/ui/switch';
    import { TrueNASService } from '../../../bindings/github.com/ironpark/toolz/desktop/charmtrue';

    let enabled = $state(false);
    let saved = $state<boolean | null>(null);
    let loading = $state(true);
    let saving = $state(false);
    let error = $state('');
    let message = $state('');

    onMount(() => { void load(); });

    async function load(): Promise<void> {
        loading = true;
        error = '';
        try {
            saved = await TrueNASService.HTTPSRedirect();
            enabled = saved;
        } catch (e) {
            error = e instanceof Error ? e.message : String(e);
        } finally {
            loading = false;
        }
    }

    async function save(): Promise<void> {
        saving = true;
        error = '';
        message = '';
        try {
            await TrueNASService.SetHTTPSRedirect(enabled);
            saved = enabled;
            message = '설정을 저장했습니다. 웹 UI가 재시작되며 연결이 끊기면 다시 연결해 주세요.';
        } catch (e) {
            error = e instanceof Error ? e.message : String(e);
        } finally {
            saving = false;
        }
    }
</script>

<Card.Root>
    <Card.Header>
        <Card.Title>웹 UI 접속 설정</Card.Title>
        <Card.Description>변경 사항을 저장하면 웹 UI가 재시작되어 연결이 잠시 끊길 수 있습니다.</Card.Description>
    </Card.Header>
    <Card.Content class="space-y-4">
        {#if error}<Alert.Root variant="destructive"><Alert.Title>설정 처리 실패</Alert.Title><Alert.Description>{error}</Alert.Description></Alert.Root>{/if}
        <div class="flex items-center justify-between gap-4">
            <div class="space-y-1">
                <label for="https-redirect" class="text-sm font-medium">HTTPS 리다이렉션 활성화</label>
                <p id="https-redirect-description" class="text-sm text-muted-foreground">TrueNAS 웹 UI의 HTTP 접속을 HTTPS로 자동 전환합니다. 활성화 전 HTTPS 접속이 가능한지 확인하세요.</p>
            </div>
            {#if loading}<Spinner aria-label="설정 불러오는 중" />{/if}
            <Switch id="https-redirect" aria-describedby="https-redirect-description" bind:checked={enabled} disabled={loading || saving || saved === null} />
        </div>
        {#if message}<p role="status" class="text-sm text-muted-foreground">{message}</p>{/if}
    </Card.Content>
    <Card.Footer class="gap-2">
        <Button disabled={loading || saving || saved === null || enabled === saved} onclick={save}>{#if saving}<Spinner />{/if}저장</Button>
        {#if saved === null && !loading}<Button variant="outline" onclick={load}>다시 불러오기</Button>{/if}
    </Card.Footer>
</Card.Root>
