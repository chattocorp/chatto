<!--
@component

Context menu content for message actions.
Rendered inside a ContextMenu when right-clicking a message.

**Props:**
- `spaceId` - Space ID
- `roomId` - Room ID
- `messageEventId` - Event ID of the message
- `eventId` - Event ID for edit/delete operations
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
  import { recentReactions } from '$lib/state/recentReactions.svelte';
  import { getEmojiByName } from '$lib/emoji';

  let {
    roomId,
    messageEventId,
    eventId,
    messageBody,
    reactions = [],
    canReact = false,
    canEdit = false,
    canDelete = false,
    onReplyInRoom,
    onReply,
    onOpenEmojiPicker,
    onClose
  }: {
    roomId: string;
    messageEventId: string;
    eventId: string;
    messageBody: string;
    reactions?: { emoji: string; hasReacted: boolean }[];
    canReact?: boolean;
    canEdit?: boolean;
    canDelete?: boolean;
    onReplyInRoom?: () => void;
    onReply?: () => void;
    onOpenEmojiPicker?: () => void;
    onClose: () => void;
  } = $props();

  const actions = useMessageActions();

  const params: MessageActionParams = $derived({
    roomId,
    messageEventId,
    eventId,
    messageBody
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

  function handleDelete() {
    actions.openDeleteConfirmation(params);
    onClose();
  }
</script>

{#if canReact}
  <div class="menu-section">
    <div class="flex justify-between">
      {#each recentReactions.quickReactions as emoji (emoji)}
        <button
          class="flex h-8 w-8 cursor-pointer items-center justify-center rounded text-base hover:bg-surface-100"
          onclick={() => handleReaction(emoji)}
          aria-label="React with {emoji}"
          role="menuitem"
        >
          {emoji}
        </button>
      {/each}
      {#if onOpenEmojiPicker}
        <button
          class="flex h-8 w-8 cursor-pointer items-center justify-center rounded text-base text-muted hover:bg-surface-100"
          onclick={() => {
            onOpenEmojiPicker();
            onClose();
          }}
          aria-label="More reactions"
          role="menuitem"
        >
          <span class="iconify text-lg uil--smile"></span>
        </button>
      {/if}
    </div>
  </div>
{/if}

{#if onReplyInRoom || onReply || canEdit || canDelete}
  <div class="menu-section">
    <nav class="sidebar-nav">
      {#if onReplyInRoom}
        <button class="sidebar-item" onclick={handleReplyInRoom} role="menuitem">
          <span class="sidebar-icon iconify uil--corner-up-left"></span>
          Reply
        </button>
      {/if}

      {#if onReply}
        <button class="sidebar-item" onclick={handleReply} role="menuitem">
          <span class="sidebar-icon iconify uil--comment-alt-lines"></span>
          Reply in thread
        </button>
      {/if}

      {#if canEdit}
        <button class="sidebar-item" onclick={handleEdit} role="menuitem">
          <span class="sidebar-icon iconify uil--pen"></span>
          Edit
        </button>
      {/if}

      {#if canDelete}
        <button
          class="sidebar-item text-danger hover:text-danger"
          onclick={handleDelete}
          role="menuitem"
        >
          <span class="sidebar-icon iconify uil--trash-alt"></span>
          Delete
        </button>
      {/if}
    </nav>
  </div>
{/if}
