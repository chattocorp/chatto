<!-- @component
Renders the open message action overlay of one timeline: the desktop context menu,
the touch action sheet, the emoji picker, or the reaction details.
`MessageActionOverlayHost` binds it to the message that owns the overlay.
-->
<script lang="ts">
  import { untrack } from 'svelte';
  import { ContextMenu, LoadingFog, LoadRetry } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import { getRecentEmojis } from '$lib/state/recentEmojis.svelte';
  import type { ReactionSummaryView } from '@chatto/client/timeline/reactions';
  import type { MessageActionModel } from './messageActionModel';
  import type {
    MessageActionOverlay,
    MessageActionOverlayState
  } from './messageActionOverlayState.svelte';

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
    overlays,
    action,
    reactions,
    roomId,
    messageEventId
  }: {
    overlays: MessageActionOverlayState;
    /** The actions of the message that owns the open overlay. */
    action: MessageActionModel;
    reactions: ReactionSummaryView[];
    roomId: string;
    messageEventId: string;
  } = $props();

  const overlay = $derived(overlays.overlay);
  // The owner keys this component by message ID. Closing unmounts this component, and
  // its props then stop resolving, so handlers that run after a close use this copy.
  const eventId = untrack(() => messageEventId);

  function close(kind: MessageActionOverlay['kind']): void {
    overlays.close({ kind, eventId });
  }

  $effect(() => {
    if (reactions.length === 0) close('reactions');
  });

  // The menu closes itself after it opens another overlay. The kind-scoped close keeps it.
  function openEmojiPicker(presentation: 'menu' | 'sheet'): void {
    overlays.open(eventId, {
      kind: 'emoji',
      position: overlay?.kind === 'menu' ? overlay.position : { x: 0, y: 0 },
      presentation: presentation === 'sheet' ? 'sheet' : 'auto'
    });
  }

  async function handleEmojiSelect(emoji: string): Promise<void> {
    const { serverId, toggleReaction } = action;
    getRecentEmojis(serverId).recordReaction(emoji);
    close('emoji');
    await toggleReaction(emoji);
  }
</script>

{#snippet loadError(onretry: () => void)}
  <LoadRetry {onretry} />
{/snippet}

{#snippet actionMenu(presentation: 'menu' | 'sheet')}
  {#await loadMessageActionMenu(messageActionMenuLoadAttempt)}
    <LoadingFog class="m-2 h-28 w-64 max-w-full" />
  {:then { default: MessageActionMenu }}
    <MessageActionMenu
      presentation={presentation === 'sheet' ? 'sheet' : undefined}
      {action}
      hasReactions={reactions.length > 0}
      onOpenReactionDetails={() => overlays.open(eventId, { kind: 'reactions' })}
      linkUrl={presentation === 'menu' ? overlays.linkUrl : null}
      imageUrl={presentation === 'menu' ? overlays.imageUrl : null}
      onOpenEmojiPicker={action.canReact ? () => openEmojiPicker(presentation) : undefined}
      onClose={() => close(presentation)}
    />
  {:catch}
    {@render loadError(() => (messageActionMenuLoadAttempt += 1))}
  {/await}
{/snippet}

{#if overlay?.kind === 'menu'}
  <ContextMenu position={overlay.position} class="min-w-72" onclose={() => close('menu')}>
    {@render actionMenu('menu')}
  </ContextMenu>
{:else if overlay?.kind === 'sheet'}
  <ContextMenu
    presentation="sheet"
    role="dialog"
    ariaLabel={m('room.message.actions.toolbar')}
    onclose={() => close('sheet')}
  >
    {@render actionMenu('sheet')}
  </ContextMenu>
{:else if overlay?.kind === 'emoji'}
  <ContextMenu
    position={overlay.position}
    presentation={overlay.presentation}
    role="dialog"
    ariaLabel={m('room.message.actions.add_reaction')}
    scrollDismissal="user"
    onclose={() => close('emoji')}
  >
    {#await loadEmojiPicker(emojiPickerLoadAttempt)}
      <LoadingFog class="m-2 h-28 w-64 max-w-full" />
    {:then { default: EmojiPicker }}
      <EmojiPicker
        serverId={action.serverId}
        onSelect={handleEmojiSelect}
        onClose={() => close('emoji')}
      />
    {:catch}
      {@render loadError(() => (emojiPickerLoadAttempt += 1))}
    {/await}
  </ContextMenu>
{:else if overlay?.kind === 'reactions' && reactions.length > 0}
  {#await import('./MessageReactionDetails.svelte')}
    <LoadingFog class="m-2 h-28 w-64 max-w-full" />
  {:then { default: MessageReactionDetails }}
    <MessageReactionDetails
      {roomId}
      {messageEventId}
      {reactions}
      onClose={() => close('reactions')}
    />
  {:catch}
    {@render loadError(() => close('reactions'))}
  {/await}
{/if}
