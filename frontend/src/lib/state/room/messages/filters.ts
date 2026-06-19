import type { RoomEventViewFragment } from '$lib/gql/graphql';

export function isRootRoomEvent(event: RoomEventViewFragment): boolean {
  const eventData = event.event;
  if (!eventData) return false;
  switch (eventData.__typename) {
    case 'MessagePostedEvent':
      // Echoes are root-level; thread replies (threadRootEventId set) are not.
      return !!eventData.echoOfEventId || !eventData.threadRootEventId;
    case 'MessageEditedEvent':
    case 'MessageRetractedEvent':
    case 'UserJoinedRoomEvent':
    case 'UserLeftRoomEvent':
    case 'RoomUpdatedEvent':
    case 'RoomDeletedEvent':
    case 'RoomArchivedEvent':
    case 'RoomUnarchivedEvent':
      return true;
    default:
      return false;
  }
}

export function isThreadEvent(
  event: RoomEventViewFragment,
  roomId: string,
  threadRootEventId: string
): boolean {
  const eventData = event.event;
  if (!eventData || !('roomId' in eventData) || eventData.roomId !== roomId) return false;
  // Thread view only shows messages, not system events.
  if (eventData.__typename !== 'MessagePostedEvent') return false;
  if (event.id === threadRootEventId) return true;
  return eventData.threadRootEventId === threadRootEventId;
}
