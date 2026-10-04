<script lang="ts">
  import type { TimelineEventView } from '@chatto/client/timeline/timelineEvents';
  import { getTimelineRowPins } from './timelineRowPins';

  let { event }: { event: TimelineEventView } = $props();

  // Stands in for a row overlay: a click opens or closes it.
  const pins = getTimelineRowPins();
  let release: (() => void) | undefined;
</script>

<button
  type="button"
  data-event-id={event.id}
  onclick={() => {
    if (release) {
      release();
      release = undefined;
    } else release = pins?.pin(event.id);
  }}
>
  {event.id}
</button>
