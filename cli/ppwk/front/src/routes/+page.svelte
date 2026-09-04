<script lang="ts">
	import * as Table from '$lib/components/ui/table/index.js';
	import * as Tabs from '$lib/components/ui/tabs/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Input } from '$lib/components/ui/input/index.js';
	import { Badge } from '$lib/components/ui/badge/index.js';
	import { Skeleton } from '$lib/components/ui/skeleton/index.js';
	import * as Empty from '$lib/components/ui/empty/index.js';
	import StatusBadge from '$lib/components/board/StatusBadge.svelte';
	import PriorityDot from '$lib/components/board/PriorityDot.svelte';
	import ActionButtons from '$lib/components/board/ActionButtons.svelte';
	import IssueSheet from '$lib/components/board/IssueSheet.svelte';
	import AddIssueDialog from '$lib/components/board/AddIssueDialog.svelte';
	import { board, isTerminal } from '$lib/api/board.svelte';
	import { api } from '$lib/api/client';
	import { toast } from 'svelte-sonner';
	import type { ListEntry } from '$lib/api/types';
	import SparklesIcon from '@lucide/svelte/icons/sparkles';

	let selected = $state<string | null>(null);
	let search = $state('');
	let tab = $state('active');
	let claiming = $state(false);

	const tabs = [
		{ value: 'active', label: '진행' },
		{ value: 'mine', label: '내 것' },
		{ value: 'done', label: '완료' },
		{ value: 'all', label: '전체' }
	];

	const filtered = $derived.by(() => {
		const agent = board.state?.agent;
		const term = search.trim().toLowerCase();
		return board.issues
			.filter((issue) => {
				if (tab === 'active') return !isTerminal(issue.status);
				if (tab === 'mine') return issue.owner === agent && !isTerminal(issue.status);
				if (tab === 'done') return isTerminal(issue.status);
				return true;
			})
			.filter((issue) => {
				if (!term) return true;
				return (
					issue.title.toLowerCase().includes(term) ||
					issue.id.toLowerCase().includes(term) ||
					(issue.owner ?? '').toLowerCase().includes(term)
				);
			});
	});

	const counts = $derived({
		active: board.issues.filter((i) => !isTerminal(i.status)).length,
		mine: board.mine.length,
		done: board.issues.filter((i) => isTerminal(i.status)).length,
		all: board.issues.length
	});

	async function claimNext() {
		claiming = true;
		const result = await board.run('next --claim', () => api.claimNext());
		claiming = false;
		if (!result) return;
		if (result.claimed) {
			toast.success(`${result.claimed.id} 를 가져왔습니다`, {
				description: result.claimed.title
			});
			selected = result.claimed.id;
		} else {
			// 후보가 없는 것과 경쟁에서 밀린 것은 다른 사건이다.
			toast.info(
				result.candidates?.length ? '후보를 모두 놓쳤습니다' : '지금 할 수 있는 일이 없습니다'
			);
		}
	}

	function rowClass(issue: ListEntry) {
		return issue.owner === board.state?.agent ? 'bg-muted/30' : '';
	}
</script>

<div class="space-y-4">
	<div class="flex flex-wrap items-center gap-3">
		<Input bind:value={search} placeholder="제목, ID, 소유자로 찾기" class="max-w-xs" />
		<div class="ml-auto flex items-center gap-2">
			<Button size="sm" variant="outline" disabled={claiming} onclick={claimNext}>
				<SparklesIcon class="size-4" /> 다음 작업 가져오기
			</Button>
			<AddIssueDialog />
		</div>
	</div>

	<Tabs.Root bind:value={tab}>
		<Tabs.List>
			{#each tabs as item (item.value)}
				<Tabs.Trigger value={item.value}>
					{item.label}
					<Badge variant="secondary" class="ml-1.5">{counts[item.value as keyof typeof counts]}</Badge>
				</Tabs.Trigger>
			{/each}
		</Tabs.List>
	</Tabs.Root>

	{#if board.loading}
		<div class="space-y-2">
			{#each { length: 5 } as _, i (i)}
				<Skeleton class="h-12 w-full" />
			{/each}
		</div>
	{:else if filtered.length === 0}
		<Empty.Root>
			<Empty.Header>
				<Empty.Title>{search ? '찾은 것이 없습니다' : '이슈가 없습니다'}</Empty.Title>
				<Empty.Description>
					{search ? '다른 말로 찾아보세요' : '새 이슈를 만들면 여기 나타납니다'}
				</Empty.Description>
			</Empty.Header>
		</Empty.Root>
	{:else}
		<div class="rounded-md border">
			<Table.Root>
				<Table.Header>
					<Table.Row>
						<Table.Head class="w-20">ID</Table.Head>
						<Table.Head>제목</Table.Head>
						<Table.Head class="w-24">상태</Table.Head>
						<Table.Head class="w-24">우선순위</Table.Head>
						<Table.Head class="w-40">소유자</Table.Head>
						<Table.Head class="w-56 text-right">동작</Table.Head>
					</Table.Row>
				</Table.Header>
				<Table.Body>
					{#each filtered as issue (issue.id)}
						<Table.Row class={rowClass(issue)}>
							<Table.Cell class="font-mono text-xs">
								<button class="hover:underline" onclick={() => (selected = issue.id)}>{issue.id}</button>
							</Table.Cell>
							<Table.Cell>
								<button class="text-left hover:underline" onclick={() => (selected = issue.id)}>
									{issue.title}
								</button>
								{#if issue.plan}
									<span class="ml-2 text-xs text-muted-foreground">{issue.plan}/{issue.phase}</span>
								{/if}
							</Table.Cell>
							<Table.Cell><StatusBadge status={issue.status} /></Table.Cell>
							<Table.Cell><PriorityDot priority={issue.priority} /></Table.Cell>
							<Table.Cell class="truncate font-mono text-xs text-muted-foreground">
								{issue.owner ?? '—'}
							</Table.Cell>
							<Table.Cell>
								<div class="flex justify-end">
									<ActionButtons id={issue.id} status={issue.status} />
								</div>
							</Table.Cell>
						</Table.Row>
					{/each}
				</Table.Body>
			</Table.Root>
		</div>
	{/if}
</div>

<IssueSheet bind:id={selected} />
