<!-- @component One followed thread in My Threads. Presentational only: the page resolves names, excerpts, attention, and actions. -->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { AccountNameIdentity } from '@chatto/client/timeline/accountName';
  import type { UserAvatarUserView } from '@chatto/client/timeline/users';
  import { NotificationAttentionLevel } from '@chatto/client/api/notifications';
  import { ActivityListRow, UnreadDot } from '$lib/ui';
  import ActivityRowText from '$lib/components/ActivityRowText.svelte';
  import ActivityAvatar from '$lib/components/ActivityAvatar.svelte';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import AccountName from '$lib/components/users/AccountName.svelte';
  import { m } from '$lib/i18n/messages';

  /** A message line: author name, when known, and a one-line excerpt. */
  type MessageLine = { name: string; identity?: AccountNameIdentity | null; excerpt: string };

  let {
    participants,
    latest,
    root = null,
    room,
    time,
    replyCount,
    unread,
    attention,
    pending = false,
    canMarkRead,
    onopen,
    onmarkread,
    onunfollow
  }: {
    /** Participant preview. The first is the avatar; the second is its badge. */
    participants: readonly UserAvatarUserView[];
    /** Latest reply, or the root message when the thread has no reply. */
    latest: MessageLine;
    /** Root message context; set only when `latest` is a reply. */
    root?: MessageLine | null;
    /** Room or direct-message label. */
    room: Snippet;
    /** Formatted time of the latest activity. */
    time: string;
    /** Localized reply count label. */
    replyCount: string;
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

{#snippet messageLine(line: MessageLine, nameClass: string)}
  {#if line.name}<span class={nameClass}
      ><AccountName name={line.name} identity={line.identity} /></span
    >: {line.excerpt}{:else}{line.excerpt}{/if}
{/snippet}

{#snippet secondAvatar()}
  {#if second}<UserAvatar user={second} size="xs" />{/if}
{/snippet}

{#snippet context()}
  <span class="font-medium">{@render room()}</span>
  {#if root}· {@render messageLine(root, '')}{/if}
{/snippet}

<ActivityListRow
  {pending}
  disabled={pending}
  important={attention === NotificationAttentionLevel.IMPORTANT}
  onclick={onopen}
  rowAttributes={{
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
    <ActivityAvatar actor={participants[0] ?? null} badge={second ? secondAvatar : undefined}>
      {#snippet indicator()}
        {#if hasAttention}
          <UnreadDot
            color={attention === NotificationAttentionLevel.IMPORTANT ? 'warning' : 'ambient'}
            overlay
            testid="thread-attention-dot"
          />
        {/if}
      {/snippet}
    </ActivityAvatar>
  {/snippet}

  {#if unread}<span class="sr-only">{m('chat.threads.filter_unread')}</span>{/if}
  {#if hasAttention}<span class="sr-only">{m('room_list.notifications', { count: 1 })}</span>{/if}
  <ActivityRowText
    {time}
    trailing={replyCount}
    emphasized={unread}
    subtitle={context}
    testId="thread-content"
  >
    {#snippet title()}
      {@render messageLine(latest, unread ? '' : 'font-medium')}
    {/snippet}
  </ActivityRowText>

  {#snippet actions()}
    <!-- Always present, so every row reserves the same action width and times align. -->
    <button
      type="button"
      class={['icon-action', !canMarkRead && 'invisible']}
      disabled={pending || !canMarkRead}
      onclick={onmarkread}
      title={m('room_list.mark_as_read')}
      aria-label={m('room_list.mark_as_read')}
    >
      <span class="iconify icon-[uil--check] text-base" aria-hidden="true"></span>
    </button>
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
  {/snippet}
</ActivityListRow>
