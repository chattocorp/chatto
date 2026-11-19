/** Console-local notifications let output readers resume their cursors after journal
 * changes or reconnects. Notifications contain no output and do not retain history. */
import { createContext } from 'svelte';

export class OutputUpdates {
  private listeners = new Set<(runId: string) => void>();
  notify(runId: string): void {
    for (const listener of this.listeners) listener(runId);
  }
  subscribe(listener: (runId: string) => void): () => void {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  }
}

export const [useOutputUpdates, provideOutputUpdates] = createContext<OutputUpdates>();
