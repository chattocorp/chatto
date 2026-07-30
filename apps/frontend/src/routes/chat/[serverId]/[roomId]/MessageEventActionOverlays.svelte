<script lang="ts">
  import BottomSheet from '$lib/ui/BottomSheet.svelte';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import * as m from '$lib/i18n/messages';
  import type { ReactionSummaryView } from '$lib/render/reactions';
  import type { MessagesStore } from '$lib/state/room';
  import type { MessageEventInteractionState } from './messageEventInteractions.svelte';

  let messageActionMenuModule: Promise<typeof import('./MessageActionMenu.svelte')> | null = null;
  let messageActionMenuLoadAttempt = $state(0);
  let emojiPickerModule: Promise<typeof import('$lib/components/EmojiPicker.svelte')> | null = null;
  let emojiPickerLoadAttempt = $state(0);

  function loadMessageActionMenu(_attempt: number) {
    messageActionMenuModule ??= import('./MessageActionMenu.svelte').catch((error: unknown) => {
      messageActionMenuModule = null;
      throw error;
    });
    return messageActionMenuModule;
  }

  function loadEmojiPicker(_attempt: number) {
    emojiPickerModule ??= import('$lib/components/EmojiPicker.svelte').catch((error: unknown) => {
      emojiPickerModule = null;
      throw error;
    });
    return emojiPickerModule;
  }

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

{#snippet loadError(onretry: () => void)}
  <div class="flex flex-col items-center gap-3 p-4 text-center" role="alert">
    <p class="text-sm text-muted">{m['common.error.network']()}</p>
    <button type="button" class="btn-secondary" onclick={onretry}>
      {m['common.retry']()}
    </button>
  </div>
{/snippet}

{#snippet actionMenu(presentation: 'menu' | 'sheet' = 'menu')}
  {#await loadMessageActionMenu(messageActionMenuLoadAttempt)}
    <p class="p-4 text-center text-sm text-muted" aria-busy="true">{m['common.loading']()}</p>
  {:then { default: MessageActionMenu }}
    <MessageActionMenu
      presentation={presentation === 'sheet' ? 'sheet' : undefined}
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
      onOpenEmojiPicker={canReact
        ? presentation === 'sheet'
          ? openSheetEmojiPicker
          : () => interactions.openEmojiPicker()
        : undefined}
      onClose={presentation === 'sheet' ? closeActionSheet : closeContextMenu}
    />
  {:catch}
    {@render loadError(() => (messageActionMenuLoadAttempt += 1))}
  {/await}
{/snippet}

{#if interactions.contextMenuPosition}
  <ContextMenu
    position={interactions.contextMenuPosition}
    class="min-w-72"
    onclose={closeContextMenu}
  >
    {@render actionMenu()}
  </ContextMenu>
{/if}

{#if interactions.emojiPickerPosition}
  <ContextMenu
    position={interactions.emojiPickerPosition}
    presentation={interactions.emojiPickerPresentation}
    scrollDismissal="user"
    onclose={closeEmojiPicker}
  >
    {#await loadEmojiPicker(emojiPickerLoadAttempt)}
      <p class="p-4 text-center text-sm text-muted" aria-busy="true">{m['common.loading']()}</p>
    {:then { default: EmojiPicker }}
      <EmojiPicker {serverId} onSelect={handleEmojiSelect} onClose={closeEmojiPicker} />
    {:catch}
      {@render loadError(() => (emojiPickerLoadAttempt += 1))}
    {/await}
  </ContextMenu>
{/if}

{#if interactions.showActionSheet}
  <BottomSheet bind:visible={interactions.showActionSheet} onclose={closeActionSheet}>
    {@render actionMenu('sheet')}
  </BottomSheet>
{/if}
