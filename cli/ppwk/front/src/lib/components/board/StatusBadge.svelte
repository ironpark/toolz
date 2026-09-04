<script lang="ts">
	import { Badge } from '$lib/components/ui/badge/index.js';
	import { statusLabel, statusVariant } from '$lib/api/vocab';
	import type { Status } from '$lib/api/types';

	let { status }: { status: Status } = $props();
</script>

<Badge variant={statusVariant[status]}>{statusLabel[status]}</Badge>
