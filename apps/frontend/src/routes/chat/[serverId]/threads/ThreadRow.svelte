<!-- @component One followed thread in My Threads. Presentational only: the page resolves names, excerpts, attention, and actions. -->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { UserAvatarUserView } from '@chatto/client/timeline/users';
  import { NotificationAttentionLevel } from '@chatto/client/api/notifications';
  import { ActivityListRow, UnreadDot } from '$lib/ui';
  import ActivityRowText from '$lib/components/ActivityRowText.svelte';
  import ActivityAvatar from '$lib/components/ActivityAvatar.svelte';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import { m } from '$lib/i18n/messages';

  let {
    participants,
    message,
    room,
    time,
    unread,
    attention,
    pending = false,
    canMarkRead,
    onopen,
    onmarkread,
    onunfollow
  }: {
    /**
     * Participant preview: the root author first, shown as the avatar,
     * then optionally one other participant, shown in its badge.
     */
    participants: readonly UserAvatarUserView[];
    /** Original thread message, including a fallback for unavailable content. */
    message: string;
    /** Room or direct-message label. */
    room: Snippet;
    /** Formatted activity or creation time, matching the selected sort order and day section. */
    time: string;
    /** The thread has unread replies. */
    unread: boolean;
    /** Unread notification attention for this thread. */
    attention: NotificationAttentionLevel;
    pending?: boolean;
    canMarkRead: boolean;
    onopen: () => void;
    onmarkread: () => void;
    onunfollow: () => void;
  } = $props();

  const hasAttention = $derived(attention !== NotificationAttentionLevel.UNSPECIFIED);
  const second = $derived(participants[1]);
</script>

{#snippet secondAvatar()}
  {#if second}<UserAvatar user={second} size="xs" />{/if}
{/snippet}

<ActivityListRow
  compact
  overlayActions
  {pending}
  disabled={pending}
  onclick={onopen}
  rowAttributes={{
    class: '@container/activity',
    'data-testid': 'my-thread-item',
    'data-thread-state': unread ? 'unread' : 'read',
    'data-thread-attention':
      attention === NotificationAttentionLevel.IMPORTANT
        ? 'important'
        : attention === NotificationAttentionLevel.AMBIENT
          ? 'ambient'
          : 'none'
  }}
>
  {#snippet leading()}
    <span class="self-start">
      <ActivityAvatar
        size="sm"
        actor={participants[0] ?? null}
        badge={second ? secondAvatar : undefined}
      >
        {#snippet indicator()}
          {#if hasAttention || unread}
            <UnreadDot
              color="ambient"
              overlay
              testid={hasAttention ? 'thread-attention-dot' : 'thread-unread-dot'}
            />
          {/if}
        {/snippet}
      </ActivityAvatar>
    </span>
  {/snippet}

  {#if unread}<span class="sr-only">{m('chat.threads.filter_unread')}</span>{/if}
  {#if hasAttention}<span class="sr-only">{m('room_list.notifications', { count: 1 })}</span>{/if}
  <ActivityRowText overlayActions={2} {time} emphasized={unread} testId="thread-content">
    {#snippet title()}
      {message}
    {/snippet}
    {#snippet location()}<bdi dir="auto">{@render room()}</bdi>{/snippet}
  </ActivityRowText>

  {#snippet actions()}
    <div class="flex justify-end">
      {#if canMarkRead}
        <button
          type="button"
          class="icon-action"
          disabled={pending}
          onclick={onmarkread}
          title={m('room_list.mark_as_read')}
          aria-label={m('room_list.mark_as_read')}
        >
          <span class="iconify icon-[uil--check] text-base" aria-hidden="true"></span>
        </button>
      {/if}
      <button
        type="button"
        class="icon-action"
        disabled={pending}
        onclick={onunfollow}
        title={m('room.message.meta.unfollow_thread')}
        aria-label={m('room.message.meta.unfollow_thread')}
      >
        <span class="iconify icon-[uil--bell-slash] text-base" aria-hidden="true"></span>
      </button>
    </div>
  {/snippet}
</ActivityListRow>
