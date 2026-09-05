import { afterEach, expect, it, vi } from 'vitest';
import { useLoadMoreWhenVisible } from './useLoadMoreWhenVisible.svelte';

const cleanups: Array<() => void> = [];
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup();
});

it('fills a nested scroller without fetching pages hidden below its edge', async () => {
  const root = document.createElement('div');
  root.style.cssText =
    'position: fixed; top: 20px; left: 20px; height: 100px; width: 200px; overflow: auto;';
  const content = document.createElement('div');
  const sentinel = document.createElement('div');
  sentinel.style.height = '10px';
  root.append(content, sentinel);
  document.body.append(root);
  cleanups.push(() => root.remove());
  let cursor = 0;
  const loadMore = vi.fn(async () => {
    cursor++;
    const row = document.createElement('div');
    row.style.height = '60px';
    content.append(row);
  });
  const cleanup = useLoadMoreWhenVisible({
    getCursor: () => (cursor < 10 ? cursor : null),
    loadMore
  })(sentinel);
  if (cleanup) cleanups.push(cleanup);
  await vi.waitFor(() => expect(loadMore).toHaveBeenCalledTimes(2));
  // Allow the native observer to deliver the negative intersection after layout.
  await new Promise<void>((resolve) =>
    requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
  );
  expect(loadMore).toHaveBeenCalledTimes(2);
  expect(sentinel.getBoundingClientRect().top).toBeLessThan(window.innerHeight);
  expect(sentinel.getBoundingClientRect().top).toBeGreaterThan(root.getBoundingClientRect().bottom);

  root.style.height = '200px';
  await vi.waitFor(() => expect(loadMore).toHaveBeenCalledTimes(4));
  await new Promise<void>((resolve) =>
    requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
  );
  expect(loadMore).toHaveBeenCalledTimes(4);
});
