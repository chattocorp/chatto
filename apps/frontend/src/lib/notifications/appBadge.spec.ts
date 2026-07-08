import { afterEach, describe, expect, it, vi } from 'vitest';
import { syncServiceWorkerNotificationBadgeState } from './appBadge';

function stubBadgeEnvironment(options: { installed: boolean }) {
  const postMessage = vi.fn();
  vi.stubGlobal('navigator', {
    setAppBadge: vi.fn(),
    clearAppBadge: vi.fn(),
    serviceWorker: {
      controller: { postMessage }
    }
  });
  vi.stubGlobal('window', {
    matchMedia: vi.fn((query: string) => ({
      matches: options.installed && query === '(display-mode: standalone)'
    }))
  });

  return { postMessage };
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
});
