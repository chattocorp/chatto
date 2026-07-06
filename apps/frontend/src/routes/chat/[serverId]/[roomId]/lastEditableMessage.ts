import type { RoomEventView } from '$lib/render/types';
import { isMessagePostedEvent } from '$lib/render/eventKinds';
import type { EditableMessage, RoomPermissions } from '$lib/state/room';

type FindLastEditableMessageOptions = {
  events: RoomEventView[];
  currentUserId: string | null | undefined;
  roomPermissions: RoomPermissions;
  messageEditWindowSeconds: number;
  nowMs: number;
};

export function findLastEditableMessage({
  events,
  currentUserId,
  roomPermissions,
  messageEditWindowSeconds,
  nowMs
}: FindLastEditableMessageOptions): EditableMessage | null {
  if (!currentUserId) return null;

  const editWindowMs = messageEditWindowSeconds * 1000;

  for (let i = events.length - 1; i >= 0; i--) {
    const event = events[i];
    const message = event.event;
    if (event.actorId !== currentUserId) continue;
    if (!isMessagePostedEvent(message)) continue;
    if (message.body == null) continue;
    if (nowMs - new Date(event.createdAt).getTime() >= editWindowMs) continue;

    const isEcho = !!message.echoOfEventId;
    const eventId = isEcho ? message.echoOfEventId! : event.id;
    const threadRootEventId = isEcho
      ? (message.echoFromThreadRootEventId ?? null)
      : (message.threadRootEventId ?? null);
    const channelEchoEventId = isEcho ? event.id : (message.channelEchoEventId ?? null);
    const canAddChannelEcho =
      !!threadRootEventId &&
      (!!channelEchoEventId || (roomPermissions.canEchoMessage && roomPermissions.canPostMessage));

    return {
      eventId,
      body: message.body,
      threadRootEventId,
      channelEchoEventId,
      canAddChannelEcho
    };
  }

  return null;
}
