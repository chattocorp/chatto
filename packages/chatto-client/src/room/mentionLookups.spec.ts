import { describe, expect, it, vi } from 'vitest';
import { MentionLookups } from './mentionLookups.js';

describe('MentionLookups', () => {
  it('bounds concurrent reads and cancels released queued handles', async () => {
    const pending: Array<() => void> = [];
    const lookup = vi.fn(
      (handle: string, signal: AbortSignal) =>
        new Promise<string>((resolve, reject) => {
          pending.push(() => resolve(handle));
          signal.addEventListener('abort', () => reject(signal.reason));
        })
    );
    const store = new MentionLookups(lookup, () => true);
    const release = store.retain(['a', 'b', 'c', 'd', 'e']);
    expect(lookup).toHaveBeenCalledTimes(4);
    pending[0]!();
    await vi.waitFor(() => expect(lookup).toHaveBeenCalledTimes(5));
    release();
    expect(store.ids).toEqual([]);
    store.dispose();
  });

  it('retries failed reads on a later retain without a retry loop', async () => {
    const lookup = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValue('alice');
    const store = new MentionLookups(lookup, () => true);
    const release = store.retain(['alice']);
    await vi.waitFor(() => expect(lookup.mock.settledResults[0]?.type).toBe('rejected'));
    expect(store.ids).toEqual([]);
    release();
    store.retain(['alice']);
    await vi.waitFor(() => expect(store.ids).toEqual(['alice']));
    expect(lookup).toHaveBeenCalledTimes(2);
    store.dispose();
  });

  it('keeps active results when the bounded inactive cache fills', async () => {
    const lookup = vi.fn(async (handle: string) => handle);
    const store = new MentionLookups(lookup, () => true);
    const handles = Array.from({ length: 260 }, (_, index) => `user${index}`);
    store.retain(handles);
    await vi.waitFor(() => expect(store.ids).toHaveLength(260));
    expect(lookup).toHaveBeenCalledTimes(260);
    store.dispose();
    expect(store.ids).toEqual([]);
    store.retain(['other']);
    expect(lookup).toHaveBeenCalledTimes(260);
  });
});
