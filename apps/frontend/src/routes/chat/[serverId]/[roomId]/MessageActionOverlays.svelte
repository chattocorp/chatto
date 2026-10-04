<!-- @component
Renders the open message action overlay of one timeline: the desktop context menu,
the touch action sheet, the emoji picker, or the reaction details.

`EventList` renders this component outside its virtualized rows, so an overlay stays open
when the virtualizer unmounts the row of its message. `EventList` renders it only while
`event` is in the loaded timeline, and keys it by the message ID.
-->
<script lang="ts" module>
  // This component remounts for each message whose overlay opens. Keep the lazy modules across
  // remounts.
  let messageActionMenuModule: Promise<typeof import('./MessageActionMenu.svelte')> | null = null;
  let emojiPickerModule: Promise<typeof import('$lib/components/EmojiPicker.svelte')> | null = null;

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
</script>

<script lang="ts">
  import { untrack } from 'svelte';
  import { ContextMenu, LoadingFog, LoadRetry } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import { getRecentEmojis } from '$lib/state/recentEmojis.svelte';
  import type { MessagesStore } from '$lib/state/room';
  import type { TimelineEventView } from '@chatto/client/timeline/timelineEvents';
  import { RoomThreadingMode } from '@chatto/client/util/roomThreading';
  import type {
    MessageActionOverlay,
    MessageActionOverlayState
  } from './messageActionOverlayState.svelte';
  import { MessageActionTarget } from './messageActionTarget.svelte';
  import type { OpenThreadHandler } from './threadOpenOptions';

  let messageActionMenuLoadAttempt = $state(0);
  let emojiPickerLoadAttempt = $state(0);

  let {
    overlays,
    event,
    roomId,
    permalinkThreadRootEventId = null,
    messageStore = null,
    onOpenThread,
    threadingMode = RoomThreadingMode.ENABLED
  }: {
    overlays: MessageActionOverlayState;
    /** The live message that owns the open overlay. */
    event: TimelineEventView;
    roomId: string;
    permalinkThreadRootEventId?: string | null;
    messageStore?: MessagesStore | null;
    onOpenThread?: OpenThreadHandler;
    threadingMode?: RoomThreadingMode;
  } = $props();

  const target = new MessageActionTarget(() => ({
    event,
    roomId,
    permalinkThreadRootEventId,
    messageStore,
    onOpenThread,
    threadingMode,
    takeReplyQuote: () => overlays.takeReplyQuote()
  }));
  const action = $derived(target.action);
  const reactions = $derived(target.messageEvent?.reactions ?? []);
  const overlay = $derived(overlays.current);
  // The owner keys this component by message ID. Closing unmounts this component, and
  // its props then stop resolving, so handlers that run after a close use this copy.
  const eventId = untrack(() => event.id);

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
      linkUrl={presentation === 'menu' ? overlay?.linkUrl : null}
      imageUrl={presentation === 'menu' ? overlay?.imageUrl : null}
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
      messageEventId={eventId}
      {reactions}
      onClose={() => close('reactions')}
    />
  {:catch}
    {@render loadError(() => close('reactions'))}
  {/await}
{/if}
