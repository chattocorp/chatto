<script lang="ts">
  import type { TimelineEventView } from '@chatto/client/timeline/timelineEvents';
  let { event }: { event: TimelineEventView } = $props();
</script>

<div data-event-id={event.id}>{event.id}</div>
