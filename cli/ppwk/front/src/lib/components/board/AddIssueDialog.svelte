<script lang="ts">
	import * as Dialog from '$lib/components/ui/dialog/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Input } from '$lib/components/ui/input/index.js';
	import { Label } from '$lib/components/ui/label/index.js';
	import { Textarea } from '$lib/components/ui/textarea/index.js';
	import * as Select from '$lib/components/ui/select/index.js';
	import { api } from '$lib/api/client';
	import { board } from '$lib/api/board.svelte';
	import { priorityLabel } from '$lib/api/vocab';
	import { toast } from 'svelte-sonner';
	import PlusIcon from '@lucide/svelte/icons/plus';

	let open = $state(false);
	let title = $state('');
	let body = $state('');
	let priority = $state('med');
	let plan = $state('');
	let phase = $state('');
	let saving = $state(false);

	const priorities = ['high', 'med', 'low', 'none'];

	// 선택한 plan 의 phase 만 고르게 한다. 없는 phase 를 넣으면 그 이슈는
	// gate 판정에서 걸려 후보에 오르지 못한다.
	const phases = $derived(board.plans.find((p) => p.plan.id === plan)?.plan.phases ?? []);

	function reset() {
		title = '';
		body = '';
		priority = 'med';
		plan = '';
		phase = '';
	}

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		if (!title.trim()) return;
		saving = true;
		const created = await board.run('이슈 생성', () =>
			api.add({
				title,
				body,
				priority,
				plan: plan || undefined,
				phase: plan ? phase || undefined : undefined
			})
		);
		saving = false;
		if (created) {
			toast.success(`${created.id} 를 만들었습니다`);
			reset();
			open = false;
		}
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Trigger>
		{#snippet child({ props })}
			<Button {...props} size="sm"><PlusIcon class="size-4" /> 새 이슈</Button>
		{/snippet}
	</Dialog.Trigger>
	<Dialog.Content class="sm:max-w-md">
		<form onsubmit={submit}>
			<Dialog.Header>
				<Dialog.Title>새 이슈</Dialog.Title>
				<Dialog.Description>제목만 있으면 됩니다. 나머지는 나중에 채울 수 있습니다.</Dialog.Description>
			</Dialog.Header>

			<div class="space-y-4 py-4">
				<div class="space-y-2">
					<Label for="title">제목</Label>
					<!-- svelte-ignore a11y_autofocus -->
					<Input id="title" bind:value={title} autofocus placeholder="무엇을 할까요" />
				</div>

				<div class="space-y-2">
					<Label for="body">본문</Label>
					<Textarea id="body" bind:value={body} rows={4} placeholder="배경, 조건, 참고" />
				</div>

				<div class="grid grid-cols-2 gap-3">
					<div class="space-y-2">
						<Label>우선순위</Label>
						<Select.Root type="single" bind:value={priority}>
							<Select.Trigger class="w-full">
								{priorityLabel[priority as keyof typeof priorityLabel]}
							</Select.Trigger>
							<Select.Content>
								{#each priorities as value (value)}
									<Select.Item {value}>{priorityLabel[value as keyof typeof priorityLabel]}</Select.Item>
								{/each}
							</Select.Content>
						</Select.Root>
					</div>

					{#if board.plans.length}
						<div class="space-y-2">
							<Label>plan</Label>
							<Select.Root type="single" bind:value={plan} onValueChange={() => (phase = '')}>
								<Select.Trigger class="w-full">{plan || '없음'}</Select.Trigger>
								<Select.Content>
									<Select.Item value="">없음</Select.Item>
									{#each board.plans as view (view.plan.id)}
										<Select.Item value={view.plan.id}>{view.plan.id} {view.plan.title}</Select.Item>
									{/each}
								</Select.Content>
							</Select.Root>
						</div>
					{/if}
				</div>

				{#if plan && phases.length}
					<div class="space-y-2">
						<Label>phase</Label>
						<Select.Root type="single" bind:value={phase}>
							<Select.Trigger class="w-full">{phase || '고르세요'}</Select.Trigger>
							<Select.Content>
								{#each phases as p (p.id)}
									<Select.Item value={p.id}>{p.id} {p.title}</Select.Item>
								{/each}
							</Select.Content>
						</Select.Root>
					</div>
				{/if}
			</div>

			<Dialog.Footer>
				<Button type="submit" disabled={saving || !title.trim()}>만들기</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
