import {
  isMessagePostedEvent,
  type TimelineEventView
} from '$lib/render/timelineEvents';

export function roomReplyTargetEventId(event: TimelineEventView): string {
  const message = isMessagePostedEvent(event.event) ? event.event : null;
  return message?.echoOfEventId ?? event.id;
}
