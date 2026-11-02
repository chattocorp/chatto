/** Preview identities follow loaded message bodies, not replaceable timeline payloads. */
import { computed, subscribe } from '@chatto/client/reactivity';
import type { MessagesStore } from '@chatto/client/room/messages/MessagesStore';
import { isMessagePostedEvent } from '@chatto/client/timeline/timelineEvents';

type Timeline = Pick<MessagesStore, 'events' | 'isDisposed'>;
type Entry = { body: string; owner: object };
const timelines = new WeakMap<Timeline, ReturnType<typeof createOwners>>();

/** One subscription per store removes cached content at its timeline boundaries. */
function createOwners(store: Timeline) {
  const owners = new Map<string, Entry>();
  const bodies = computed(() => {
    const current = new Map<string, string>();
    if (!store.isDisposed) {
      for (const row of store.events) {
        if (isMessagePostedEvent(row.event) && row.event.body && !row.event.deletedAt) {
          current.set(row.id, row.event.body);
        }
      }
    }
    return current;
  });
  const stop = subscribe(bodies, () => {
    const current = bodies.peek();
    for (const [id, entry] of owners) {
      if (current.get(id) !== entry.body) owners.delete(id);
    }
    if (store.isDisposed) stop();
  });
  return {
    get(id: string, body: string) {
      if (bodies.get().get(id) !== body) return undefined;
      let entry = owners.get(id);
      if (entry?.body !== body) {
        entry = { body, owner: {} };
        owners.set(id, entry);
      }
      return entry.owner;
    }
  };
}

/**
 * Return a stable identity for one loaded body. Reactions, pins and thread
 * summaries keep it; edits, removal, reset and disposal discard it. The weak
 * registry cannot keep a store alive. Rows outside the store use local owners.
 */
export function messageArtifactOwner(
  store: Timeline | null,
  eventId: string,
  body: string | null | undefined
): object | undefined {
  if (!store || store.isDisposed || !body) return undefined;
  let owners = timelines.get(store);
  if (!owners) {
    owners = createOwners(store);
    timelines.set(store, owners);
  }
  return owners.get(eventId, body);
}
