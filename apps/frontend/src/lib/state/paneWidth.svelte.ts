/**
 * Reactive state for one resizable pane or sidebar width.
 *
 * The class mirrors the persisted slot in memory so UI keeps working when
 * browser storage is unavailable (SSR, privacy mode): `set()` always updates
 * the reactive value with the clamped width even if persistence was skipped.
 * Clamping itself lives in the storage layer (`$lib/storage/paneWidth`);
 * instances exist only to hand one config to this factory.
 *
 * Each surface exports its own singleton to preserve per-surface module
 * identity and imports, e.g. `$lib/state/threadPaneWidth.svelte`.
 */

import type { PaneWidthSlot } from '$lib/storage/paneWidth';

export class PaneWidthState {
  readonly #slot: PaneWidthSlot;
  #width = $state(0);

  constructor(slot: PaneWidthSlot) {
    this.#slot = slot;
    this.#width = slot.get();
  }

  get value(): number {
    return this.#width;
  }

  set(width: number): void {
    this.#width = this.#slot.set(width);
  }

  reset(): void {
    this.set(this.#slot.defaultValue);
  }
}

export function createPaneWidthState(slot: PaneWidthSlot): PaneWidthState {
  return new PaneWidthState(slot);
}
