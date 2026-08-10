<script lang="ts">
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { notificationTarget } from '$lib/state/server/notifications.svelte';
  import UnreadDot from '$lib/ui/UnreadDot.svelte';
  import { m } from '$lib/i18n/messages';

  let { active }: { active: boolean } = $props();

  const serverScope = useServerScope();
  const serverId = $derived(serverScope.serverId);
  const notificationStore = $derived(serverScope.store.notifications);

  const hasUnread = $derived(
    notificationStore.notifications.some((n) => notificationTarget(n).threadRootId !== null)
  );
</script>

<a
  href={resolve('/chat/[serverId]/threads', { serverId: serverIdToSegment(serverId) })}
  class={['sidebar-item', active ? 'bg-surface' : '']}
>
  <span class="iconify sidebar-icon icon-[uil--comment-alt-lines]"></span>
  {m('chat.threads.title')}
  {#if hasUnread}
    <UnreadDot class="ms-auto" testid="my-threads-unread-dot" />
  {/if}
</a>
