<script lang="ts">
  import type { TimelineEventView } from '$lib/render/timelineEvents';
  let { event }: { event: TimelineEventView } = $props();
</script>

<div data-event-id={event.id}>{event.id}</div>
