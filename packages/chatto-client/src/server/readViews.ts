import { SvelteSet } from 'svelte/reactivity';
import { appState } from '$lib/state/globals.svelte';

/** One visible reading surface. Room views do not cover their threads. */
export type ReadViewTarget = {
  roomId: string;
  threadRootId?: string | null;
};

/** Per-server view registrations; these change local attention, never server read state. */
export class ReadViewRegistry {
  readonly #views = new SvelteSet<ReadViewTarget>();

  /** Register one eligible, visible pane. Cleanup removes only this registration. */
  register(target: ReadViewTarget): () => void {
    const view = { ...target };
    this.#views.add(view);
    return () => {
      this.#views.delete(view);
    };
  }

  /** Whether a focused, visible client is reading this exact target. */
  covers(roomId: string | null | undefined, threadRootId?: string | null): boolean {
    if (!roomId || !appState.isPresent) return false;
    return [...this.#views.values()].some(
      (view) => view.roomId === roomId && (view.threadRootId || null) === (threadRootId || null)
    );
  }

  /** Discard registrations at an authentication boundary or store disposal. */
  clear(): void {
    this.#views.clear();
  }
}
