<script lang="ts">
	import { resolve } from '$app/paths';
	import { instanceIdToSegment } from '$lib/navigation';
	import { getActiveInstance } from '$lib/state/activeInstance.svelte';
	import { instanceRegistry } from '$lib/state/instance/registry.svelte';
	import { notificationTarget } from '$lib/state/instance/notifications.svelte';
	import UnreadDot from '$lib/ui/UnreadDot.svelte';

	const getInstanceId = getActiveInstance();

	let { spaceId, active }: { spaceId: string; active: boolean } = $props();

	const notificationStore = instanceRegistry.getStore(getInstanceId()).notifications;

	const hasUnread = $derived(
		notificationStore.notifications.some((n) => {
			const t = notificationTarget(n);
			return t.spaceId === spaceId && t.threadRootId !== null;
		})
	);
</script>

<a
	href={resolve('/chat/[instanceId]/[spaceId]/threads', { instanceId: instanceIdToSegment(getInstanceId()), spaceId })}
	class={['sidebar-item', active ? 'bg-surface-100' : 'text-muted']}
>
	<span class="sidebar-icon iconify uil--comment-alt-lines"></span>
	My Threads
	{#if hasUnread}
		<UnreadDot class="ml-auto" testid="my-threads-unread-dot" />
	{/if}
</a>
