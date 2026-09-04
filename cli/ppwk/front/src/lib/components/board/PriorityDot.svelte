<script lang="ts">
	import { priorityLabel } from '$lib/api/vocab';
	import type { Priority } from '$lib/api/types';

	let { priority }: { priority: Priority } = $props();

	// 색만으로 구분하지 않는다. title 로 이름을 함께 준다.
	const tone: Record<Priority, string> = {
		high: 'bg-red-500',
		med: 'bg-amber-500',
		low: 'bg-sky-500',
		none: 'bg-muted-foreground/40'
	};
</script>

<span class="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
	<span class="size-2 rounded-full {tone[priority] ?? tone.none}"></span>
	{priorityLabel[priority] ?? priority}
</span>
