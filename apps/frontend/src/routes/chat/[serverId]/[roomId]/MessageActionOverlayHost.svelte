<!-- @component
Binds the open message action overlay of a timeline to the live message that owns it.

The timeline renders this host outside its virtualized rows, so an overlay stays open
when the virtualizer unmounts the row of its message. The owner renders the host only
while `event` is in the loaded timeline, and keys it by the message ID.
-->
<script lang="ts">
  import type { MessagesStore } from '$lib/state/room';
  import type { TimelineEventView } from '@chatto/client/timeline/timelineEvents';
  import { RoomThreadingMode } from '@chatto/client/util/roomThreading';
  import MessageActionOverlays from './MessageActionOverlays.svelte';
  import type { MessageActionOverlayState } from './messageActionOverlayState.svelte';
  import { MessageActionTarget } from './messageActionTarget.svelte';
  import type { OpenThreadHandler } from './threadOpenOptions';

  let {
    overlays,
    event,
    roomId,
    permalinkThreadRootEventId = null,
    messageStore = null,
    onOpenThread,
    threadingMode = RoomThreadingMode.ENABLED
  }: {
    overlays: MessageActionOverlayState;
    /** The live message that owns the open overlay. */
    event: TimelineEventView;
    roomId: string;
    permalinkThreadRootEventId?: string | null;
    messageStore?: MessagesStore | null;
    onOpenThread?: OpenThreadHandler;
    threadingMode?: RoomThreadingMode;
  } = $props();

  const target = new MessageActionTarget(() => ({
    event,
    roomId,
    permalinkThreadRootEventId,
    messageStore,
    onOpenThread,
    threadingMode,
    takeReplyQuote: () => overlays.takeReplyQuote()
  }));
</script>

<MessageActionOverlays
  {overlays}
  action={target.action}
  reactions={target.messageEvent?.reactions ?? []}
  {roomId}
  messageEventId={event.id}
/>
