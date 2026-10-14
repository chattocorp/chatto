import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { getToasts, toast } from '$lib/ui/toast';
import CopyId from './CopyId.svelte';

describe('CopyId', () => {
  beforeEach(() => toast.clear());
  afterEach(() => {
    vi.restoreAllMocks();
    toast.clear();
  });

  it('copies the ID and confirms success', async () => {
    const writeText = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined);
    const { container } = render(CopyId, { value: 'USR-7Q9M2N' });

    await userEvent.click(container.querySelector('button')!);

    expect(writeText).toHaveBeenCalledWith('USR-7Q9M2N');
    expect(getToasts()).toMatchObject([{ tone: 'success', message: 'Copied to clipboard' }]);
  });

  it.each(['denied', 'unavailable'])(
    'reports %s clipboard access and leaves the ID visible',
    async (failure) => {
      if (failure === 'denied') {
        vi.spyOn(navigator.clipboard, 'writeText').mockRejectedValue(new Error('private detail'));
      } else {
        vi.spyOn(navigator, 'clipboard', 'get').mockReturnValue(undefined!);
      }
      const { container } = render(CopyId, { value: 'USR-7Q9M2N' });

      await userEvent.click(container.querySelector('button')!);

      expect(getToasts()).toMatchObject([
        {
          tone: 'error',
          message: 'Could not copy to clipboard. Select the text and copy it manually.'
        }
      ]);
      expect(container.querySelector('code')?.textContent).toBe('USR-7Q9M2N');
    }
  );
});
