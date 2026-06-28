<script lang="ts">
  import type { RoomEventView } from '$lib/render/types';
  import type { MessagesStore, QuoteInsertionContent } from '$lib/state/room';
  import { isMessagePostedEvent } from '$lib/render/eventKinds';
  import MessageEvent from './MessageEvent.svelte';
  import SystemEvent from './SystemEvent.svelte';

  let {
    event,
    compact = false,
    roomId,
    messageStore = null,
    onOpenThread
  }: {
    event: RoomEventView;
    compact?: boolean;
    roomId: string;
    messageStore?: MessagesStore | null;
    onOpenThread?: (
      threadRootEventId: string,
      highlightEventId?: string,
      quoteText?: QuoteInsertionContent
    ) => void;
  } = $props();

  // Join/leave events are confusing in DM 1:1 conversations. Post-PR(b) we
  // can no longer derive "is this a DM room" from a spaceId — the backend
  // routes both kinds through the same surface. We always render join/leave
  // for now; a future iteration can teach Room.svelte to pass `isDM` down
  // and we can revive the suppression here.
  const isDMJoinLeave = $derived(false);
</script>

{#if !event?.event || isDMJoinLeave}
  <!-- Skip unknown event types, stale virtualizer items, and join/leave events in DM rooms -->
{:else if isMessagePostedEvent(event.event)}
  <MessageEvent {event} {compact} {roomId} {messageStore} {onOpenThread} />
{:else}
  <SystemEvent {event} />
{/if}
