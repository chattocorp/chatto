<script lang="ts">
  import type { TimelineEventView } from '$lib/render/timelineEvents';
  import type { MessagesStore } from '$lib/state/room';
  import type { TimelineReadPosition } from './readThroughTracker';

  let {
    messageStore,
    events = [],
    unreadAfterEventId = null,
    showStartMarker = true,
    emptyMessage = '',
    pendingHighlightId = null,
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
    onScrollToEventComplete?: (landed: boolean) => void;
    onReadPosition?: (position: TimelineReadPosition) => void;
  } = $props();

  const isLoading = $derived(messageStore?.isInitialLoading ?? false);
</script>

<output data-testid="event-list-start-marker">{showStartMarker}</output>
{#if !isLoading}
  <output data-testid="event-list-empty-message">{emptyMessage}</output>
{/if}
<output data-testid="event-list-unread-after">{unreadAfterEventId ?? ''}</output>
<output data-testid="room-event-ids">{events.map((event) => event.id).join(',')}</output>
<output data-testid="pending-highlight-id">{pendingHighlightId ?? ''}</output>
<button type="button" data-testid="event-list-reached-bottom" onclick={onReachedBottom}>
  reached bottom
</button>
<button
  type="button"
  data-testid="complete-highlight"
  onclick={() => onScrollToEventComplete?.(true)}
>
  complete highlight
</button>
<button type="button" data-testid="fail-highlight" onclick={() => onScrollToEventComplete?.(false)}>
  fail highlight
</button>
<button
  type="button"
  data-testid="report-history-position"
  onclick={() => onReadPosition?.({ eventId: 'history-event', createdAtMs: 1000, latest: false })}
>
  report history position
</button>
