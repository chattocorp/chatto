<script lang="ts">
  import type { TimelineEventView } from '@chatto/client/timeline/timelineEvents';
  import type { MessagesStore } from '$lib/state/room';
  import { getComposerContext } from '$lib/state/room';
  import type { TimelineReadPosition } from './readThroughTracker';
  import type { PendingHighlight } from '$lib/state/server/pendingHighlight';

  let {
    messageStore,
    events = [],
    unreadAfterEventId = null,
    showStartMarker = true,
    emptyMessage = '',
    pendingHighlightId = null,
    highlightRequest = null,
    onJumpToPresent,
    onReachedBottom,
    onScrollToEventComplete,
    onReadPosition
  }: {
    messageStore?: Pick<MessagesStore, 'isInitialLoading'>;
    events?: TimelineEventView[];
    unreadAfterEventId?: string | null;
    showStartMarker?: boolean;
    emptyMessage?: string;
    pendingHighlightId?: string | null;
    onReachedBottom?: () => void;
    onScrollToEventComplete?: (
      landed: boolean,
      eventId: string,
      request: PendingHighlight | null
    ) => void;
    highlightRequest?: PendingHighlight | null;
    onJumpToPresent?: () => void;
    onReadPosition?: (position: TimelineReadPosition) => void;
  } = $props();

  const isLoading = $derived(messageStore?.isInitialLoading ?? false);
  const { jumpState } = getComposerContext();
</script>

<output data-testid="event-list-start-marker">{showStartMarker}</output>
{#if !isLoading}
  <output data-testid="event-list-empty-message">{emptyMessage}</output>
{/if}
<output data-testid="event-list-unread-after">{unreadAfterEventId ?? ''}</output>
<output data-testid="room-event-ids">{events.map((event) => event.id).join(',')}</output>
<output data-testid="pending-highlight-id">{pendingHighlightId ?? ''}</output>
<output data-testid="scroll-target-id">{jumpState.scrollToEventId ?? ''}</output>
<button
  type="button"
  data-testid="request-present"
  onclick={() => {
    onJumpToPresent?.();
    jumpState.reset();
  }}
>
  jump to present
</button>
<button
  type="button"
  data-testid="reply-quote-jump"
  onclick={() => jumpState.jumpToMessage('msg-quoted')}
>
  jump to quoted message
</button>
<button
  type="button"
  data-testid="complete-scroll"
  disabled={!jumpState.scrollToEventId}
  onclick={() => onScrollToEventComplete?.(true, jumpState.scrollToEventId ?? '', highlightRequest)}
>
  complete scroll
</button>
<button type="button" data-testid="event-list-reached-bottom" onclick={onReachedBottom}>
  reached bottom
</button>
<button
  type="button"
  data-testid="complete-highlight"
  disabled={jumpState.scrollToEventId !== pendingHighlightId}
  onclick={() => onScrollToEventComplete?.(true, pendingHighlightId ?? '', highlightRequest)}
>
  complete highlight
</button>
<button
  type="button"
  data-testid="fail-highlight"
  disabled={jumpState.scrollToEventId !== pendingHighlightId}
  onclick={() => onScrollToEventComplete?.(false, pendingHighlightId ?? '', highlightRequest)}
>
  fail highlight
</button>
<button
  type="button"
  data-testid="report-history-position"
  onclick={() => onReadPosition?.({ eventId: 'history-event', createdAtMs: 1000, latest: false })}
>
  report history position
</button>
