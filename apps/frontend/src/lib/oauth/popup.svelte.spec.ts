import { afterEach, describe, expect, it, vi } from 'vitest';
import { openOAuthPopup } from './popup';

const mocks = vi.hoisted(() => ({ open: vi.fn() }));
vi.mock('./authorizationWindow', () => ({ openAuthorizationWindow: mocks.open }));

afterEach(() => vi.useRealTimers());

describe('provider popup outcomes', () => {
  it('identifies a blocked window', () => {
    mocks.open.mockReturnValue(null);
    expect(() => openOAuthPopup('blocked')).toThrowError(
      expect.objectContaining({ reason: 'blocked' })
    );
  });

  it('identifies a closed window as cancellation and stops polling', async () => {
    vi.useFakeTimers();
    const popupWindow = {
      detachOpener: vi.fn(),
      navigate: vi.fn(),
      close: vi.fn(),
      isClosed: vi.fn().mockResolvedValue(true),
      messageSource: null
    };
    mocks.open.mockReturnValue(popupWindow);
    const popup = openOAuthPopup('cancelled');
    const result = expect(popup.response).rejects.toMatchObject({ reason: 'cancelled' });
    await vi.advanceTimersByTimeAsync(250);
    await result;
    await vi.advanceTimersByTimeAsync(1000);
    expect(popupWindow.isClosed).toHaveBeenCalledOnce();
    expect(popupWindow.close).toHaveBeenCalledOnce();
  });
});
