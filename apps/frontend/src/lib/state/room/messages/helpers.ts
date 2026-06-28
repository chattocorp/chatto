import { useRenderData } from '$lib/render/data';
import {
  RoomEventViewDocument,
  type RoomEventView
} from '$lib/render/types';
import type { RenderType } from '$lib/render/data';

export type RawEvent = RenderType<typeof RoomEventViewDocument>;

export type EventConnectionPage = {
  events: readonly RawEvent[];
  startCursor?: string | null;
  endCursor?: string | null;
  hasOlder: boolean;
  hasNewer: boolean;
};

export function unmask(raw: readonly RawEvent[]): RoomEventView[] {
  return raw
    .map((e) => useRenderData(RoomEventViewDocument, e))
    .filter((e): e is RoomEventView => e !== null);
}

export function getActorId(actor: RoomEventView['actor']): string | undefined {
  return actor ? (actor as { id?: string }).id : undefined;
}
