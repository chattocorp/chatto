import { createHash } from 'node:crypto';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  disablePushOnAllServers,
  enablePushOnAllServers,
  ensureRegistered,
  getPushCapability,
  getPushRegistrationTargets,
  hasSavedPushRegistration,
  isPushDisabledOnThisDevice,
  pushRegistrationFailure,
  retryPushRegistration,
  PUSH_REGISTRATION_REFRESH_INTERVAL_MS,
  refreshPushSubscriptions,
  unsubscribeBeforeLeaving
} from './pushNotifications';
import { onNotificationClick } from './notificationClick';
import {
  notificationRoomTargetFromPathname,
  prepareUiForNotificationPath,
  prepareUiForNotificationTarget
} from './notificationNavigationUi';
import {
  resumePushRegistrationAfterAuthentication,
  takeLegacyDisabledPushRegistration
} from './pushRegistrationCoordinator';

const mocks = vi.hoisted(() => ({
  createPushNotificationAPI: vi.fn(),
  subscribePush: vi.fn(),
  unsubscribePush: vi.fn(),
  deleteByCapabilityPush: vi.fn(),
  appUi: {
    disableRoomCallWideFor: vi.fn()
  },
  segmentToServerId: vi.fn((segment: string) => {
    if (segment === '-') return 'origin';
    if (segment === 'remote.example.com') return 'remote';
    return null;
  }),
  serverIdToSegment: vi.fn((serverId: string) =>
    serverId === 'origin' ? '-' : 'remote.example.com'
  ),
  serverStores: {
    origin: {
      isAuthenticated: true,
      currentUser: { user: { id: 'origin-user' } },
      get accountId(): string | null {
        return this.currentUser.user.id;
      },
      serverInfo: {
        pushNotificationsEnabled: true,
        vapidPublicKey: 'origin-vapid'
      }
    },
    remote: {
      isAuthenticated: true,
      currentUser: { user: { id: 'remote-user' } },
      get accountId(): string | null {
        return this.currentUser.user.id;
      },
      serverInfo: {
        pushNotificationsEnabled: true,
        vapidPublicKey: 'remote-vapid'
      }
    }
  }
}));

vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: {
    servers: [{ id: 'origin' }, { id: 'remote' }],
    isOriginServer: (serverId: string) => serverId === 'origin',
    tryGetStore: (serverId: 'origin' | 'remote') => mocks.serverStores[serverId],
    getServer: (serverId: string) => ({
      id: serverId,
      url: serverId === 'origin' ? 'https://app.test' : 'https://remote.example.com'
    })
  },
  serverConnectionManager: {
    getClient: (serverId: string) => ({
      connectBaseUrl: `https://${serverId}.test/api/connect`,
      bearerToken: `${serverId}-token`,
      getAPI: (factory: (config: never) => unknown) =>
        factory({
          baseUrl: `https://${serverId}.test/api/connect`,
          bearerToken: `${serverId}-token`
        } as never)
    })
  }
}));

vi.mock('$lib/api/pushNotifications', () => ({
  createPushNotificationAPI: mocks.createPushNotificationAPI
}));

vi.mock('$lib/navigation', () => ({
  segmentToServerId: mocks.segmentToServerId,
  serverIdToSegment: mocks.serverIdToSegment
}));

type TestPushSubscription = PushSubscription & {
  unsubscribe: ReturnType<typeof vi.fn>;
};

let permission: NotificationPermission;
let requestPermission: ReturnType<typeof vi.fn>;
let getSubscription: ReturnType<typeof vi.fn>;
let subscribe: ReturnType<typeof vi.fn>;
let legacyGetSubscription: ReturnType<typeof vi.fn>;

const originTarget = { serverId: 'origin', userId: 'origin-user', vapidPublicKey: 'dmFwaWQ' };
const remoteTarget = { serverId: 'remote', userId: 'remote-user', vapidPublicKey: 'dmFwaWQ' };

/** The narrow push scope that the module derives from a server origin. */
function pushScope(serverOrigin: string): string {
  return `/__chatto/push/${createHash('sha256').update(serverOrigin).digest('hex')}/`;
}

function deferred<T = void>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });

  return { promise, resolve, reject };
}

function makeSubscription(endpoint: string): TestPushSubscription {
  return {
    endpoint,
    toJSON: () => ({
      endpoint,
      keys: {
        p256dh: 'p256dh-key',
        auth: 'auth-secret'
      }
    }),
    options: { applicationServerKey: null },
    unsubscribe: vi.fn().mockResolvedValue(true)
  } as unknown as TestPushSubscription;
}

function installPushGlobals() {
  const storage = new Map<string, string>();
  const lockTails = new Map<string, Promise<unknown>>();
  const localStorage = {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => storage.set(key, value),
    removeItem: (key: string) => storage.delete(key)
  };
  vi.stubGlobal('localStorage', localStorage);
  requestPermission = vi.fn(async () => {
    permission = 'granted';
    return permission;
  });
  getSubscription = vi.fn();
  subscribe = vi.fn();
  legacyGetSubscription = vi.fn().mockResolvedValue(null);
  // The origin server's own narrow scope.
  const originRegistration = {
    scope: 'https://app.test' + pushScope('https://app.test'),
    active: {},
    pushManager: {
      getSubscription,
      subscribe
    }
  };
  // The root registration that earlier versions used for the origin server.
  const legacyRootRegistration = {
    scope: 'https://app.test/',
    pushManager: {
      getSubscription: legacyGetSubscription,
      subscribe: vi.fn()
    }
  };

  vi.stubGlobal('Notification', {
    get permission() {
      return permission;
    },
    requestPermission
  });
  vi.stubGlobal('window', {
    Notification,
    PushManager: class PushManager {},
    atob: (value: string) => Buffer.from(value, 'base64').toString('binary'),
    location: { origin: 'https://app.test', host: 'app.test', protocol: 'https:' },
    localStorage
  });
  vi.stubGlobal('navigator', {
    serviceWorker: {
      register: vi.fn().mockResolvedValue(originRegistration),
      getRegistrations: vi.fn().mockResolvedValue([originRegistration, legacyRootRegistration])
    },
    locks: {
      request: vi.fn(<T>(name: string, callback: () => Promise<T> | T): Promise<T> => {
        const previous = lockTails.get(name) ?? Promise.resolve();
        const current = previous.catch(() => undefined).then(callback);
        lockTails.set(name, current);
        return current.finally(() => {
          if (lockTails.get(name) === current) lockTails.delete(name);
        });
      })
    },
    userAgent: 'test-agent'
  });
}

function installCapabilityGlobals(options: {
  userAgent: string;
  platform?: string;
  maxTouchPoints?: number;
  hasPushManager?: boolean;
  hasWebLocks?: boolean;
  hasLocalStorage?: boolean;
  hasNotification?: boolean;
  standalone?: boolean;
  displayModeStandalone?: boolean;
  protocol?: string;
}) {
  const storage = new Map<string, string>();
  const localStorage = {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => storage.set(key, value),
    removeItem: (key: string) => storage.delete(key)
  };
  const notification = {
    permission: 'default',
    requestPermission: vi.fn()
  };
  vi.stubGlobal('Notification', options.hasNotification === false ? undefined : notification);
  vi.stubGlobal('window', {
    ...(options.hasNotification === false ? {} : { Notification: notification }),
    ...(options.hasPushManager === false ? {} : { PushManager: class PushManager {} }),
    ...(options.hasLocalStorage === false ? {} : { localStorage }),
    matchMedia: vi.fn((query: string) => ({
      matches: query === '(display-mode: standalone)' && options.displayModeStandalone === true,
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn()
    })),
    location: { protocol: options.protocol ?? 'https:' }
  });
  vi.stubGlobal('navigator', {
    serviceWorker: {},
    ...(options.hasWebLocks === false ? {} : { locks: {} }),
    userAgent: options.userAgent,
    platform: options.platform ?? '',
    maxTouchPoints: options.maxTouchPoints ?? 0,
    standalone: options.standalone
  });
}

function stubServiceWorker() {
  const listeners = new Set<(event: MessageEvent) => void>();

  vi.stubGlobal('navigator', {
    serviceWorker: {
      addEventListener: vi.fn((type: string, listener: (event: MessageEvent) => void) => {
        if (type === 'message') listeners.add(listener);
      }),
      removeEventListener: vi.fn((type: string, listener: (event: MessageEvent) => void) => {
        if (type === 'message') listeners.delete(listener);
      })
    }
  });

  return {
    dispatchMessage(event: Pick<MessageEvent, 'data' | 'ports'>) {
      for (const listener of listeners) {
        listener(event as MessageEvent);
      }
    },
    listenerCount() {
      return listeners.size;
    }
  };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('pushNotifications.getPushCapability', () => {
  it('returns supported when browser push and durable coordination APIs are available', () => {
    installCapabilityGlobals({
      userAgent: 'Mozilla/5.0 Chrome/125.0',
      platform: 'Linux x86_64'
    });

    expect(getPushCapability()).toBe('supported');
  });

  it('returns ios_home_screen_required for iOS browser context before Home Screen launch', () => {
    installCapabilityGlobals({
      userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15',
      platform: 'iPhone',
      hasPushManager: false,
      hasNotification: false
    });

    expect(getPushCapability()).toBe('ios_home_screen_required');
  });

  it('returns supported for iOS standalone contexts when the Push API is available', () => {
    installCapabilityGlobals({
      userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15',
      platform: 'iPhone',
      standalone: true
    });

    expect(getPushCapability()).toBe('supported');
  });

  it('returns unsupported when a non-iOS browser lacks the Push API', () => {
    installCapabilityGlobals({
      userAgent: 'Mozilla/5.0 Firefox/120.0',
      platform: 'Linux x86_64',
      hasPushManager: false
    });

    expect(getPushCapability()).toBe('unsupported');
  });

  it('returns unsupported when cross-tab registration cannot be serialized', () => {
    installCapabilityGlobals({
      userAgent: 'Mozilla/5.0 Chrome/125.0',
      platform: 'Linux x86_64',
      hasWebLocks: false
    });

    expect(getPushCapability()).toBe('unsupported');
  });

  it('returns unsupported when cross-tab suspension cannot survive a reload', () => {
    installCapabilityGlobals({
      userAgent: 'Mozilla/5.0 Chrome/125.0',
      platform: 'Linux x86_64'
    });
    Object.defineProperty(window, 'localStorage', {
      configurable: true,
      get: () => {
        throw new DOMException('storage denied', 'SecurityError');
      }
    });

    expect(getPushCapability()).toBe('unsupported');
  });

  it('returns unsupported on a native Desktop origin even if Electron exposes browser APIs', () => {
    installCapabilityGlobals({
      userAgent: 'Chatto Desktop',
      platform: 'MacIntel',
      protocol: 'chatto:'
    });

    expect(getPushCapability()).toBe('unsupported');
    expect(getPushRegistrationTargets()).toEqual([]);
  });
});

describe('pushNotifications.getPushRegistrationTargets', () => {
  beforeEach(() => {
    installPushGlobals();
    mocks.serverStores.origin.isAuthenticated = true;
    mocks.serverStores.origin.serverInfo.pushNotificationsEnabled = true;
    mocks.serverStores.remote.isAuthenticated = true;
    mocks.serverStores.remote.currentUser.user.id = 'remote-user';
    mocks.serverStores.remote.serverInfo.pushNotificationsEnabled = true;
  });

  it('includes authenticated origin and remote servers', () => {
    expect(getPushRegistrationTargets()).toEqual([
      { serverId: 'origin', userId: 'origin-user', vapidPublicKey: 'origin-vapid' },
      { serverId: 'remote', userId: 'remote-user', vapidPublicKey: 'remote-vapid' }
    ]);
  });

  it('supports a remote-only authenticated client', () => {
    mocks.serverStores.origin.isAuthenticated = false;

    expect(getPushRegistrationTargets()).toEqual([
      { serverId: 'remote', userId: 'remote-user', vapidPublicKey: 'remote-vapid' }
    ]);
  });
});

function setUpRegistrationTest() {
  {
    permission = 'default';
    installPushGlobals();
    mocks.serverStores.origin.isAuthenticated = true;
    mocks.serverStores.origin.currentUser.user.id = 'origin-user';
    mocks.serverStores.origin.serverInfo.pushNotificationsEnabled = true;
    mocks.serverStores.origin.serverInfo.vapidPublicKey = 'origin-vapid';
    mocks.serverStores.remote.isAuthenticated = true;
    mocks.serverStores.remote.currentUser.user.id = 'remote-user';
    mocks.serverStores.remote.serverInfo.pushNotificationsEnabled = true;
    mocks.serverStores.remote.serverInfo.vapidPublicKey = 'remote-vapid';
    resumePushRegistrationAfterAuthentication('origin');
    resumePushRegistrationAfterAuthentication('remote');
    mocks.createPushNotificationAPI.mockReset();
    mocks.createPushNotificationAPI.mockReturnValue({
      subscribe: mocks.subscribePush,
      unsubscribe: mocks.unsubscribePush,
      deleteByCapability: mocks.deleteByCapabilityPush
    });
    mocks.subscribePush.mockReset();
    mocks.subscribePush.mockResolvedValue({ subscribed: true });
    mocks.unsubscribePush.mockReset();
    mocks.unsubscribePush.mockResolvedValue(true);
    mocks.deleteByCapabilityPush.mockReset();
    mocks.deleteByCapabilityPush.mockResolvedValue(true);
  }
}

describe('pushNotifications.ensureRegistered', () => {
  beforeEach(setUpRegistrationTest);

  it('does not prompt or mutate when permission is default', async () => {
    getSubscription.mockResolvedValue(null);

    await expect(ensureRegistered(originTarget)).resolves.toBe(false);
    expect(requestPermission).not.toHaveBeenCalled();
    expect(getSubscription).not.toHaveBeenCalled();
    expect(subscribe).not.toHaveBeenCalled();
    expect(mocks.subscribePush).not.toHaveBeenCalled();
  });

  it('saves an existing subscription when permission is granted', async () => {
    permission = 'granted';
    const subscription = makeSubscription('https://push.example/existing');
    getSubscription.mockResolvedValue(subscription);

    await expect(ensureRegistered(originTarget)).resolves.toBe(true);
    expect(subscribe).not.toHaveBeenCalled();
    expect(mocks.createPushNotificationAPI).toHaveBeenCalledWith({
      baseUrl: 'https://origin.test/api/connect',
      bearerToken: 'origin-token'
    });
    expect(mocks.subscribePush).toHaveBeenCalledWith(
      {
        endpoint: 'https://push.example/existing',
        p256dh: 'p256dh-key',
        auth: 'auth-secret',
        clientHost: 'app.test',
        cleanupToken: expect.stringMatching(/^[a-f0-9]{32}$/),
        userAgent: 'test-agent'
      },
      { signal: expect.any(AbortSignal) }
    );
  });

  it('creates and saves a subscription when permission is granted and none exists', async () => {
    permission = 'granted';
    const subscription = makeSubscription('https://push.example/created');
    getSubscription.mockResolvedValue(null);
    subscribe.mockResolvedValue(subscription);

    await expect(ensureRegistered(originTarget)).resolves.toBe(true);
    expect(subscribe).toHaveBeenCalledWith({
      userVisibleOnly: true,
      applicationServerKey: expect.any(Uint8Array)
    });
    expect(mocks.subscribePush).toHaveBeenCalledWith(
      expect.objectContaining({
        endpoint: 'https://push.example/created'
      }),
      { signal: expect.any(AbortSignal) }
    );
  });

  it('registers the origin server under its own narrow scope', async () => {
    permission = 'granted';
    getSubscription.mockResolvedValue(makeSubscription('https://push.example/origin-scoped'));

    await expect(ensureRegistered(originTarget)).resolves.toBe(true);

    expect(navigator.serviceWorker.register).toHaveBeenCalledWith('/service-worker.js', {
      scope: pushScope('https://app.test'),
      type: 'module'
    });
    expect(legacyGetSubscription).toHaveBeenCalled();
  });

  it('retires the subscription of the legacy root registration after the origin save', async () => {
    permission = 'granted';
    const legacy = makeSubscription('https://push.example/legacy-root');
    legacyGetSubscription.mockResolvedValue(legacy);
    getSubscription.mockResolvedValue(makeSubscription('https://push.example/origin-current'));

    await expect(ensureRegistered(originTarget)).resolves.toBe(true);

    expect(legacy.unsubscribe).toHaveBeenCalledOnce();
    await vi.waitFor(() =>
      expect(mocks.unsubscribePush).toHaveBeenCalledWith('https://push.example/legacy-root')
    );
    expect(mocks.subscribePush).toHaveBeenCalledWith(
      expect.objectContaining({ endpoint: 'https://push.example/origin-current' }),
      { signal: expect.any(AbortSignal) }
    );
  });

  it('removes the legacy root subscription when leaving the origin server', async () => {
    permission = 'granted';
    const current = makeSubscription('https://push.example/origin-leaving');
    const legacy = makeSubscription('https://push.example/legacy-leaving');
    getSubscription.mockResolvedValue(current);
    legacyGetSubscription.mockResolvedValue(legacy);

    await expect(unsubscribeBeforeLeaving('origin')).resolves.toBeUndefined();

    expect(current.unsubscribe).toHaveBeenCalledOnce();
    expect(legacy.unsubscribe).toHaveBeenCalledOnce();
    await vi.waitFor(() => expect(mocks.unsubscribePush).toHaveBeenCalledTimes(2));
  });

  it('uses a dedicated service worker scope and stores the client host for a remote server', async () => {
    permission = 'granted';
    const remoteSubscription = makeSubscription('https://push.example/remote');
    const remoteGetSubscription = vi.fn().mockResolvedValue(null);
    const remoteSubscribe = vi.fn().mockResolvedValue(remoteSubscription);
    const register = vi.fn().mockResolvedValue({
      active: {},
      pushManager: {
        getSubscription: remoteGetSubscription,
        subscribe: remoteSubscribe
      }
    });
    Object.assign(navigator.serviceWorker, { register });

    await expect(ensureRegistered(remoteTarget)).resolves.toBe(true);

    expect(register).toHaveBeenCalledWith('/service-worker.js', {
      scope: expect.stringMatching(/^\/__chatto\/push\/[a-f0-9]{64}\/$/),
      type: 'module'
    });
    expect(mocks.createPushNotificationAPI).toHaveBeenCalledWith({
      baseUrl: 'https://remote.test/api/connect',
      bearerToken: 'remote-token'
    });
    expect(mocks.subscribePush).toHaveBeenCalledWith(
      expect.objectContaining({
        endpoint: 'https://push.example/remote',
        clientHost: 'app.test'
      }),
      { signal: expect.any(AbortSignal) }
    );
  });

  it('serializes concurrent registration refreshes for the same server', async () => {
    permission = 'granted';
    const remoteSubscription = makeSubscription('https://push.example/remote-serialized');
    const register = vi.fn().mockResolvedValue({
      active: {},
      pushManager: {
        getSubscription: vi.fn().mockResolvedValue(remoteSubscription),
        subscribe: vi.fn()
      }
    });
    Object.assign(navigator.serviceWorker, { register });
    const firstSave = deferred<{ subscribed: boolean }>();
    mocks.subscribePush
      .mockReturnValueOnce(firstSave.promise)
      .mockResolvedValue({ subscribed: true });

    const first = ensureRegistered(remoteTarget);
    await vi.waitFor(() => expect(mocks.subscribePush).toHaveBeenCalledOnce());
    const second = ensureRegistered(remoteTarget);
    await Promise.resolve();
    expect(mocks.subscribePush).toHaveBeenCalledOnce();

    firstSave.resolve({ subscribed: true });
    await expect(first).resolves.toBe(true);
    await expect(second).resolves.toBe(true);
    expect(mocks.subscribePush).toHaveBeenCalledTimes(2);
  });

  it('aborts an unbounded active refresh, cancels queued work, and cleans up before leaving', async () => {
    permission = 'granted';
    const remoteSubscription = makeSubscription('https://push.example/remote-leaving-race');
    const firstSave = deferred<{ subscribed: boolean }>();
    const remoteGetSubscription = vi
      .fn()
      .mockResolvedValueOnce(null)
      .mockResolvedValue(remoteSubscription);
    const remoteRegistration = {
      scope: '',
      active: {},
      pushManager: {
        getSubscription: remoteGetSubscription,
        subscribe: vi.fn().mockResolvedValue(remoteSubscription)
      }
    };
    const register = vi.fn(async (_script: string, options: RegistrationOptions) => {
      remoteRegistration.scope = new URL(options.scope ?? '/', window.location.origin).toString();
      return remoteRegistration;
    });
    Object.assign(navigator.serviceWorker, {
      register,
      getRegistrations: vi.fn().mockImplementation(async () => [remoteRegistration])
    });
    mocks.subscribePush.mockReturnValueOnce(firstSave.promise);

    const activeRefresh = ensureRegistered(remoteTarget);
    await vi.waitFor(() => expect(mocks.subscribePush).toHaveBeenCalledOnce());
    const queuedRefresh = ensureRegistered(remoteTarget);
    const leaving = unsubscribeBeforeLeaving('remote');

    await expect(activeRefresh).resolves.toBe(false);
    await expect(queuedRefresh).resolves.toBe(false);
    await expect(leaving).resolves.toBeUndefined();

    expect(mocks.subscribePush).toHaveBeenCalledOnce();
    expect(mocks.subscribePush.mock.calls[0]?.[1]?.signal.aborted).toBe(true);
    expect(remoteSubscription.unsubscribe).toHaveBeenCalledOnce();
    expect(mocks.unsubscribePush).toHaveBeenCalledWith(remoteSubscription.endpoint);

    remoteSubscription.unsubscribe.mockClear();
    mocks.unsubscribePush.mockClear();
    // A different tab installs new authentication and clears shared suspension;
    // this realm intentionally retains its local block and obsolete credentials.
    window.localStorage.removeItem('chatto.push-registration.suspended.remote');
    await expect(ensureRegistered(remoteTarget)).resolves.toBe(false);

    // A transport that ignores abort may settle after a new session starts.
    // Its stale continuation must delete only the obsolete account's server
    // record without invalidating the other tab's browser replacement.
    firstSave.resolve({ subscribed: true });
    await firstSave.promise;
    await vi.waitFor(() => expect(mocks.deleteByCapabilityPush).toHaveBeenCalledOnce());
    expect(remoteSubscription.unsubscribe).not.toHaveBeenCalled();
    expect(mocks.unsubscribePush).not.toHaveBeenCalled();
    expect(mocks.deleteByCapabilityPush).toHaveBeenCalledWith(
      remoteSubscription.endpoint,
      'auth-secret',
      mocks.subscribePush.mock.calls[0]?.[0].cleanupToken
    );

    const refreshKey = 'chatto.push-registration.refresh.remote';
    expect(window.localStorage.getItem(refreshKey)).toEqual(expect.any(String));
    resumePushRegistrationAfterAuthentication('remote');
    await refreshPushSubscriptions();
    expect(window.localStorage.getItem(refreshKey)).toBeNull();
  });

  it('keeps leaving suspension visible across tabs until new authentication is installed', async () => {
    permission = 'granted';
    const remoteSubscription = makeSubscription('https://push.example/remote-reauthenticated');
    const register = vi.fn().mockResolvedValue({
      active: {},
      pushManager: {
        getSubscription: vi.fn().mockResolvedValue(remoteSubscription),
        subscribe: vi.fn()
      }
    });
    Object.assign(navigator.serviceWorker, { register });

    await expect(unsubscribeBeforeLeaving('remote')).resolves.toBeUndefined();
    expect(window.localStorage.getItem('chatto.push-registration.suspended.remote')).toBe(
      'leaving'
    );

    await expect(ensureRegistered(remoteTarget)).resolves.toBe(false);
    expect(requestPermission).not.toHaveBeenCalled();
    expect(register).not.toHaveBeenCalled();
    expect(mocks.subscribePush).not.toHaveBeenCalled();

    resumePushRegistrationAfterAuthentication('remote');

    await expect(ensureRegistered(remoteTarget)).resolves.toBe(true);
    expect(mocks.subscribePush).toHaveBeenCalledOnce();
  });

  it("does not let delayed leaving cleanup remove another tab's replacement", async () => {
    permission = 'granted';
    const replacement = makeSubscription('https://push.example/cross-tab-replacement');
    const delayedLookup = deferred<PushSubscription | null>();
    getSubscription.mockReturnValue(delayedLookup.promise);

    const leaving = unsubscribeBeforeLeaving('origin');
    await Promise.resolve();
    // Another realm installs authentication and the replacement while this
    // realm's shared-subscription lookup is still pending.
    window.localStorage.removeItem('chatto.push-registration.suspended.origin');
    delayedLookup.resolve(replacement);

    await expect(leaving).resolves.toBeUndefined();
    expect(replacement.unsubscribe).not.toHaveBeenCalled();
    expect(mocks.unsubscribePush).not.toHaveBeenCalled();
  });

  it('holds the cross-tab lock until browser cleanup hands off to reauthentication', async () => {
    permission = 'granted';
    const existing = makeSubscription('https://push.example/cross-tab-lock');
    const browserCleanup = deferred<boolean>();
    existing.unsubscribe.mockReturnValue(browserCleanup.promise);
    getSubscription.mockResolvedValue(existing);

    const leaving = unsubscribeBeforeLeaving('origin');
    await vi.waitFor(() => expect(existing.unsubscribe).toHaveBeenCalledOnce());

    window.localStorage.removeItem('chatto.push-registration.suspended.origin');
    let replacementStarted = false;
    const replacement = navigator.locks.request('chatto.push-registration.origin', async () => {
      replacementStarted = true;
    });
    await Promise.resolve();
    expect(replacementStarted).toBe(false);

    browserCleanup.resolve(true);
    await expect(leaving).resolves.toBeUndefined();
    await replacement;

    expect(replacementStarted).toBe(true);
    expect(mocks.unsubscribePush).not.toHaveBeenCalled();
  });

  it('keeps an existing remote subscription when saving it fails', async () => {
    permission = 'granted';
    const remoteSubscription = makeSubscription('https://push.example/remote-failed-response');
    const register = vi.fn().mockResolvedValue({
      active: {},
      pushManager: {
        getSubscription: vi.fn().mockResolvedValue(remoteSubscription),
        subscribe: vi.fn()
      }
    });
    Object.assign(navigator.serviceWorker, { register });
    mocks.subscribePush.mockRejectedValueOnce(new Error('response lost'));

    await expect(ensureRegistered(remoteTarget)).resolves.toBe(false);

    expect(mocks.unsubscribePush).not.toHaveBeenCalled();
    expect(remoteSubscription.unsubscribe).not.toHaveBeenCalled();
  });

  it('prompts once and registers the origin and remote servers', async () => {
    const originSubscription = makeSubscription('https://push.example/origin-all');
    const remoteSubscription = makeSubscription('https://push.example/remote-all');
    getSubscription.mockResolvedValue(null);
    subscribe.mockResolvedValue(originSubscription);
    const register = vi.fn().mockResolvedValue({
      active: {},
      pushManager: {
        getSubscription: vi.fn().mockResolvedValue(null),
        subscribe: vi.fn().mockResolvedValue(remoteSubscription)
      }
    });
    Object.assign(navigator.serviceWorker, { register });

    await expect(enablePushOnAllServers()).resolves.toEqual({
      permission: 'granted',
      registrations: [
        expect.objectContaining({ serverId: 'origin', registered: true }),
        expect.objectContaining({ serverId: 'remote', registered: true })
      ]
    });
    expect(requestPermission).toHaveBeenCalledOnce();
    expect(mocks.subscribePush).toHaveBeenCalledTimes(2);
  });

  it('registers a remote-only client through the same enable operation', async () => {
    mocks.serverStores.origin.isAuthenticated = false;
    const remoteSubscription = makeSubscription('https://push.example/remote-only');
    Object.assign(navigator.serviceWorker, {
      register: vi.fn().mockResolvedValue({
        active: {},
        pushManager: {
          getSubscription: vi.fn().mockResolvedValue(null),
          subscribe: vi.fn().mockResolvedValue(remoteSubscription)
        }
      })
    });

    const result = await enablePushOnAllServers();
    expect(result.registrations).toEqual([
      expect.objectContaining({ serverId: 'remote', registered: true })
    ]);
    expect(requestPermission).toHaveBeenCalledOnce();
  });

  it('does not attempt remaining servers when permission is denied', async () => {
    requestPermission.mockImplementationOnce(async () => {
      permission = 'denied';
      return permission;
    });

    const result = await enablePushOnAllServers();
    expect(result).toEqual({
      permission: 'denied',
      registrations: [
        expect.objectContaining({ serverId: 'origin', registered: false }),
        expect.objectContaining({ serverId: 'remote', registered: false })
      ]
    });
    expect(mocks.subscribePush).not.toHaveBeenCalled();
  });

  it('shares one permission request across concurrent enable operations', async () => {
    const permissionChoice = deferred<NotificationPermission>();
    requestPermission.mockImplementationOnce(() => permissionChoice.promise);

    const first = enablePushOnAllServers();
    const second = enablePushOnAllServers();
    expect(first).toBe(second);
    expect(requestPermission).toHaveBeenCalledOnce();

    permission = 'denied';
    permissionChoice.resolve('denied');
    await expect(first).resolves.toEqual({
      permission: 'denied',
      registrations: [
        expect.objectContaining({ serverId: 'origin', registered: false }),
        expect.objectContaining({ serverId: 'remote', registered: false })
      ]
    });
  });

  it('registers the current server list after the permission prompt closes', async () => {
    mocks.serverStores.remote.serverInfo.pushNotificationsEnabled = false;
    const permissionChoice = deferred<NotificationPermission>();
    requestPermission.mockImplementationOnce(() => permissionChoice.promise);
    const remoteSubscription = makeSubscription('https://push.example/remote-added');
    Object.assign(navigator.serviceWorker, {
      register: vi.fn().mockResolvedValue({
        active: {},
        pushManager: {
          getSubscription: vi.fn().mockResolvedValue(null),
          subscribe: vi.fn().mockResolvedValue(remoteSubscription)
        }
      })
    });

    const enabling = enablePushOnAllServers();
    mocks.serverStores.origin.isAuthenticated = false;
    mocks.serverStores.remote.serverInfo.pushNotificationsEnabled = true;
    mocks.serverStores.remote.currentUser.user.id = 'new-remote-user';
    mocks.serverStores.remote.serverInfo.vapidPublicKey = 'new-remote-vapid';
    permission = 'granted';
    permissionChoice.resolve('granted');

    await expect(enabling).resolves.toEqual({
      permission: 'granted',
      registrations: [
        expect.objectContaining({
          serverId: 'remote',
          userId: 'new-remote-user',
          vapidPublicKey: 'new-remote-vapid',
          registered: true
        })
      ]
    });
    expect(mocks.createPushNotificationAPI).not.toHaveBeenCalledWith(
      expect.objectContaining({ baseUrl: 'https://origin.test/api/connect' })
    );
  });

  it('reconciles targets added while explicit activation is still registering', async () => {
    mocks.serverStores.remote.serverInfo.pushNotificationsEnabled = false;
    const permissionChoice = deferred<NotificationPermission>();
    const originSave = deferred<{ subscribed: boolean }>();
    requestPermission.mockImplementationOnce(() => permissionChoice.promise);
    getSubscription.mockResolvedValue(makeSubscription('https://push.example/origin-race'));
    mocks.subscribePush
      .mockReturnValueOnce(originSave.promise)
      .mockResolvedValue({ subscribed: true });
    const remoteSubscription = makeSubscription('https://push.example/remote-race');
    Object.assign(navigator.serviceWorker, {
      register: vi.fn().mockResolvedValue({
        active: {},
        pushManager: {
          getSubscription: vi.fn().mockResolvedValue(remoteSubscription),
          subscribe: vi.fn()
        }
      })
    });

    const enabling = enablePushOnAllServers();
    permission = 'granted';
    permissionChoice.resolve('granted');
    await vi.waitFor(() => expect(mocks.subscribePush).toHaveBeenCalledOnce());

    mocks.serverStores.remote.serverInfo.pushNotificationsEnabled = true;
    const refreshing = refreshPushSubscriptions();
    originSave.resolve({ subscribed: true });

    await expect(enabling).resolves.toEqual(expect.objectContaining({ permission: 'granted' }));
    await expect(refreshing).resolves.toBeUndefined();
    expect(mocks.createPushNotificationAPI).toHaveBeenCalledWith({
      baseUrl: 'https://remote.test/api/connect',
      bearerToken: 'remote-token'
    });
  });

  it('reports all targets as failed when the permission request rejects', async () => {
    requestPermission.mockRejectedValueOnce(new Error('permission prompt unavailable'));

    await expect(enablePushOnAllServers()).resolves.toEqual({
      permission: 'default',
      registrations: [
        expect.objectContaining({ serverId: 'origin', registered: false }),
        expect.objectContaining({ serverId: 'remote', registered: false })
      ]
    });
    expect(mocks.subscribePush).not.toHaveBeenCalled();
  });

  it('reports a remote registration failure without losing the origin success', async () => {
    const originSubscription = makeSubscription('https://push.example/origin-partial');
    const remoteSubscription = makeSubscription('https://push.example/remote-partial');
    getSubscription.mockResolvedValue(null);
    subscribe.mockResolvedValue(originSubscription);
    Object.assign(navigator.serviceWorker, {
      register: vi.fn().mockResolvedValue({
        active: {},
        pushManager: {
          getSubscription: vi.fn().mockResolvedValue(null),
          subscribe: vi.fn().mockResolvedValue(remoteSubscription)
        }
      })
    });
    mocks.subscribePush
      .mockResolvedValueOnce({ subscribed: true })
      .mockResolvedValueOnce({ subscribed: false });

    const result = await enablePushOnAllServers();
    expect(result.registrations).toEqual([
      expect.objectContaining({ serverId: 'origin', registered: true }),
      expect.objectContaining({ serverId: 'remote', registered: false })
    ]);
  });

  it('keeps a newly created subscription when the server save fails', async () => {
    permission = 'granted';
    const createdSubscription = makeSubscription('https://push.example/created');
    getSubscription.mockResolvedValueOnce(null);
    subscribe.mockResolvedValueOnce(createdSubscription);
    mocks.subscribePush.mockResolvedValueOnce({ subscribed: false });

    await expect(ensureRegistered(originTarget)).resolves.toBe(false);
    // The next attempt saves the same subscription instead of creating
    // another push service registration.
    expect(createdSubscription.unsubscribe).not.toHaveBeenCalled();
    expect(mocks.unsubscribePush).not.toHaveBeenCalled();
  });

  it('still performs local leaving cleanup when browser storage is denied', async () => {
    permission = 'granted';
    const subscription = makeSubscription('https://push.example/storage-denied');
    getSubscription.mockResolvedValue(subscription);
    Object.defineProperty(window, 'localStorage', {
      configurable: true,
      get: () => {
        throw new DOMException('storage denied', 'SecurityError');
      }
    });

    await expect(unsubscribeBeforeLeaving('origin')).resolves.toBeUndefined();

    expect(subscription.unsubscribe).toHaveBeenCalledOnce();
    expect(mocks.unsubscribePush).toHaveBeenCalledWith(subscription.endpoint);
  });

  it('keeps leaving incomplete when service-worker lookup fails and succeeds on retry', async () => {
    permission = 'granted';
    const subscription = makeSubscription('https://push.example/lookup-retry');
    const getRegistrations = vi.mocked(navigator.serviceWorker.getRegistrations);
    getRegistrations.mockRejectedValueOnce(new DOMException('worker database unavailable'));
    getSubscription.mockResolvedValue(subscription);

    await expect(unsubscribeBeforeLeaving('origin')).rejects.toThrow('worker database unavailable');
    expect(subscription.unsubscribe).not.toHaveBeenCalled();
    expect(mocks.unsubscribePush).not.toHaveBeenCalled();

    await expect(unsubscribeBeforeLeaving('origin')).resolves.toBeUndefined();
    expect(subscription.unsubscribe).toHaveBeenCalledOnce();
    expect(mocks.unsubscribePush).toHaveBeenCalledWith(subscription.endpoint);
  });

  it('keeps leaving incomplete when PushManager lookup fails', async () => {
    permission = 'granted';
    getSubscription.mockRejectedValueOnce(new DOMException('push database unavailable'));

    await expect(unsubscribeBeforeLeaving('origin')).rejects.toThrow('push database unavailable');
    expect(mocks.unsubscribePush).not.toHaveBeenCalled();
  });

  it('waits for server deletion when browser invalidation fails', async () => {
    permission = 'granted';
    const subscription = makeSubscription('https://push.example/browser-cleanup-failed');
    subscription.unsubscribe.mockResolvedValue(false);
    const serverCleanup = deferred<boolean>();
    getSubscription.mockResolvedValue(subscription);
    mocks.unsubscribePush.mockReturnValueOnce(serverCleanup.promise);

    let left = false;
    const leaving = unsubscribeBeforeLeaving('origin').then(() => {
      left = true;
    });
    await vi.waitFor(() => expect(mocks.unsubscribePush).toHaveBeenCalledOnce());
    expect(left).toBe(false);

    serverCleanup.resolve(true);
    await leaving;
    expect(left).toBe(true);
  });

  it('keeps leaving incomplete when neither browser nor server delivery can be disabled', async () => {
    permission = 'granted';
    const subscription = makeSubscription('https://push.example/cleanup-unavailable');
    subscription.unsubscribe.mockRejectedValueOnce(new DOMException('push database unavailable'));
    getSubscription.mockResolvedValue(subscription);
    mocks.unsubscribePush.mockRejectedValueOnce(new Error('server unavailable'));

    await expect(unsubscribeBeforeLeaving('origin')).rejects.toThrow(
      'Push delivery could not be disabled before leaving the server'
    );
  });

  it('finishes local invalidation before leaving without waiting for server cleanup', async () => {
    permission = 'granted';
    const subscription = makeSubscription('https://push.example/leaving');
    getSubscription.mockResolvedValue(subscription);
    mocks.unsubscribePush.mockReturnValueOnce(new Promise<boolean>(() => {}));

    await expect(unsubscribeBeforeLeaving('origin')).resolves.toBeUndefined();

    expect(subscription.unsubscribe).toHaveBeenCalledOnce();
    expect(mocks.unsubscribePush).toHaveBeenCalledWith(subscription.endpoint);
    expect(subscription.unsubscribe.mock.invocationCallOrder[0]).toBeLessThan(
      mocks.unsubscribePush.mock.invocationCallOrder[0]
    );
  });
});

describe('pushNotifications device-wide opt-out', () => {
  beforeEach(() => {
    setUpRegistrationTest();
    permission = 'granted';
  });

  it('turns push off for every server on this device', async () => {
    const subscription = makeSubscription('https://push.example/device');
    getSubscription.mockResolvedValue(subscription);
    await expect(ensureRegistered(originTarget)).resolves.toBe(true);
    mocks.subscribePush.mockClear();

    await disablePushOnAllServers();

    expect(isPushDisabledOnThisDevice()).toBe(true);
    expect(getPushRegistrationTargets()).toEqual([]);
    expect(hasSavedPushRegistration('origin', 'origin-user')).toBe(false);
    expect(subscription.unsubscribe).toHaveBeenCalled();
    expect(mocks.unsubscribePush).toHaveBeenCalledWith('https://push.example/device');

    await refreshPushSubscriptions();
    await expect(ensureRegistered(originTarget)).resolves.toBe(false);
    expect(mocks.subscribePush).not.toHaveBeenCalled();
  });

  it('turns push on again for every server from Enable', async () => {
    getSubscription.mockResolvedValue(makeSubscription('https://push.example/device'));
    await disablePushOnAllServers();
    mocks.subscribePush.mockClear();

    const result = await enablePushOnAllServers();

    expect(isPushDisabledOnThisDevice()).toBe(false);
    expect(result.registrations).toEqual([
      expect.objectContaining({ serverId: 'origin', registered: true }),
      expect.objectContaining({ serverId: 'remote', registered: true })
    ]);
    expect(mocks.subscribePush).toHaveBeenCalledTimes(2);
  });

  it('stops a registration in progress when another tab turns push off', async () => {
    const subscription = makeSubscription('https://push.example/in-progress');
    getSubscription.mockResolvedValue(subscription);
    const save = deferred<{ subscribed: boolean }>();
    mocks.subscribePush.mockReturnValueOnce(save.promise);

    const registration = ensureRegistered(originTarget);
    await vi.waitFor(() => expect(mocks.subscribePush).toHaveBeenCalledOnce());
    // Another tab stores the opt-out while this tab waits for the server.
    window.localStorage.setItem('chatto.push-registration.disabled', '1');
    save.resolve({ subscribed: true });

    await expect(registration).resolves.toBe(false);
    expect(subscription.unsubscribe).toHaveBeenCalled();
    expect(mocks.unsubscribePush).toHaveBeenCalledWith('https://push.example/in-progress');
  });

  it('keeps an earlier opt-out for a single server as a device-wide opt-out', () => {
    const storage = new Map<string, string>([
      ['chatto.push-registration.suspended.origin', 'disabled'],
      ['chatto.push-registration.suspended.remote', 'leaving']
    ]);
    vi.stubGlobal('window', {
      localStorage: {
        get length() {
          return storage.size;
        },
        key: (index: number) => [...storage.keys()][index] ?? null,
        getItem: (key: string) => storage.get(key) ?? null,
        removeItem: (key: string) => storage.delete(key)
      }
    });

    expect(takeLegacyDisabledPushRegistration()).toBe(true);
    expect([...storage.keys()]).toEqual(['chatto.push-registration.suspended.remote']);
    expect(takeLegacyDisabledPushRegistration()).toBe(false);
  });
});

describe('pushNotifications.refreshPushSubscriptions', () => {
  const start = new Date('2026-10-01T12:00:00Z');

  beforeEach(async () => {
    setUpRegistrationTest();
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(start);
    permission = 'granted';
    getSubscription.mockResolvedValue(makeSubscription('https://push.example/refresh'));
    // Both servers share one PushManager fixture; the remote scope must also
    // exist so the refresh can compare its browser subscription.
    const registrations = await navigator.serviceWorker.getRegistrations();
    vi.mocked(navigator.serviceWorker.getRegistrations).mockResolvedValue([
      ...registrations,
      {
        ...registrations[0],
        scope: 'https://app.test' + pushScope('https://remote.example.com')
      } as ServiceWorkerRegistration
    ]);
    // Start each test from a known save time for every eligible server.
    for (const target of getPushRegistrationTargets()) {
      await expect(ensureRegistered(target)).resolves.toBe(true);
    }
    mocks.subscribePush.mockClear();
    mocks.createPushNotificationAPI.mockClear();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('does not save again within the refresh interval', async () => {
    vi.setSystemTime(start.getTime() + PUSH_REGISTRATION_REFRESH_INTERVAL_MS - 1);

    await refreshPushSubscriptions();

    expect(mocks.subscribePush).not.toHaveBeenCalled();
  });

  it('saves every server again once the refresh interval has passed', async () => {
    vi.setSystemTime(start.getTime() + PUSH_REGISTRATION_REFRESH_INTERVAL_MS);

    await refreshPushSubscriptions();

    expect(mocks.subscribePush).toHaveBeenCalledTimes(2);
  });

  it('saves again when the browser replaced the subscription', async () => {
    getSubscription.mockResolvedValue(makeSubscription('https://push.example/replaced'));

    await refreshPushSubscriptions();

    expect(mocks.subscribePush).toHaveBeenCalledTimes(2);
    expect(mocks.subscribePush).toHaveBeenCalledWith(
      expect.objectContaining({ endpoint: 'https://push.example/replaced' }),
      { signal: expect.any(AbortSignal) }
    );
  });

  it('logs why it saves a subscription', async () => {
    const debug = vi.spyOn(console, 'debug').mockImplementation(() => {});
    try {
      vi.setSystemTime(start.getTime() + PUSH_REGISTRATION_REFRESH_INTERVAL_MS);

      await refreshPushSubscriptions();

      expect(debug).toHaveBeenCalledWith('[push] Refreshing push subscription', {
        serverId: 'origin',
        reason: 'refresh-interval-elapsed'
      });
      expect(debug).toHaveBeenCalledWith('[push] Push subscription refresh finished', {
        serverId: 'origin',
        reason: 'refresh-interval-elapsed',
        registered: true
      });
    } finally {
      debug.mockRestore();
    }
  });

  it('saves a server that becomes eligible later without asking again', async () => {
    mocks.serverStores.remote.isAuthenticated = false;
    await unsubscribeBeforeLeaving('remote');
    mocks.subscribePush.mockClear();

    // The account signs in to the server again, for example after it was added.
    resumePushRegistrationAfterAuthentication('remote');
    mocks.serverStores.remote.isAuthenticated = true;
    await refreshPushSubscriptions();

    expect(requestPermission).not.toHaveBeenCalled();
    expect(mocks.subscribePush).toHaveBeenCalledOnce();
    expect(mocks.createPushNotificationAPI).toHaveBeenLastCalledWith({
      baseUrl: 'https://remote.test/api/connect',
      bearerToken: 'remote-token'
    });
  });

  it('saves a server again when its account changes', async () => {
    mocks.serverStores.remote.currentUser.user.id = 'another-remote-user';

    await refreshPushSubscriptions();

    expect(mocks.subscribePush).toHaveBeenCalledOnce();
    expect(mocks.createPushNotificationAPI).toHaveBeenLastCalledWith({
      baseUrl: 'https://remote.test/api/connect',
      bearerToken: 'remote-token'
    });
  });

  it('saves each server once when refreshes overlap', async () => {
    vi.setSystemTime(start.getTime() + PUSH_REGISTRATION_REFRESH_INTERVAL_MS);

    await Promise.all([refreshPushSubscriptions(), refreshPushSubscriptions()]);

    expect(mocks.subscribePush).toHaveBeenCalledTimes(2);
  });

  it('reports which account this page saved for each server', async () => {
    expect(hasSavedPushRegistration('origin', 'origin-user')).toBe(true);
    expect(hasSavedPushRegistration('origin', 'another-user')).toBe(false);

    await unsubscribeBeforeLeaving('origin');

    expect(hasSavedPushRegistration('origin', 'origin-user')).toBe(false);
  });

  it('reports a failed save until a later save succeeds', async () => {
    const originSubscribe = vi
      .fn()
      .mockRejectedValueOnce(new Error('rejected'))
      .mockResolvedValue({ subscribed: true });
    mocks.createPushNotificationAPI.mockImplementation((config: { baseUrl: string }) => ({
      subscribe: config.baseUrl.includes('origin') ? originSubscribe : mocks.subscribePush,
      unsubscribe: mocks.unsubscribePush,
      deleteByCapability: mocks.deleteByCapabilityPush
    }));
    vi.setSystemTime(start.getTime() + PUSH_REGISTRATION_REFRESH_INTERVAL_MS);

    await refreshPushSubscriptions();
    expect(pushRegistrationFailure('remote')).toBeNull();
    expect(pushRegistrationFailure('origin')).toBe('Error: rejected');

    await retryPushRegistration('origin');
    expect(pushRegistrationFailure('origin')).toBeNull();
    expect(hasSavedPushRegistration('origin', 'origin-user')).toBe(true);
  });

  it('retries a failed save on the next check without a new browser subscription', async () => {
    const created = makeSubscription('https://push.example/created');
    getSubscription.mockResolvedValueOnce(null).mockResolvedValue(created);
    subscribe.mockResolvedValueOnce(created);
    const originSubscribe = vi
      .fn()
      .mockRejectedValueOnce(new Error('unavailable'))
      .mockResolvedValue({ subscribed: true });
    mocks.createPushNotificationAPI.mockImplementation((config: { baseUrl: string }) => ({
      subscribe: config.baseUrl.includes('origin') ? originSubscribe : mocks.subscribePush,
      unsubscribe: mocks.unsubscribePush,
      deleteByCapability: mocks.deleteByCapabilityPush
    }));
    vi.setSystemTime(start.getTime() + PUSH_REGISTRATION_REFRESH_INTERVAL_MS);

    await refreshPushSubscriptions();
    expect(pushRegistrationFailure('origin')).toBe('Error: unavailable');

    // A focus or hourly check soon after tries the save again.
    vi.setSystemTime(start.getTime() + PUSH_REGISTRATION_REFRESH_INTERVAL_MS + 60_000);
    await refreshPushSubscriptions();

    expect(originSubscribe).toHaveBeenCalledTimes(2);
    expect(originSubscribe).toHaveBeenLastCalledWith(
      expect.objectContaining({ endpoint: 'https://push.example/created' }),
      { signal: expect.any(AbortSignal) }
    );
    expect(subscribe).toHaveBeenCalledOnce();
    expect(pushRegistrationFailure('origin')).toBeNull();
  });

  it('stops creating browser subscriptions after a failed browser subscribe', async () => {
    getSubscription.mockResolvedValue(null);
    subscribe.mockRejectedValue(
      new DOMException('Registration failed - push service error', 'AbortError')
    );
    vi.setSystemTime(start.getTime() + PUSH_REGISTRATION_REFRESH_INTERVAL_MS);

    await refreshPushSubscriptions();
    await refreshPushSubscriptions();

    // Both servers need a new subscription on both checks, but only the first
    // attempt asks the browser; the others fail without another registration.
    expect(subscribe).toHaveBeenCalledOnce();
    expect(pushRegistrationFailure('origin')).toContain('push service error');
    expect(pushRegistrationFailure('remote')).toContain('push service error');

    // An explicit retry still asks the browser and lifts the stop on success.
    const recovered = makeSubscription('https://push.example/recovered');
    subscribe.mockResolvedValue(recovered);
    await expect(retryPushRegistration('origin')).resolves.toBe(true);
    expect(subscribe).toHaveBeenCalledTimes(2);
    expect(pushRegistrationFailure('origin')).toBeNull();

    // The servers share one PushManager fixture, so the next check finds the
    // recovered subscription for the other server too and only saves it.
    getSubscription.mockResolvedValue(recovered);
    await refreshPushSubscriptions();
    expect(subscribe).toHaveBeenCalledTimes(2);
    expect(pushRegistrationFailure('remote')).toBeNull();
  });

  it('lets Enable create a browser subscription after a failed attempt', async () => {
    getSubscription.mockResolvedValue(null);
    subscribe.mockRejectedValueOnce(
      new DOMException('Registration failed - push service error', 'AbortError')
    );
    vi.setSystemTime(start.getTime() + PUSH_REGISTRATION_REFRESH_INTERVAL_MS);
    await refreshPushSubscriptions();
    expect(subscribe).toHaveBeenCalledOnce();

    subscribe.mockResolvedValue(makeSubscription('https://push.example/enabled'));
    const result = await enablePushOnAllServers();

    expect(result.registrations.every((registration) => registration.registered)).toBe(true);
    expect(subscribe).toHaveBeenCalledTimes(3);
  });

  it('never asks for permission', async () => {
    permission = 'default';
    vi.setSystemTime(start.getTime() + PUSH_REGISTRATION_REFRESH_INTERVAL_MS);

    await refreshPushSubscriptions();

    expect(requestPermission).not.toHaveBeenCalled();
    expect(mocks.subscribePush).not.toHaveBeenCalled();
  });
});

describe('notification navigation UI routing', () => {
  beforeEach(() => {
    mocks.appUi.disableRoomCallWideFor.mockClear();
    mocks.segmentToServerId.mockClear();
  });

  it('extracts the server and room target from chat room paths', () => {
    expect(notificationRoomTargetFromPathname('/chat/-/room-1/thread-1')).toEqual({
      serverId: 'origin',
      roomId: 'room-1'
    });
    expect(notificationRoomTargetFromPathname('/chat/remote.example.com/room%202')).toEqual({
      serverId: 'remote',
      roomId: 'room 2'
    });
  });

  it('prepares shared UI state for notification room paths', () => {
    prepareUiForNotificationPath(mocks.appUi, '/chat/-/room-1');

    expect(mocks.appUi.disableRoomCallWideFor).toHaveBeenCalledWith('origin', 'room-1');
  });

  it('prepares shared UI state for notification targets', () => {
    prepareUiForNotificationTarget(mocks.appUi, 'origin', { roomId: 'room-1' });

    expect(mocks.appUi.disableRoomCallWideFor).toHaveBeenCalledWith('origin', 'room-1');
  });

  it('ignores non-room notification paths', () => {
    prepareUiForNotificationPath(mocks.appUi, '/chat/notifications');
    prepareUiForNotificationPath(mocks.appUi, '/settings');

    expect(mocks.appUi.disableRoomCallWideFor).not.toHaveBeenCalled();
  });
});

describe('onNotificationClick', () => {
  it('acknowledges after the notification callback completes', async () => {
    const serviceWorker = stubServiceWorker();
    const navigation = deferred();
    const callback = vi.fn(() => navigation.promise);
    const responsePort = { postMessage: vi.fn() };
    const stop = onNotificationClick(callback);

    serviceWorker.dispatchMessage({
      data: {
        type: 'notification-click',
        url: 'https://chatto.example/chat/-/room-1'
      },
      ports: [responsePort as unknown as MessagePort]
    });

    await Promise.resolve();
    expect(callback).toHaveBeenCalledWith('https://chatto.example/chat/-/room-1');
    expect(responsePort.postMessage).not.toHaveBeenCalled();

    navigation.resolve();
    await navigation.promise;
    await Promise.resolve();

    expect(responsePort.postMessage).toHaveBeenCalledWith({ type: 'notification-click-ack' });

    stop();
    expect(serviceWorker.listenerCount()).toBe(0);
  });

  it('does not acknowledge when the callback rejects', async () => {
    const serviceWorker = stubServiceWorker();
    const callback = vi.fn(async () => {
      throw new Error('navigation failed');
    });
    const responsePort = { postMessage: vi.fn() };
    onNotificationClick(callback);

    serviceWorker.dispatchMessage({
      data: {
        type: 'notification-click',
        url: 'https://chatto.example/chat/-/room-1'
      },
      ports: [responsePort as unknown as MessagePort]
    });

    await Promise.resolve();
    await Promise.resolve();

    expect(callback).toHaveBeenCalledOnce();
    expect(responsePort.postMessage).not.toHaveBeenCalled();
  });
});
