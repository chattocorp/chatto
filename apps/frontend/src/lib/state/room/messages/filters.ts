import {
  isMessagePostedEvent,
  timelineEventKind,
  TimelineEventKind,
  type TimelineEventView
} from '$lib/render/timelineEvents';

export function isRootRoomEvent(event: TimelineEventView): boolean {
  const eventData = event.event;
  if (!eventData) return false;
  if (isMessagePostedEvent(eventData)) {
    // Echoes are root-level; thread replies (threadRootEventId set) are not.
    return !!eventData.echoOfEventId || !eventData.threadRootEventId;
  }
  switch (timelineEventKind(eventData)) {
    case TimelineEventKind.UserJoinedRoom:
    case TimelineEventKind.UserLeftRoom:
    case TimelineEventKind.RoomUpdated:
    case TimelineEventKind.RoomDeleted:
    case TimelineEventKind.RoomArchived:
    case TimelineEventKind.RoomUnarchived:
    case TimelineEventKind.RoomCreated:
    case TimelineEventKind.RoomThreadingModeChanged:
    case TimelineEventKind.CallStarted:
    case TimelineEventKind.CallEnded:
      return true;
    default:
      return false;
  }
}

export function isThreadEvent(
  event: TimelineEventView,
  roomId: string,
  threadRootEventId: string
): boolean {
  const eventData = event.event;
  if (!eventData || !('roomId' in eventData) || eventData.roomId !== roomId) return false;
  // Thread view only shows messages, not system events.
  if (!isMessagePostedEvent(eventData)) return false;
  if (event.id === threadRootEventId) return true;
  return eventData.threadRootEventId === threadRootEventId;
}
