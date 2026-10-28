<!--
@component

Message meta bar shown beneath a message when it has thread replies, reactions,
or a pin indicator. Contains compact message-state badges and actions.

Reaction mutations use the same bound message-action model as the hover,
context-menu, and touch surfaces. Thread navigation and tooltip state remain
local to the footer.
-->
<script lang="ts">
  import { formatAccountName } from '@chatto/client/timeline/accountName';
  import { getReactiveLocale } from '$lib/i18n/state.svelte';
  import { conversationPath } from '$lib/navigation/conversationPath';
  import { on } from 'svelte/events';
  import type { MessagePostedPayload } from '@chatto/client/timeline/timelineEvents';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import { UnreadDot, FloatingPopover, ConfirmDialog } from '$lib/ui';
  import { getReactionDisplay, getEmojiDisplayName, getEmojiByName } from '$lib/emoji';
  import { m } from '$lib/i18n/messages';
  import type { MessageActionModel } from './messageActionModel';

  // Extract the MessagePostedEvent type from the union
  type ReactionSummary = MessagePostedPayload['reactions'][number];

  // Shared base style for all meta bar buttons. Uses the `meta-badge` utility
  // for shape and background states. Border color is set per-button to avoid
  // Tailwind v4 specificity conflicts on overrides.
  const baseButtonClass = 'meta-badge h-[25px] cursor-pointer text-muted';

  let {
    roomId,
    serverId,
    threadRootEventId,
    reactions,
    edited = false,
    channelEchoEventId,
    action,
    replyCount = 0,
    threadExists = false,
    threadParticipants,
    hasThreadNotification = false,
    hasThreadUnread = false,
    isFollowingThread = false,
    isThreadFollowPending = false,
    onToggleThreadFollow,
    onOpenThread,
    onOpenEmojiPicker,
    isEchoEvent = false
  }: {
    roomId: string;
    serverId: string;
    threadRootEventId?: string | null;
    reactions: ReactionSummary[];
    /** Show the edited status before reactions. */
    edited?: boolean;
    /** Room echo to open through the message jump and highlight route. */
    channelEchoEventId?: string | null;
    action?: MessageActionModel;
    replyCount?: number;
    threadExists?: boolean;
    threadParticipants?: MessagePostedPayload['threadParticipants'];
    hasThreadNotification?: boolean;
    hasThreadUnread?: boolean;
    isFollowingThread?: boolean;
    isThreadFollowPending?: boolean;
    onToggleThreadFollow?: (e: MouseEvent) => void;
    onOpenThread?: () => void;
    onOpenEmojiPicker?: (e: MouseEvent) => void;
    isEchoEvent?: boolean;
  } = $props();

  const replyCountLabel = $derived(
    replyCount === 1
      ? m('room.message.meta.reply_count_one')
      : m('room.message.meta.reply_count_many', { count: replyCount })
  );
  const reactionTooltipId = `reaction-tooltip-${crypto.randomUUID().slice(0, 8)}`;
  let tooltipReactionEmoji = $state<string | null>(null);
  let tooltipAnchor = $state<{ top: number; bottom: number; left: number } | null>(null);
  const tooltipReaction = $derived(
    tooltipReactionEmoji ? (reactions.find((r) => r.emoji === tooltipReactionEmoji) ?? null) : null
  );
  let unpinConfirmationVisible = $state(false);
  let unpinning = $state(false);
  const REACTION_TOOLTIP_USER_LIMIT = 5;
  function reactionTooltipUsers(reaction: ReactionSummary): {
    users: ReactionSummary['users'];
    remaining: number;
  } {
    const users = reaction.users.slice(0, REACTION_TOOLTIP_USER_LIMIT);
    return {
      users,
      remaining: Math.max(0, reaction.count - users.length)
    };
  }

  function showReactionTooltip(e: MouseEvent | FocusEvent, reaction: ReactionSummary) {
    if (reaction.users.length === 0) return;

    const button = e.currentTarget as HTMLElement;
    const rect = button.getBoundingClientRect();
    tooltipReactionEmoji = reaction.emoji;
    tooltipAnchor = { top: rect.top, bottom: rect.bottom, left: rect.left };
  }

  function hideReactionTooltip() {
    tooltipReactionEmoji = null;
    tooltipAnchor = null;
  }

  async function toggleReaction(reaction: ReactionSummary) {
    await action?.toggleReaction(reaction.emoji);
  }

  function requestUnpin(event: MouseEvent): void {
    event.stopPropagation();
    unpinConfirmationVisible = true;
  }

  function closeUnpinConfirmation(): void {
    unpinConfirmationVisible = false;
  }

  async function confirmUnpin(): Promise<void> {
    if (!action?.canPin || !action.isPinned) {
      closeUnpinConfirmation();
      return;
    }

    unpinning = true;
    try {
      await action.togglePin();
      closeUnpinConfirmation();
    } finally {
      unpinning = false;
    }
  }

  function openThreadFromLink(e: MouseEvent) {
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) {
      return;
    }

    e.preventDefault();
    onOpenThread?.();
  }

  function stopMessageGesturePropagation(e: Event) {
    e.stopPropagation();
  }

  function threadLinkGestureBoundary(el: HTMLAnchorElement) {
    const removeTouchStart = on(el, 'touchstart', stopMessageGesturePropagation, {
      capture: true
    });
    const removeMouseDown = on(el, 'mousedown', stopMessageGesturePropagation, {
      capture: true
    });

    return () => {
      removeTouchStart();
      removeMouseDown();
    };
  }
</script>

<div class="mt-1 flex flex-wrap items-center gap-1">
  <!-- Echo "Thread" indicator -->
  {#if isEchoEvent && onOpenThread && threadRootEventId}
    <a
      href={conversationPath(serverId, roomId, { threadId: threadRootEventId })}
      class="{baseButtonClass} gap-2 border-transparent px-2 text-xs whitespace-nowrap"
      onclick={openThreadFromLink}
      {@attach threadLinkGestureBoundary}
    >
      <span aria-hidden="true" class="iconify icon-[uil--corner-up-right] rtl:-scale-x-100"></span>
      <span>{m('room.message.meta.thread')}</span>
    </a>
  {/if}

  <!-- Thread reply button -->
  {#if (threadExists || replyCount > 0) && onOpenThread && threadRootEventId}
    <a
      href={conversationPath(serverId, roomId, { threadId: threadRootEventId })}
      class="{baseButtonClass} gap-2 border-transparent px-2 text-xs whitespace-nowrap"
      onclick={openThreadFromLink}
      {@attach threadLinkGestureBoundary}
    >
      <span aria-hidden="true" class="iconify icon-[uil--comment-alt-lines]"></span>
      {#if replyCount > 0 && threadParticipants && threadParticipants.length > 0}
        <div class="flex -space-x-1.5">
          {#each threadParticipants.slice(0, 3) as participant, i (i)}
            {@const p = participant}
            {#if p}
              <UserAvatar user={p} size="xs" />
            {/if}
          {/each}
        </div>
      {/if}
      <span>
        {replyCount > 0 ? replyCountLabel : m('room.message.meta.thread')}
      </span>
      {#if hasThreadNotification || hasThreadUnread}
        <UnreadDot
          color={hasThreadNotification ? 'warning' : 'neutral'}
          testid={hasThreadNotification ? 'thread-notification-dot' : 'thread-unread-dot'}
        />
      {/if}
    </a>
    {#if onToggleThreadFollow}
      <button
        class={[
          baseButtonClass,
          'justify-center border-transparent px-1.5',
          isFollowingThread ? 'text-text' : ''
        ]}
        onclick={onToggleThreadFollow}
        disabled={isThreadFollowPending}
        aria-label={m('room.message.meta.follow_thread')}
        aria-pressed={isFollowingThread}
        title={isFollowingThread
          ? m('room.message.meta.unfollow_thread')
          : m('room.message.meta.follow_thread')}
      >
        <span
          class={[
            'iconify text-base',
            isFollowingThread ? 'icon-[uil--bell]' : 'icon-[uil--bell-slash]'
          ]}
          aria-hidden="true"
        ></span>
      </button>
    {/if}
  {/if}

  {#if action?.isPinned}
    {#if action.canPin}
      <button
        class="{baseButtonClass} justify-center border-transparent px-1.5"
        onclick={requestUnpin}
        aria-label={m('room.pins.unpin')}
        title={m('room.pins.unpin')}
      >
        <span class="iconify icon-[mdi--pin-outline] text-base" aria-hidden="true"></span>
      </button>
    {:else}
      <span
        class="meta-badge h-[25px] cursor-default border-transparent px-1.5 text-muted"
        role="img"
        aria-label={m('room.message.meta.pinned')}
        title={m('room.message.meta.pinned')}
      >
        <span class="iconify icon-[mdi--pin-outline] text-base" aria-hidden="true"></span>
      </span>
    {/if}
  {/if}

  {#if edited}
    <span
      class="meta-badge h-[25px] gap-2 border-transparent px-2 text-xs whitespace-nowrap text-muted"
    >
      <span class="iconify icon-[uil--pen]" aria-hidden="true"></span>
      <span>{m('room.message.meta.edited_short')}</span>
    </span>
  {/if}

  {#if channelEchoEventId}
    <a
      href={conversationPath(serverId, roomId, { messageId: channelEchoEventId })}
      class="{baseButtonClass} gap-2 border-transparent px-2 text-xs whitespace-nowrap"
      title={m('room.message.meta.echoed_to_channel')}
      {@attach threadLinkGestureBoundary}
    >
      <span class="iconify icon-[uil--megaphone]" aria-hidden="true"></span>
      <span>{m('room.message.meta.echo')}</span>
    </a>
  {/if}

  <!-- Reaction pills -->
  {#each reactions as reaction (reaction.emoji)}
    <span
      role="group"
      class="max-w-full min-w-0"
      onmouseenter={(e) => showReactionTooltip(e, reaction)}
      onmouseleave={hideReactionTooltip}
    >
      <button
        class={[
          baseButtonClass,
          'max-w-full gap-1 px-2 text-sm disabled:cursor-default disabled:opacity-60',
          reaction.hasReacted ? 'border-action/50' : 'border-transparent'
        ]}
        onclick={() => action?.canReact && toggleReaction(reaction)}
        onfocus={(e) => showReactionTooltip(e, reaction)}
        onblur={hideReactionTooltip}
        disabled={!action?.canReact}
        aria-describedby={tooltipReactionEmoji === reaction.emoji ? reactionTooltipId : undefined}
        aria-label={`${getReactionDisplay(reaction.emoji)} ${reaction.count}`}
        aria-pressed={reaction.hasReacted}
      >
        <span class="max-w-48 truncate" aria-hidden="true"
          >{getReactionDisplay(reaction.emoji)}</span
        >
        <span class="shrink-0 text-xs" aria-hidden="true">{reaction.count}</span>
      </button>
    </span>
  {/each}

  <!-- Add reaction button -->
  {#if onOpenEmojiPicker}
    <button
      class="{baseButtonClass} justify-center border-transparent px-1.5"
      onclick={(e) => onOpenEmojiPicker(e)}
      aria-label={m('room.message.actions.add_reaction')}
    >
      <span aria-hidden="true" class="iconify icon-[uil--smile] text-base"></span>
    </button>
  {/if}
</div>

{#if action?.isPinned && action.canPin}
  <ConfirmDialog
    bind:visible={unpinConfirmationVisible}
    title={m('room.pins.unpin')}
    tone="warning"
    actionLabel={m('room.pins.unpin')}
    actionIcon="iconify icon-[mdi--pin-outline]"
    loading={unpinning}
    onconfirm={confirmUnpin}
    onclose={closeUnpinConfirmation}
  >
    {m('room.pins.unpin_prompt')}
  </ConfirmDialog>
{/if}

<FloatingPopover
  open={!!tooltipReaction && !!tooltipAnchor}
  anchor={tooltipAnchor}
  role="tooltip"
  id={reactionTooltipId}
  class="pointer-events-none w-64 menu"
>
  {#if tooltipReaction}
    {@const tooltipUsers = reactionTooltipUsers(tooltipReaction)}
    {@const glyph = getEmojiByName(tooltipReaction.emoji)}
    {@const names = tooltipUsers.users.map((user) => formatAccountName(user.displayName, user))}
    {@const usersText = new Intl.ListFormat(getReactiveLocale(), { type: 'conjunction' }).format(
      tooltipUsers.remaining ? [...names, `+${tooltipUsers.remaining}`] : names
    )}
    <!-- Split only translation placeholders, before inserting any user-controlled text. -->
    {@const sentenceParts = m('emoji.reaction_summary', {
      emoji: '{emoji}',
      users: '{users}'
    }).split(/(\{emoji\}|\{users\})/)}
    <div class="flex min-w-0 items-start gap-3 menu-section p-3 text-sm">
      {#if glyph}
        <span
          class="flex h-11 w-11 shrink-0 items-center justify-center text-4xl"
          aria-hidden="true">{glyph}</span
        >
      {/if}
      <p class="min-w-0 flex-1 leading-relaxed text-muted">
        {#each sentenceParts as part, index (index)}
          {#if part === '{emoji}'}<strong class="font-semibold break-words text-text"
              >{getEmojiDisplayName(tooltipReaction.emoji)}</strong
            >
          {:else if part === '{users}'}<span
              class="break-words"
              data-testid="reaction-tooltip-users">{usersText}</span
            >
          {:else}{part}{/if}
        {/each}
      </p>
    </div>
  {/if}
</FloatingPopover>
