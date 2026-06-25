<script lang="ts">
  import { useMessageActions, type MessageActionParams } from '$lib/hooks';
  import { getRecentEmojis } from '$lib/state/recentEmojis.svelte';
  import { getEmojiByName } from '$lib/emoji';
  import * as m from '$lib/i18n/messages';

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
    onReplyInRoom?: () => void;
    onReply?: () => void;
    onOpenEmojiPicker?: () => void;
    onClose: () => void;
  } = $props();

  const quickReactions = $derived(getRecentEmojis(serverId).quickReactions);

  const actions = useMessageActions();

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

<div class="flex flex-col gap-2">
  <!-- Quick reactions row -->
  {#if canReact}
    <div class="flex justify-between menu-section px-2 py-1.5">
      {#each quickReactions as emoji (emoji)}
        <button
          class="flex h-10 w-10 cursor-pointer items-center justify-center rounded-full text-xl active:bg-surface-100"
          onclick={() => handleReaction(emoji)}
          aria-label={m['room.message.actions.react_with']({ emoji })}
        >
          {emoji}
        </button>
      {/each}
      {#if onOpenEmojiPicker}
        <button
          class="flex h-10 w-10 cursor-pointer items-center justify-center rounded-full text-xl text-muted active:bg-surface-100"
          onclick={() => {
            onOpenEmojiPicker();
            onClose();
          }}
          aria-label={m['room.message.actions.more_reactions']()}
        >
          <span class="iconify uil--smile"></span>
        </button>
      {/if}
    </div>
  {/if}

  <nav class="sidebar-nav gap-0 menu-section p-1">
    {#if onReplyInRoom}
      <button class="sidebar-item min-h-11 gap-3 px-3 py-2.5 text-base" onclick={handleReplyInRoom}>
        <span class="sidebar-icon iconify uil--corner-up-left"></span>
        {m['room.message.actions.reply']()}
      </button>
    {/if}

    {#if onReply}
      <button class="sidebar-item min-h-11 gap-3 px-3 py-2.5 text-base" onclick={handleReply}>
        <span class="sidebar-icon iconify uil--comment-alt-lines"></span>
        {m['room.message.actions.reply_thread']()}
      </button>
    {/if}

    {#if canEdit}
      <button class="sidebar-item min-h-11 gap-3 px-3 py-2.5 text-base" onclick={handleEdit}>
        <span class="sidebar-icon iconify uil--pen"></span>
        {m['room.message.actions.edit_short']()}
      </button>
    {/if}

    <button class="sidebar-item min-h-11 gap-3 px-3 py-2.5 text-base" onclick={handleCopyLink}>
      <span class="sidebar-icon iconify uil--copy"></span>
      {m['room.message.actions.copy_link']()}
    </button>

    {#if canDelete}
      <button
        class="sidebar-item min-h-11 gap-3 px-3 py-2.5 text-base text-danger hover:text-danger"
        onclick={handleDelete}
      >
        <span class="sidebar-icon iconify uil--trash-alt"></span>
        {m['common.delete']()}
      </button>
    {/if}
  </nav>
</div>
