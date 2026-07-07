<!--
@component

Context menu content for message actions.
Rendered inside a ContextMenu when right-clicking a message.

**Props:**
- `spaceId` - Space ID
- `roomId` - Room ID
- `messageEventId` - Event ID of the message
- `eventId` - Event ID for edit operations
- `deleteEventId` - Event ID for delete operations (defaults to `eventId`)
- `messageBody` - Current message body text
- `canReact` - Whether the user can add reactions
- `canEdit` - Whether the user can edit this message
- `canDelete` - Whether the user can delete this message
- `onReplyInRoom` - Callback to reply in room (attribution only)
- `onReply` - Callback to open the thread pane
- `onClose` - Callback to close the context menu
-->
<script lang="ts">
  import { useMessageActions, type MessageActionParams } from '$lib/hooks';
  import * as m from '$lib/i18n/messages';
  import { getRecentEmojis } from '$lib/state/recentEmojis.svelte';
  import { getEmojiByName } from '$lib/emoji';

  let {
    serverId,
    roomId,
    messageEventId,
    eventId,
    deleteEventId = eventId,
    messageBody,
    threadRootEventId = null,
    channelEchoEventId = null,
    canAddChannelEcho = false,
    reactions = [],
    canReact = false,
    canEdit = false,
    canDelete = false,
    replyInRoomLabel,
    replyThreadLabel,
    onReplyInRoom,
    onReply,
    onOpenEmojiPicker,
    onClose
  }: {
    serverId: string;
    roomId: string;
    messageEventId: string;
    eventId: string;
    deleteEventId?: string;
    messageBody: string;
    threadRootEventId?: string | null;
    channelEchoEventId?: string | null;
    canAddChannelEcho?: boolean;
    reactions?: { emoji: string; hasReacted: boolean }[];
    canReact?: boolean;
    canEdit?: boolean;
    canDelete?: boolean;
    replyInRoomLabel?: string;
    replyThreadLabel?: string;
    onReplyInRoom?: () => void;
    onReply?: () => void;
    onOpenEmojiPicker?: () => void;
    onClose: () => void;
  } = $props();

  const recentEmojis = $derived(getRecentEmojis(serverId));
  const quickReactions = $derived(recentEmojis.quickReactions);

  const actions = useMessageActions();
  const replyInRoomActionLabel = $derived(replyInRoomLabel ?? m['room.message.actions.reply']());
  const replyThreadActionLabel = $derived(
    replyThreadLabel ?? m['room.message.actions.reply_thread']()
  );

  const params: MessageActionParams = $derived({
    serverId,
    roomId,
    messageEventId,
    eventId,
    deleteEventId,
    messageBody,
    threadRootEventId,
    channelEchoEventId,
    canAddChannelEcho
  });

  /** Set of Unicode emojis the current user has already reacted with (API returns shortcodes) */
  const myReactions = $derived(
    new Set(reactions.filter((r) => r.hasReacted).map((r) => getEmojiByName(r.emoji) ?? r.emoji))
  );

  function hasReacted(emoji: string): boolean {
    return myReactions.has(emoji);
  }

  async function handleReaction(emoji: string) {
    await actions.toggleReaction(params, emoji, hasReacted(emoji));
    onClose();
  }

  function handleReplyInRoom() {
    onReplyInRoom?.();
    onClose();
  }

  function handleReply() {
    onReply?.();
    onClose();
  }

  function handleEdit() {
    actions.startEdit(params);
    onClose();
  }

  async function handleCopyLink() {
    await actions.copyMessageLink(params);
    onClose();
  }

  function handleDelete() {
    actions.openDeleteConfirmation(params);
    onClose();
  }
</script>

{#if canReact}
  <div class="menu-section">
    <div class="flex justify-between">
      {#each quickReactions as emoji (emoji)}
        <button
          class="flex h-10 w-10 cursor-pointer items-center justify-center rounded text-base transition-[background-color,scale] hover:bg-surface-100 active:scale-[0.96]"
          onclick={() => handleReaction(emoji)}
          aria-label={m['room.message.actions.react_with']({ emoji })}
          role="menuitem"
        >
          {emoji}
        </button>
      {/each}
      {#if onOpenEmojiPicker}
        <button
          class="flex h-10 w-10 cursor-pointer items-center justify-center rounded text-base text-muted transition-[background-color,scale] hover:bg-surface-100 active:scale-[0.96]"
          onclick={() => {
            onOpenEmojiPicker();
            onClose();
          }}
          aria-label={m['room.message.actions.more_reactions']()}
          role="menuitem"
        >
          <span class="iconify text-lg uil--smile"></span>
        </button>
      {/if}
    </div>
  </div>
{/if}

<div class="menu-section">
  <nav class="sidebar-nav">
    {#if onReplyInRoom}
      <button class="sidebar-item" onclick={handleReplyInRoom} role="menuitem">
        <span class="sidebar-icon iconify uil--corner-up-left"></span>
        {replyInRoomActionLabel}
      </button>
    {/if}

    {#if onReply}
      <button class="sidebar-item" onclick={handleReply} role="menuitem">
        <span class="sidebar-icon iconify uil--comment-alt-lines"></span>
        {replyThreadActionLabel}
      </button>
    {/if}

    {#if canEdit}
      <button class="sidebar-item" onclick={handleEdit} role="menuitem">
        <span class="sidebar-icon iconify uil--pen"></span>
        {m['room.message.actions.edit_short']()}
      </button>
    {/if}

    <button class="sidebar-item" onclick={handleCopyLink} role="menuitem">
      <span class="sidebar-icon iconify uil--copy"></span>
      {m['room.message.actions.copy_link']()}
    </button>

    {#if canDelete}
      <button
        class="sidebar-item text-danger hover:text-danger"
        onclick={handleDelete}
        role="menuitem"
      >
        <span class="sidebar-icon iconify uil--trash-alt"></span>
        {m['common.delete']()}
      </button>
    {/if}
  </nav>
</div>
