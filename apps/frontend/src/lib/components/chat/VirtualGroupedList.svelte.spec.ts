import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { VirtualListGroup } from './groupedListItems';
import { resetRoomGroupCollapseForTests } from './roomGroupCollapse';
import VirtualGroupedListTestHarness from './VirtualGroupedListTestHarness.svelte';

type Row = { id: string };

function rows(prefix: string, count: number): Row[] {
  return Array.from({ length: count }, (_, index) => ({ id: `${prefix}-${index}` }));
}

function group(id: string, items: Row[]): VirtualListGroup<Row> {
  return { id, label: id, items, persistKey: `test:virtual-grouped-list:${id}` };
}

function viewport(container: HTMLElement): HTMLElement {
  return container.querySelector('[data-testid="virtual-viewport"]')!;
}

/** The first row whose top edge is inside the viewport, and its distance from the viewport top. */
function firstVisibleRow(container: HTMLElement): { id: string; top: number } {
  const scroller = viewport(container);
  const viewportTop = scroller.getBoundingClientRect().top;
  const row = Array.from(container.querySelectorAll('[data-testid="virtual-row"]'))
    .map((element) => ({
      id: element.textContent ?? '',
      top: element.getBoundingClientRect().top - viewportTop
    }))
    .filter((entry) => entry.top >= 0)
    .sort((left, right) => left.top - right.top)[0];
  if (!row) throw new Error('No visible row');
  return row;
}

async function scrollTo(container: HTMLElement, offset: number): Promise<void> {
  viewport(container).scrollTop = offset;
  await vi.waitFor(() => {
    expect(firstVisibleRow(container).id).not.toBe('online-0');
  });
}

describe('VirtualGroupedList', () => {
  beforeEach(() => {
    resetRoomGroupCollapseForTests();
    localStorage.clear();
  });

  afterEach(() => {
    localStorage.clear();
  });

  it('keeps the visible rows in place when rows are inserted above them', async () => {
    const online = rows('online', 10);
    const offline = rows('offline', 100);
    const { container, rerender } = render(VirtualGroupedListTestHarness, {
      props: { groups: [group('online', online), group('offline', offline)] }
    });
    await vi.waitFor(() => expect(firstVisibleRow(container).id).toBe('online-0'));

    await scrollTo(container, 2000);
    const before = firstVisibleRow(container);

    await rerender({
      groups: [group('online', [...rows('new', 3), ...online]), group('offline', offline)]
    });

    await vi.waitFor(() => {
      const after = firstVisibleRow(container);
      expect(after.id).toBe(before.id);
      expect(Math.abs(after.top - before.top)).toBeLessThanOrEqual(1);
    });
  });

  it('keeps the visible rows in place when a row below moves into a group above', async () => {
    const online = rows('online', 10);
    const offline = rows('offline', 100);
    const { container, rerender } = render(VirtualGroupedListTestHarness, {
      props: { groups: [group('online', online), group('offline', offline)] }
    });
    await vi.waitFor(() => expect(firstVisibleRow(container).id).toBe('online-0'));

    await scrollTo(container, 2000);
    const before = firstVisibleRow(container);
    const moved = offline[99];

    await rerender({
      groups: [
        group('online', [moved, ...online]),
        group(
          'offline',
          offline.filter((row) => row !== moved)
        )
      ]
    });

    await vi.waitFor(() => {
      const after = firstVisibleRow(container);
      expect(after.id).toBe(before.id);
      expect(Math.abs(after.top - before.top)).toBeLessThanOrEqual(1);
    });
  });

  it('stays at the top when rows are inserted while the list is scrolled to the top', async () => {
    const online = rows('online', 10);
    const { container, rerender } = render(VirtualGroupedListTestHarness, {
      props: { groups: [group('online', online)] }
    });
    await vi.waitFor(() => expect(firstVisibleRow(container).id).toBe('online-0'));

    await rerender({ groups: [group('online', [...rows('new', 2), ...online])] });

    await vi.waitFor(() => expect(firstVisibleRow(container).id).toBe('new-0'));
    expect(viewport(container).scrollTop).toBe(0);
  });
});
