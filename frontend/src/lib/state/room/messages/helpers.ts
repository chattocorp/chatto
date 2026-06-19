import type { RoomEventViewFragment } from '$lib/chatTypes';

export type RawEvent = RoomEventViewFragment;

export type EventConnectionPage = {
  events: readonly RawEvent[];
  startCursor?: string | null;
  endCursor?: string | null;
  hasOlder: boolean;
  hasNewer: boolean;
};

export function unmask(raw: readonly RawEvent[]): RoomEventViewFragment[] {
  return raw.filter(isRoomEventView);
}

function isRoomEventView(value: RawEvent): value is RoomEventViewFragment {
  return Boolean(
    value &&
    typeof value === 'object' &&
    '__typename' in value &&
    value.__typename === 'Event' &&
    'event' in value
  );
}

export function getActorId(actor: RoomEventViewFragment['actor']): string | undefined {
  return actor ? (actor as { id?: string }).id : undefined;
}

export function threadRepliesConnection(
  root:
    | {
        event?: {
          __typename?: string;
          threadReplies?: EventConnectionPage;
        } | null;
      }
    | null
    | undefined
): EventConnectionPage | null {
  if (root?.event?.__typename !== 'MessagePostedEvent') return null;
  return root.event.threadReplies ?? null;
}
