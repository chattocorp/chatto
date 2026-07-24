import type { RoomEventView } from '$lib/render/types';
export type RawEvent = RoomEventView;

export type EventConnectionPage = {
  events: readonly RawEvent[];
  startCursor?: string | null;
  endCursor?: string | null;
  hasOlder: boolean;
  hasNewer: boolean;
};

export function unmask(raw: readonly RawEvent[]): RoomEventView[] {
  return raw.filter((event): event is RoomEventView => event !== null);
}

export function getActorId(actor: RoomEventView['actor']): string | undefined {
  return actor ? (actor as { id?: string }).id : undefined;
}
