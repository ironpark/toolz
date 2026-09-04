<script lang="ts">
	import * as Sheet from '$lib/components/ui/sheet/index.js';
	import { Badge } from '$lib/components/ui/badge/index.js';
	import { Separator } from '$lib/components/ui/separator/index.js';
	import { Skeleton } from '$lib/components/ui/skeleton/index.js';
	import StatusBadge from './StatusBadge.svelte';
	import PriorityDot from './PriorityDot.svelte';
	import ActionButtons from './ActionButtons.svelte';
	import { api } from '$lib/api/client';
	import { board } from '$lib/api/board.svelte';
	import { ago } from '$lib/api/vocab';
	import type { HistoryEvent, IssueDetail } from '$lib/api/types';

	let { id = $bindable() }: { id: string | null } = $props();

	let detail = $state<IssueDetail | null>(null);
	let history = $state<HistoryEvent[]>([]);
	let failed = $state<string | null>(null);

	// 이슈가 바뀌거나 보드가 갱신되면 상세도 다시 읽는다. 목록만 갱신되고
	// 열려 있는 상세가 옛 상태를 보여주면 사용자는 무엇을 믿어야 할지 모른다.
	$effect(() => {
		const current = id;
		const _ = board.issues;
		if (!current) {
			detail = null;
			history = [];
			return;
		}
		let stale = false;
		void (async () => {
			try {
				const [d, h] = await Promise.all([api.issue(current), api.history(current)]);
				if (!stale) {
					detail = d;
					history = h;
					failed = null;
				}
			} catch (err) {
				if (!stale) failed = err instanceof Error ? err.message : String(err);
			}
		})();
		return () => {
			stale = true;
		};
	});
</script>

<Sheet.Root open={id !== null} onOpenChange={(open) => !open && (id = null)}>
	<Sheet.Content class="w-full overflow-y-auto sm:max-w-lg">
		{#if failed}
			<Sheet.Header>
				<Sheet.Title>{id}</Sheet.Title>
				<Sheet.Description>{failed}</Sheet.Description>
			</Sheet.Header>
		{:else if !detail}
			<div class="space-y-3 p-4">
				<Skeleton class="h-6 w-2/3" />
				<Skeleton class="h-4 w-1/3" />
				<Skeleton class="h-24 w-full" />
			</div>
		{:else}
			{@const issue = detail.issue}
			<Sheet.Header>
				<Sheet.Title class="flex items-center gap-2">
					<span class="font-mono text-muted-foreground">{issue.id}</span>
					{issue.title}
				</Sheet.Title>
				<Sheet.Description class="flex flex-wrap items-center gap-2">
					<StatusBadge status={issue.status} />
					<PriorityDot priority={issue.priority} />
					{#if detail.archived}
						<Badge variant="outline">archive</Badge>
					{/if}
				</Sheet.Description>
			</Sheet.Header>

			<div class="space-y-5 px-4 pb-6">
				<ActionButtons id={issue.id} status={issue.status} />

				<dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-sm">
					{#if issue.owner}
						<dt class="text-muted-foreground">소유자</dt>
						<dd class="font-mono text-xs">{issue.owner}</dd>
					{/if}
					{#if issue.plan}
						<dt class="text-muted-foreground">plan</dt>
						<dd>{issue.plan} / {issue.phase} <span class="text-muted-foreground">seq {issue.seq ?? 0}</span></dd>
					{/if}
					{#if issue.depends_on?.length}
						<dt class="text-muted-foreground">의존</dt>
						<dd class="flex flex-wrap gap-1">
							{#each issue.depends_on as dep (dep)}
								<button class="font-mono text-xs underline-offset-2 hover:underline" onclick={() => (id = dep)}>
									{dep}
								</button>
							{/each}
						</dd>
					{/if}
					{#if issue.labels?.length}
						<dt class="text-muted-foreground">라벨</dt>
						<dd class="flex flex-wrap gap-1">
							{#each issue.labels as label (label)}
								<Badge variant="secondary">{label}</Badge>
							{/each}
						</dd>
					{/if}
					<dt class="text-muted-foreground">수정</dt>
					<dd>{ago(issue.updated_at)} <span class="text-muted-foreground">· {issue.updated_by}</span></dd>
				</dl>

				{#if detail.body.trim()}
					<Separator />
					<pre class="whitespace-pre-wrap text-sm leading-relaxed">{detail.body}</pre>
				{/if}

				{#if detail.decisions.length}
					<Separator />
					<section>
						<h3 class="mb-2 text-sm font-medium">결정</h3>
						<ul class="space-y-1 text-sm">
							{#each detail.decisions as decision (decision.id)}
								<li class="flex gap-2">
									<span class="font-mono text-xs text-muted-foreground">{decision.id}</span>
									<span class:line-through={!!decision.superseded_by}>{decision.title}</span>
								</li>
							{/each}
						</ul>
					</section>
				{/if}

				<Separator />
				<section>
					<h3 class="mb-2 text-sm font-medium">이력</h3>
					<ol class="space-y-2 text-sm">
						{#each history as event (event.commit)}
							<li class="flex gap-3">
								<span class="font-mono text-xs text-muted-foreground">{event.short}</span>
								<span class="flex-1">{event.subject}</span>
								<span class="text-xs text-muted-foreground">{event.who}</span>
							</li>
						{/each}
					</ol>
				</section>
			</div>
		{/if}
	</Sheet.Content>
</Sheet.Root>
