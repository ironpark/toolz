<script lang="ts">
	import * as Card from '$lib/components/ui/card/index.js';
	import { Badge } from '$lib/components/ui/badge/index.js';
	import { Switch } from '$lib/components/ui/switch/index.js';
	import { Label } from '$lib/components/ui/label/index.js';
	import * as Empty from '$lib/components/ui/empty/index.js';
	import { api } from '$lib/api/client';
	import type { DecisionEntry } from '$lib/api/types';

	let showAll = $state(false);
	let decisions = $state<DecisionEntry[]>([]);
	let loading = $state(true);

	$effect(() => {
		const all = showAll;
		let stale = false;
		loading = true;
		void api
			.decisions(all)
			.then((result) => {
				if (!stale) decisions = result;
			})
			.finally(() => {
				if (!stale) loading = false;
			});
		return () => {
			stale = true;
		};
	});
</script>

<div class="space-y-4">
	<div class="flex items-center gap-2">
		<Switch id="all" bind:checked={showAll} />
		<Label for="all">대체된 결정도 보기</Label>
	</div>

	{#if !loading && decisions.length === 0}
		<Empty.Root>
			<Empty.Header>
				<Empty.Title>기록된 결정이 없습니다</Empty.Title>
				<Empty.Description>
					<code>ppwk decide</code> 로 남깁니다. 결정은 불변이고 브랜치와 무관하게 공유됩니다.
				</Empty.Description>
			</Empty.Header>
		</Empty.Root>
	{:else}
		<div class="space-y-3">
			{#each decisions as decision (decision.id)}
				<Card.Root>
					<Card.Header>
						<Card.Title class="flex items-center gap-2 text-base">
							<span class="font-mono text-sm text-muted-foreground">{decision.id}</span>
							<span class:line-through={!!decision.superseded_by}>{decision.title}</span>
							{#if decision.superseded_by}
								<Badge variant="outline">{decision.superseded_by} 로 대체됨</Badge>
							{/if}
						</Card.Title>
					</Card.Header>
				</Card.Root>
			{/each}
		</div>
	{/if}
</div>
