import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useLoadMoreWhenVisible } from './useLoadMoreWhenVisible.svelte';

let intersectionCallback: IntersectionObserverCallback;
const observe = vi.fn();
const unobserve = vi.fn();
const disconnect = vi.fn();
const intersect = (visible = true) =>
  intersectionCallback(
    [{ isIntersecting: visible } as IntersectionObserverEntry],
    {} as IntersectionObserver
  );

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal(
    'IntersectionObserver',
    class {
      constructor(callback: IntersectionObserverCallback) {
        intersectionCallback = callback;
      }
      observe = observe;
      unobserve = unobserve;
      disconnect = disconnect;
    }
  );
});
afterEach(() => vi.unstubAllGlobals());

function mount(options: Parameters<typeof useLoadMoreWhenVisible>[0]) {
  const node = document.createElement('div');
  return useLoadMoreWhenVisible(options)(node);
}

const settle = async () => {
  await tick();
  await tick();
};

describe('useLoadMoreWhenVisible', () => {
  it('requests a new visibility measurement after progress, including offset zero', async () => {
    let cursor: number | null = 0;
    const loadMore = vi.fn(async () => {
      cursor = cursor === 0 ? 1 : null;
    });
    mount({ getCursor: () => cursor, loadMore });
    intersect();
    await vi.waitFor(() => expect(observe).toHaveBeenCalledTimes(2));
    expect(unobserve).toHaveBeenCalledOnce();
    expect(loadMore).toHaveBeenCalledOnce();
    // Only the browser's next positive intersection starts another page.
    intersect(false);
    expect(loadMore).toHaveBeenCalledOnce();
    intersect();
    await settle();
    expect(loadMore).toHaveBeenCalledTimes(2);
    expect(observe).toHaveBeenCalledTimes(2);
  });

  it('does not loop when loading leaves the cursor unchanged', async () => {
    const loadMore = vi.fn(async () => {});
    mount({ getCursor: () => 'same', loadMore });
    intersect();
    await settle();
    expect(loadMore).toHaveBeenCalledOnce();
    expect(unobserve).not.toHaveBeenCalled();
  });

  it('ignores repeated intersections while a request is pending', async () => {
    let finish!: () => void;
    const loadMore = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          finish = resolve;
        })
    );
    mount({ getCursor: () => 'first', loadMore });
    intersect();
    intersect();
    intersect();
    expect(loadMore).toHaveBeenCalledOnce();
    finish();
    await settle();
  });

  it('stops on an error and allows loading after recovery and a new intersection', async () => {
    let cursor = 1;
    let error = false;
    const loadMore = vi.fn(async () => {
      cursor++;
      error = true;
    });
    mount({ getCursor: () => cursor, loadMore, hasError: () => error });
    intersect();
    await settle();
    expect(observe).toHaveBeenCalledOnce();
    intersect();
    expect(loadMore).toHaveBeenCalledOnce();
    error = false;
    intersect();
    await settle();
    expect(loadMore).toHaveBeenCalledTimes(2);
  });

  it('does not load exhausted pages', () => {
    const loadMore = vi.fn(async () => {});
    mount({ getCursor: () => null, loadMore });
    intersect();
    expect(loadMore).not.toHaveBeenCalled();
  });

  it('handles rejected loads without an automatic retry', async () => {
    const loadMore = vi.fn(async () => {
      throw new Error('request failed');
    });
    mount({ getCursor: () => 'first', loadMore });
    intersect();
    await settle();
    expect(loadMore).toHaveBeenCalledOnce();
    expect(unobserve).not.toHaveBeenCalled();
  });

  it('discards queued callbacks and late completion after detach', async () => {
    let finish!: () => void;
    let cursor = 1;
    const loadMore = vi.fn(async () => {
      await new Promise<void>((resolve) => {
        finish = resolve;
      });
      cursor++;
    });
    const cleanup = mount({ getCursor: () => cursor, loadMore });
    intersect();
    if (cleanup) cleanup();
    intersect();
    finish();
    await settle();
    expect(disconnect).toHaveBeenCalledOnce();
    expect(loadMore).toHaveBeenCalledOnce();
    expect(observe).toHaveBeenCalledOnce();
  });
});
