import { m } from './messages';

type ItemContext = { itemLabel: string; position: number; count: number };

/**
 * Localized screen reader announcements for svelte-dnd-action, for its
 * `setAriaStrings`. The announcements read the messages when they run, so they
 * follow the current locale. The instructions are rendered when they are set.
 *
 * The strings assume one list per drag type and the default keyboard drag
 * trigger (Space or Enter). The moves between lists keep the English defaults.
 */
export function dragAndDropAriaStrings() {
  return {
    dragStarted: ({ itemLabel }: ItemContext) =>
      m('ui.drag_and_drop.drag_started', { item: itemLabel }),
    movedToPosition: ({ itemLabel, position, count }: ItemContext) =>
      m('ui.drag_and_drop.moved', { item: itemLabel, position, count }),
    dropped: ({ itemLabel }: ItemContext) => m('ui.drag_and_drop.dropped', { item: itemLabel }),
    zoneActiveInstruction: () => m('ui.drag_and_drop.zone_active'),
    zoneDragDisabledInstruction: m('ui.drag_and_drop.zone_disabled')
  };
}
