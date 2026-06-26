export type RoomMessageMutationReason =
  | 'message-deleted'
  | 'attachment-deleted'
  | 'link-preview-deleted';

export type RoomMessageMutatedDetail = {
  roomId: string;
  eventId: string;
  reason: RoomMessageMutationReason;
};

export const ROOM_MESSAGE_MUTATED_EVENT = 'chatto:room-message-mutated';

export function notifyRoomMessageMutated(detail: RoomMessageMutatedDetail): void {
  if (typeof window === 'undefined') return;
  window.dispatchEvent(new CustomEvent(ROOM_MESSAGE_MUTATED_EVENT, { detail }));
}

export function onRoomMessageMutated(
  handler: (detail: RoomMessageMutatedDetail) => void
): () => void {
  if (typeof window === 'undefined') return () => {};

  const listener = (event: Event) => {
    const detail = (event as CustomEvent<RoomMessageMutatedDetail>).detail;
    if (!detail?.roomId || !detail.eventId) return;
    handler(detail);
  };

  window.addEventListener(ROOM_MESSAGE_MUTATED_EVENT, listener);
  return () => window.removeEventListener(ROOM_MESSAGE_MUTATED_EVENT, listener);
}
