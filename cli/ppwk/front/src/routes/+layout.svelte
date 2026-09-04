<script lang="ts">
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';
	import { Toaster } from '$lib/components/ui/sonner/index.js';
	import { Badge } from '$lib/components/ui/badge/index.js';
	import { board } from '$lib/api/board.svelte';
	import { page } from '$app/state';
	import LayoutListIcon from '@lucide/svelte/icons/layout-list';
	import MapIcon from '@lucide/svelte/icons/map';
	import GavelIcon from '@lucide/svelte/icons/gavel';
	import ActivityIcon from '@lucide/svelte/icons/activity';

	let { children } = $props();

	const nav = [
		{ href: '/', label: '보드', icon: LayoutListIcon },
		{ href: '/plans', label: 'plan', icon: MapIcon },
		{ href: '/decisions', label: '결정', icon: GavelIcon },
		{ href: '/agents', label: '에이전트', icon: ActivityIcon }
	];

	$effect(() => {
		void board.start();
		return () => board.stop();
	});
</script>

<svelte:head><link rel="icon" href={favicon} /><title>ppwk</title></svelte:head>

<Toaster />

<div class="flex min-h-svh flex-col">
	<header class="sticky top-0 z-10 border-b bg-background/95 backdrop-blur">
		<div class="mx-auto flex h-14 w-full max-w-6xl items-center gap-6 px-4">
			<span class="font-semibold tracking-tight">ppwk</span>

			<nav class="flex items-center gap-1">
				{#each nav as item (item.href)}
					{@const active = page.url.pathname === item.href}
					<a
						href={item.href}
						class="flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm transition-colors
							{active ? 'bg-muted font-medium' : 'text-muted-foreground hover:text-foreground'}"
					>
						<item.icon class="size-4" />
						{item.label}
					</a>
				{/each}
			</nav>

			<div class="ml-auto flex items-center gap-3 text-xs text-muted-foreground">
				{#if board.state}
					<span class="font-mono">{board.state.agent}</span>
					{#if board.state.read_only}
						<Badge variant="destructive">읽기 전용</Badge>
					{/if}
				{/if}
				<!-- 스트림이 끊기면 화면이 옛 상태를 보여주게 된다. 숨기지 않는다. -->
				<span
					class="size-2 rounded-full {board.connected ? 'bg-emerald-500' : 'bg-muted-foreground/40'}"
					title={board.connected ? '변경 감지 중' : '연결 끊김'}
				></span>
			</div>
		</div>
	</header>

	<main class="mx-auto w-full max-w-6xl flex-1 px-4 py-6">
		{#if board.error}
			<div class="rounded-md border border-destructive/50 bg-destructive/5 p-4 text-sm">
				<p class="font-medium">보드를 읽지 못했습니다</p>
				<p class="mt-1 text-muted-foreground">{board.error}</p>
			</div>
		{:else}
			{@render children()}
		{/if}
	</main>
</div>
