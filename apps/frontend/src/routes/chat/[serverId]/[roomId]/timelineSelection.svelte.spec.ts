import { afterEach, describe, expect, it } from 'vitest';
import {
  keptIndexes,
  selectionEndpointKeys,
  TIMELINE_ITEM_KEY_ATTRIBUTE
} from './timelineSelection';
import type { VirtualItem } from './virtualItems';

const items: VirtualItem[] = ['a', 'b', 'c', 'd', 'e'].map((key) => ({
  type: 'unread-separator',
  key
}));

/** Renders a header, a list with the given mounted item keys, and a footer. */
function renderTimeline(mountedKeys: string[]) {
  const header = document.createElement('header');
  header.textContent = 'header';
  const container = document.createElement('div');
  for (const key of mountedKeys) {
    const item = document.createElement('div');
    item.setAttribute(TIMELINE_ITEM_KEY_ATTRIBUTE, key);
    item.innerHTML = `<p>message ${key}</p>`;
    container.append(item);
  }
  const footer = document.createElement('footer');
  footer.textContent = 'footer';
  document.body.append(header, container, footer);
  return { header, container, footer };
}

function textOf(container: Element, key: string) {
  return container.querySelector(`[${TIMELINE_ITEM_KEY_ATTRIBUTE}="${key}"] p`)!.firstChild!;
}

function select(anchor: Node, anchorOffset: number, focus: Node, focusOffset: number) {
  const selection = window.getSelection()!;
  selection.setBaseAndExtent(anchor, anchorOffset, focus, focusOffset);
  return selection;
}

describe('selectionEndpointKeys', () => {
  afterEach(() => {
    window.getSelection()?.removeAllRanges();
    document.body.replaceChildren();
  });

  it('returns the item keys at both ends of a selection', () => {
    const { container } = renderTimeline(['b', 'c', 'd']);
    const selection = select(textOf(container, 'd'), 3, textOf(container, 'b'), 1);

    expect(selectionEndpointKeys(selection, container)).toEqual({
      anchor: 'd',
      focus: 'b'
    });
  });

  it('keeps a collapsed caret', () => {
    const { container } = renderTimeline(['b', 'c']);
    const selection = select(textOf(container, 'c'), 2, textOf(container, 'c'), 2);

    expect(selectionEndpointKeys(selection, container)).toEqual({
      anchor: 'c',
      focus: 'c'
    });
  });

  it('ignores a selection with an end outside the list', () => {
    const { header, container, footer } = renderTimeline(['c']);

    expect(
      selectionEndpointKeys(select(textOf(container, 'c'), 0, header, 0), container)
    ).toBeNull();
    expect(selectionEndpointKeys(select(header, 0, footer, 0), container)).toBeNull();
  });

  it('ignores an end inside the list but outside an item', () => {
    const { container } = renderTimeline(['b', 'c']);

    expect(
      selectionEndpointKeys(select(textOf(container, 'b'), 0, container, 1), container)
    ).toBeNull();
  });

  it('ignores an empty selection', () => {
    const { container } = renderTimeline(['c']);
    window.getSelection()?.removeAllRanges();

    expect(selectionEndpointKeys(window.getSelection(), container)).toBeNull();
  });
});

describe('keptIndexes', () => {
  it('returns every index between the keys in either direction', () => {
    expect(keptIndexes(items, { anchor: 'b', focus: 'd' })).toEqual([1, 2, 3]);
    expect(keptIndexes(items, { anchor: 'd', focus: 'b' })).toEqual([1, 2, 3]);
    expect(keptIndexes(items, { anchor: 'c', focus: 'c' })).toEqual([2]);
  });

  it('returns nothing when a key is no longer in the items', () => {
    expect(keptIndexes(items, { anchor: 'b', focus: 'gone' })).toEqual([]);
  });
});
