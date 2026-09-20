import type { MessageResource } from '$lib/api-client/messageResources';

type Change = { insert: boolean };
type RoomQueue = {
  pending: Map<string, Change>;
  active: Map<string, Change>;
  cursor?: string;
  completion: Promise<void>;
};

/**
 * Shares message reads across all views of a server. Each room has one serial
 * queue. Cursors follow arrival order; opaque cursor strings are never sorted.
 * Invalidating a room or the server also fences every outstanding response.
 */
export class MessageReconciler {
  private readonly rooms = new Map<string, RoomQueue>();

  constructor(
    private readonly read: (
      roomId: string, ids: string[], cursor?: string
    ) => Promise<MessageResource[]>,
    private readonly capture: (roomId: string, cursor?: string) => (
      id: string, resource: MessageResource | null, insert: boolean
    ) => void
  ) {}

  /** Schedule a change; callers must await completion before saving its cursor. */
  enqueue(roomId: string, id: string, insert: boolean, cursor?: string): Promise<void> {
    let queue = this.rooms.get(roomId);
    if (!queue) {
      queue = { pending: new Map(), active: new Map(), completion: Promise.resolve() };
      this.rooms.set(roomId, queue);
      const owned = queue;
      // A short bounded delay combines adjacent WebSocket frames, not just one task.
      queue.completion = new Promise<void>((resolve) => setTimeout(resolve, 10))
        .then(() => this.drain(roomId, owned))
        .finally(() => {
          if (this.rooms.get(roomId) === owned) this.rooms.delete(roomId);
        });
    }
    const previous = queue.pending.get(id) ?? queue.active.get(id);
    queue.pending.set(id, { insert: insert || previous?.insert === true });
    if (cursor) queue.cursor = cursor;
    return queue.completion;
  }

  invalidateRoom(roomId: string): void {
    this.rooms.delete(roomId);
  }

  reset(): void {
    this.rooms.clear();
  }

  private async drain(roomId: string, queue: RoomQueue): Promise<void> {
    const current = () => this.rooms.get(roomId) === queue;
    const visited = new Set<string>();
    while (current() && queue.pending.size > 0) {
      queue.active = new Map([...queue.pending].slice(0, 100));
      for (const id of queue.active.keys()) queue.pending.delete(id);
      const apply = this.capture(roomId, queue.cursor);
      let resources: MessageResource[];
      try {
        resources = await this.read(roomId, [...queue.active.keys()], queue.cursor);
      } catch (error) {
        if (!current()) return;
        throw error;
      }
      if (!current()) return;
      for (const id of queue.active.keys()) visited.add(id);
      const byId = new Map(resources.map((resource) => [resource.message.id, resource]));
      for (const [id, change] of queue.active) {
        // A newer event owns this ID. Its queued read must win over this result.
        if (queue.pending.has(id)) continue;
        const resource = byId.get(id) ?? null;
        if (resource) {
          // Closed threads and off-window echoes can reveal related rows only
          // after the first read. Resolve each dependency once per drain.
          const message = resource.message;
          for (const related of [message.threadRootEventId, message.echoOfEventId, message.channelEchoEventId]) {
            if (related && !visited.has(related) && !queue.pending.has(related)) {
              queue.pending.set(related, { insert: false });
            }
          }
        }
        apply(id, resource, change.insert);
      }
      queue.active.clear();
    }
  }
}
