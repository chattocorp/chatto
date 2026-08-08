<!--
@component

Meta bar shown beneath a message when it has thread replies or reactions.
Contains the thread reply button, reaction pills, and an add-reaction button.

Reaction mutations use the same bound message-action model as the hover,
context-menu, and touch surfaces. Thread navigation and tooltip state remain
local to the footer.
-->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { on } from 'svelte/events';
  import type { MessagePostedPayload } from '$lib/render/timelineEvents';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import UnreadDot from '$lib/ui/UnreadDot.svelte';
  import FloatingPopover from '$lib/ui/FloatingPopover.svelte';
  import { getEmojiByName, getEmojiDisplayName } from '$lib/emoji';
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
    serverSegment,
    threadRootEventId,
    reactions,
    action,
    replyCount = 0,
    threadExists = false,
    threadParticipants,
    hasThreadNotification = false,
    isFollowingThread = false,
    isThreadFollowPending = false,
    onToggleThreadFollow,
    onOpenThread,
    onOpenEmojiPicker,
    isEchoEvent = false
  }: {
    roomId: string;
    serverSegment: string;
    threadRootEventId?: string | null;
    reactions: ReactionSummary[];
    action: MessageActionModel;
    replyCount?: number;
    threadExists?: boolean;
    threadParticipants?: MessagePostedPayload['threadParticipants'];
    hasThreadNotification?: boolean;
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
  const REACTION_TOOLTIP_USER_LIMIT = 5;
  function reactionTooltipUsers(reaction: ReactionSummary): {
    names: string[];
    remaining: number;
  } {
    const names = reaction.users
      .slice(0, REACTION_TOOLTIP_USER_LIMIT)
      .map((user) => user.displayName);
    return {
      names,
      remaining: Math.max(0, reaction.count - names.length)
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
    await action.toggleReaction(reaction.emoji);
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
      href={resolve('/chat/[serverId]/[roomId]/[threadId]', {
        serverId: serverSegment,
        roomId,
        threadId: threadRootEventId
      })}
      class="{baseButtonClass} gap-2 border-transparent px-2 text-xs"
      onclick={openThreadFromLink}
      {@attach threadLinkGestureBoundary}
    >
      <span class="iconify icon-[uil--corner-up-right] rtl:-scale-x-100"></span>
      <span>{m('room.message.meta.thread')}</span>
    </a>
  {/if}

  <!-- Thread reply button -->
  {#if (threadExists || replyCount > 0) && onOpenThread && threadRootEventId}
    <a
      href={resolve('/chat/[serverId]/[roomId]/[threadId]', {
        serverId: serverSegment,
        roomId,
        threadId: threadRootEventId
      })}
      class="{baseButtonClass} gap-2 border-transparent px-2 text-xs"
      onclick={openThreadFromLink}
      {@attach threadLinkGestureBoundary}
    >
      <span class="iconify icon-[uil--comment-alt-lines]"></span>
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
      {#if hasThreadNotification}
        <UnreadDot testid="thread-notification-dot" />
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
        title={isFollowingThread
          ? m('room.message.meta.unfollow_thread')
          : m('room.message.meta.follow_thread')}
      >
        <span
          class={[
            'iconify text-base',
            isFollowingThread ? 'icon-[uil--bell]' : 'icon-[uil--bell-slash]'
          ]}
        ></span>
      </button>
    {/if}
  {/if}

  <!-- Reaction pills -->
  {#each reactions as reaction (reaction.emoji)}
    <span
      role="group"
      onmouseenter={(e) => showReactionTooltip(e, reaction)}
      onmouseleave={hideReactionTooltip}
    >
      <button
        class={[
          baseButtonClass,
          'gap-1 px-2 text-sm',
          action.canReact ? '' : '!cursor-default opacity-60',
          reaction.hasReacted ? 'border-action/50' : 'border-transparent'
        ]}
        onclick={() => action.canReact && toggleReaction(reaction)}
        onfocus={(e) => showReactionTooltip(e, reaction)}
        onblur={hideReactionTooltip}
        disabled={!action.canReact}
        aria-describedby={tooltipReactionEmoji === reaction.emoji ? reactionTooltipId : undefined}
        aria-label={reaction.hasReacted
          ? m('room.message.meta.remove_reaction_label', {
              emoji: getEmojiByName(reaction.emoji) ?? reaction.emoji,
              count: reaction.count
            })
          : m('room.message.meta.add_reaction_label', {
              emoji: getEmojiByName(reaction.emoji) ?? reaction.emoji,
              count: reaction.count
            })}
        aria-pressed={reaction.hasReacted}
      >
        <span aria-hidden="true">{getEmojiByName(reaction.emoji) ?? reaction.emoji}</span>
        <span class="text-xs" aria-hidden="true">{reaction.count}</span>
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
      <span class="iconify icon-[uil--smile] text-base"></span>
    </button>
  {/if}
</div>

<FloatingPopover
  open={!!tooltipReaction && !!tooltipAnchor}
  anchor={tooltipAnchor}
  role="tooltip"
  id={reactionTooltipId}
  class="pointer-events-none w-64 menu"
>
  {#if tooltipReaction}
    {@const tooltipUsers = reactionTooltipUsers(tooltipReaction)}
    <div class="flex min-w-0 flex-col gap-1 menu-section px-3 py-2 text-xs">
      <strong class="font-semibold">{getEmojiDisplayName(tooltipReaction.emoji)}</strong>
      <span class="flex min-w-0 flex-col gap-0.5 text-muted">
        {#each tooltipUsers.names as name (name)}
          <span class="break-words" data-testid="reaction-tooltip-user">{name}</span>
        {/each}
        {#if tooltipUsers.remaining > 0}
          <span class="text-muted/80">
            {m('room.message.meta.reaction_users_more', { count: tooltipUsers.remaining })}
          </span>
        {/if}
      </span>
    </div>
  {/if}
</FloatingPopover>
