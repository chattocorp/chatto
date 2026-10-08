import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  createMessageSend,
  MESSAGE_SEND_RETRY_WINDOW_MS,
  MessageSendExpiredError
} from './messageSend.js';

afterEach(() => vi.useRealTimers());

describe('prepared message sends', () => {
  it('reuses the exact prepared request after a lost response', async () => {
    const prepare = vi.fn(async (key: string) => ({ key, assets: ['uploaded-asset'] }));
    const post = vi
      .fn()
      .mockRejectedValueOnce(new Error('response lost'))
      .mockResolvedValue('message-id');
    const operation = createMessageSend({ prepare, post });
    await expect(operation.send()).rejects.toThrow('response lost');
    await expect(operation.send()).resolves.toBe('message-id');
    expect(prepare).toHaveBeenCalledExactlyOnceWith(operation.idempotencyKey);
    expect(post.mock.calls[0]![0]).toBe(post.mock.calls[1]![0]);
  });

  it('retries failed preparation and shares a concurrent attempt', async () => {
    let resolvePost!: (result: string) => void;
    const prepare = vi
      .fn()
      .mockRejectedValueOnce(new Error('upload failed'))
      .mockResolvedValue({ key: 'key' });
    const post = vi.fn(
      () =>
        new Promise<string>((resolve) => {
          resolvePost = resolve;
        })
    );
    const operation = createMessageSend({ prepare, post });
    await expect(operation.send()).rejects.toThrow('upload failed');
    const first = operation.send();
    expect(operation.send()).toBe(first);
    await vi.waitFor(() => expect(post).toHaveBeenCalledOnce());
    resolvePost('message-id');
    await expect(first).resolves.toBe('message-id');
    expect(prepare).toHaveBeenCalledTimes(2);
  });

  it('stops at the fixed retry deadline without extending it on retry', async () => {
    vi.useFakeTimers();
    const post = vi.fn().mockRejectedValue(new Error('response lost'));
    const operation = createMessageSend({ prepare: async (key) => ({ key }), post });
    await expect(operation.send()).rejects.toThrow('response lost');
    vi.advanceTimersByTime(MESSAGE_SEND_RETRY_WINDOW_MS - 1);
    await expect(operation.send()).rejects.toThrow('response lost');
    vi.advanceTimersByTime(1);
    await expect(operation.send()).rejects.toBeInstanceOf(MessageSendExpiredError);
    expect(post).toHaveBeenCalledTimes(2);
  });
});
