<!-- @component One consolidated notification group in the notifications list. Presentational only: the page resolves summaries, targets, and mutations. -->
<script lang="ts">
  import type { AccountNameIdentity } from '@chatto/client/timeline/accountName';
  import { NotificationSignalKind, type NotificationActor } from '@chatto/client/api/notifications';
  import { ActivityListRow } from '$lib/ui';
  import ActivityRowText from '$lib/components/ActivityRowText.svelte';
  import ActivityAvatar, { type ActivityServer } from '$lib/components/ActivityAvatar.svelte';
  import AccountNameTokens from '$lib/components/users/AccountNameTokens.svelte';
  import { m } from '$lib/i18n/messages';

  let {
    summary,
    summaryAccounts,
    actor,
    signalKind,
    reactionEmoji = null,
    roomName,
    time,
    unread,
    important,
    interactive = true,
    pending = false,
    server = null,
    onopen,
    ondismiss
  }: {
    /** Localized summary containing account tokens made by `accountNameToken`. */
    summary: string;
    summaryAccounts: readonly { name: string; identity?: AccountNameIdentity | null }[];
    /** Actor shown as the avatar; null when the actor is no longer visible. */
    actor: NotificationActor | null;
    signalKind: NotificationSignalKind | undefined;
    /** Rendered reaction emoji for the cause badge of a reaction group. */
    reactionEmoji?: string | null;
    roomName: string | null;
    /** Relative time for today's activity; local clock time for older days. */
    time: string;
    unread: boolean;
    /** Attention classification for row metadata; never changes the background. */
    important: boolean;
    /** False when this client cannot open the notification target. */
    interactive?: boolean;
    /** A mutation for this row is in flight. */
    pending?: boolean;
    /** Server marker, set only when more than one server is signed in. */
    server?: ActivityServer | null;
    onopen: () => void;
    ondismiss: () => void;
  } = $props();

  /** Icon that identifies the notification cause on the avatar badge. */
  function signalIcon(kind: NotificationSignalKind | undefined): string {
    switch (kind) {
      case NotificationSignalKind.ROOM_INVITATION:
        return 'icon-[uil--envelope-alt]';
      case NotificationSignalKind.DIRECT_MESSAGE:
        return 'icon-[uil--envelope]';
      case NotificationSignalKind.REPLY:
      case NotificationSignalKind.FOLLOWED_THREAD:
        return 'icon-[uil--comments]';
      case NotificationSignalKind.DIRECT_MENTION:
      case NotificationSignalKind.ROLE_MENTION:
      case NotificationSignalKind.HERE:
      case NotificationSignalKind.ALL:
        return 'icon-[uil--at]';
      case NotificationSignalKind.ROOM_MESSAGE:
      case NotificationSignalKind.FOLLOWED_ROOM:
        return 'icon-[uil--comment-alt-lines]';
      default:
        return 'icon-[uil--bell]';
    }
  }
</script>

{#snippet location()}
  {#if roomName}<bdi dir="auto">#{roomName}</bdi>{/if}
{/snippet}

<ActivityListRow
  compact
  overlayActions
  {interactive}
  {pending}
  disabled={pending || !interactive}
  onclick={onopen}
  rowAttributes={{
    class: '@container/activity',
    'data-testid': 'notification-group',
    'data-notification-state': unread ? 'unread' : 'read',
    'data-notification-attention': unread ? (important ? 'important' : 'ambient') : 'none'
  }}
>
  {#snippet leading()}
    <span class="shrink-0 self-start">
      <ActivityAvatar {actor} {server} size="sm">
        {#snippet badge()}
          {#if reactionEmoji}
            <!-- Centre the 12px glyph, not its advance width (which can include trailing space).
                 The half-pixel baseline offset balances the visible glyph inside the 16px circle.
                 Keep its coordinate direction fixed when the surrounding layout is RTL. -->
            <svg class="size-full" viewBox="0 0 16 16" aria-hidden="true">
              <text
                x="2"
                y="7.5"
                direction="ltr"
                dominant-baseline="central"
                font-family="Apple Color Emoji, Segoe UI Emoji, Noto Color Emoji, sans-serif"
                font-size="12">{reactionEmoji}</text
              >
            </svg>
          {:else}
            <span class="iconify {signalIcon(signalKind)}" aria-hidden="true"></span>
          {/if}
        {/snippet}
      </ActivityAvatar>
    </span>
  {/snippet}
  {#if unread}
    <span class="sr-only">{m('chat.notifications.unread')}</span>
  {/if}
  {#if server}
    <span class="sr-only" data-testid="notification-server-name">{server.name}</span>
  {/if}
  <ActivityRowText
    {time}
    hideTimeOnNarrow
    emphasized={unread}
    overlayActions={1}
    location={roomName ? location : undefined}
    testId="notification-content"
  >
    {#snippet title()}
      <AccountNameTokens text={summary} accounts={summaryAccounts} />
    {/snippet}
  </ActivityRowText>
  {#snippet actions()}
    <button
      type="button"
      class="icon-action"
      disabled={pending}
      aria-label={m('common.delete')}
      title={m('common.delete')}
      onclick={ondismiss}
    >
      <span class="iconify icon-[uil--trash-alt] text-base" aria-hidden="true"></span>
    </button>
  {/snippet}
</ActivityListRow>
