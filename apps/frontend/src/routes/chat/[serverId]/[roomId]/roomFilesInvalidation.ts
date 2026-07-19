import type { RealtimeProjectionEvent } from '@chatto/api-types/realtime/v1/realtime_pb';

/** Whether an authoritative projection event can change the current room file list. */
export function projectionEventInvalidatesRoomFiles(
  event: RealtimeProjectionEvent,
  roomId: string,
  hasFilesForMessage: (messageEventId: string) => boolean
): boolean {
  const upserts = event.operations.flatMap((operation) => {
    if (operation.operation.case !== 'roomTimelineEventUpsert') return [];
    const upsert = operation.operation.value;
    if (upsert.roomId !== roomId || upsert.event?.event.case !== 'messagePosted') return [];
    return [{ upsert, message: upsert.event.event.value.message }];
  });
  const hasPrimaryMessageUpsert = upserts.some(({ upsert }) => upsert.event?.id === event.id);
  const replacesThreadViewerState = event.operations.some(
    (operation) => operation.operation.case === 'threadViewerStatesReplace'
  );

  return upserts.some(({ upsert, message }) => {
    if (upsert.reactionChange || replacesThreadViewerState) return false;
    const messageEventId = upsert.event?.id ?? '';
    if (hasPrimaryMessageUpsert && messageEventId !== event.id) return false;

    return (
      (message?.attachments.length ?? 0) > 0 ||
      !!message?.deletedAt ||
      hasFilesForMessage(messageEventId) ||
      (!hasPrimaryMessageUpsert && messageEventId !== event.id)
    );
  });
}
