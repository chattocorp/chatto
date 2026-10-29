<!-- @component One consolidated notification group in the notifications list. Presentational only: the page resolves summaries, targets, and mutations. -->
<script lang="ts">
  import type { AccountNameIdentity } from '@chatto/client/timeline/accountName';
  import { NotificationSignalKind, type NotificationActor } from '@chatto/client/api/notifications';
  import { ActivityListRow } from '$lib/ui';
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
  {#if roomName}<bdi dir="auto" class="hidden @min-[36rem]/notification:block">#{roomName}</bdi
    >{/if}
  {#if roomName && server}<span aria-hidden="true" class="hidden @min-[36rem]/notification:block"
      >·</span
    >{/if}
  {#if server}<bdi dir="auto" data-testid="notification-server-name">{server.name}</bdi>{/if}
{/snippet}

<ActivityListRow
  compact
  overlayActions
  {interactive}
  {pending}
  disabled={pending || !interactive}
  onclick={onopen}
  rowAttributes={{
    class: '@container/notification',
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
            <span class="leading-none">{reactionEmoji}</span>
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
  <span
    class="grid min-w-0 flex-1 grid-cols-[minmax(0,1fr)_auto] items-baseline gap-x-4 gap-y-0.5 text-muted @min-[36rem]/notification:grid-cols-[minmax(0,1fr)_fit-content(40%)_5rem]"
    data-testid="notification-content"
  >
    <bdi
      dir="auto"
      class={[
        'col-start-1 row-start-1 min-w-0 truncate',
        unread ? 'font-semibold text-text-top' : 'text-muted'
      ]}
    >
      <AccountNameTokens text={summary} accounts={summaryAccounts} />
    </bdi>
    {#if roomName || server}
      <span
        class={[
          'col-start-1 row-start-2 min-w-0 flex-wrap items-baseline gap-1 text-sm text-subdued @min-[36rem]/notification:col-start-2 @min-[36rem]/notification:row-start-1 @min-[36rem]/notification:flex @min-[36rem]/notification:flex-nowrap [&>bdi]:min-w-0 [&>bdi]:wrap-anywhere @min-[36rem]/notification:[&>bdi]:truncate',
          server ? 'flex' : 'hidden'
        ]}
      >
        {@render location()}
      </span>
    {/if}
    <span
      class="col-start-2 row-start-1 text-end text-sm whitespace-nowrap text-subdued tabular-nums @min-[36rem]/notification:col-start-3 [@media(hover:hover)_and_(pointer:fine)]:group-focus-within:opacity-0 [@media(hover:hover)_and_(pointer:fine)]:group-hover:opacity-0"
      >{time}</span
    >
  </span>
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
