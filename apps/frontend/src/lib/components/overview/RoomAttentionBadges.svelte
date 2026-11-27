<!--
@component

The notification badges of one room or direct message, as in the sidebar:
other notifications first, then important ones at the row edge. Each badge is
a button that asks the owner to open the newest notification it counts.
Renders nothing when there are no notifications.
-->
<script lang="ts">
  import { NotificationAttentionLevel } from '@chatto/client/api/notifications';
  import { m } from '$lib/i18n/messages';
  import { NotificationBadge } from '$lib/ui';

  let {
    roomId,
    isDM = false,
    importantCount,
    otherCount,
    onopen
  }: {
    roomId: string;
    isDM?: boolean;
    /** Important notification occurrences that need attention. */
    importantCount: number;
    /** All other notification occurrences that need attention. */
    otherCount: number;
    /** Open the newest notification of the room at this attention level. */
    onopen: (roomId: string, attentionLevel: NotificationAttentionLevel) => void;
  } = $props();
</script>

{#snippet badge(attentionLevel: NotificationAttentionLevel, count: number)}
  {@const important = attentionLevel === NotificationAttentionLevel.IMPORTANT}
  <button
    type="button"
    class="flex h-6 min-w-6 cursor-pointer items-center justify-center notification-dot"
    aria-label={!important
      ? m('room_list.go_to_other_notifications', { count })
      : isDM
        ? m('room_list.go_to_dm_notifications', { count })
        : m('room_list.go_to_notifications', { count })}
    onclick={() => onopen(roomId, attentionLevel)}
  >
    <NotificationBadge {count} color={important ? 'warning' : 'ambient'} />
  </button>
{/snippet}

{#if otherCount > 0}
  {@render badge(NotificationAttentionLevel.AMBIENT, otherCount)}
{/if}
{#if importantCount > 0}
  {@render badge(NotificationAttentionLevel.IMPORTANT, importantCount)}
{/if}
