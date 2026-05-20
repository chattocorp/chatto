<!--
@component

Quick actions toolbar that appears on hover at the upper-right of a message.
Shows quick reaction emoji and action icons (reply, edit, more menu) inline.
Desktop only (pointer-fine); mobile uses the long-press action sheet instead.

**Props:**
- `serverId` - Server ID (scopes the recent-emoji slots per server)
- `roomId` - Room ID
- `messageEventId` - Event ID of the message
- `eventId` - Event ID for edit/delete operations
- `messageBody` - Current message body text
- `reactions` - Current reactions on the message (for toggle behavior)
- `canReact` - Whether the user can add reactions
- `canEdit` - Whether the user can edit this message
- `forceVisible` - Keep toolbar visible (e.g. while emoji picker is open)
- `onReplyInRoom` - Callback to reply in room (attribution only, no thread)
- `onReply` - Callback to open the thread pane
- `onOpenEmojiPicker` - Callback to open the full emoji picker
- `onOpenMenu` - Callback to open the full context menu
-->
<script lang="ts">
  import { useMessageActions, type MessageActionParams } from '$lib/hooks';
  import { getRecentEmojis } from '$lib/state/recentEmojis.svelte';
  import { getEmojiByName } from '$lib/emoji';

  let {
    serverId,
    roomId,
    messageEventId,
    eventId,
    messageBody,
    reactions = [],
    canReact = false,
    canEdit = false,
    forceVisible = false,
    onReplyInRoom,
    onReply,
    onOpenEmojiPicker,
    onOpenMenu
  }: {
    serverId: string;
    roomId: string;
    messageEventId: string;
    eventId: string;
    messageBody: string;
    reactions?: { emoji: string; hasReacted: boolean }[];
    canReact?: boolean;
    canEdit?: boolean;
    forceVisible?: boolean;
    onReplyInRoom?: () => void;
    onReply?: () => void;
    onOpenEmojiPicker?: (e: MouseEvent) => void;
    onOpenMenu?: (e: MouseEvent) => void;
  } = $props();

  const quickReactions = $derived(getRecentEmojis(serverId).quickReactions);

  const actions = useMessageActions();

  const params: MessageActionParams = $derived({
    roomId,
    messageEventId,
    eventId,
    messageBody
  });

  const hasActions = $derived(!!onReplyInRoom || !!onReply || canEdit || !!onOpenMenu);

  /** Set of Unicode emojis the current user has already reacted with (API returns shortcodes) */
  const myReactions = $derived(
    new Set(reactions.filter((r) => r.hasReacted).map((r) => getEmojiByName(r.emoji) ?? r.emoji))
  );

  function hasReacted(emoji: string): boolean {
    return myReactions.has(emoji);
  }

  async function handleReaction(emoji: string) {
    await actions.toggleReaction(params, emoji, hasReacted(emoji));
  }

  function handleReplyInRoom() {
    onReplyInRoom?.();
  }

  function handleReply() {
    onReply?.();
  }

  function handleEdit() {
    actions.startEdit(params);
  }
</script>

<div
  class={[
    'invisible absolute right-0 bottom-full z-10 mb-[-6px] hidden flex-row gap-0.5 rounded-t-md rounded-b-none border border-b-0 border-border bg-surface-100 p-0.5 md:flex',
    'pointer-fine:group-hover:visible'
  ]}
  class:!visible={forceVisible}
  role="toolbar"
  tabindex="-1"
  aria-label="Message actions"
  onmousedown={(e) => e.stopPropagation()}
>
  {#if canReact}
    <div class="flex items-center menu-section-sm">
      {#each quickReactions as emoji (emoji)}
        <button
          class="flex h-7 w-7 cursor-pointer items-center justify-center rounded text-base hover:bg-surface-100"
          onclick={() => handleReaction(emoji)}
          aria-label={hasReacted(emoji) ? `Remove ${emoji}` : `React with ${emoji}`}
        >
          {emoji}
        </button>
      {/each}
      {#if onOpenEmojiPicker}
        <button
          class="flex h-7 w-7 cursor-pointer items-center justify-center rounded text-muted hover:bg-surface-100 hover:text-text"
          onclick={onOpenEmojiPicker}
          aria-label="More reactions"
        >
          <span class="iconify text-base uil--smile"></span>
        </button>
      {/if}
    </div>
  {/if}

  {#if hasActions}
    <div class="flex items-center menu-section-sm">
      {#if onReplyInRoom}
        <button
          class="flex h-7 w-7 cursor-pointer items-center justify-center rounded text-muted hover:bg-surface-100 hover:text-text"
          onclick={handleReplyInRoom}
          aria-label="Reply"
        >
          <span class="iconify text-base uil--corner-up-left"></span>
        </button>
      {/if}

      {#if onReply}
        <button
          class="flex h-7 w-7 cursor-pointer items-center justify-center rounded text-muted hover:bg-surface-100 hover:text-text"
          onclick={handleReply}
          aria-label="Reply in thread"
        >
          <span class="iconify text-base uil--comment-alt-lines"></span>
        </button>
      {/if}

      {#if canEdit}
        <button
          class="flex h-7 w-7 cursor-pointer items-center justify-center rounded text-muted hover:bg-surface-100 hover:text-text"
          onclick={handleEdit}
          aria-label="Edit message"
        >
          <span class="iconify text-base uil--pen"></span>
        </button>
      {/if}

      {#if onOpenMenu}
        <button
          class="flex h-7 w-7 cursor-pointer items-center justify-center rounded text-muted hover:bg-surface-100 hover:text-text"
          onclick={onOpenMenu}
          aria-label="More actions"
        >
          <span class="iconify text-base uil--ellipsis-v"></span>
        </button>
      {/if}
    </div>
  {/if}
</div>
