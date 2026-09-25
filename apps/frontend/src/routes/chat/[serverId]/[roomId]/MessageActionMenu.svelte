<!--
@component

Shared message actions for the desktop context menu and touch action sheet.
The action model and ordering stay identical while `presentation` controls the
surface-specific sizing and menu semantics.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  import { m } from '$lib/i18n/messages';
  import { getRecentEmojis } from '$lib/state/recentEmojis.svelte';
  import MenuItem from '$lib/ui/MenuItem.svelte';
  import MenuSection from '$lib/ui/MenuSection.svelte';
  import { toast } from '$lib/ui/toast';
  import { copyImageToClipboard } from '$lib/attachments/copyImage';
  import type { MessageActionModel } from './messageActionModel';

  let {
    presentation = 'menu',
    action,
    linkUrl = null,
    imageUrl = null,
    onOpenEmojiPicker,
    hasReactions = false,
    onOpenReactionDetails,
    onClose
  }: {
    presentation?: 'menu' | 'sheet';
    action: MessageActionModel;
    /** Resolved URL of the message-body link that opened this context menu. */
    linkUrl?: string | null;
    /** URL of the image attachment that opened this context menu. */
    imageUrl?: string | null;
    onOpenEmojiPicker?: () => void;
    hasReactions?: boolean;
    onOpenReactionDetails?: () => void;
    onClose: () => void;
  } = $props();

  const isSheet = $derived(presentation === 'sheet');
  const recentEmojis = $derived(getRecentEmojis(action.serverId));
  const quickReactions = $derived(recentEmojis.quickReactions);

  async function handleReaction(emoji: string) {
    await action.toggleReaction(emoji);
    onClose();
  }

  function handleReplyInRoom() {
    action.replyInRoom?.();
    onClose();
  }

  function handleOpenReactionDetails() {
    onClose();
    onOpenReactionDetails?.();
  }

  function handleReply() {
    action.replyThread?.();
    onClose();
  }

  function handleSecondaryReplyInRoom() {
    action.secondaryReplyInRoom?.();
    onClose();
  }

  function handleEdit() {
    action.edit();
    onClose();
  }

  async function handleCopyText() {
    await action.copyText();
    onClose();
  }

  async function handleCopyLink() {
    await action.copyLink();
    onClose();
  }

  async function handleCopyTargetLink() {
    if (!linkUrl) return;
    try {
      await navigator.clipboard.writeText(linkUrl);
      toast.success(m('room.message.actions.link_copied'));
    } catch {
      toast.error(m('room.message.actions.copy_link_failed'));
    }
    onClose();
  }

  async function handleCopyImage() {
    if (!imageUrl) return;
    try {
      await copyImageToClipboard(imageUrl);
      toast.success(m('room.message.actions.image_copied'));
    } catch {
      toast.error(m('room.message.actions.copy_image_failed'));
    }
    onClose();
  }

  function handleDelete() {
    action.delete();
    onClose();
  }

  async function handlePin() {
    await action.togglePin();
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
          : 'rounded text-base transition-[background-color,scale] feedback-quick hover:bg-surface active:scale-[0.96]'
      ]}
      onclick={() => handleReaction(emoji)}
      aria-label={m('room.message.actions.react_with', { emoji })}
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
          : 'rounded text-base transition-[background-color,scale] feedback-quick hover:bg-surface active:scale-[0.96]'
      ]}
      onclick={() => {
        onOpenEmojiPicker();
        onClose();
      }}
      aria-label={m('room.message.actions.more_reactions')}
      role={isSheet ? undefined : 'menuitem'}
    >
      <span class={['iconify icon-[uil--smile]', !isSheet && 'text-lg']}></span>
    </button>
  {/if}
{/snippet}

{#snippet actionButton(
  label: string,
  icon: string,
  onclick: () => void | Promise<void>,
  destructive = false,
  mirrorInRtl = false
)}
  <MenuItem
    {icon}
    tone={destructive ? 'danger' : 'default'}
    mirrorIconInRtl={mirrorInRtl}
    {onclick}
  >
    {label}
  </MenuItem>
{/snippet}

{#snippet actionGroup(content: Snippet)}
  <MenuSection>{@render content()}</MenuSection>
{/snippet}

{#snippet menuContent()}
  {#if action.canReact}
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

  {#if hasReactions && onOpenReactionDetails}
    {@render actionGroup(reactionDetailsAction)}
  {/if}

  {#if action.replyInRoom || action.replyThread || action.secondaryReplyInRoom || action.canEdit}
    {@render actionGroup(primaryActions)}
  {/if}

  {@render actionGroup(copyActions)}

  {#if action.canPin}
    {@render actionGroup(pinAction)}
  {/if}

  {#if action.canDelete}
    {@render actionGroup(deleteAction)}
  {/if}
{/snippet}

{#snippet reactionDetailsAction()}
  {@render actionButton(
    m('room.message.actions.reactions'),
    'icon-[uil--smile]',
    handleOpenReactionDetails
  )}
{/snippet}

{#snippet pinAction()}
  {@render actionButton(
    action.isPinned ? m('room.pins.unpin') : m('room.pins.pin'),
    'icon-[mdi--pin]',
    handlePin
  )}
{/snippet}

{#snippet primaryActions()}
  {@render replyInRoomAction()}
  {@render replyThreadAction()}
  {#if action.secondaryReplyInRoom && action.secondaryReplyInRoomLabel}
    {@render actionButton(
      action.secondaryReplyInRoomLabel,
      'icon-[uil--corner-up-left]',
      handleSecondaryReplyInRoom,
      false,
      true
    )}
  {/if}
  {#if action.canEdit}
    {@render actionButton(m('room.message.actions.edit_short'), 'icon-[uil--pen]', handleEdit)}
  {/if}
{/snippet}

{#snippet replyInRoomAction()}
  {#if action.replyInRoom}
    {@render actionButton(
      action.replyInRoomLabel,
      'icon-[uil--corner-up-left]',
      handleReplyInRoom,
      false,
      true
    )}
  {/if}
{/snippet}

{#snippet replyThreadAction()}
  {#if action.replyThread}
    {@render actionButton(action.replyThreadLabel, 'icon-[uil--comment-alt-lines]', handleReply)}
  {/if}
{/snippet}

{#snippet copyActions()}
  {#if action.messageBody}
    {@render actionButton(
      m('room.message.actions.copy_text'),
      'icon-[uil--clipboard-notes]',
      handleCopyText
    )}
  {/if}
  {#if !isSheet && linkUrl}
    {@render actionButton(
      m('room.message.actions.copy_link'),
      'icon-[uil--link]',
      handleCopyTargetLink
    )}
  {/if}
  {#if !isSheet && imageUrl}
    {@render actionButton(
      m('room.message.actions.copy_image'),
      'icon-[uil--image]',
      handleCopyImage
    )}
  {/if}
  {@render actionButton(
    m('room.message.actions.copy_message_link'),
    'icon-[uil--link]',
    handleCopyLink
  )}
{/snippet}

{#snippet deleteAction()}
  {@render actionButton(m('common.delete'), 'icon-[uil--trash-alt]', handleDelete, true)}
{/snippet}

{#if isSheet}
  <div class="flex flex-col gap-2">
    {@render menuContent()}
  </div>
{:else}
  {@render menuContent()}
{/if}
