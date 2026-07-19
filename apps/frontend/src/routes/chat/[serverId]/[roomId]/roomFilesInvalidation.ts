import type { RealtimeProjectionEvent } from '@chatto/api-types/realtime/v1/realtime_pb';

/** Whether an authoritative projection event can change the current room file list. */
export function projectionEventInvalidatesRoomFiles(
  event: RealtimeProjectionEvent,
  roomId: string
): boolean {
  if (event.operations.some(({ operation }) => operation.case === 'threadViewerStatesReplace'))
    return false;

  // The server orders the source message before derived root/echo updates.
  const update = event.operations.flatMap(({ operation }) => {
    if (operation.case !== 'roomTimelineEventUpsert') return [];
    const timelineEvent = operation.value.event;
    if (operation.value.roomId !== roomId || timelineEvent?.event.case !== 'messagePosted') return [];
    return [
      {
        eventId: timelineEvent.id,
        hasAttachments: (timelineEvent.event.value.message?.attachments.length ?? 0) > 0,
        isReaction: !!operation.value.reactionChange
      }
    ];
  })[0];

  return !!update && !update.isReaction && (update.eventId !== event.id || update.hasAttachments);
}
