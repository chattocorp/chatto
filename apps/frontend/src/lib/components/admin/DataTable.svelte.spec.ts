import { beforeEach, afterEach, describe, it, expect, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { testSnippet } from '$lib/test-utils';
import DataTable from './DataTable.svelte';

let originalIntersectionObserver: typeof IntersectionObserver;
let observers: MockIntersectionObserver[] = [];

class MockIntersectionObserver implements IntersectionObserver {
  readonly root: Element | Document | null;
  readonly rootMargin: string;
  readonly thresholds: ReadonlyArray<number> = [];
  private elements: Element[] = [];

  constructor(
    private readonly callback: IntersectionObserverCallback,
    options?: IntersectionObserverInit
  ) {
    this.root = options?.root ?? null;
    this.rootMargin = options?.rootMargin ?? '0px';
    observers.push(this);
  }

  observe = (target: Element) => {
    this.elements.push(target);
  };

  unobserve = (target: Element) => {
    this.elements = this.elements.filter((element) => element !== target);
  };

  disconnect = () => {
    this.elements = [];
  };

  takeRecords = () => [];

  trigger(isIntersecting: boolean) {
    const target = this.elements[0] ?? document.createElement('tr');
    this.callback(
      [
        {
          boundingClientRect: target.getBoundingClientRect(),
          intersectionRatio: isIntersecting ? 1 : 0,
          intersectionRect: target.getBoundingClientRect(),
          isIntersecting,
          rootBounds: null,
          target,
          time: performance.now()
        }
      ],
      this
    );
  }
}

function renderTable(
  props: {
    fitContent?: boolean;
    stickyHeader?: boolean;
    fillHeight?: boolean;
    hoverable?: boolean;
    hasMore?: boolean;
    loadingMore?: boolean;
    onLoadMore?: () => void | Promise<void>;
    loadMoreRoot?: HTMLElement;
  } = {}
) {
  return render(DataTable, {
    props: {
      items: [{ id: '1', name: 'Alice' }],
      columns: 1,
      header: testSnippet('<th>Name</th>'),
      // The row snippet receives the item as `params`. We inline a
      // self-contained row that ignores it — `testSnippet` builds a
      // generic Snippet from the HTML string.
      row: testSnippet('<td data-testid="row-cell">cell</td>'),
      ...props
    }
  });
}

describe('DataTable.hoverable', () => {
  beforeEach(() => {
    originalIntersectionObserver = globalThis.IntersectionObserver;
    observers = [];
    globalThis.IntersectionObserver =
      MockIntersectionObserver as unknown as typeof IntersectionObserver;
  });

  afterEach(() => {
    globalThis.IntersectionObserver = originalIntersectionObserver;
  });

  it('applies hover bg by default', async () => {
    const { container } = renderTable();
    const tr = container.querySelector('tbody tr') as HTMLElement;
    expect(tr.className).toContain('hover:bg-surface/70');
  });

  it('omits hover bg when hoverable=false', async () => {
    const { container } = renderTable({ hoverable: false });
    const tr = container.querySelector('tbody tr') as HTMLElement;
    expect(tr.className).not.toContain('hover:bg-surface/70');
  });

  it('uses a rounded background viewport beneath the surface header', async () => {
    const { container } = renderTable();
    const table = container.querySelector('table') as HTMLTableElement;
    const viewport = table.parentElement as HTMLElement;
    const header = container.querySelector('thead tr') as HTMLElement;
    const body = container.querySelector('tbody') as HTMLElement;

    expect(viewport.className).toContain('rounded-md');
    expect(viewport.className).toContain('overflow-x-auto');
    expect(header.className).toContain('panel-header');
    expect(body.className).toContain('bg-background');
  });

  it('keeps matrix-style tables at their intrinsic width', async () => {
    const { container } = renderTable({ fitContent: true });
    const table = container.querySelector('table') as HTMLTableElement;

    expect(table.className).toContain('w-max');
    expect(table.className).not.toContain('w-full');
  });

  it('configures a bounded matrix viewport with a sticky header', async () => {
    const { container } = render(DataTable, {
      props: {
        items: Array.from({ length: 40 }, (_, index) => ({ id: String(index) })),
        columns: 1,
        stickyHeader: true,
        header: testSnippet('<th>Permission</th>'),
        row: testSnippet('<td class="h-12">Permission</td>')
      }
    });
    const table = container.querySelector('table') as HTMLTableElement;
    const viewport = table.parentElement?.parentElement as HTMLElement;
    const header = container.querySelector('thead') as HTMLElement;

    expect(viewport.className).toContain('max-h-[70dvh]');
    expect((table.parentElement as HTMLElement).className).toContain('overflow-y-auto');
    expect((table.parentElement as HTMLElement).className).toContain('overflow-x-auto');
    expect(header.className).toContain('sticky');
  });

  it('fills a flex parent instead of using the sticky-header viewport cap when requested', () => {
    const { container } = renderTable({ stickyHeader: true, fillHeight: true });
    const viewport = (container.querySelector('table') as HTMLTableElement).parentElement
      ?.parentElement as HTMLElement;

    expect(viewport.className).toContain('flex-1');
    expect(viewport.className).not.toContain('max-h-[70dvh]');
  });

  it('still renders cursor-pointer on hoverable=false rows when onRowClick is set', async () => {
    const onRowClick = vi.fn();
    const { container } = render(DataTable, {
      props: {
        items: [{ id: '1' }],
        columns: 1,
        header: testSnippet('<th>X</th>'),
        row: testSnippet('<td>x</td>'),
        hoverable: false,
        onRowClick
      }
    });
    const tr = container.querySelector('tbody tr') as HTMLElement;
    expect(tr.className).toContain('cursor-pointer');
  });

  it('does not render an auto-load sentinel by default', async () => {
    const { container } = renderTable();
    expect(container.querySelectorAll('tbody tr')).toHaveLength(1);
    expect(observers).toHaveLength(0);
  });

  it('calls onLoadMore when the trailing sentinel intersects the scroll root', async () => {
    const root = document.createElement('div');
    const onLoadMore = vi.fn();
    renderTable({ hasMore: true, loadMoreRoot: root, onLoadMore });

    expect(observers).toHaveLength(1);
    expect(observers[0].root).toBe(root);
    expect(observers[0].rootMargin).toBe('0px 0px 160px 0px');

    observers[0].trigger(true);

    expect(onLoadMore).toHaveBeenCalledTimes(1);
  });

  it('does not call onLoadMore when already loading more', async () => {
    const root = document.createElement('div');
    const onLoadMore = vi.fn();
    renderTable({ hasMore: true, loadingMore: true, loadMoreRoot: root, onLoadMore });

    expect(observers).toHaveLength(0);
    expect(onLoadMore).not.toHaveBeenCalled();
  });

  it('does not call onLoadMore when no more rows are available', async () => {
    const root = document.createElement('div');
    const onLoadMore = vi.fn();
    renderTable({ hasMore: false, loadMoreRoot: root, onLoadMore });

    expect(observers).toHaveLength(0);
    expect(onLoadMore).not.toHaveBeenCalled();
  });
});
