import { SvelteMap } from 'svelte/reactivity';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';

/**
 * Observed presence of the users of one server.
 *
 * The server store writes it from the realtime stream of that server, whether
 * or not the server is on screen. A user without an entry has no known
 * presence; readers then use the presence of the profile they already have.
 */
export class ServerPresence {
  #statuses = new SvelteMap<string, PresenceStatus>();
  /** The version of the latest change for each user, for the preview fence. */
  #changedAt = new SvelteMap<string, number>();

  #version = $state(0);

  /** Increases with every change. Read it to react to any presence change. */
  get version(): number {
    return this.#version;
  }

  /** The known presence of a user, or undefined when it is not known. */
  get(userId: string): PresenceStatus | undefined {
    return this.#statuses.get(userId);
  }

  /** Apply a presence change or a fresh profile value. An unspecified status changes nothing. */
  set(userId: string, status: PresenceStatus): void {
    if (status === PresenceStatus.UNSPECIFIED) return;
    this.#version++;
    this.#statuses.set(userId, status);
    this.#changedAt.set(userId, this.#version);
  }

  /**
   * Apply the presence of a user resource. A complete replacement first
   * forgets every user; a partial one only updates the listed users.
   */
  applySnapshot(statuses: Iterable<readonly [string, PresenceStatus]>, replace: boolean): void {
    if (replace) this.clear();
    for (const [userId, status] of statuses) this.set(userId, status);
  }

  /**
   * Apply a status from a presence-filtered read that started at `readVersion`.
   * A change that arrived while the read was in flight takes precedence.
   */
  applyPreview(userId: string, status: PresenceStatus, readVersion: number): void {
    if ((this.#changedAt.get(userId) ?? 0) > readVersion) return;
    this.set(userId, status);
  }

  /** Forget all presence, for example at a projection reset. */
  clear(): void {
    this.#statuses.clear();
    this.#changedAt.clear();
    this.#version++;
  }
}
