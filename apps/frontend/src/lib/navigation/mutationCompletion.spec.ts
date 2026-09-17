import { describe, expect, it, vi } from 'vitest';
import { StaleResponseError } from '$lib/api-client/connect';
import { completeMutation, NavigationVisits } from './mutationCompletion';

describe('mutation completion across resets', () => {
  it('can navigate after a successful result is discarded without returning old data', async () => {
    const visits = new NavigationVisits();
    const navigate = vi.fn();
    const result = await completeMutation(
      async () => {
        throw new StaleResponseError(true);
      },
      visits.capture(),
      navigate
    );
    expect(result).toBeUndefined();
    expect(navigate).toHaveBeenCalledOnce();
  });

  it('does not navigate after leaving and returning to the same page', async () => {
    const visits = new NavigationVisits();
    const navigate = vi.fn();
    let finish!: () => void;
    const request = completeMutation(
      () =>
        new Promise<void>((resolve) => {
          finish = resolve;
        }),
      visits.capture(),
      navigate
    );
    visits.leave();
    visits.leave();
    finish();
    await request;
    expect(navigate).not.toHaveBeenCalled();
  });

  it('does not treat a failed request as success', async () => {
    const navigate = vi.fn();
    await expect(
      completeMutation(
        async () => {
          throw new Error('denied');
        },
        () => true,
        navigate
      )
    ).rejects.toThrow('denied');
    expect(navigate).not.toHaveBeenCalled();
  });
});
