<script lang="ts">
  import { onMount } from 'svelte';
  import {
    TimelineEventKind,
    type TimelineEventView
  } from '@chatto/client/timeline/timelineEvents';

  let {
    canPost = false,
    onReady,
    onMessageSent
  }: {
    canPost?: boolean;
    onReady?: (api: { focus: () => void; addFiles: () => void }) => void;
    onMessageSent?: (event: TimelineEventView) => void;
  } = $props();

  onMount(() => onReady?.({ focus: () => {}, addFiles: () => {} }));
</script>

<button
  data-testid="thread-composer-send"
  disabled={!canPost}
  onclick={() =>
    onMessageSent?.({
      id: 'sent-reply',
      createdAt: '2026-07-04T12:00:00Z',
      actorId: 'test-user',
      actor: null,
      event: {
        kind: TimelineEventKind.MessagePosted,
        roomId: 'room-1',
        threadRootEventId: 'thread-root'
      }
    } as TimelineEventView)}>Send</button
>
