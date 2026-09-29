<script lang="ts">
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { notificationTarget } from '$lib/state/server/notifications.svelte';
  import { NotificationAttentionLevel } from '$lib/api-client/notifications';
  import { NotificationBadge, UnreadDot } from '$lib/ui';
  import { m } from '$lib/i18n/messages';

  let { active }: { active: boolean } = $props();

  const serverScope = useServerScope();
  const serverId = serverScope.serverId;
  const notificationStore = $derived(serverScope.store.notifications);
  const threadNotifications = $derived(
    notificationStore.attentionOccurrences.filter((notification) => {
      const target = notificationTarget(notification);
      if (!target.roomId || !target.threadRootId) return false;
      // A loaded room timeline can prove that a thread is not followed. When
      // the timeline is not loaded, trust the authoritative notification
      // occurrence until the explicit thread read supplies that state.
      return (
        serverScope.store.loadedThreadFollowState(target.roomId, target.threadRootId) !== false
      );
    })
  );
  const notificationCount = $derived(threadNotifications.length);

  const hasImportantAttention = $derived(
    threadNotifications.some(
      (notification) => notification.attentionLevel === NotificationAttentionLevel.IMPORTANT
    )
  );
</script>

<a
  href={resolve('/chat/[serverId]/threads', { serverId: serverIdToSegment(serverId) })}
  aria-current={active ? 'page' : undefined}
  class="sidebar-item"
>
  <span aria-hidden="true" class="iconify sidebar-icon icon-[uil--comment-alt-lines]"></span>
  {m('chat.threads.title')}
  {#if notificationCount > 0}
    <NotificationBadge
      class="ms-auto"
      count={notificationCount}
      color={hasImportantAttention ? 'warning' : 'ambient'}
      testid="my-threads-notification-badge"
    />
    <span class="sr-only"
      >{m('chat.threads.notifications_count', { count: notificationCount })}</span
    >
  {:else if serverScope.store.hasUnreadFollowedThreadInLoadedRooms()}
    <UnreadDot class="ms-auto" color="neutral" testid="my-threads-unread-dot" />
  {/if}
</a>
