<script lang="ts">
	import { resolve } from '$app/paths';
	import { serverIdToSegment } from '$lib/navigation';
	import { getActiveServer } from '$lib/state/activeServer.svelte';
	import { serverRegistry } from '$lib/state/server/registry.svelte';
	import { notificationTarget } from '$lib/state/server/notifications.svelte';
	import UnreadDot from '$lib/ui/UnreadDot.svelte';

	const getInstanceId = getActiveServer();

	let { active }: { active: boolean } = $props();

	const notificationStore = serverRegistry.getStore(getInstanceId()).notifications;

	const hasUnread = $derived(
		notificationStore.notifications.some((n) => notificationTarget(n).threadRootId !== null)
	);
</script>

<a
	href={resolve('/chat/[serverId]/(chrome)/threads', { serverId: serverIdToSegment(getInstanceId()) })}
	class={['sidebar-item', active ? 'bg-surface-100' : 'text-muted']}
>
	<span class="sidebar-icon iconify uil--comment-alt-lines"></span>
	My Threads
	{#if hasUnread}
		<UnreadDot class="ml-auto" testid="my-threads-unread-dot" />
	{/if}
</a>
