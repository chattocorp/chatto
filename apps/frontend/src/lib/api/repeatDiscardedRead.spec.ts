import { StaleResponseError } from '@chatto/client/api/connect';
import { describe, expect, it, vi } from 'vitest';
import { repeatDiscardedRead } from './repeatDiscardedRead';

describe('repeatDiscardedRead', () => {
  it('repeats a read once after a privacy reset discarded it', async () => {
    const read = vi
      .fn()
      .mockRejectedValueOnce(new StaleResponseError(false))
      .mockResolvedValueOnce('fresh');
    await expect(repeatDiscardedRead(read)).resolves.toBe('fresh');
    expect(read).toHaveBeenCalledTimes(2);
  });

  it('does not repeat a second discard, a mutation, or another failure', async () => {
    const twice = vi.fn().mockRejectedValue(new StaleResponseError(false));
    await expect(repeatDiscardedRead(twice)).rejects.toBeInstanceOf(StaleResponseError);
    expect(twice).toHaveBeenCalledTimes(2);

    for (const error of [new StaleResponseError(true), new Error('boom')]) {
      const read = vi.fn().mockRejectedValue(error);
      await expect(repeatDiscardedRead(read)).rejects.toBe(error);
      expect(read).toHaveBeenCalledOnce();
    }
  });
});
