<!-- @component Exercises the native message action sheet with a mounted composer. -->
<script lang="ts">
  import type { TimelineEventView } from '@chatto/client/timeline/timelineEvents';
  import { createComposerContext } from '$lib/state/room/composerContext.svelte';
  import MessageActionOverlays from '../../../routes/chat/[serverId]/[roomId]/MessageActionOverlays.svelte';
  import { MessageActionOverlayState } from '../../../routes/chat/[serverId]/[roomId]/messageActionOverlayState.svelte';
  import MessageComposer from './MessageComposer.svelte';

  let {
    event,
    threadRootEventId = null,
    quote = null
  }: {
    event: TimelineEventView;
    threadRootEventId?: string | null;
    quote?: string | null;
  } = $props();
  createComposerContext();
  const overlays = new MessageActionOverlayState();
</script>

<button onclick={() => overlays.open(event.id, { kind: 'sheet' }, { replyQuote: quote })}>
  Open message actions
</button>
{#if overlays.current}
  <MessageActionOverlays
    {overlays}
    {event}
    roomId="reply-focus-room"
    permalinkThreadRootEventId={threadRootEventId}
  />
{/if}
<MessageComposer roomId="reply-focus-room" inThread={threadRootEventId ?? undefined} />
