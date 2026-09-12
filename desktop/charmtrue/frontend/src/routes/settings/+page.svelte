<script lang="ts">
    import { Button } from '$lib/components/ui/button';
    import * as Card from '$lib/components/ui/card';
    import { getAppContext } from '$lib/app/context.svelte';

    const app = getAppContext();
</script>

<section class="space-y-6">
    <p class="text-sm text-muted-foreground">CharmTrue 앱의 서버 연결과 저장된 로그인 정보를 관리합니다.</p>
    <Card.Root>
        <Card.Header>
            <Card.Title>서버 연결</Card.Title>
            <Card.Description>이 앱에 저장된 서버를 선택하거나 추가하고, 저장된 서버 정보를 삭제할 수 있습니다.</Card.Description>
        </Card.Header>
        <Card.Content>
            <dl class="space-y-3 text-sm">
                <div class="flex flex-wrap justify-between gap-2"><dt class="text-muted-foreground">연결 상태</dt><dd>{app.connection?.connected ? '연결됨' : '연결 안 됨'}</dd></div>
                {#if app.connection?.connected}
                    <div class="flex flex-wrap justify-between gap-2"><dt class="text-muted-foreground">서버</dt><dd>{app.connection.system.hostname || '—'}</dd></div>
                    <div class="flex flex-wrap justify-between gap-2"><dt class="text-muted-foreground">주소</dt><dd class="break-all">{app.connection.endpoint}</dd></div>
                {/if}
            </dl>
        </Card.Content>
        <Card.Footer><Button variant="outline" onclick={() => app.openConnectModal()}>저장된 서버 및 연결 관리</Button></Card.Footer>
    </Card.Root>
</section>
