<script lang="ts">
	import * as Table from '$lib/components/ui/table/index.js';
	import * as Card from '$lib/components/ui/card/index.js';
	import { Badge } from '$lib/components/ui/badge/index.js';
	import * as Empty from '$lib/components/ui/empty/index.js';
	import { board } from '$lib/api/board.svelte';
	import { api } from '$lib/api/client';
	import { ago } from '$lib/api/vocab';
	import type { Finding } from '$lib/api/types';

	let findings = $state<Finding[]>([]);

	// 보드가 바뀌면 무결성 검사도 다시 한다. 방금 한 일이 무언가를 깨뜨렸다면
	// 그 자리에서 보이는 편이 낫다.
	$effect(() => {
		const _ = board.issues;
		let stale = false;
		void api.fsck().then((result) => {
			if (!stale) findings = result;
		});
		return () => {
			stale = true;
		};
	});

	const errors = $derived(findings.filter((f) => f.level === 'error'));
	const warnings = $derived(findings.filter((f) => f.level !== 'error'));
</script>

<div class="space-y-6">
	<Card.Root>
		<Card.Header>
			<Card.Title>에이전트</Card.Title>
			<Card.Description>
				잠금 파일에 남은 기록입니다. 이 파일들은 machine-local 이라 저장소 이력에
				들어가지 않습니다.
			</Card.Description>
		</Card.Header>
		<Card.Content>
			{#if board.agents.length === 0}
				<p class="text-sm text-muted-foreground">기록이 없습니다</p>
			{:else}
				<Table.Root>
					<Table.Header>
						<Table.Row>
							<Table.Head>에이전트</Table.Head>
							<Table.Head class="w-24">생존</Table.Head>
							<Table.Head class="w-32">마지막 활동</Table.Head>
							<Table.Head class="w-28">감지 근거</Table.Head>
							<Table.Head>worktree</Table.Head>
						</Table.Row>
					</Table.Header>
					<Table.Body>
						{#each board.agents as agent (agent.agent + agent.session)}
							<Table.Row>
								<Table.Cell class="font-mono text-xs">
									{agent.agent}
									{#if agent.agent === board.state?.agent}
										<Badge variant="secondary" class="ml-1.5">나</Badge>
									{/if}
								</Table.Cell>
								<Table.Cell>
									<Badge variant={agent.alive ? 'default' : 'outline'}>
										{agent.alive ? '살아 있음' : '죽음'}
									</Badge>
								</Table.Cell>
								<Table.Cell class="text-sm text-muted-foreground">
									{ago(agent.last_activity)}
								</Table.Cell>
								<Table.Cell class="text-xs text-muted-foreground">
									{agent.hook_pid ? `hook_pid ${agent.hook_pid}` : 'last_activity'}
								</Table.Cell>
								<Table.Cell class="truncate font-mono text-xs text-muted-foreground">
									{agent.worktree}
								</Table.Cell>
							</Table.Row>
						{/each}
					</Table.Body>
				</Table.Root>
			{/if}
		</Card.Content>
	</Card.Root>

	<Card.Root>
		<Card.Header>
			<Card.Title class="flex items-center gap-2">
				무결성
				{#if errors.length}
					<Badge variant="destructive">{errors.length}</Badge>
				{/if}
				{#if warnings.length}
					<Badge variant="secondary">{warnings.length}</Badge>
				{/if}
			</Card.Title>
		</Card.Header>
		<Card.Content>
			{#if findings.length === 0}
				<Empty.Root>
					<Empty.Header>
						<Empty.Title>깨끗합니다</Empty.Title>
					</Empty.Header>
				</Empty.Root>
			{:else}
				<ul class="space-y-2 text-sm">
					{#each [...errors, ...warnings] as finding, i (finding.check + finding.id + i)}
						<li class="flex gap-3">
							<Badge variant={finding.level === 'error' ? 'destructive' : 'secondary'}>
								{finding.level}
							</Badge>
							<span class="font-mono text-xs text-muted-foreground">
								{finding.id || finding.check}
							</span>
							<span class="flex-1">{finding.message}</span>
						</li>
					{/each}
				</ul>
			{/if}
		</Card.Content>
	</Card.Root>
</div>
