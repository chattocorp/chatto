import { useFragment } from '$lib/gql';
import {
  RoomEventViewFragmentDoc,
  type RoomEventViewFragment
} from '$lib/gql/graphql';
import type { FragmentType } from '$lib/gql/fragment-masking';

export type RawEvent = FragmentType<typeof RoomEventViewFragmentDoc>;

export type EventConnectionPage = {
  events: readonly RawEvent[];
  startCursor?: string | null;
  endCursor?: string | null;
  hasOlder: boolean;
  hasNewer: boolean;
};

export function unmask(raw: readonly RawEvent[]): RoomEventViewFragment[] {
  return raw
    .map((e) => useFragment(RoomEventViewFragmentDoc, e))
    .filter((e): e is RoomEventViewFragment => e !== null);
}

export function getActorId(actor: RoomEventViewFragment['actor']): string | undefined {
  return actor ? (actor as { id?: string }).id : undefined;
}
