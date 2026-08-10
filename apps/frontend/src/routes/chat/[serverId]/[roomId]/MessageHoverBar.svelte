<!--
@component

Quick actions toolbar that appears on hover at the upper inline-end of a message.
Shows quick reaction emoji and action icons (reply, edit, more menu) inline.
Hover-capable input only; pure touch devices use the long-press action sheet instead.

Receives the same bound message-action model as the context menu and touch
sheet, plus toolbar-only controls for opening those surfaces.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { getRecentEmojis } from '$lib/state/recentEmojis.svelte';
  import type { MessageActionModel } from './messageActionModel';

  let {
    action,
    forceVisible = false,
    onOpenEmojiPicker,
    onOpenMenu
  }: {
    action: MessageActionModel;
    forceVisible?: boolean;
    onOpenEmojiPicker?: (e: MouseEvent) => void;
    onOpenMenu?: (e: MouseEvent) => void;
  } = $props();

  const recentEmojis = $derived(getRecentEmojis(action.serverId));
  const quickReactions = $derived(recentEmojis.quickReactions);

  const hasActions = $derived(
    !!action.replyInRoom || !!action.replyThread || action.canEdit || !!onOpenMenu
  );
</script>

<div
  class={[
    'invisible absolute end-0 bottom-full z-10 mb-[-6px] hidden flex-row gap-0.5 rounded-t-md rounded-b-none border border-b-0 border-border bg-surface p-0.5 hover-actions:flex',
    'hover-actions:group-hover:visible'
  ]}
  class:!visible={forceVisible}
  role="toolbar"
  tabindex="-1"
  aria-label={m('room.message.actions.toolbar')}
  onmousedown={(e) => {
    e.preventDefault();
    e.stopPropagation();
  }}
>
  {#if action.canReact}
    <div class="flex items-center menu-section-sm">
      {#each quickReactions as emoji (emoji)}
        <button
          class="flex h-7 w-7 cursor-pointer items-center justify-center rounded text-base transition-[background-color,scale] hover:bg-surface active:scale-[0.96]"
          onclick={() => action.toggleReaction(emoji)}
          aria-label={action.hasReacted(emoji)
            ? m('room.message.actions.remove_reaction', { emoji })
            : m('room.message.actions.react_with', { emoji })}
        >
          {emoji}
        </button>
      {/each}
      {#if onOpenEmojiPicker}
        <button
          class="flex h-7 w-7 cursor-pointer items-center justify-center rounded text-muted transition-[background-color,color,scale] hover:bg-surface hover:text-text active:scale-[0.96]"
          onclick={onOpenEmojiPicker}
          aria-label={m('room.message.actions.more_reactions')}
        >
          <span class="iconify icon-[uil--smile] text-base"></span>
        </button>
      {/if}
    </div>
  {/if}

  {#if hasActions}
    <div class="flex items-center menu-section-sm">
      {#if action.replyInRoom}
        <button
          class="flex h-7 w-7 cursor-pointer items-center justify-center rounded text-muted transition-[background-color,color,scale] hover:bg-surface hover:text-text active:scale-[0.96]"
          onclick={action.replyInRoom}
          aria-label={action.replyInRoomLabel}
        >
          <span class="iconify icon-[uil--corner-up-left] text-base rtl:-scale-x-100"></span>
        </button>
      {/if}

      {#if action.replyThread}
        <button
          class="flex h-7 w-7 cursor-pointer items-center justify-center rounded text-muted transition-[background-color,color,scale] hover:bg-surface hover:text-text active:scale-[0.96]"
          onclick={action.replyThread}
          aria-label={action.replyThreadLabel}
        >
          <span class="iconify icon-[uil--comment-alt-lines] text-base"></span>
        </button>
      {/if}

      {#if action.canEdit}
        <button
          class="flex h-7 w-7 cursor-pointer items-center justify-center rounded text-muted transition-[background-color,color,scale] hover:bg-surface hover:text-text active:scale-[0.96]"
          onclick={action.edit}
          aria-label={m('room.message.actions.edit')}
        >
          <span class="iconify icon-[uil--pen] text-base"></span>
        </button>
      {/if}

      {#if onOpenMenu}
        <button
          class="flex h-7 w-7 cursor-pointer items-center justify-center rounded text-muted transition-[background-color,color,scale] hover:bg-surface hover:text-text active:scale-[0.96]"
          onclick={onOpenMenu}
          aria-label={m('room.message.actions.more')}
        >
          <span class="iconify icon-[uil--ellipsis-v] text-base"></span>
        </button>
      {/if}
    </div>
  {/if}
</div>
