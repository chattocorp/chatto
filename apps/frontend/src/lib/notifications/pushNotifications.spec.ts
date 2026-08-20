import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  ensureRegistered,
  getPushCapability,
  getPushRegistrationTargets,
  getSubscription as getPushSubscription,
  onNotificationClick,
  refreshPushSubscriptions,
  unsubscribe,
  unsubscribeBeforeLeaving
} from './pushNotifications';
import {
  notificationRoomTargetFromPathname,
  prepareUiForNotificationPath,
  prepareUiForNotificationTarget
} from './notificationNavigationUi';
import {
  resumePushRegistration,
  resumePushRegistrationAfterAuthentication
} from './pushRegistrationCoordinator';

const mocks = vi.hoisted(() => ({
  createPushNotificationAPI: vi.fn(),
  subscribePush: vi.fn(),
  subscribeForClientPush: vi.fn(),
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
      serverInfo: {
        pushNotificationsEnabled: true,
        vapidPublicKey: 'origin-vapid',
        supportsFeature: vi.fn((_feature: string) => true)
      }
    },
    remote: {
      isAuthenticated: true,
      currentUser: { user: { id: 'remote-user' } },
      serverInfo: {
        pushNotificationsEnabled: true,
        vapidPublicKey: 'remote-vapid',
        supportsFeature: vi.fn((_feature: string) => true)
      }
    }
  }
}));

vi.mock('$lib/api-client/pushNotifications', () => ({
  createPushNotificationAPI: mocks.createPushNotificationAPI
}));

vi.mock('$lib/state/server/serverConnection.svelte', () => ({
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

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    servers: [{ id: 'origin' }, { id: 'remote' }],
    isOriginServer: (serverId: string) => serverId === 'origin',
    tryGetStore: (serverId: 'origin' | 'remote') => mocks.serverStores[serverId],
    getServer: (serverId: string) => ({
      id: serverId,
      url: serverId === 'origin' ? 'https://app.test' : 'https://remote.example.com'
    })
  }
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
  const rootRegistration = {
    scope: 'https://app.test/',
    pushManager: {
      getSubscription,
      subscribe
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
      getRegistrations: vi.fn().mockResolvedValue([rootRegistration]),
      ready: Promise.resolve(rootRegistration)
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
    mocks.serverStores.remote.serverInfo.pushNotificationsEnabled = true;
    mocks.serverStores.remote.serverInfo.supportsFeature.mockReturnValue(true);
  });

  it('includes compatible authenticated origin and remote servers', () => {
    expect(getPushRegistrationTargets()).toEqual([
      { serverId: 'origin', userId: 'origin-user', vapidPublicKey: 'origin-vapid' },
      { serverId: 'remote', userId: 'remote-user', vapidPublicKey: 'remote-vapid' }
    ]);
  });

  it('supports a remote-only authenticated client and excludes incompatible remotes', () => {
    mocks.serverStores.origin.isAuthenticated = false;

    expect(getPushRegistrationTargets()).toEqual([
      { serverId: 'remote', userId: 'remote-user', vapidPublicKey: 'remote-vapid' }
    ]);

    mocks.serverStores.remote.serverInfo.supportsFeature.mockReturnValue(false);
    expect(getPushRegistrationTargets()).toEqual([]);
  });
});

describe('pushNotifications.ensureRegistered', () => {
  beforeEach(() => {
    permission = 'default';
    installPushGlobals();
    resumePushRegistrationAfterAuthentication('origin');
    resumePushRegistrationAfterAuthentication('remote');
    mocks.createPushNotificationAPI.mockReset();
    mocks.createPushNotificationAPI.mockReturnValue({
      subscribe: mocks.subscribePush,
      subscribeForClient: mocks.subscribeForClientPush,
      unsubscribe: mocks.unsubscribePush,
      deleteByCapability: mocks.deleteByCapabilityPush
    });
    mocks.subscribePush.mockReset();
    mocks.subscribePush.mockResolvedValue({ subscribed: true });
    mocks.subscribeForClientPush.mockReset();
    mocks.subscribeForClientPush.mockResolvedValue({ subscribed: true });
    mocks.unsubscribePush.mockReset();
    mocks.unsubscribePush.mockResolvedValue(true);
    mocks.deleteByCapabilityPush.mockReset();
    mocks.deleteByCapabilityPush.mockResolvedValue(true);
  });

  it('does not prompt or mutate when permission is default and prompt is false', async () => {
    getSubscription.mockResolvedValue(null);

    await expect(ensureRegistered('origin', 'dmFwaWQ', { prompt: false })).resolves.toBe(false);
    expect(requestPermission).not.toHaveBeenCalled();
    expect(getSubscription).not.toHaveBeenCalled();
    expect(subscribe).not.toHaveBeenCalled();
    expect(mocks.subscribePush).not.toHaveBeenCalled();
  });

  it('does not mistake the root registration for a missing remote scope', async () => {
    permission = 'granted';

    await expect(getPushSubscription('remote')).resolves.toBeNull();
    expect(getSubscription).not.toHaveBeenCalled();
  });

  it('saves an existing subscription when permission is granted', async () => {
    permission = 'granted';
    const subscription = makeSubscription('https://push.example/existing');
    getSubscription.mockResolvedValue(subscription);

    await expect(ensureRegistered('origin', 'dmFwaWQ', { prompt: false })).resolves.toBe(true);
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
        cleanupToken: expect.stringMatching(/^[a-f0-9]{32}$/),
        userAgent: 'test-agent'
      },
      { signal: expect.any(AbortSignal) }
    );
    expect(mocks.subscribeForClientPush).not.toHaveBeenCalled();
  });

  it('creates and saves a subscription when permission is granted and none exists', async () => {
    permission = 'granted';
    const subscription = makeSubscription('https://push.example/created');
    getSubscription.mockResolvedValue(null);
    subscribe.mockResolvedValue(subscription);

    await expect(ensureRegistered('origin', 'dmFwaWQ', { prompt: false })).resolves.toBe(true);
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

    await expect(ensureRegistered('remote', 'dmFwaWQ', { prompt: false })).resolves.toBe(true);

    expect(register).toHaveBeenCalledWith('/service-worker.js', {
      scope: expect.stringMatching(/^\/__chatto\/push\/[a-f0-9]{64}\/$/),
      type: 'module'
    });
    expect(mocks.createPushNotificationAPI).toHaveBeenCalledWith({
      baseUrl: 'https://remote.test/api/connect',
      bearerToken: 'remote-token'
    });
    expect(mocks.subscribeForClientPush).toHaveBeenCalledWith(
      expect.objectContaining({
        endpoint: 'https://push.example/remote',
        clientHost: 'app.test'
      }),
      { signal: expect.any(AbortSignal) }
    );
    expect(mocks.subscribePush).not.toHaveBeenCalled();
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
    mocks.subscribeForClientPush
      .mockReturnValueOnce(firstSave.promise)
      .mockResolvedValue({ subscribed: true });

    const first = ensureRegistered('remote', 'dmFwaWQ', { prompt: false });
    await vi.waitFor(() => expect(mocks.subscribeForClientPush).toHaveBeenCalledOnce());
    const second = ensureRegistered('remote', 'dmFwaWQ', { prompt: false });
    await Promise.resolve();
    expect(mocks.subscribeForClientPush).toHaveBeenCalledOnce();

    firstSave.resolve({ subscribed: true });
    await expect(first).resolves.toBe(true);
    await expect(second).resolves.toBe(true);
    expect(mocks.subscribeForClientPush).toHaveBeenCalledTimes(2);
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
    mocks.subscribeForClientPush.mockReturnValueOnce(firstSave.promise);

    const activeRefresh = ensureRegistered('remote', 'dmFwaWQ', { prompt: false });
    await vi.waitFor(() => expect(mocks.subscribeForClientPush).toHaveBeenCalledOnce());
    const queuedRefresh = ensureRegistered('remote', 'dmFwaWQ', { prompt: false });
    const leaving = unsubscribeBeforeLeaving('remote');

    await expect(activeRefresh).resolves.toBe(false);
    await expect(queuedRefresh).resolves.toBe(false);
    await expect(leaving).resolves.toBeUndefined();

    expect(mocks.subscribeForClientPush).toHaveBeenCalledOnce();
    expect(mocks.subscribeForClientPush.mock.calls[0]?.[1]?.signal.aborted).toBe(true);
    expect(remoteSubscription.unsubscribe).toHaveBeenCalledOnce();
    expect(mocks.unsubscribePush).toHaveBeenCalledWith(remoteSubscription.endpoint);

    remoteSubscription.unsubscribe.mockClear();
    mocks.unsubscribePush.mockClear();
    // A different tab installs new authentication and clears shared suspension;
    // this realm intentionally retains its local block and obsolete credentials.
    window.localStorage.removeItem('chatto.push-registration.suspended.remote');
    await expect(ensureRegistered('remote', 'dmFwaWQ', { prompt: false })).resolves.toBe(false);

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
      mocks.subscribeForClientPush.mock.calls[0]?.[0].cleanupToken
    );

    const refreshKey = 'chatto.push-registration.refresh.remote';
    expect(window.localStorage.getItem(refreshKey)).toEqual(expect.any(String));
    resumePushRegistrationAfterAuthentication('remote');
    await refreshPushSubscriptions([
      { serverId: 'remote', userId: 'replacement-user', vapidPublicKey: 'dmFwaWQ' }
    ]);
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
    // A different realm has independent in-memory state, modelled by clearing
    // only this module's local suspension while retaining the shared tombstone.
    resumePushRegistration('remote');

    await expect(ensureRegistered('remote', 'dmFwaWQ', { prompt: true })).resolves.toBe(false);
    expect(requestPermission).not.toHaveBeenCalled();
    expect(register).not.toHaveBeenCalled();
    expect(mocks.subscribeForClientPush).not.toHaveBeenCalled();

    resumePushRegistrationAfterAuthentication('remote');

    await expect(ensureRegistered('remote', 'dmFwaWQ', { prompt: false })).resolves.toBe(true);
    expect(mocks.subscribeForClientPush).toHaveBeenCalledOnce();
  });

  it('does not let delayed leaving cleanup remove another tab\'s replacement', async () => {
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

  it('rejects and removes a remote subscription when the route-aware RPC is unavailable', async () => {
    permission = 'granted';
    const remoteSubscription = makeSubscription('https://push.example/remote-old-server');
    const register = vi.fn().mockResolvedValue({
      active: {},
      pushManager: {
        getSubscription: vi.fn().mockResolvedValue(remoteSubscription),
        subscribe: vi.fn()
      }
    });
    Object.assign(navigator.serviceWorker, { register });
    mocks.subscribeForClientPush.mockRejectedValueOnce(new Error('unimplemented'));

    await expect(ensureRegistered('remote', 'dmFwaWQ', { prompt: false })).resolves.toBe(false);

    expect(mocks.unsubscribePush).toHaveBeenCalledWith(remoteSubscription.endpoint);
    expect(remoteSubscription.unsubscribe).toHaveBeenCalledOnce();
  });

  it('removes a remote subscription when client-host acknowledgement is indeterminate', async () => {
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
    mocks.subscribeForClientPush.mockRejectedValueOnce(new Error('response lost'));

    await expect(ensureRegistered('remote', 'dmFwaWQ', { prompt: false })).resolves.toBe(false);

    expect(mocks.unsubscribePush).toHaveBeenCalledWith(remoteSubscription.endpoint);
    expect(remoteSubscription.unsubscribe).toHaveBeenCalledOnce();
  });

  it('prompts during explicit enable when permission is default', async () => {
    const subscription = makeSubscription('https://push.example/prompted');
    getSubscription.mockResolvedValue(null);
    subscribe.mockResolvedValue(subscription);

    await expect(ensureRegistered('origin', 'dmFwaWQ', { prompt: true })).resolves.toBe(true);
    expect(requestPermission).toHaveBeenCalledOnce();
    expect(subscribe).toHaveBeenCalledOnce();
    expect(mocks.subscribePush).toHaveBeenCalledOnce();
  });

  it('cleans up only a newly created subscription when server save fails', async () => {
    permission = 'granted';
    const existingSubscription = makeSubscription('https://push.example/existing');
    getSubscription.mockResolvedValueOnce(existingSubscription);
    mocks.subscribePush.mockResolvedValueOnce({ subscribed: false });

    await expect(ensureRegistered('origin', 'dmFwaWQ', { prompt: false })).resolves.toBe(false);
    expect(existingSubscription.unsubscribe).not.toHaveBeenCalled();

    const createdSubscription = makeSubscription('https://push.example/created');
    getSubscription.mockResolvedValueOnce(null);
    subscribe.mockResolvedValueOnce(createdSubscription);
    mocks.subscribePush.mockResolvedValueOnce({ subscribed: false });

    await expect(ensureRegistered('origin', 'dmFwaWQ', { prompt: false })).resolves.toBe(false);
    expect(createdSubscription.unsubscribe).toHaveBeenCalledOnce();
  });

  it('unsubscribes the browser before removing the server record', async () => {
    permission = 'granted';
    const subscription = makeSubscription('https://push.example/existing');
    getSubscription.mockResolvedValue(subscription);

    await expect(unsubscribe('origin')).resolves.toBe(true);

    expect(mocks.unsubscribePush).toHaveBeenCalledWith('https://push.example/existing');
    expect(subscription.unsubscribe).toHaveBeenCalledOnce();
    expect(subscription.unsubscribe.mock.invocationCallOrder[0]).toBeLessThan(
      mocks.unsubscribePush.mock.invocationCallOrder[0]
    );
  });

  it('still unsubscribes the browser when server cleanup fails', async () => {
    permission = 'granted';
    const subscription = makeSubscription('https://push.example/server-offline');
    getSubscription.mockResolvedValue(subscription);
    mocks.unsubscribePush.mockRejectedValueOnce(new Error('server offline'));

    await expect(unsubscribe('origin')).resolves.toBe(false);

    expect(mocks.unsubscribePush).toHaveBeenCalledWith(subscription.endpoint);
    expect(subscription.unsubscribe).toHaveBeenCalledOnce();
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
