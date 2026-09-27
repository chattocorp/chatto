import { SvelteMap } from 'svelte/reactivity';
import { Codecs, StorageSlot } from '$lib/storage/slot';

/**
 * Reactive cache of collapsed sidebar sections, keyed by their persist key.
 *
 * Every surface that renders a collapsible sidebar section reads through this
 * cache, so a `RoomGroupSection` and a virtualized list that renders
 * `RoomGroupSectionHeader` directly stay in sync for the same key. Values are
 * also saved to browser storage as a UI preference.
 */
const collapsedByKey = new SvelteMap<string, boolean>();

/** Returns the collapsed state for `key`, or `fallback` when nothing is stored. */
export function loadCollapsed(key: string, fallback: boolean): boolean {
  const cached = collapsedByKey.get(key);
  if (cached !== undefined) return cached;
  return new StorageSlot(key, fallback, Codecs.boolean).get();
}

/** Stores the collapsed state for `key` in memory and in browser storage. */
export function saveCollapsed(key: string, value: boolean): void {
  collapsedByKey.set(key, value);
  new StorageSlot(key, value, Codecs.boolean).set(value);
}

/** Clears the in-memory cache so each test starts from browser storage. */
export function resetRoomGroupCollapseForTests(): void {
  collapsedByKey.clear();
}
