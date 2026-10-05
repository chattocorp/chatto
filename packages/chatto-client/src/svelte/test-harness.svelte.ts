/** Test helper that records a value from a Svelte effect until stopped. */

import { flushSync } from 'svelte';

export function observe<T>(
  read: () => T,
  options: { derived?: boolean } = {}
): { values: T[]; stop: () => void } {
  const values: T[] = [];
  const stop = $effect.root(() => {
    if (options.derived) {
      const value = $derived(read());
      $effect(() => {
        values.push(value);
      });
    } else {
      $effect(() => {
        values.push(read());
      });
    }
  });
  // Run the initial effects synchronously.
  flushSync();
  return { values, stop };
}
