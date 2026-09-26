import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { startServerRecovery } from './serverRecovery';

describe('startup server recovery', () => {
  let stop: (() => void) | undefined;
  let failed: Set<string>;
  const registry = {
    servers: [{ id: 'a' }, { id: 'b' }],
    needsRecovery: (id: string) => failed.has(id),
    recoverServer: vi.fn<(id: string) => Promise<void>>()
  };

  beforeEach(() => {
    vi.useFakeTimers();
    failed = new Set(['a']);
    registry.servers = [{ id: 'a' }, { id: 'b' }];
    registry.recoverServer.mockReset().mockResolvedValue(undefined);
    vi.spyOn(navigator, 'onLine', 'get').mockReturnValue(true);
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible');
  });

  afterEach(() => {
    stop?.();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('backs off independently, caps at 30 seconds, and stops after recovery', async () => {
    stop = startServerRecovery(registry);
    for (const delay of [1000, 2000, 4000, 8000, 16000, 30000, 30000]) {
      const count = registry.recoverServer.mock.calls.length;
      await vi.advanceTimersByTimeAsync(delay - 1);
      expect(registry.recoverServer).toHaveBeenCalledTimes(count);
      await vi.advanceTimersByTimeAsync(1);
      expect(registry.recoverServer).toHaveBeenCalledTimes(count + 1);
    }
    expect(registry.recoverServer.mock.calls.every(([id]) => id === 'a')).toBe(true);
    failed.clear();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(registry.recoverServer).toHaveBeenCalledTimes(7);
  });

  it('retries on network return, visibility and native resume; pauses while hidden or offline', async () => {
    stop = startServerRecovery(registry);
    vi.spyOn(navigator, 'onLine', 'get').mockReturnValue(false);
    await vi.advanceTimersByTimeAsync(30_000);
    expect(registry.recoverServer).not.toHaveBeenCalled();
    vi.spyOn(navigator, 'onLine', 'get').mockReturnValue(true);
    window.dispatchEvent(new Event('online'));
    await vi.advanceTimersByTimeAsync(0);
    expect(registry.recoverServer).toHaveBeenCalledTimes(1);
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden');
    await vi.advanceTimersByTimeAsync(30_000);
    expect(registry.recoverServer).toHaveBeenCalledTimes(1);
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible');
    document.dispatchEvent(new Event('visibilitychange'));
    await vi.advanceTimersByTimeAsync(0);
    expect(registry.recoverServer).toHaveBeenCalledTimes(2);
    document.dispatchEvent(new Event('pause'));
    await vi.advanceTimersByTimeAsync(30_000);
    expect(registry.recoverServer).toHaveBeenCalledTimes(2);
    document.dispatchEvent(new Event('resume'));
    await vi.advanceTimersByTimeAsync(0);
    expect(registry.recoverServer).toHaveBeenCalledTimes(3);
  });

  it('does not overlap attempts and cleans up pending work on removal or teardown', async () => {
    let finish!: () => void;
    registry.recoverServer.mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          finish = resolve;
        })
    );
    stop = startServerRecovery(registry);
    await vi.advanceTimersByTimeAsync(1000);
    window.dispatchEvent(new Event('online'));
    document.dispatchEvent(new Event('resume'));
    await vi.advanceTimersByTimeAsync(60_000);
    expect(registry.recoverServer).toHaveBeenCalledTimes(1);
    registry.servers = [];
    await vi.advanceTimersByTimeAsync(1000);
    finish();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(registry.recoverServer).toHaveBeenCalledTimes(1);
    stop();
    registry.servers = [{ id: 'b' }];
    failed.add('b');
    window.dispatchEvent(new Event('online'));
    await vi.advanceTimersByTimeAsync(60_000);
    expect(registry.recoverServer).toHaveBeenCalledTimes(1);
  });
});
