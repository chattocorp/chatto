import { beforeEach, describe, expect, it, vi } from 'vitest';
import { getToasts, toast } from '$lib/ui/toast';
import { copyMessageLinkToClipboard } from './messageLinks';

describe('copyMessageLinkToClipboard', () => {
  const writeText = vi.fn();

  beforeEach(() => {
    toast.clear();
    writeText.mockReset();
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true
    });
  });

  it('copies the message link and shows a success toast', async () => {
    writeText.mockResolvedValue(undefined);

    await copyMessageLinkToClipboard('server-1', 'room-1', 'message-1');

    expect(writeText).toHaveBeenCalledWith(expect.stringContaining('/chat/-/room-1/m/message-1'));
    expect(getToasts().map((t) => t.message)).toContain('Message link copied');
  });

  it('shows an error toast when clipboard copy fails', async () => {
    writeText.mockRejectedValue(new Error('denied'));

    await copyMessageLinkToClipboard('server-1', 'room-1', 'message-1');

    expect(getToasts().map((t) => t.message)).toContain('Failed to copy link');
  });
});
