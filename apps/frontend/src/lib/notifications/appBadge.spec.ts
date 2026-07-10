import { afterEach, describe, expect, it, vi } from 'vitest';
import { syncServiceWorkerNotificationBadgeState } from './appBadge';

function stubBadgeEnvironment(options: { installed: boolean; controlled?: boolean }) {
  const controllerPostMessage = vi.fn();
  const activePostMessage = vi.fn();
  let controller =
    options.controlled === false
      ? null
      : ({ postMessage: controllerPostMessage } as unknown as ServiceWorker);
  let controllerChange: (() => void) | undefined;
  const serviceWorker = {
    get controller() {
      return controller;
    },
    ready: Promise.resolve({ active: { postMessage: activePostMessage } }),
    addEventListener: vi.fn((type: string, listener: () => void) => {
      if (type === 'controllerchange') controllerChange = listener;
    })
  };
  vi.stubGlobal('navigator', {
    setAppBadge: vi.fn(),
    clearAppBadge: vi.fn(),
    serviceWorker
  });
  vi.stubGlobal('window', {
    matchMedia: vi.fn((query: string) => ({
      matches: options.installed && query === '(display-mode: standalone)'
    }))
  });

  return {
    postMessage: controllerPostMessage,
    activePostMessage,
    replaceController() {
      const postMessage = vi.fn();
      controller = { postMessage } as unknown as ServiceWorker;
      controllerChange?.();
      return postMessage;
    }
  };
}

describe('syncServiceWorkerNotificationBadgeState', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('tells the service worker to skip worker-side badging in a browser tab', () => {
    const { postMessage } = stubBadgeEnvironment({ installed: false });

    syncServiceWorkerNotificationBadgeState({ kind: 'count', count: 3 });

    expect(postMessage).toHaveBeenCalledWith({
      type: 'chatto-badge-state',
      badgeIntent: { kind: 'count', count: 3 },
      notificationCount: 3,
      serviceWorkerAppBadgeEnabled: false
    });
  });

  it('allows worker-side badging in an installed app display mode', () => {
    const { postMessage } = stubBadgeEnvironment({ installed: true });

    syncServiceWorkerNotificationBadgeState({ kind: 'flag' });

    expect(postMessage).toHaveBeenCalledWith({
      type: 'chatto-badge-state',
      badgeIntent: { kind: 'flag' },
      notificationCount: 1,
      serviceWorkerAppBadgeEnabled: true
    });
  });

  it('sends the latest state to an active worker before the page is controlled', async () => {
    const { activePostMessage } = stubBadgeEnvironment({ installed: true, controlled: false });

    syncServiceWorkerNotificationBadgeState({ kind: 'clear' });
    await vi.waitFor(() => expect(activePostMessage).toHaveBeenCalledOnce());

    expect(activePostMessage).toHaveBeenCalledWith({
      type: 'chatto-badge-state',
      badgeIntent: { kind: 'clear' },
      notificationCount: 0,
      serviceWorkerAppBadgeEnabled: true
    });
  });

  it('does not deliver an older badge intent while worker readiness is pending', async () => {
    const { activePostMessage } = stubBadgeEnvironment({ installed: true, controlled: false });

    syncServiceWorkerNotificationBadgeState({ kind: 'count', count: 2 });
    syncServiceWorkerNotificationBadgeState({ kind: 'clear' });
    await vi.waitFor(() => expect(activePostMessage).toHaveBeenCalledOnce());

    expect(activePostMessage).toHaveBeenCalledWith(
      expect.objectContaining({ badgeIntent: { kind: 'clear' }, notificationCount: 0 })
    );
  });

  it('replays authoritative state when the controlling worker changes', () => {
    const environment = stubBadgeEnvironment({ installed: true });
    syncServiceWorkerNotificationBadgeState({ kind: 'clear' });

    const replacementPostMessage = environment.replaceController();

    expect(replacementPostMessage).toHaveBeenCalledOnce();
    expect(replacementPostMessage).toHaveBeenCalledWith({
      type: 'chatto-badge-state',
      badgeIntent: { kind: 'clear' },
      notificationCount: 0,
      serviceWorkerAppBadgeEnabled: true
    });
  });
});
