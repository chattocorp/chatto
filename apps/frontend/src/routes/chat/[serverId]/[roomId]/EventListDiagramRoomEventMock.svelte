<!-- @component Real Markdown rendering in timeline regression tests. -->
<script lang="ts">
  import type { TimelineEventView } from '@chatto/client/timeline/timelineEvents';
  import MessageContent from '$lib/components/MessageContent.svelte';
  let { event }: { event: TimelineEventView } = $props();
  const body = $derived(
    '```svg rendered\n<svg xmlns="http://www.w3.org/2000/svg" width="600" height="800"><title>' +
      event.id +
      '</title><rect width="600" height="800" fill="#164e63"/></svg>\n```\n\n```mermaid rendered\nflowchart TB\naccTitle: ' +
      event.id +
      '\nA-->B-->C-->D\n```'
  );
</script>

<div data-event-id={event.id}><MessageContent {body} /></div>
