<script lang="ts">
    import { onMount } from 'svelte';
    import { Copy, FolderDown, Plus, ShieldCheck, Trash2, Upload } from '@lucide/svelte';
    import * as Alert from '$lib/components/ui/alert';
    import { Badge } from '$lib/components/ui/badge';
    import { Button } from '$lib/components/ui/button';
    import * as Card from '$lib/components/ui/card';
    import * as Dialog from '$lib/components/ui/dialog';
    import * as Empty from '$lib/components/ui/empty';
    import { Input } from '$lib/components/ui/input';
    import { Spinner } from '$lib/components/ui/spinner';
    import * as Table from '$lib/components/ui/table';
    import { Textarea } from '$lib/components/ui/textarea';
    import type { CertificateInfo, GeneratedCertificate, SelfSignedCertificateRequest } from '../../../bindings/github.com/ironpark/toolz/desktop/charmtrue';
    import ConfirmActionDialog from './ConfirmActionDialog.svelte';
    import { getAppContext } from './context.svelte';

    type Editor = 'generate' | 'import' | null;
    type GenerateForm = {
        name: string; commonName: string; country: string; state: string; locality: string; organization: string; organizationalUnit: string;
        ipAddresses: string; dnsNames: string; days: number; keyBits: number; saveFiles: boolean; install: boolean; applyToUi: boolean;
    };
    type ImportForm = { name: string; certificate: string; privateKey: string; applyToUi: boolean };

    const app = getAppContext();
    const selectClass = 'h-9 w-full rounded-md border border-input bg-transparent px-2.5 py-1 text-sm shadow-xs outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30';
    let editor = $state<Editor>(null);
    let busy = $state('');
    let editorError = $state('');
    let notice = $state('');
    let generateForm = $state<GenerateForm>(emptyGenerateForm());
    let importForm = $state<ImportForm>({ name: '', certificate: '', privateKey: '', applyToUi: true });
    let result = $state<GeneratedCertificate | null>(null);
    let resultSummary = $state<string[]>([]);
    let confirmOpen = $state(false);
    let confirmation = $state<{ kind: 'apply' | 'delete'; target: CertificateInfo | null }>({ kind: 'apply', target: null });
    const certificates = $derived(app.certificates?.certificates ?? []);

    onMount(() => { if (!app.certificates) void app.refreshCertificates(); });

    function emptyGenerateForm(): GenerateForm {
        return { name: 'charmtrue-local', commonName: 'truenas.local', country: 'KR', state: 'Seoul', locality: 'Seoul', organization: 'MyHomeNAS', organizationalUnit: 'IT', ipAddresses: '', dnsNames: 'truenas.local\n*.truenas.local', days: 825, keyBits: 2048, saveFiles: true, install: true, applyToUi: true };
    }

    function lines(value: string): string[] {
        return value.split(/\r?\n|,/).map((item) => item.trim()).filter(Boolean);
    }

    async function openGenerate(): Promise<void> {
        editorError = '';
        notice = '';
        result = null;
        generateForm = emptyGenerateForm();
        try {
            const defaults = await app.selfSignedCertificateDefaults();
            generateForm = { ...generateForm, name: defaults.name, commonName: defaults.commonName, country: defaults.country, state: defaults.state, locality: defaults.locality, organization: defaults.organization, organizationalUnit: defaults.organizationalUnit, ipAddresses: (defaults.ipAddresses ?? []).join('\n'), dnsNames: (defaults.dnsNames ?? []).join('\n'), days: defaults.days, keyBits: defaults.keyBits };
        } catch {
            // Defaults are a convenience; the form still works without them.
        }
        editor = 'generate';
    }

    function openImport(): void {
        editorError = '';
        notice = '';
        result = null;
        importForm = { name: '', certificate: '', privateKey: '', applyToUi: true };
        editor = 'import';
    }

    function message(error: unknown): string {
        return error instanceof Error ? error.message : String(error);
    }

    async function runGenerate(event: SubmitEvent): Promise<void> {
        event.preventDefault();
        editorError = '';
        busy = 'generate';
        const summary: string[] = [];
        try {
            const request: SelfSignedCertificateRequest = {
                name: generateForm.name, commonName: generateForm.commonName, country: generateForm.country, state: generateForm.state, locality: generateForm.locality,
                organization: generateForm.organization, organizationalUnit: generateForm.organizationalUnit, ipAddresses: lines(generateForm.ipAddresses), dnsNames: lines(generateForm.dnsNames),
                days: Number(generateForm.days), keyBits: Number(generateForm.keyBits),
            };
            const generated = await app.generateSelfSignedCertificate(request);
            result = generated;
            summary.push(`인증서와 개인키를 생성했습니다 (만료: ${new Date(generated.notAfter).toLocaleDateString()})`);
            if (generateForm.saveFiles) {
                const dir = await app.saveCertificateFiles(generated);
                summary.push(dir ? `${dir} 폴더에 .crt / .key / san.cnf 파일을 저장했습니다` : '파일 저장을 건너뛰었습니다');
            }
            if (generateForm.install && app.connection?.connected) {
                const id = await app.installCertificate({ name: generated.name, certificate: generated.certificate, privateKey: generated.privateKey, applyToUi: generateForm.applyToUi });
                summary.push(`TrueNAS 인증서 목록에 "${generated.name}" (ID ${id})으로 가져왔습니다`);
                if (generateForm.applyToUi) summary.push('웹 GUI 인증서로 적용했습니다. 몇 초 뒤 웹 서비스가 재시작되며 브라우저에서 새 인증서를 신뢰하도록 등록해야 경고가 사라집니다.');
            } else if (generateForm.install) {
                summary.push('TrueNAS에 연결되어 있지 않아 가져오기를 건너뛰었습니다');
            }
            resultSummary = summary;
        } catch (error) {
            resultSummary = summary;
            editorError = message(error);
        } finally {
            busy = '';
        }
    }

    async function runImport(event: SubmitEvent): Promise<void> {
        event.preventDefault();
        editorError = '';
        busy = 'import';
        try {
            const id = await app.installCertificate({ name: importForm.name, certificate: importForm.certificate, privateKey: importForm.privateKey, applyToUi: importForm.applyToUi });
            notice = `"${importForm.name}" (ID ${id}) 인증서를 가져왔습니다.${importForm.applyToUi ? ' 웹 GUI 인증서로 적용되어 웹 서비스가 재시작됩니다.' : ''}`;
            editor = null;
        } catch (error) {
            editorError = message(error);
        } finally {
            busy = '';
        }
    }

    function ask(kind: 'apply' | 'delete', target: CertificateInfo): void {
        confirmation = { kind, target };
        confirmOpen = true;
    }

    async function confirmAction(): Promise<void> {
        const target = confirmation.target;
        if (!target) return;
        busy = `${confirmation.kind}-${target.id}`;
        notice = '';
        try {
            if (confirmation.kind === 'apply') {
                await app.setUICertificate(target.id);
                notice = `"${target.name}" 인증서를 웹 GUI에 적용했습니다. 웹 서비스가 재시작됩니다.`;
            } else {
                await app.deleteCertificate(target.id);
            }
        } catch {
            // The context surfaces the error through certificatesError.
        } finally {
            busy = '';
        }
    }

    function formatDate(value: string): string {
        if (!value) return '—';
        const parsed = new Date(value);
        return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleDateString();
    }

    async function copy(text: string): Promise<void> {
        await navigator.clipboard.writeText(text);
    }
</script>

<Card.Root>
    <Card.Header class="flex flex-row items-start justify-between gap-4">
        <div><Card.Title>SSL 인증서</Card.Title><Card.Description>자가 서명 SAN 인증서를 만들고 TrueNAS 웹 GUI에 바로 적용합니다.</Card.Description></div>
        <div class="flex gap-2"><Button size="sm" variant="outline" onclick={openImport}><Upload />가져오기</Button><Button size="sm" onclick={openGenerate}><Plus />자가 서명 인증서 만들기</Button></div>
    </Card.Header>
    <Card.Content class="space-y-4">
        {#if app.certificatesError}<Alert.Root variant="destructive"><Alert.Title>인증서 작업 실패</Alert.Title><Alert.Description>{app.certificatesError}</Alert.Description></Alert.Root>{/if}
        {#if notice}<Alert.Root><Alert.Title>완료</Alert.Title><Alert.Description>{notice}</Alert.Description></Alert.Root>{/if}
        {#if app.certificatesLoading && !app.certificates}
            <div class="grid min-h-32 place-items-center"><Spinner class="size-6" aria-label="인증서 불러오는 중" /></div>
        {:else if certificates.length}
            <Table.Root>
                <Table.Header><Table.Row><Table.Head>이름</Table.Head><Table.Head>CN / SAN</Table.Head><Table.Head>만료</Table.Head><Table.Head>상태</Table.Head><Table.Head class="text-right">작업</Table.Head></Table.Row></Table.Header>
                <Table.Body>
                    {#each certificates as cert (cert.id)}
                        <Table.Row>
                            <Table.Cell class="font-medium">{cert.name}</Table.Cell>
                            <Table.Cell><div class="font-mono text-xs">{cert.commonName || '—'}</div>{#if cert.subjectAltNames?.length}<div class="text-xs text-muted-foreground">{cert.subjectAltNames.join(', ')}</div>{/if}</Table.Cell>
                            <Table.Cell class="text-xs">{formatDate(cert.until)}</Table.Cell>
                            <Table.Cell class="space-x-1">{#if cert.uiActive}<Badge variant="secondary"><ShieldCheck />GUI 사용 중</Badge>{/if}{#if cert.expired}<Badge variant="destructive">만료됨</Badge>{/if}{#if !cert.hasPrivateKey}<Badge variant="outline">개인키 없음</Badge>{/if}</Table.Cell>
                            <Table.Cell class="space-x-2 text-right">
                                {#if !cert.uiActive}<Button size="sm" variant="outline" disabled={!cert.hasPrivateKey || busy === `apply-${cert.id}`} onclick={() => ask('apply', cert)}>{#if busy === `apply-${cert.id}`}<Spinner />{/if}GUI에 적용</Button><Button size="sm" variant="ghost" disabled={busy === `delete-${cert.id}`} onclick={() => ask('delete', cert)}><Trash2 />삭제</Button>{/if}
                            </Table.Cell>
                        </Table.Row>
                    {/each}
                </Table.Body>
            </Table.Root>
        {:else}
            <Empty.Root class="min-h-32 border-0 p-6"><Empty.Media variant="icon"><ShieldCheck /></Empty.Media><Empty.Header><Empty.Title>인증서가 없습니다</Empty.Title><Empty.Description>자가 서명 인증서를 만들면 TrueNAS에 가져오고 웹 GUI에 적용할 수 있습니다.</Empty.Description></Empty.Header></Empty.Root>
        {/if}
    </Card.Content>
</Card.Root>

<Dialog.Root open={editor !== null} onOpenChange={(open) => { if (!open && busy === '') editor = null; }}>
    <Dialog.Content class="max-h-[88dvh] overflow-y-auto sm:max-w-2xl">
        {#if editor === 'generate'}
            {#if result}
                <div class="space-y-5">
                    <Dialog.Header><Dialog.Title>인증서 생성 완료</Dialog.Title><Dialog.Description>SHA-256 지문: <span class="font-mono text-xs break-all">{result.fingerprint}</span></Dialog.Description></Dialog.Header>
                    <ul class="list-disc space-y-1 pl-5 text-sm">{#each resultSummary as line}<li>{line}</li>{/each}</ul>
                    {#if editorError}<Alert.Root variant="destructive"><Alert.Title>일부 단계가 실패했습니다</Alert.Title><Alert.Description>{editorError} — 아래 PEM 내용을 복사해 TrueNAS의 Credentials › Certificates › Import에 직접 붙여넣을 수 있습니다.</Alert.Description></Alert.Root>{/if}
                    <div class="grid gap-4">
                        <div class="grid gap-2"><div class="flex items-center justify-between text-sm font-medium">인증서 (truenas.crt)<Button size="sm" variant="ghost" onclick={() => copy(result!.certificate)}><Copy />복사</Button></div><Textarea class="min-h-28 font-mono text-xs" readonly value={result.certificate} /></div>
                        <div class="grid gap-2"><div class="flex items-center justify-between text-sm font-medium">개인키 (truenas.key)<Button size="sm" variant="ghost" onclick={() => copy(result!.privateKey)}><Copy />복사</Button></div><Textarea class="min-h-28 font-mono text-xs" readonly value={result.privateKey} /></div>
                        <div class="grid gap-2"><div class="flex items-center justify-between text-sm font-medium">OpenSSL 설정 (san.cnf)<Button size="sm" variant="ghost" onclick={() => copy(result!.opensslConfig)}><Copy />복사</Button></div><Textarea class="min-h-28 font-mono text-xs" readonly value={result.opensslConfig} /></div>
                    </div>
                    <Dialog.Footer><Button variant="outline" onclick={() => app.saveCertificateFiles(result!).then((dir) => { if (dir) notice = `${dir} 폴더에 파일을 저장했습니다.`; }).catch((error) => (editorError = message(error)))}><FolderDown />파일로 저장</Button><Button onclick={() => (editor = null)}>닫기</Button></Dialog.Footer>
                </div>
            {:else}
                <form class="space-y-5" onsubmit={runGenerate}>
                    <Dialog.Header><Dialog.Title>자가 서명 인증서 만들기</Dialog.Title><Dialog.Description>san.cnf 작성 → openssl 실행 → TrueNAS 가져오기 → GUI 적용을 한 번에 처리합니다. IP와 도메인은 TrueNAS에 접속할 때 쓰는 주소로 입력하세요.</Dialog.Description></Dialog.Header>
                    <div class="grid gap-4 sm:grid-cols-2">
                        <label class="grid gap-2 text-sm font-medium">TrueNAS 인증서 이름<Input bind:value={generateForm.name} required placeholder="openssl-local" /></label>
                        <label class="grid gap-2 text-sm font-medium">CN (Common Name)<Input bind:value={generateForm.commonName} required placeholder="truenas.local" /></label>
                        <label class="grid gap-2 text-sm font-medium">IP 주소 (IP.n)<Textarea class="min-h-20 resize-y font-mono text-sm" bind:value={generateForm.ipAddresses} spellcheck={false} autocapitalize="none" placeholder={'192.168.1.100'} /></label>
                        <label class="grid gap-2 text-sm font-medium">도메인 (DNS.n)<Textarea class="min-h-20 resize-y font-mono text-sm" bind:value={generateForm.dnsNames} spellcheck={false} autocapitalize="none" placeholder={'truenas.local\n*.truenas.local'} /></label>
                        <label class="grid gap-2 text-sm font-medium">유효기간 (일, 최대 825)<Input type="number" min="1" max="825" bind:value={generateForm.days} required /></label>
                        <label class="grid gap-2 text-sm font-medium">키 길이<select class={selectClass} bind:value={generateForm.keyBits}><option value={2048}>RSA 2048</option><option value={3072}>RSA 3072</option><option value={4096}>RSA 4096</option></select></label>
                    </div>
                    <details class="rounded-lg border p-4"><summary class="cursor-pointer text-sm font-medium">주체 정보 (C / ST / L / O / OU)</summary>
                        <div class="mt-4 grid gap-4 sm:grid-cols-2">
                            <label class="grid gap-2 text-sm font-medium">국가 (C)<Input bind:value={generateForm.country} maxlength={2} placeholder="KR" /></label>
                            <label class="grid gap-2 text-sm font-medium">시/도 (ST)<Input bind:value={generateForm.state} /></label>
                            <label class="grid gap-2 text-sm font-medium">도시 (L)<Input bind:value={generateForm.locality} /></label>
                            <label class="grid gap-2 text-sm font-medium">조직 (O)<Input bind:value={generateForm.organization} /></label>
                            <label class="grid gap-2 text-sm font-medium">부서 (OU)<Input bind:value={generateForm.organizationalUnit} /></label>
                        </div>
                    </details>
                    <div class="grid gap-3 rounded-lg border p-4">
                        <label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={generateForm.saveFiles} />.crt / .key / san.cnf 파일로 저장 (저장 위치를 물어봅니다)</label>
                        <label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={generateForm.install} disabled={!app.connection?.connected} />TrueNAS 인증서 목록에 가져오기 (Credentials › Certificates › Import)</label>
                        <label class="flex items-center gap-2 pl-6 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={generateForm.applyToUi} disabled={!generateForm.install || !app.connection?.connected} />웹 GUI SSL 인증서로 적용하고 웹 서비스 재시작</label>
                    </div>
                    <p class="text-xs text-muted-foreground">Chrome, Safari 등은 유효기간이 825일을 넘는 자가 서명 인증서를 차단하므로 기본값 825일을 권장합니다. GUI에 적용하면 이 앱의 연결도 새 인증서를 사용하게 되므로 "사설 인증서 허용"으로 다시 연결해야 할 수 있습니다.</p>
                    {#if editorError}<Alert.Root variant="destructive"><Alert.Title>생성하지 못했습니다</Alert.Title><Alert.Description>{editorError}</Alert.Description></Alert.Root>{/if}
                    <Dialog.Footer><Button type="button" variant="outline" disabled={busy === 'generate'} onclick={() => (editor = null)}>취소</Button><Button type="submit" disabled={busy === 'generate'}>{#if busy === 'generate'}<Spinner />{/if}{busy === 'generate' ? '처리하는 중' : '생성 및 적용'}</Button></Dialog.Footer>
                </form>
            {/if}
        {:else if editor === 'import'}
            <form class="space-y-5" onsubmit={runImport}>
                <Dialog.Header><Dialog.Title>기존 인증서 가져오기</Dialog.Title><Dialog.Description>truenas.crt와 truenas.key 파일 내용을 그대로 붙여넣으세요.</Dialog.Description></Dialog.Header>
                <div class="grid gap-4">
                    <label class="grid gap-2 text-sm font-medium">인증서 이름<Input bind:value={importForm.name} required placeholder="openssl-local" /></label>
                    <label class="grid gap-2 text-sm font-medium">인증서 (PEM)<Textarea class="min-h-32 resize-y font-mono text-xs" bind:value={importForm.certificate} spellcheck={false} autocapitalize="none" required placeholder="-----BEGIN CERTIFICATE-----" /></label>
                    <label class="grid gap-2 text-sm font-medium">개인키 (PEM)<Textarea class="min-h-32 resize-y font-mono text-xs" bind:value={importForm.privateKey} spellcheck={false} autocapitalize="none" required placeholder="-----BEGIN PRIVATE KEY-----" /></label>
                    <label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4 accent-primary" bind:checked={importForm.applyToUi} />웹 GUI SSL 인증서로 적용하고 웹 서비스 재시작</label>
                </div>
                {#if editorError}<Alert.Root variant="destructive"><Alert.Title>가져오지 못했습니다</Alert.Title><Alert.Description>{editorError}</Alert.Description></Alert.Root>{/if}
                <Dialog.Footer><Button type="button" variant="outline" disabled={busy === 'import'} onclick={() => (editor = null)}>취소</Button><Button type="submit" disabled={busy === 'import'}>{#if busy === 'import'}<Spinner />{/if}{busy === 'import' ? '가져오는 중' : '가져오기'}</Button></Dialog.Footer>
            </form>
        {/if}
    </Dialog.Content>
</Dialog.Root>

<ConfirmActionDialog
    bind:open={confirmOpen}
    title={confirmation.kind === 'apply' ? `"${confirmation.target?.name}"을(를) 웹 GUI에 적용할까요?` : `"${confirmation.target?.name}" 인증서를 삭제할까요?`}
    description={confirmation.kind === 'apply' ? '웹 서비스가 재시작되며 브라우저와 이 앱의 연결이 잠시 끊길 수 있습니다.' : '삭제한 인증서는 복구할 수 없습니다.'}
    confirmLabel={confirmation.kind === 'apply' ? 'GUI에 적용' : '삭제'}
    busy={busy === `${confirmation.kind}-${confirmation.target?.id}`}
    onconfirm={confirmAction}
/>
