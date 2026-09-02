<script lang="ts">
    import { AlertTriangle } from '@lucide/svelte';
    import * as AlertDialog from '$lib/components/ui/alert-dialog';
    import { Spinner } from '$lib/components/ui/spinner';

    let {
        open = $bindable(false),
        title,
        description,
        confirmLabel = '계속',
        busy = false,
        destructive = true,
        onconfirm,
    }: {
        open?: boolean;
        title: string;
        description: string;
        confirmLabel?: string;
        busy?: boolean;
        destructive?: boolean;
        onconfirm: () => void | Promise<void>;
    } = $props();
</script>

<AlertDialog.Root bind:open>
    <AlertDialog.Content>
        <AlertDialog.Header>
            <AlertDialog.Media class={destructive ? 'bg-destructive/10 text-destructive' : 'bg-amber-500/10 text-amber-700 dark:text-amber-400'}>
                <AlertTriangle />
            </AlertDialog.Media>
            <AlertDialog.Title>{title}</AlertDialog.Title>
            <AlertDialog.Description>{description}</AlertDialog.Description>
        </AlertDialog.Header>
        <AlertDialog.Footer>
            <AlertDialog.Cancel disabled={busy}>취소</AlertDialog.Cancel>
            <AlertDialog.Action variant={destructive ? 'destructive' : 'default'} disabled={busy} onclick={onconfirm}>
                {#if busy}<Spinner aria-label="처리 중" />{/if}
                {busy ? '처리 중…' : confirmLabel}
            </AlertDialog.Action>
        </AlertDialog.Footer>
    </AlertDialog.Content>
</AlertDialog.Root>
