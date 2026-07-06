import type { RoomEventView } from '$lib/render/types';
import { isMessagePostedEvent } from '$lib/render/eventKinds';

export function roomReplyTargetEventId(event: RoomEventView): string {
  const message = isMessagePostedEvent(event.event) ? event.event : null;
  return message?.echoOfEventId ?? event.id;
}
