import { ReplyState } from '$lib/state/room/composerContext.svelte';

/** Reply targets for one server session. Message content stays in memory. */
export class DraftReplies {
  readonly #drafts = new Map<string, { roomId: string; reply: ReplyState }>();

  /** Select a room or thread draft. Keep a reply selected before the composer mounted. */
  select(key: string, roomId: string, initial?: ReplyState): ReplyState {
    const saved = this.#drafts.get(key);
    if (saved) return saved.reply;
    const reply = initial ?? new ReplyState();
    this.#drafts.set(key, { roomId, reply });
    return reply;
  }

  /** Clear a sent draft, including any mounted reference to its reply target. */
  clearDraft(key: string): void {
    this.#drafts.get(key)?.reply.cancelReply();
  }

  /** Remove cached names and excerpts when a reply target is retracted. */
  clearMessage(messageEventId: string): void {
    for (const draft of this.#drafts.values()) {
      if (draft.reply.messageEventId === messageEventId) draft.reply.cancelReply();
    }
  }

  /** Remove reply metadata when the viewer loses room access. */
  clearRoom(roomId: string): void {
    for (const draft of this.#drafts.values()) {
      if (draft.roomId !== roomId) continue;
      draft.reply.cancelReply();
    }
  }

  /** Clear active and retained reply targets at a session privacy boundary. */
  clear(): void {
    for (const draft of this.#drafts.values()) draft.reply.cancelReply();
    // Keep empty state objects so mounted composers still write to the registry.
  }
}
