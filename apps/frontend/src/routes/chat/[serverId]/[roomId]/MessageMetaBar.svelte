<!--
@component

Meta bar shown beneath a message when it has thread replies or reactions.
Contains the thread reply button, reaction pills, and an add-reaction button.

**Props:**
- `spaceId` - Space ID
- `roomId` - Room ID
- `messageEventId` - Event ID of the message
- `serverSegment` - URL segment for the active server
- `threadRootEventId` - Root event ID for the linked thread
- `reactions` - Array of reaction summaries
- `replyCount` - Number of thread replies
- `threadParticipants` - Thread participant user fragments (for avatars)
- `hasThreadNotification` - Whether there's an unread thread notification
- `canReact` - Whether the user can add reactions
- `isFollowingThread` - Whether the viewer is following this thread
- `onToggleThreadFollow` - Callback to toggle thread follow state
- `onOpenThread` - Callback to open the thread pane
- `onOpenEmojiPicker` - Callback to open the emoji picker
-->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { on } from 'svelte/events';
  import type { RoomEventView } from '$lib/render/types';
  import UserAvatar, { UserAvatarViewData } from '$lib/components/UserAvatar.svelte';
  import UnreadDot from '$lib/ui/UnreadDot.svelte';
  import { useRenderData } from '$lib/render/data';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { toast } from '$lib/ui/toast';
  import FloatingTooltip from '$lib/ui/FloatingTooltip.svelte';
  import { getEmojiByName, getEmojiDisplayName } from '$lib/emoji';
  import { createReactionAPI } from '$lib/api-client/reactions';
  import * as m from '$lib/i18n/messages';

  // Extract the MessagePostedEvent type from the union
  type MessagePostedEvent = Extract<RoomEventView['event'], { kind: 'messagePosted' }>;
  type ReactionSummary = MessagePostedEvent['reactions'][number];

  // Shared base style for all meta bar buttons. Uses the `meta-badge` utility
  // for shape and background states. Border color is set per-button to avoid
  // Tailwind v4 specificity conflicts on overrides.
  const baseButtonClass = 'meta-badge h-[25px] cursor-pointer text-muted';

  let {
    roomId,
    messageEventId,
    serverSegment,
    threadRootEventId,
    reactions,
    replyCount = 0,
    threadParticipants,
    hasThreadNotification = false,
    canReact = false,
    isFollowingThread = false,
    onToggleThreadFollow,
    onOpenThread,
    onOpenEmojiPicker,
    isEchoEvent = false
  }: {
    roomId: string;
    messageEventId: string;
    serverSegment: string;
    threadRootEventId?: string | null;
    reactions: ReactionSummary[];
    replyCount?: number;
    threadParticipants?: MessagePostedEvent['threadParticipants'];
    hasThreadNotification?: boolean;
    canReact?: boolean;
    isFollowingThread?: boolean;
    onToggleThreadFollow?: (e: MouseEvent) => void;
    onOpenThread?: () => void;
    onOpenEmojiPicker?: (e: MouseEvent) => void;
    isEchoEvent?: boolean;
  } = $props();

  const connection = useConnection();
  const replyCountLabel = $derived(
    replyCount === 1
      ? m['room.message.meta.reply_count_one']()
      : m['room.message.meta.reply_count_many']({ count: replyCount })
  );
  const reactionTooltipId = `reaction-tooltip-${crypto.randomUUID().slice(0, 8)}`;
  let tooltipReactionEmoji = $state<string | null>(null);
  let tooltipAnchor = $state<{ top: number; bottom: number; left: number } | null>(null);
  const tooltipReaction = $derived(
    tooltipReactionEmoji ? (reactions.find((r) => r.emoji === tooltipReactionEmoji) ?? null) : null
  );

  function formatReactionUsers(users: { displayName: string }[], count: number): string {
    const maxDisplay = 5;
    const names = users.slice(0, maxDisplay).map((u) => u.displayName);
    const remaining = Math.max(0, count - names.length);

    if (remaining > 0) {
      return names.join(', ') + ` + ${remaining}`;
    }
    return names.join(', ');
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
    try {
      const conn = connection();
      const api = createReactionAPI({
        serverId: conn.serverId ?? serverSegment,
        baseUrl: conn.connectBaseUrl,
        bearerToken: conn.bearerToken
      });
      const input = { roomId, messageEventId, emoji: reaction.emoji };
      if (reaction.hasReacted) {
        await api.removeReaction(input);
      } else {
        await api.addReaction(input);
      }
    } catch {
      toast.error('Failed to update reaction');
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
      href={resolve('/chat/[serverId]/[roomId]/[threadId]', {
        serverId: serverSegment,
        roomId,
        threadId: threadRootEventId
      })}
      class="{baseButtonClass} gap-2 border-transparent px-2 text-xs"
      onclick={openThreadFromLink}
      {@attach threadLinkGestureBoundary}
    >
      <span class="iconify uil--corner-up-right"></span>
      <span>{m['room.message.meta.thread']()}</span>
    </a>
  {/if}

  <!-- Thread reply button -->
  {#if replyCount > 0 && onOpenThread && threadRootEventId}
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
      <span class="iconify uil--comment-alt-lines"></span>
      {#if threadParticipants && threadParticipants.length > 0}
        <div class="flex -space-x-1.5">
          {#each threadParticipants.slice(0, 3) as participant, i (i)}
            {@const p = useRenderData(UserAvatarViewData, participant)}
            {#if p}
              <UserAvatar user={p} size="xs" />
            {/if}
          {/each}
        </div>
      {/if}
      <span>
        {replyCountLabel}
      </span>
      {#if hasThreadNotification}
        <UnreadDot />
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
        title={isFollowingThread
          ? m['room.message.meta.unfollow_thread']()
          : m['room.message.meta.follow_thread']()}
      >
        <span class={['iconify text-base', isFollowingThread ? 'uil--bell' : 'uil--bell-slash']}
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
          canReact ? '' : '!cursor-default opacity-60',
          reaction.hasReacted ? 'border-accent/50' : 'border-transparent'
        ]}
        onclick={() => canReact && toggleReaction(reaction)}
        onfocus={(e) => showReactionTooltip(e, reaction)}
        onblur={hideReactionTooltip}
        disabled={!canReact}
        aria-describedby={tooltipReactionEmoji === reaction.emoji ? reactionTooltipId : undefined}
        aria-label={reaction.hasReacted
          ? m['room.message.meta.remove_reaction_label']({
              emoji: getEmojiByName(reaction.emoji) ?? reaction.emoji,
              count: reaction.count
            })
          : m['room.message.meta.add_reaction_label']({
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
      aria-label={m['room.message.actions.add_reaction']()}
    >
      <span class="iconify text-base uil--smile"></span>
    </button>
  {/if}
</div>

<FloatingTooltip
  open={!!tooltipReaction && !!tooltipAnchor}
  anchor={tooltipAnchor}
  id={reactionTooltipId}
>
  {#if tooltipReaction}
    <span class="whitespace-nowrap">
      <strong class="font-semibold">{getEmojiDisplayName(tooltipReaction.emoji)}</strong>
      <span> · {formatReactionUsers(tooltipReaction.users, tooltipReaction.count)}</span>
    </span>
  {/if}
</FloatingTooltip>
