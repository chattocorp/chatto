import { SvelteMap } from 'svelte/reactivity';

/**
 * Lightweight per-instance cache of room names keyed by `${spaceId}:${roomId}`.
 *
 * Populated by `RoomList` after the sidebar's room query resolves, then read by
 * `Room.svelte` so the header can show `# <name>` immediately when the user
 * navigates from the sidebar — no waiting for the room metadata query, no
 * skeleton flash for data we already have. Falls back to a header skeleton on
 * a cold load (direct URL or refresh) where the name isn't cached yet.
 */
export class RoomNamesStore {
  #names = new SvelteMap<string, string>();

  #key(spaceId: string, roomId: string): string {
    return `${spaceId}:${roomId}`;
  }

  set(spaceId: string, roomId: string, name: string): void {
    this.#names.set(this.#key(spaceId, roomId), name);
  }

  get(spaceId: string, roomId: string): string | undefined {
    return this.#names.get(this.#key(spaceId, roomId));
  }
}
