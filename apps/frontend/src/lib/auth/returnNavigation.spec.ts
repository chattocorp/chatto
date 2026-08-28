import { beforeEach, describe, expect, it, vi } from 'vitest';

const { gotoMock } = vi.hoisted(() => ({
  gotoMock: vi.fn()
}));

vi.mock('$app/navigation', () => ({
  goto: gotoMock
}));

function memoryStorage(): Storage {
  const values = new Map<string, string>();
  return {
    get length() {
      return values.size;
    },
    clear: () => {
      values.clear();
    },
    getItem: (key) => values.get(key) ?? null,
    key: (index) => [...values.keys()][index] ?? null,
    removeItem: (key) => {
      values.delete(key);
    },
    setItem: (key, value) => {
      values.set(key, value);
    }
  };
}

async function loadModule() {
  vi.resetModules();
  return import('./returnNavigation');
}

describe('return navigation', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    gotoMock.mockReset().mockResolvedValue(undefined);
    vi.stubGlobal('sessionStorage', memoryStorage());
    vi.stubGlobal('window', {
      location: {
        pathname: '/login',
        search: '',
        hash: ''
      }
    });
  });

  it('stores only safe internal return paths and supersedes stale navigation', async () => {
    const { saveReturnUrl } = await loadModule();
    sessionStorage.setItem('returnUrl:navigating', '/chat/old');

    saveReturnUrl('/chat/-/settings?tab=profile');

    expect(sessionStorage.getItem('returnUrl')).toBe('/chat/-/settings?tab=profile');
    expect(sessionStorage.getItem('returnUrl:navigating')).toBeNull();

    saveReturnUrl('//attacker.example/path');
    expect(sessionStorage.getItem('returnUrl')).toBeNull();
  });

  it('rejects unsafe stored paths while checking pending state', async () => {
    const { hasPendingReturnNavigation } = await loadModule();
    sessionStorage.setItem('returnUrl', 'javascript:alert(1)');
    sessionStorage.setItem('returnUrl:navigating', '/\\attacker.example/path');

    expect(hasPendingReturnNavigation()).toBe(false);
    expect(sessionStorage.getItem('returnUrl')).toBeNull();
    expect(sessionStorage.getItem('returnUrl:navigating')).toBeNull();
  });

  it('claims, navigates to, and clears a stored return path', async () => {
    gotoMock.mockImplementation(async (target: string) => {
      expect(target).toBe('/chat/-/settings?tab=profile');
      expect(sessionStorage.getItem('returnUrl')).toBeNull();
      expect(sessionStorage.getItem('returnUrl:navigating')).toBe(target);
    });
    const { resumeReturnNavigation } = await loadModule();
    sessionStorage.setItem('returnUrl', '/chat/-/settings?tab=profile');

    await expect(resumeReturnNavigation()).resolves.toBe(true);

    expect(gotoMock).toHaveBeenCalledOnce();
    expect(sessionStorage.getItem('returnUrl:navigating')).toBeNull();
  });

  it('does not start a second navigation while one is underway', async () => {
    const { resumeReturnNavigation } = await loadModule();
    sessionStorage.setItem('returnUrl:navigating', '/chat/-/settings');

    await expect(resumeReturnNavigation()).resolves.toBe(true);

    expect(gotoMock).not.toHaveBeenCalled();
  });

  it('takes a queued return path for a route-load redirect', async () => {
    const { takeReturnNavigationTarget } = await loadModule();
    sessionStorage.setItem('returnUrl', '/chat/-/settings?tab=profile');

    expect(takeReturnNavigationTarget()).toBe('/chat/-/settings?tab=profile');
    expect(sessionStorage.getItem('returnUrl')).toBeNull();
  });

  it('does not take a return path while navigation is in progress', async () => {
    const { takeReturnNavigationTarget } = await loadModule();
    sessionStorage.setItem('returnUrl:navigating', '/chat/-/settings');
    sessionStorage.setItem('returnUrl', '/chat/-/settings?tab=profile');

    expect(takeReturnNavigationTarget()).toBeNull();
    expect(sessionStorage.getItem('returnUrl')).toBe('/chat/-/settings?tab=profile');
  });

  it('consumes a return path that already matches the current URL', async () => {
    const { resumeReturnNavigation } = await loadModule();
    window.location.pathname = '/chat/-/settings';
    window.location.search = '?tab=profile';
    sessionStorage.setItem('returnUrl', '/chat/-/settings?tab=profile');

    await expect(resumeReturnNavigation()).resolves.toBe(true);

    expect(gotoMock).not.toHaveBeenCalled();
    expect(sessionStorage.getItem('returnUrl')).toBeNull();
  });

  it('cleans up the navigation marker when navigation fails', async () => {
    const warning = vi.spyOn(console, 'warn').mockImplementation(() => {});
    gotoMock.mockRejectedValue(new Error('navigation failed'));
    const { resumeReturnNavigation } = await loadModule();
    sessionStorage.setItem('returnUrl', '/chat/-/settings');

    await expect(resumeReturnNavigation()).resolves.toBe(true);

    expect(sessionStorage.getItem('returnUrl:navigating')).toBeNull();
    expect(warning).toHaveBeenCalledWith(
      'Return URL navigation failed:',
      expect.objectContaining({ message: 'navigation failed' })
    );
  });

  it('hands backend OAuth return paths back to the browser', async () => {
    const { resumeReturnNavigation } = await loadModule();
    sessionStorage.setItem('returnUrl', '/oauth/authorize?client_id=client-1');

    await expect(resumeReturnNavigation()).resolves.toBe(true);

    expect(window.location.href).toBe('/oauth/authorize?client_id=client-1');
    expect(gotoMock).not.toHaveBeenCalled();
  });

  it('falls back to the app root for unsafe post-authentication paths', async () => {
    const { navigateAfterAuthentication } = await loadModule();

    await navigateAfterAuthentication('//attacker.example/path');

    expect(gotoMock).toHaveBeenCalledWith('/');
  });

  it('preserves literal route-pattern characters in a concrete return URL', async () => {
    const { navigateAfterAuthentication } = await loadModule();

    await navigateAfterAuthentication('/chat/-/settings?tab=[profile]');

    expect(gotoMock).toHaveBeenCalledWith('/chat/-/settings?tab=[profile]');
  });

  it('reports no work when no return navigation exists', async () => {
    const { resumeReturnNavigation } = await loadModule();

    await expect(resumeReturnNavigation()).resolves.toBe(false);

    expect(gotoMock).not.toHaveBeenCalled();
  });
});
