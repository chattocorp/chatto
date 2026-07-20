import { beforeEach, describe, expect, it, vi } from 'vitest';
import { getToasts, toast } from '$lib/ui/toast';
import { copyMessageTextToClipboard } from './useMessageActions.svelte';

describe('copyMessageTextToClipboard', () => {
  const writeText = vi.fn();

  beforeEach(() => {
    toast.clear();
    writeText.mockReset();
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true
    });
  });

  it('copies the original plain message body and confirms success', async () => {
    writeText.mockResolvedValue(undefined);

    await copyMessageTextToClipboard('Hello **world**');

    expect(writeText).toHaveBeenCalledWith('Hello **world**');
    expect(getToasts().map((item) => item.message)).toContain('Copied to clipboard');
  });

  it('shows an error when clipboard access fails', async () => {
    writeText.mockRejectedValue(new Error('denied'));

    await copyMessageTextToClipboard('Hello');

    expect(getToasts().map((item) => item.message)).toContain('Failed to copy text');
  });
});
