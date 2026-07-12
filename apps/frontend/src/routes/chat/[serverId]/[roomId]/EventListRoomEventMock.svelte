<script lang="ts">
  import type { RoomEventView } from '$lib/render/types';

  let { event }: { event: RoomEventView } = $props();
</script>

<div data-event-id={event.id}>{event.id}</div>
