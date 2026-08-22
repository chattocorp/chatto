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
  import type { MessageActionModel } from './messageActionModel';

  let {
    presentation = 'menu',
    action,
    onOpenEmojiPicker,
    onClose
  }: {
    presentation?: 'menu' | 'sheet';
    action: MessageActionModel;
    onOpenEmojiPicker?: () => void;
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
          : 'rounded text-base transition-[background-color,scale] hover:bg-surface active:scale-[0.96]'
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
          : 'rounded text-base transition-[background-color,scale] hover:bg-surface active:scale-[0.96]'
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
  <button
    class={[
      'sidebar-item',
      isSheet && 'min-h-11 gap-3 px-3 py-2.5 text-base',
      destructive && 'text-danger hover:text-danger'
    ]}
    {onclick}
    role={isSheet ? undefined : 'menuitem'}
  >
    <span class={['iconify sidebar-icon', icon, mirrorInRtl && 'rtl:-scale-x-100']}></span>
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
  {@render actionButton(m('room.message.actions.copy_link'), 'icon-[uil--link]', handleCopyLink)}
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
