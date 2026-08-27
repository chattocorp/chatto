import { createThreadAPI } from '$lib/api-client/threads';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';

type Target = { roomId: string; threadRootEventId: string };
export type ThreadFollowSnapshot = Target & { following: boolean | null };

type Optimistic = { rollback(): void };
type ThreadFollowOptions = {
  getConnection: () => ServerConnection;
  getSnapshot: () => ThreadFollowSnapshot;
  beginOptimistic?: (target: Target, following: boolean) => Optimistic | undefined;
  commit?: (target: Target, following: boolean) => void;
};

export class ThreadFollowState {
  following = $state(false);
  pending = $state(false);

  #snapshot: ThreadFollowSnapshot | null = null;
  #request = { id: 0, optimistic: undefined as Optimistic | undefined };
  #options: ThreadFollowOptions;

  constructor(options: ThreadFollowOptions) {
    this.#options = options;
    $effect(() => this.#observe(options.getSnapshot()));
  }

  toggle = async (): Promise<void> => {
    const snapshot = this.#snapshot;
    if (!snapshot || this.pending) return;
    const { following: _, ...target } = snapshot;
    const wasFollowing = this.following;
    const nextFollowing = !wasFollowing;
    const requestId = ++this.#request.id;
    this.#request.optimistic = this.#options.beginOptimistic?.(target, nextFollowing);
    this.pending = true;
    this.following = nextFollowing;
    try {
      const api = this.#options.getConnection().getAPI(createThreadAPI);
      const result = await (wasFollowing ? api.unfollowThread : api.followThread)(target);
      if (requestId !== this.#request.id) return;
      this.#request.optimistic = undefined;
      this.pending = false;
      this.following = result.following;
      this.#options.commit?.(target, result.following);
    } catch {
      if (requestId !== this.#request.id) return;
      this.pending = false;
      this.following = wasFollowing;
      this.#request.optimistic?.rollback();
      this.#request.optimistic = undefined;
    }
  };

  #observe(snapshot: ThreadFollowSnapshot): void {
    const current = this.#snapshot;
    const authoritative = snapshot.following;
    const changed =
      !current ||
      snapshot.roomId !== current.roomId ||
      snapshot.threadRootEventId !== current.threadRootEventId;
    if (changed) {
      this.#request.id += 1;
      this.pending = false;
      this.#request.optimistic?.rollback();
      this.#request.optimistic = undefined;
      this.#snapshot = snapshot;
      this.following = snapshot.following ?? false;
    } else if (!this.pending && authoritative !== null && authoritative !== current.following) {
      this.#snapshot = snapshot;
      this.following = authoritative;
    }
  }
}
