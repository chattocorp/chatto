/** Device-local server display order, shared with other same-origin tabs. */
import { Codecs, globalSlot } from '@chatto/client/storage/slot';
import { untrack } from 'svelte';

const orderSlot = globalSlot(
  'serverGutterOrder',
  [] as string[],
  Codecs.json<string[]>(
    (value): value is string[] =>
      Array.isArray(value) && value.every((id) => typeof id === 'string')
  )
);

/**
 * Keeps committed order separate from a component's drag preview. Remote
 * changes update memory only; the last successful storage write wins.
 */
export class ServerGutterOrder {
  #ids = $state.raw<string[]>(orderSlot.get());
  /** Retain local moves across focus changes if persistence failed. */
  #storageWriteFailed = false;

  /** Known IDs in saved order, followed by new IDs in catalogue order. */
  ordered(knownIds: readonly string[]): string[] {
    const ids = [...this.#ids.filter((id) => knownIds.includes(id)), ...knownIds];
    return ids.filter((id, index) => ids.indexOf(id) === index);
  }

  /** Commit a complete remote-server order and notify other tabs through storage. */
  save(ids: readonly string[]): void {
    this.#ids = ids.filter((id, index) => ids.indexOf(id) === index);
    orderSlot.set(this.#ids);
    const stored = orderSlot.get();
    this.#storageWriteFailed =
      stored.length !== this.#ids.length || stored.some((id, index) => id !== this.#ids[index]);
  }

  /** Remove a forgotten server so adding it again places it after the retained servers. */
  forget(id: string): void {
    if (this.#ids.includes(id)) this.save(this.#ids.filter((knownId) => knownId !== id));
  }

  /** Move one known server by one position; list boundaries are no-ops. */
  move(id: string, direction: -1 | 1, knownIds: readonly string[]): void {
    const ids = this.ordered(knownIds);
    const index = ids.indexOf(id);
    const target = index + direction;
    if (index < 0 || target < 0 || target >= ids.length) return;
    [ids[index], ids[target]] = [ids[target], ids[index]];
    this.save(ids);
  }

  /**
   * Watch while the gutter is mounted. Re-read storage to avoid applying an
   * older queued event after a newer write. Resume also catches missed events.
   * Returns the cleanup function; receiving a change never writes it back.
   */
  listen(): () => void {
    const refresh = () => untrack(() => this.#refresh());
    const storage = (event: StorageEvent) => {
      if (
        event.storageArea === window.localStorage &&
        (event.key === orderSlot.key || event.key === null)
      ) {
        this.#storageWriteFailed = false;
        refresh();
      }
    };
    window.addEventListener('storage', storage);
    window.addEventListener('pageshow', refresh);
    window.addEventListener('focus', refresh);
    refresh();
    return () => {
      window.removeEventListener('storage', storage);
      window.removeEventListener('pageshow', refresh);
      window.removeEventListener('focus', refresh);
    };
  }

  #refresh(): void {
    if (this.#storageWriteFailed) return;
    // A blocked storage area must not clear the usable in-memory order.
    try {
      window.localStorage.getItem(orderSlot.key);
    } catch {
      return;
    }
    const ids = orderSlot.get();
    if (ids.length !== this.#ids.length || ids.some((id, index) => id !== this.#ids[index])) {
      this.#ids = ids;
    }
  }
}

/** The frontend's committed server order. Sessions and registry order are separate. */
export const serverGutterOrder = new ServerGutterOrder();
