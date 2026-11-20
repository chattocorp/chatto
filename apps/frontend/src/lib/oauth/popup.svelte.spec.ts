import { afterEach, describe, expect, it, vi } from 'vitest';
import { OAUTH_POPUP_RESPONSE_TYPE, oauthPopupChannelName, openOAuthPopup } from './popup';

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
    vi.useFakeTimers();
    const subscribe = vi.fn<(callback: (name: string) => void) => () => void>();
    const unsubscribe = vi.fn();
    subscribe.mockReturnValue(unsubscribe);
    vi.stubGlobal('chattoDesktop', { onAuthorizationWindowClosed: subscribe });
    const isClosed = vi.fn().mockResolvedValue(true);
    mocks.open.mockReturnValue({
      detachOpener: vi.fn(),
      close: vi.fn(),
      navigate: vi.fn(),
      isClosed
    });
    const popup = openOAuthPopup('desktop');
    const result = expect(popup.response).rejects.toMatchObject({ reason: 'cancelled' });
    await vi.advanceTimersByTimeAsync(500);
    expect(isClosed).not.toHaveBeenCalled();
    await popup.navigate('https://provider.example/');
    const [notify] = subscribe.mock.calls[0];
    notify('chatto-oauth-other');
    expect(unsubscribe).not.toHaveBeenCalled();
    notify('chatto-oauth-desktop');
    await result;
    expect(unsubscribe).toHaveBeenCalledOnce();
  });

  it('preserves a successful callback when the native window closes afterward', async () => {
    const unsubscribe = vi.fn();
    const subscribe = vi.fn((_callback: (name: string) => void) => unsubscribe);
    vi.stubGlobal('chattoDesktop', { onAuthorizationWindowClosed: subscribe });
    const close = vi.fn();
    mocks.open.mockReturnValue({ detachOpener: vi.fn(), close });
    const state = 'completed';
    const popup = openOAuthPopup(state);
    const channel = new BroadcastChannel(oauthPopupChannelName(state));
    try {
      const response = { type: OAUTH_POPUP_RESPONSE_TYPE, state, completed: true };
      channel.postMessage(response);
      await expect(popup.response).resolves.toEqual(response);
      const [notify] = subscribe.mock.calls[0];
      notify('chatto-oauth-completed');
      expect(unsubscribe).toHaveBeenCalledOnce();
      expect(close).toHaveBeenCalledOnce();
    } finally {
      channel.close();
    }
  });

  it('keeps waiting when an installed app loses its launch window handle', async () => {
    vi.useFakeTimers();
    vi.stubGlobal('matchMedia', (query: string) => ({
      matches: query === '(display-mode: standalone)'
    }));
    const close = vi.fn();
    mocks.open.mockReturnValue({ detachOpener: vi.fn(), close, isClosed: async () => true });
    const popup = openOAuthPopup('standalone');
    const result = expect(popup.response).rejects.toMatchObject({ reason: 'cancelled' });
    await vi.advanceTimersByTimeAsync(500);
    expect(close).not.toHaveBeenCalled();
    popup.cancel();
    await result;
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
