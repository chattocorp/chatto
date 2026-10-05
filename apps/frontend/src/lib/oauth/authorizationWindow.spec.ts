import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { authorizationLaunchTarget } from '$lib/test-utils/authorizationWindow';
import {
  AUTHORIZATION_LAUNCH_TTL_MS,
  authorizationLaunchStorageKey,
  openAuthorizationWindow,
  readAuthorizationLaunchTarget
} from './authorizationWindow';

vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));

function memoryStorage(): Storage {
  const values = new Map<string, string>();
  return {
    get length() {
      return values.size;
    },
    clear: () => values.clear(),
    getItem: (key) => values.get(key) ?? null,
    key: (index) => [...values.keys()][index] ?? null,
    removeItem: (key) => values.delete(key),
    setItem: (key, value) => values.set(key, value)
  };
}

function fakePopup() {
  return {
    closed: false,
    opener: {} as Window | null,
    location: { replace: vi.fn() },
    close: vi.fn(function (this: { closed: boolean }) {
      this.closed = true;
    })
  };
}

describe('authorization window', () => {
  beforeEach(() => {
    vi.stubGlobal('localStorage', memoryStorage());
  });
  afterEach(() => vi.unstubAllGlobals());

  function stubOpen(result: ReturnType<typeof fakePopup> | null) {
    const open = vi.fn(() => result);
    vi.stubGlobal('window', { location: { origin: 'https://app.example' }, open });
    return open;
  }

  it('opens on the same-origin launch page, never on about:blank', () => {
    const open = stubOpen(fakePopup());
    expect(openAuthorizationWindow('chatto-oauth', 'popup')).not.toBeNull();
    expect(open).toHaveBeenCalledWith(
      expect.stringMatching(/^https:\/\/app\.example\/servers\/authorize#[\w-]{20,}$/),
      'chatto-oauth',
      'popup'
    );
  });

  it('returns null when the browser blocks the window', () => {
    stubOpen(null);
    expect(openAuthorizationWindow('chatto-oauth', 'popup')).toBeNull();
  });

  it('gives the target to the launch page and removes it on close', async () => {
    const popup = fakePopup();
    const open = stubOpen(popup);
    const authorizationWindow = openAuthorizationWindow('chatto-oauth', 'popup')!;

    await authorizationWindow.navigate('https://remote.example/oauth/authorize?state=abc');
    expect(authorizationLaunchTarget(open)).toBe(
      'https://remote.example/oauth/authorize?state=abc'
    );
    expect(popup.location.replace).not.toHaveBeenCalled();

    await authorizationWindow.close();
    expect(authorizationLaunchTarget(open)).toBeNull();
    expect(popup.close).toHaveBeenCalledOnce();
  });

  it('removes expired launch records that a closed client left behind', () => {
    const stale = authorizationLaunchStorageKey('stale');
    const fresh = authorizationLaunchStorageKey('fresh');
    localStorage.setItem(stale, JSON.stringify({ url: 'https://remote.example/', createdAt: 0 }));
    localStorage.setItem(
      fresh,
      JSON.stringify({ url: 'https://remote.example/', createdAt: Date.now() })
    );
    localStorage.setItem('chatto:locale', 'en-GB');
    stubOpen(fakePopup());

    openAuthorizationWindow('chatto-oauth', 'popup');

    expect(localStorage.getItem(stale)).toBeNull();
    expect(localStorage.getItem(fresh)).not.toBeNull();
    expect(localStorage.getItem('chatto:locale')).toBe('en-GB');
  });

  it('navigates the window directly when storage is unavailable', async () => {
    const popup = fakePopup();
    stubOpen(popup);
    vi.stubGlobal('localStorage', {
      setItem: () => {
        throw new DOMException('Storage is full', 'QuotaExceededError');
      },
      removeItem: () => {}
    });
    const authorizationWindow = openAuthorizationWindow('chatto-oauth', 'popup')!;

    await authorizationWindow.navigate('https://remote.example/oauth/authorize');
    expect(popup.location.replace).toHaveBeenCalledWith('https://remote.example/oauth/authorize');
  });
});

describe('authorization launch target', () => {
  const key = authorizationLaunchStorageKey('launch-id');

  beforeEach(() => {
    vi.stubGlobal('localStorage', memoryStorage());
  });
  afterEach(() => vi.unstubAllGlobals());

  it('returns null until the opener stores a target', () => {
    expect(readAuthorizationLaunchTarget('launch-id')).toBeNull();
  });

  it('leaves a fresh target for every launch page that reads it', () => {
    localStorage.setItem(
      key,
      JSON.stringify({ url: 'https://remote.example/oauth/authorize', createdAt: 1_000 })
    );
    // Firefox for Android can load the launch page in a detached window too.
    expect(readAuthorizationLaunchTarget('launch-id', 2_000)).toBe(
      'https://remote.example/oauth/authorize'
    );
    expect(readAuthorizationLaunchTarget('launch-id', 2_000)).toBe(
      'https://remote.example/oauth/authorize'
    );
  });

  it.each([
    ['an expired record', { url: 'https://remote.example/', createdAt: 0 }],
    ['a script URL', { url: 'javascript:alert(1)', createdAt: AUTHORIZATION_LAUNCH_TTL_MS }],
    ['a record without a URL', { createdAt: AUTHORIZATION_LAUNCH_TTL_MS }]
  ])('rejects %s', (_name, record) => {
    localStorage.setItem(key, JSON.stringify(record));
    expect(() =>
      readAuthorizationLaunchTarget('launch-id', AUTHORIZATION_LAUNCH_TTL_MS + 1)
    ).toThrow();
  });
});
