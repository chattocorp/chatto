/** A client cache that stays current while a screen retains it. */
type RetainedResource = { retain(): () => void };

export type RoomResourcesSelection = {
  files: RetainedResource | null;
  pins: RetainedResource | null;
  notice: RetainedResource | null;
};

/**
 * Own the client cache leases of one mounted room view. A selection change
 * releases only replaced resources, before acquiring their replacements.
 * Unchanged caches stay retained across panel and permission updates.
 */
export class RoomResources {
  #leases = new Map<
    keyof RoomResourcesSelection,
    { resource: RetainedResource; release(): void }
  >();

  update(selection: RoomResourcesSelection, membersToLoad: { ensureLoaded(): void } | null): void {
    for (const [key, lease] of this.#leases) {
      if (selection[key] === lease.resource) continue;
      this.#leases.delete(key);
      lease.release();
    }
    for (const key of ['files', 'pins', 'notice'] as const) {
      const resource = selection[key];
      if (resource && !this.#leases.has(key)) {
        this.#leases.set(key, { resource, release: resource.retain() });
      }
    }
    membersToLoad?.ensureLoaded();
  }

  /** Release all leases when the route leaves the room view. Idempotent. */
  dispose(): void {
    for (const lease of this.#leases.values()) lease.release();
    this.#leases.clear();
  }
}
