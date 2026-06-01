<script lang="ts">
  import { useMessageActions, type MessageActionParams } from '$lib/hooks';
  import { getRecentEmojis } from '$lib/state/recentEmojis.svelte';
  import { getEmojiByName } from '$lib/emoji';

  let {
    serverId,
    roomId,
    messageEventId,
    eventId,
    deleteEventId = eventId,
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
    serverId: string;
    roomId: string;
    messageEventId: string;
    eventId: string;
    deleteEventId?: string;
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

  const quickReactions = $derived(getRecentEmojis(serverId).quickReactions);

  const actions = useMessageActions();

  const params: MessageActionParams = $derived({
    roomId,
    messageEventId,
    eventId,
    deleteEventId,
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

<div class="flex flex-col rounded-xl bg-background">
  <!-- Quick reactions row -->
  {#if canReact}
    <div class="flex justify-around py-2">
      {#each quickReactions as emoji (emoji)}
        <button
          class="flex h-11 w-11 cursor-pointer items-center justify-center rounded-full text-2xl active:bg-surface-100"
          onclick={() => handleReaction(emoji)}
          aria-label="React with {emoji}"
        >
          {emoji}
        </button>
      {/each}
      {#if onOpenEmojiPicker}
        <button
          class="flex h-11 w-11 cursor-pointer items-center justify-center rounded-full text-2xl text-muted active:bg-surface-100"
          onclick={() => {
            onOpenEmojiPicker();
            onClose();
          }}
          aria-label="More reactions"
        >
          <span class="iconify uil--smile"></span>
        </button>
      {/if}
    </div>

    <hr class="my-1 border-border" />
  {/if}

  <!-- Action list using sidebar-item styling with extra padding for mobile tap targets -->
  {#if onReplyInRoom || onReply || canEdit || canDelete}
    <nav class="sidebar-nav">
      {#if onReplyInRoom}
        <button class="sidebar-item py-3.5" onclick={handleReplyInRoom}>
          <span class="sidebar-icon iconify uil--corner-up-left"></span>
          Reply
        </button>
      {/if}

      {#if onReply}
        <button class="sidebar-item py-3.5" onclick={handleReply}>
          <span class="sidebar-icon iconify uil--comment-alt-lines"></span>
          Reply in thread
        </button>
      {/if}

      {#if canEdit}
        <button class="sidebar-item py-3.5" onclick={handleEdit}>
          <span class="sidebar-icon iconify uil--pen"></span>
          Edit
        </button>
      {/if}

      {#if canDelete}
        <button class="sidebar-item py-3.5" onclick={handleDelete}>
          <span class="sidebar-icon iconify uil--trash-alt"></span>
          Delete
        </button>
      {/if}
    </nav>
  {/if}
</div>
