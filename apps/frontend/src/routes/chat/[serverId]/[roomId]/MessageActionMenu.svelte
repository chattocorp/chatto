<!--
@component

Shared message actions for the desktop context menu and touch action sheet.
The action model and ordering stay identical while `presentation` controls the
surface-specific sizing and menu semantics.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  import { useMessageActions, type MessageActionParams } from '$lib/hooks';
  import * as m from '$lib/i18n/messages';
  import type { MessagesStore } from '$lib/state/room';
  import { getRecentEmojis } from '$lib/state/recentEmojis.svelte';
  import { getEmojiByName } from '$lib/emoji';

  let {
    presentation = 'menu',
    serverId,
    roomId,
    messageEventId,
    eventId,
    deleteEventId = eventId,
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
    onOpenEmojiPicker,
    onClose
  }: {
    presentation?: 'menu' | 'sheet';
    serverId: string;
    roomId: string;
    messageEventId: string;
    eventId: string;
    deleteEventId?: string;
    messageBody: string;
    permalinkThreadRootEventId?: string | null;
    threadRootEventId?: string | null;
    channelEchoEventId?: string | null;
    canAddChannelEcho?: boolean;
    messageStore?: MessagesStore | null;
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

  const isSheet = $derived(presentation === 'sheet');
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
    permalinkThreadRootEventId,
    threadRootEventId,
    channelEchoEventId,
    canAddChannelEcho,
    messageStore
  });

  /** Set of Unicode emojis the current user has already reacted with (API returns shortcodes). */
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

  async function handleCopyText() {
    await actions.copyMessageText(params);
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

{#snippet reactionButtons()}
  {#each quickReactions as emoji (emoji)}
    <button
      class={[
        'flex h-10 w-10 cursor-pointer items-center justify-center',
        isSheet
          ? 'rounded-full text-xl active:bg-surface'
          : 'rounded text-base transition-[background-color,scale] hover:bg-surface active:scale-[0.96]'
      ]}
      onclick={() => handleReaction(emoji)}
      aria-label={m['room.message.actions.react_with']({ emoji })}
      role={isSheet ? undefined : 'menuitem'}
    >
      {emoji}
    </button>
  {/each}
  {#if onOpenEmojiPicker}
    <button
      class={[
        'flex h-10 w-10 cursor-pointer items-center justify-center text-muted',
        isSheet
          ? 'rounded-full text-xl active:bg-surface'
          : 'rounded text-base transition-[background-color,scale] hover:bg-surface active:scale-[0.96]'
      ]}
      onclick={() => {
        onOpenEmojiPicker();
        onClose();
      }}
      aria-label={m['room.message.actions.more_reactions']()}
      role={isSheet ? undefined : 'menuitem'}
    >
      <span class={['iconify uil--smile', !isSheet && 'text-lg']}></span>
    </button>
  {/if}
{/snippet}

{#snippet actionButton(
  label: string,
  icon: string,
  onclick: () => void | Promise<void>,
  destructive = false
)}
  <button
    class={[
      'sidebar-item',
      isSheet && 'min-h-11 gap-3 px-3 py-2.5 text-base',
      destructive && 'text-danger hover:text-danger'
    ]}
    {onclick}
    role={isSheet ? undefined : 'menuitem'}
  >
    <span class={['sidebar-icon iconify', icon]}></span>
    {label}
  </button>
{/snippet}

{#snippet actionGroup(content: Snippet)}
  <div class={isSheet ? undefined : 'menu-section'}>
    <nav class={['sidebar-nav', isSheet && 'gap-0 menu-section p-1']}>
      {@render content()}
    </nav>
  </div>
{/snippet}

{#snippet menuContent()}
  {#if canReact}
    {#if isSheet}
      <div class="flex justify-between menu-section px-2 py-1.5">
        {@render reactionButtons()}
      </div>
    {:else}
      <div class="menu-section">
        <div class="flex justify-between">
          {@render reactionButtons()}
        </div>
      </div>
    {/if}
  {/if}

  {#if onReplyInRoom || onReply || canEdit}
    {@render actionGroup(primaryActions)}
  {/if}

  {@render actionGroup(copyActions)}

  {#if canDelete}
    {@render actionGroup(deleteAction)}
  {/if}
{/snippet}

{#snippet primaryActions()}
  {#if onReplyInRoom}
    {@render
      actionButton(replyInRoomActionLabel, 'uil--corner-up-left', handleReplyInRoom)}
  {/if}
  {#if onReply}
    {@render actionButton(replyThreadActionLabel, 'uil--comment-alt-lines', handleReply)}
  {/if}
  {#if canEdit}
    {@render actionButton(m['room.message.actions.edit_short'](), 'uil--pen', handleEdit)}
  {/if}
{/snippet}

{#snippet copyActions()}
  {#if messageBody}
    {@render
      actionButton(m['room.message.actions.copy_text'](), 'uil--clipboard-notes', handleCopyText)}
  {/if}
  {@render actionButton(m['room.message.actions.copy_link'](), 'uil--link', handleCopyLink)}
{/snippet}

{#snippet deleteAction()}
  {@render actionButton(m['common.delete'](), 'uil--trash-alt', handleDelete, true)}
{/snippet}

{#if isSheet}
  <div class="flex flex-col gap-2">
    {@render menuContent()}
  </div>
{:else}
  {@render menuContent()}
{/if}
