<script lang="ts">
	import { Button } from '$lib/components/ui/button/index.js';
	import * as AlertDialog from '$lib/components/ui/alert-dialog/index.js';
	import { actionLabel, allowed, destructive } from '$lib/api/vocab';
	import { board } from '$lib/api/board.svelte';
	import { api } from '$lib/api/client';
	import type { Action, Status } from '$lib/api/types';

	let {
		id,
		status,
		size = 'sm'
	}: { id: string; status: Status; size?: 'sm' | 'default' } = $props();

	let confirming = $state<Action | null>(null);
	let running = $state(false);

	async function run(action: Action) {
		running = true;
		await board.run(`${id} ${actionLabel[action]}`, () => api.transition(id, action));
		running = false;
		confirming = null;
	}

	function click(action: Action) {
		if (destructive.includes(action)) confirming = action;
		else void run(action);
	}
</script>

<div class="flex flex-wrap gap-1.5">
	{#each allowed[status] as action (action)}
		<Button
			{size}
			variant={destructive.includes(action) ? 'ghost' : 'outline'}
			disabled={running}
			onclick={() => click(action)}
		>
			{actionLabel[action]}
		</Button>
	{/each}
</div>

<AlertDialog.Root open={confirming !== null} onOpenChange={(o) => !o && (confirming = null)}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{id} 를 취소할까요?</AlertDialog.Title>
			<AlertDialog.Description>
				취소는 되돌릴 수 없습니다. 이슈는 남고 상태만 cancelled 가 되며, 이 이슈에
				의존하는 작업은 영원히 후보에 오르지 않습니다.
			</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>그만두기</AlertDialog.Cancel>
			<AlertDialog.Action onclick={() => confirming && run(confirming)}>취소하기</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
