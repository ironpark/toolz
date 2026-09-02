<script lang="ts">
    import { Cable } from '@lucide/svelte'; import { Button } from '$lib/components/ui/button'; import * as Card from '$lib/components/ui/card';
    let { title, description, onconnect }: { title: string; description: string; onconnect: () => void } = $props();
</script>
<Card.Root class="mx-auto mt-24 max-w-xl border-dashed"><Card.Content class="grid justify-items-center gap-4 py-14 text-center"><span class="grid size-12 place-items-center rounded-full bg-muted"><Cable/></span><div><Card.Title>{title}</Card.Title><Card.Description class="mt-2">{description}</Card.Description></div><Button onclick={onconnect}>시스템 연결</Button></Card.Content></Card.Root>
