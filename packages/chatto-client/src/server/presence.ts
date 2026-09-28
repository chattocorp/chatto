import { ReactiveMap, signal } from '../reactivity/index.js';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';

/**
 * Observed presence of the users of one server.
 *
 * The server store writes it from the realtime stream of that server, whether
 * or not the server is on screen. A user without an entry has no known
 * presence; readers then use the presence of the profile they already have.
 */
export class ServerPresence {
  #statuses = new ReactiveMap<string, PresenceStatus>();
  /** The version of the latest change for each user, for the read fence. */
  #changedAt = new ReactiveMap<string, number>();

  readonly #versionSignal = signal(0);
  get #version() {
    return this.#versionSignal.get();
  }
  set #version(value) {
    this.#versionSignal.set(value);
  }

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
   * forgets every user; a partial one only updates the listed users. Give
   * `readVersion` for a partial resource that this client read itself: a change
   * that arrived while that read was in flight then takes precedence. A
   * complete replacement forgets those changes first, so the fence has no effect.
   */
  applySnapshot(
    statuses: Iterable<readonly [string, PresenceStatus]>,
    replace: boolean,
    readVersion?: number
  ): void {
    if (replace) this.clear();
    for (const [userId, status] of statuses) {
      if (readVersion === undefined) this.set(userId, status);
      else this.applyRead(userId, status, readVersion);
    }
  }

  /**
   * Apply a status from a read that started at `readVersion`. A change that
   * arrived while the read was in flight takes precedence.
   */
  applyRead(userId: string, status: PresenceStatus, readVersion: number): void {
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
