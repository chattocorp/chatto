import { untrack } from 'svelte';

/**
 * Keeps a scalar draft when a remote field changes. Create it during component
 * initialization so its subscription ends with the editor. The owner must reset
 * the editor when the resource changes or a save succeeds.
 */
export class DraftField<T extends string | number | boolean> {
  /** Current input value, including unsaved edits. */
  value = $state<T>() as T;
  /** Latest remote value used to build a sparse update. */
  original = $state<T>() as T;

  constructor(read: () => T) {
    this.value = this.original = untrack(read);
    $effect(() => {
      const next = read();
      untrack(() => {
        const pristine = this.value === this.original;
        this.original = next;
        if (pristine) this.value = next;
      });
    });
  }
}
