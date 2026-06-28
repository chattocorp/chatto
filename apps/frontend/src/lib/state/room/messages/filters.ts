import { isMessagePostedEvent, roomEventKind, RoomEventKind } from '$lib/render/eventKinds';
import type { RoomEventView } from '$lib/render/types';

export function isRootRoomEvent(event: RoomEventView): boolean {
  const eventData = event.event;
  if (!eventData) return false;
  if (isMessagePostedEvent(eventData)) {
    // Echoes are root-level; thread replies (threadRootEventId set) are not.
    return !!eventData.echoOfEventId || !eventData.threadRootEventId;
  }
  switch (roomEventKind(eventData)) {
    case RoomEventKind.MessageEdited:
    case RoomEventKind.MessageRetracted:
    case RoomEventKind.UserJoinedRoom:
    case RoomEventKind.UserLeftRoom:
    case RoomEventKind.RoomUpdated:
    case RoomEventKind.RoomDeleted:
    case RoomEventKind.RoomArchived:
    case RoomEventKind.RoomUnarchived:
      return true;
    default:
      return false;
  }
}

export function isThreadEvent(
  event: RoomEventView,
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
