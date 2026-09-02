<script lang="ts">
    import { goto } from '$app/navigation';
    import Overview from '$lib/app/Overview.svelte';
    import { getAppContext } from '$lib/app/context.svelte';
    import { hrefForView } from '$lib/app/types';

    const app = getAppContext();
</script>

<Overview connection={app.connection} onnavigate={(view) => goto(hrefForView(view))} />
