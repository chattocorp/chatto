<script lang="ts">
  import type { TimelineEventView } from '@chatto/client/timeline/timelineEvents';
  import type { MessagesStore, RoomMember } from '$lib/state/room';
  import type { UserAvatarUserView } from '@chatto/client/timeline/users';
  import { isMessagePostedEvent } from '@chatto/client/timeline/timelineEvents';
  import MessageEvent from './MessageEvent.svelte';
  import SystemEvent from './SystemEvent.svelte';
  import type { OpenThreadHandler } from './threadOpenOptions';
  import type { MessageActionOverlayState } from './messageActionOverlayState.svelte';
  import { RoomThreadingMode } from '@chatto/client/util/roomThreading';

  let {
    event,
    compact = false,
    roomId,
    permalinkThreadRootEventId = null,
    messageStore = null,
    actionOverlays,
    onOpenThread,
    activeCallId = null,
    onOpenCall,
    onOpenUser,
    threadingMode = RoomThreadingMode.ENABLED
  }: {
    event: TimelineEventView;
    compact?: boolean;
    roomId: string;
    permalinkThreadRootEventId?: string | null;
    messageStore?: MessagesStore | null;
    actionOverlays: MessageActionOverlayState;
    onOpenThread?: OpenThreadHandler;
    activeCallId?: string | null;
    onOpenCall?: () => void;
    onOpenUser?: (user: UserAvatarUserView | RoomMember, anchorRect: DOMRect | null) => void;
    threadingMode?: RoomThreadingMode;
  } = $props();
</script>

{#if !event?.event}
  <!-- Skip unknown event types and stale virtualizer items -->
{:else if isMessagePostedEvent(event.event)}
  <MessageEvent
    {event}
    {compact}
    {roomId}
    {permalinkThreadRootEventId}
    {messageStore}
    {actionOverlays}
    {onOpenThread}
    {onOpenUser}
    {threadingMode}
  />
{:else}
  <SystemEvent {event} {activeCallId} {onOpenCall} />
{/if}
