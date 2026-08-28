import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useLoadMoreWhenVisible } from './useLoadMoreWhenVisible.svelte';

let intersectionCallback: IntersectionObserverCallback;
const disconnectObserver = vi.fn();

beforeEach(() => {
  disconnectObserver.mockReset();
  vi.stubGlobal(
    'IntersectionObserver',
    class {
      constructor(callback: IntersectionObserverCallback) {
        intersectionCallback = callback;
      }
      observe = vi.fn();
      disconnect = disconnectObserver;
    }
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
  document.body.replaceChildren();
});

describe('useLoadMoreWhenVisible', () => {
  it('loads successive pages while the sentinel remains near the viewport', async () => {
    let cursor: string | null = 'first';
    const loadMore = vi.fn(async () => {
      cursor = cursor === 'first' ? 'second' : null;
    });
    const attachment = useLoadMoreWhenVisible({
      getCursor: () => cursor,
      loadMore,
      hasError: () => false
    });
    const sentinel = document.createElement('div');
    document.body.append(sentinel);
    const cleanup = attachment(sentinel);

    intersectionCallback([{ isIntersecting: true } as IntersectionObserverEntry], {} as never);

    await vi.waitFor(() => expect(loadMore).toHaveBeenCalledTimes(2));
    if (typeof cleanup === 'function') cleanup();
    expect(disconnectObserver).toHaveBeenCalledOnce();
  });

  it('stops when loading does not advance the cursor', async () => {
    const loadMore = vi.fn(async () => {});
    const attachment = useLoadMoreWhenVisible({
      getCursor: () => 'unchanged',
      loadMore,
      hasError: () => false
    });
    const sentinel = document.createElement('div');
    document.body.append(sentinel);
    attachment(sentinel);

    intersectionCallback([{ isIntersecting: true } as IntersectionObserverEntry], {} as never);

    await vi.waitFor(() => expect(loadMore).toHaveBeenCalledOnce());
  });
});
