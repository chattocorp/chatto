<script lang="ts">
  import type { TimelineEventView } from '@chatto/client/timeline/timelineEvents';
  import type { MessageActionOverlayState } from './messageActionOverlayState.svelte';

  let {
    event,
    actionOverlays
  }: { event: TimelineEventView; actionOverlays: MessageActionOverlayState } = $props();
</script>

<!-- Stands in for a message row: a click requests the touch action sheet. -->
<button
  type="button"
  data-event-id={event.id}
  onclick={() => actionOverlays.open(event.id, { kind: 'sheet' })}
>
  {event.id}
</button>
