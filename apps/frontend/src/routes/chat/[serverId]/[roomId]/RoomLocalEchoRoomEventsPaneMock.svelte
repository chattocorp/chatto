<script lang="ts">
  import { getComposerContext, type MessagesStore } from '$lib/state/room';

  let {
    roomId,
    messageStore,
    pendingHighlightId = null,
    onHighlightComplete
  }: {
    roomId: string;
    messageStore: MessagesStore;
    pendingHighlightId?: string | null;
    onHighlightComplete?: () => void;
  } = $props();

  const jumpState = getComposerContext().jumpState;
  jumpState.setJumpHandler((eventId: string) => messageStore.jumpToMessage(eventId, jumpState));

  $effect(() => {
    messageStore.setRoom(roomId);
  });

  const eventIds = $derived(messageStore.rootEvents.map((event) => event.id).join(','));
</script>

<output data-testid="room-event-ids">{eventIds}</output>
<output data-testid="pending-highlight-id">{pendingHighlightId ?? ''}</output>
<button
  type="button"
  data-testid="complete-highlight"
  onclick={() => {
    jumpState.scrollToEventId = null;
    onHighlightComplete?.();
  }}
>
  complete highlight
</button>
