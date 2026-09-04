<script lang="ts">
	import * as Card from '$lib/components/ui/card/index.js';
	import { Progress } from '$lib/components/ui/progress/index.js';
	import { Badge } from '$lib/components/ui/badge/index.js';
	import { Separator } from '$lib/components/ui/separator/index.js';
	import * as Empty from '$lib/components/ui/empty/index.js';
	import StatusBadge from '$lib/components/board/StatusBadge.svelte';
	import IssueSheet from '$lib/components/board/IssueSheet.svelte';
	import { board } from '$lib/api/board.svelte';
	import LockIcon from '@lucide/svelte/icons/lock';
	import UnlockIcon from '@lucide/svelte/icons/unlock';

	let selected = $state<string | null>(null);

	const gateLabel: Record<string, string> = {
		all_done: '앞 단계 전부 완료',
		any_done: '앞 단계 하나 완료',
		manual: '수동 개방'
	};

	function percent(done: number, total: number) {
		return total === 0 ? 0 : Math.round((done / total) * 100);
	}
</script>

{#if board.plans.length === 0}
	<Empty.Root>
		<Empty.Header>
			<Empty.Title>plan 이 없습니다</Empty.Title>
			<Empty.Description>
				<code>ppwk plan new</code> 로 만들 수 있습니다. plan 은 구조만 담고, 진행률과
				열림 여부는 이슈 상태에서 매번 파생됩니다.
			</Empty.Description>
		</Empty.Header>
	</Empty.Root>
{:else}
	<div class="space-y-6">
		{#each board.plans as view (view.plan.id)}
			<Card.Root>
				<Card.Header>
					<Card.Title class="flex items-center gap-2">
						<span class="font-mono text-muted-foreground">{view.plan.id}</span>
						{view.plan.title}
						<Badge variant={view.plan.status === 'active' ? 'default' : 'secondary'}>
							{view.plan.status}
						</Badge>
					</Card.Title>
					<Card.Description>
						{view.done} / {view.total} 완료
					</Card.Description>
					<Progress value={percent(view.done, view.total)} class="mt-2" />
				</Card.Header>

				<Card.Content class="space-y-4">
					{#each view.phases as phase, index (phase.id)}
						{#if index > 0}<Separator />{/if}
						<section>
							<div class="mb-2 flex items-center gap-2">
								{#if phase.open}
									<UnlockIcon class="size-4 text-emerald-600" />
								{:else}
									<LockIcon class="size-4 text-muted-foreground" />
								{/if}
								<span class="font-medium">{phase.title}</span>
								<span class="font-mono text-xs text-muted-foreground">{phase.id}</span>
								{#if phase.current}<Badge>현재</Badge>{/if}
								<span class="ml-auto text-xs text-muted-foreground">
									{phase.done}/{phase.total}
									{#if !phase.open}· {gateLabel[phase.gate] ?? phase.gate} 필요{/if}
								</span>
							</div>

							{#if phase.tasks?.length}
								<ul class="space-y-1">
									{#each phase.tasks as task (task.id)}
										<li class="flex items-center gap-2 text-sm">
											<span class="w-14 font-mono text-xs text-muted-foreground">{task.id}</span>
											<button class="flex-1 text-left hover:underline" onclick={() => (selected = task.id)}>
												{task.title}
											</button>
											<StatusBadge status={task.status} />
										</li>
									{/each}
								</ul>
							{:else}
								<p class="text-sm text-muted-foreground">이 단계에 작업이 없습니다</p>
							{/if}
						</section>
					{/each}
				</Card.Content>
			</Card.Root>
		{/each}
	</div>
{/if}

<IssueSheet bind:id={selected} />
