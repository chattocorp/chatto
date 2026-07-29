<script lang="ts">
  import BottomSheet from '$lib/ui/BottomSheet.svelte';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import EmojiPicker from '$lib/components/EmojiPicker.svelte';
  import MessageContextMenu from '$lib/components/menus/MessageContextMenu.svelte';
  import type { ReactionSummaryView } from '$lib/render/reactions';
  import type { MessagesStore } from '$lib/state/room';
  import MessageActionSheet from './MessageActionSheet.svelte';
  import type { MessageEventInteractionState } from './messageEventInteractions.svelte';

  let {
    interactions,
    serverId,
    roomId,
    messageEventId,
    eventId,
    deleteEventId,
    messageBody,
    permalinkThreadRootEventId = null,
    threadRootEventId = null,
    channelEchoEventId = null,
    canAddChannelEcho = false,
    messageStore = null,
    reactions = [],
    canReact = false,
    canEdit = false,
    canDelete = false,
    replyInRoomLabel,
    replyThreadLabel,
    onReplyInRoom,
    onReply,
    onEmojiSelect,
    onClose
  }: {
    interactions: MessageEventInteractionState;
    serverId: string;
    roomId: string;
    messageEventId: string;
    eventId: string;
    deleteEventId: string;
    messageBody: string;
    permalinkThreadRootEventId?: string | null;
    threadRootEventId?: string | null;
    channelEchoEventId?: string | null;
    canAddChannelEcho?: boolean;
    messageStore?: MessagesStore | null;
    reactions?: ReactionSummaryView[];
    canReact?: boolean;
    canEdit?: boolean;
    canDelete?: boolean;
    replyInRoomLabel?: string;
    replyThreadLabel?: string;
    onReplyInRoom?: () => void;
    onReply?: () => void;
    onEmojiSelect: (emoji: string) => void | Promise<void>;
    onClose?: () => void;
  } = $props();

  function closeContextMenu(): void {
    interactions.closeContextMenu();
    onClose?.();
  }

  function closeEmojiPicker(): void {
    interactions.closeEmojiPicker();
    onClose?.();
  }

  function closeActionSheet(): void {
    interactions.closeActionSheet();
    onClose?.();
  }

  function openSheetEmojiPicker(): void {
    interactions.openEmojiPicker('sheet');
  }

  async function handleEmojiSelect(emoji: string): Promise<void> {
    closeEmojiPicker();
    await onEmojiSelect(emoji);
  }
</script>

{#if interactions.contextMenuPosition}
  <ContextMenu
    position={interactions.contextMenuPosition}
    class="min-w-72"
    onclose={closeContextMenu}
  >
    <MessageContextMenu
      {serverId}
      {roomId}
      {messageEventId}
      {eventId}
      {deleteEventId}
      {messageBody}
      {permalinkThreadRootEventId}
      {threadRootEventId}
      {channelEchoEventId}
      {canAddChannelEcho}
      {messageStore}
      {reactions}
      {canReact}
      {canEdit}
      {canDelete}
      {replyInRoomLabel}
      {replyThreadLabel}
      {onReplyInRoom}
      {onReply}
      onOpenEmojiPicker={canReact ? () => interactions.openEmojiPicker() : undefined}
      onClose={closeContextMenu}
    />
  </ContextMenu>
{/if}

{#if interactions.emojiPickerPosition}
  <ContextMenu
    position={interactions.emojiPickerPosition}
    presentation={interactions.emojiPickerPresentation}
    scrollDismissal="user"
    onclose={closeEmojiPicker}
  >
    <EmojiPicker {serverId} onSelect={handleEmojiSelect} onClose={closeEmojiPicker} />
  </ContextMenu>
{/if}

{#if interactions.showActionSheet}
  <BottomSheet bind:visible={interactions.showActionSheet} onclose={closeActionSheet}>
    <MessageActionSheet
      {serverId}
      {roomId}
      {messageEventId}
      {eventId}
      {deleteEventId}
      {messageBody}
      {permalinkThreadRootEventId}
      {threadRootEventId}
      {channelEchoEventId}
      {canAddChannelEcho}
      {messageStore}
      {reactions}
      {canReact}
      {canEdit}
      {canDelete}
      {replyInRoomLabel}
      {replyThreadLabel}
      {onReplyInRoom}
      {onReply}
      onOpenEmojiPicker={canReact ? openSheetEmojiPicker : undefined}
      onClose={closeActionSheet}
    />
  </BottomSheet>
{/if}
