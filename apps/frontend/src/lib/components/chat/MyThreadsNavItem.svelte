<script lang="ts">
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { notificationTarget } from '$lib/state/server/notifications.svelte';
  import { NotificationAttentionLevel } from '$lib/api-client/notifications';
  import UnreadDot from '$lib/ui/UnreadDot.svelte';
  import { m } from '$lib/i18n/messages';

  let { active }: { active: boolean } = $props();

  const serverScope = useServerScope();
  const serverId = $derived(serverScope.serverId);
  const notificationStore = $derived(serverScope.store.notifications);
  const threadNotifications = $derived(
    notificationStore.unreadOccurrences.filter((notification) => {
      const target = notificationTarget(notification);
      if (!target.roomId || !target.threadRootId) return false;
      return (
        serverScope.store.projection.threadViewerStates.get(
          `${target.roomId}\u0000${target.threadRootId}`
        )?.isFollowing === true
      );
    })
  );
  const hasNotification = $derived(threadNotifications.length > 0);

  const hasUnread = $derived(
    hasNotification ||
      [...serverScope.store.projection.threadViewerStates.values()].some(
        (state) => state.isFollowing && state.hasUnreadReplies
      )
  );

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
  <span class="iconify sidebar-icon icon-[uil--comment-alt-lines]"></span>
  {m('chat.threads.title')}
  {#if hasUnread}
    <UnreadDot
      class="ms-auto"
      color={hasImportantAttention ? 'warning' : 'neutral'}
      testid="my-threads-unread-dot"
    />
  {/if}
</a>
