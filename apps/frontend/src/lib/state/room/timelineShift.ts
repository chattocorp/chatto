/** Classify each store update before its virtualized view reads the new rows. */
import { computed, signal, subscribe, type Signal } from '@chatto/client/reactivity';
import type { MessagesStore } from '@chatto/client/room/messages/MessagesStore';

const shifts = new WeakMap<MessagesStore, Signal<boolean>>();

/**
 * Virtua shifts from the end only for head insertion/removal. A pending older
 * page is not enough: live appends during the read must keep start anchoring.
 * One store-owned subscription records only window boundaries, not messages.
 */
export function timelineShiftsFromEnd(store: MessagesStore): boolean {
  let shift = shifts.get(store);
  if (!shift) {
    shift = signal(false);
    shifts.set(store, shift);
    const current = computed(() => {
      const rows = store.isDisposed ? [] : store.events;
      return { count: rows.length, first: rows[0]?.id, last: rows.at(-1)?.id };
    });
    let previous = current.peek();
    const state = shift;
    const stop = subscribe(current, () => {
      const next = current.peek();
      state.set(
        previous.first !== undefined &&
          next.first !== undefined &&
          next.count !== previous.count &&
          next.first !== previous.first &&
          next.last === previous.last
      );
      previous = next;
      if (store.isDisposed) stop();
    });
  }
  return shift.get();
}
