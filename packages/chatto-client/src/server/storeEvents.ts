/**
 * Boundary events of a server store.
 *
 * Hosts that keep their own copies of server data, such as a UI's search
 * results or query cache, subscribe to these events and clear their copies.
 * State that derives from the store needs no subscription: it follows the
 * store. The store emits each event synchronously, inside the write that
 * crossed the boundary, after it updated its own state.
 */

import type { RealtimeProjectionUpdate } from '../realtime/eventBus.js';

/** A projection reset; see `ServerStateStore.onReset`. */
export interface ProjectionReset {
  /**
   * The reset also removed the viewer's authority, for example at sign-in,
   * sign-out, or an account change. Remove all private data of the server.
   */
  readonly privacy: boolean;
  /**
   * The current view stays readable while the store checks a replacement
   * snapshot, for example after a reconnect. Keep displayed data; the store
   * reports each change that the snapshot causes.
   */
  readonly retainView: boolean;
}

/** The viewer lost access to a room; see `ServerStateStore.onRoomAccessLost`. */
export interface RoomAccessLoss {
  readonly roomId: string;
  /**
   * Only the permission to read messages changed. The viewer can still be a
   * member and join the room's call. Remove message content of the room.
   */
  readonly messagesOnly: boolean;
  /**
   * The room left the projection, for example because it was deleted or is
   * no longer visible. Remove all state of the room.
   */
  readonly removed: boolean;
}

/** A change of the viewer's server authority; see `ServerStateStore.onAuthorityChanged`. */
export interface AuthorityChange {
  /**
   * The viewer lost a grant or changed account. Remove data that the old
   * authority allowed. When false, only read such data again.
   */
  readonly lost: boolean;
}

/** One store event: an ordered set of listeners. */
export class StoreEvent<Args extends unknown[], Result = void> {
  readonly #listeners = new Set<(...args: Args) => Result>();

  /** Add a listener. Returns a function that removes it. */
  subscribe(listener: (...args: Args) => Result): () => void {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  }

  /**
   * Call every listener in order. A throwing listener is logged and does not
   * stop the others. Returns the listeners' results and whether all of them
   * returned without an error.
   */
  emit(...args: Args): { results: Result[]; complete: boolean } {
    const results: Result[] = [];
    let complete = true;
    for (const listener of [...this.#listeners]) {
      try {
        results.push(listener(...args));
      } catch (error) {
        complete = false;
        console.error('[chatto-client] a server store listener failed', error);
      }
    }
    return { results, complete };
  }

  /** Remove every listener. */
  clear(): void {
    this.#listeners.clear();
  }
}

/** The events of one server store. */
export class StoreEvents {
  readonly reset = new StoreEvent<[ProjectionReset]>();
  readonly roomAccessLost = new StoreEvent<[RoomAccessLoss]>();
  readonly roomAccessRestored = new StoreEvent<[roomId: string]>();
  readonly userDeleted = new StoreEvent<[userId: string]>();
  readonly authorityChanged = new StoreEvent<[AuthorityChange]>();
  readonly permissionsChanged = new StoreEvent<[], void | Promise<unknown>>();
  readonly update = new StoreEvent<[RealtimeProjectionUpdate]>();
  readonly sessionEnded = new StoreEvent<[]>();
  readonly dispose = new StoreEvent<[]>();

  /** Remove every listener of every event. */
  clear(): void {
    for (const event of [
      this.reset,
      this.roomAccessLost,
      this.roomAccessRestored,
      this.userDeleted,
      this.authorityChanged,
      this.permissionsChanged,
      this.update,
      this.sessionEnded,
      this.dispose
    ]) {
      event.clear();
    }
  }
}
