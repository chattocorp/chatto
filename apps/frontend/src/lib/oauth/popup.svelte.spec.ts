import { afterEach, describe, expect, it, vi } from 'vitest';
import { openOAuthPopup } from './popup';

const mocks = vi.hoisted(() => ({ open: vi.fn() }));
vi.mock('./authorizationWindow', () => ({
  openAuthorizationWindow: mocks.open,
  authorizationWindowFeatures: () => 'popup'
}));

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe('provider popup outcomes', () => {
  it('identifies a blocked window', () => {
    mocks.open.mockReturnValue(null);
    expect(() => openOAuthPopup('blocked')).toThrowError(
      expect.objectContaining({ reason: 'blocked' })
    );
  });

  it('without a channel, identifies a closed window as cancellation and stops polling', async () => {
    vi.useFakeTimers();
    vi.stubGlobal('BroadcastChannel', undefined);
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

  it('detects closure on the launch page even with a callback channel', async () => {
    vi.useFakeTimers();
    mocks.open.mockReturnValue({
      detachOpener: vi.fn(),
      close: vi.fn(),
      isClosed: async () => true
    });
    const popup = openOAuthPopup('launch');
    const result = expect(popup.response).rejects.toMatchObject({ reason: 'cancelled' });
    await vi.advanceTimersByTimeAsync(250);
    await result;
  });

  it('uses the native closure of the matching desktop window after navigation', async () => {
    const unsubscribe = vi.fn();
    let notify!: (name: string) => void;
    window.chattoDesktop = {
      onAuthorizationWindowClosed(callback) {
        notify = callback;
        return unsubscribe;
      }
    };
    try {
      mocks.open.mockReturnValue({
        detachOpener: vi.fn(),
        close: vi.fn(),
        navigate: vi.fn(),
        isClosed: async () => true
      });
      const popup = openOAuthPopup('desktop');
      await popup.navigate('https://provider.example/');
      notify('chatto-oauth-other');
      expect(unsubscribe).not.toHaveBeenCalled();
      const result = expect(popup.response).rejects.toMatchObject({ reason: 'cancelled' });
      notify('chatto-oauth-desktop');
      await result;
      expect(unsubscribe).toHaveBeenCalledOnce();
    } finally {
      delete window.chattoDesktop;
    }
  });

  it('keeps waiting when a provider page hides the window, until the user cancels', async () => {
    vi.useFakeTimers();
    const popupWindow = {
      detachOpener: vi.fn(),
      navigate: vi.fn(),
      close: vi.fn(),
      // A Cross-Origin-Opener-Policy page makes the handle report `closed`.
      isClosed: vi.fn().mockResolvedValue(true),
      messageSource: null
    };
    mocks.open.mockReturnValue(popupWindow);
    const popup = openOAuthPopup('severed');
    await popup.navigate('https://provider.example/');
    let settled = false;
    popup.response.then(
      () => (settled = true),
      () => (settled = true)
    );
    await vi.advanceTimersByTimeAsync(2000);
    expect(settled).toBe(false);
    expect(popupWindow.isClosed).toHaveBeenCalledOnce();

    const result = expect(popup.response).rejects.toMatchObject({ reason: 'cancelled' });
    popup.cancel();
    await result;
  });
});
