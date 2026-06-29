import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ensureRegistered, onNotificationClick, unsubscribe } from './pushNotifications';

const mocks = vi.hoisted(() => ({
  createPushNotificationAPI: vi.fn(),
  subscribePush: vi.fn(),
  unsubscribePush: vi.fn()
}));

vi.mock('@chatto/api-client/pushNotifications', () => ({
  createPushNotificationAPI: mocks.createPushNotificationAPI
}));

vi.mock('$lib/state/server/serverConnection.svelte', () => ({
  serverConnectionManager: {
    originClient: {
      connectBaseUrl: 'https://origin.test/api/connect',
      bearerToken: 'origin-token'
    }
  }
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
    unsubscribe: vi.fn().mockResolvedValue(true)
  } as unknown as TestPushSubscription;
}

function installPushGlobals() {
  requestPermission = vi.fn(async () => {
    permission = 'granted';
    return permission;
  });
  getSubscription = vi.fn();
  subscribe = vi.fn();

  vi.stubGlobal('Notification', {
    get permission() {
      return permission;
    },
    requestPermission
  });
  vi.stubGlobal('window', {
    Notification,
    PushManager: class PushManager {},
    atob: (value: string) => Buffer.from(value, 'base64').toString('binary')
  });
  vi.stubGlobal('navigator', {
    serviceWorker: {
      ready: Promise.resolve({
        pushManager: {
          getSubscription,
          subscribe
        }
      })
    },
    userAgent: 'test-agent'
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

describe('pushNotifications.ensureRegistered', () => {
  beforeEach(() => {
    permission = 'default';
    installPushGlobals();
    mocks.createPushNotificationAPI.mockReset();
    mocks.createPushNotificationAPI.mockReturnValue({
      subscribe: mocks.subscribePush,
      unsubscribe: mocks.unsubscribePush
    });
    mocks.subscribePush.mockReset();
    mocks.subscribePush.mockResolvedValue(true);
    mocks.unsubscribePush.mockReset();
    mocks.unsubscribePush.mockResolvedValue(true);
  });

  it('does not prompt or mutate when permission is default and prompt is false', async () => {
    getSubscription.mockResolvedValue(null);

    await expect(ensureRegistered('dmFwaWQ', { prompt: false })).resolves.toBe(false);
    expect(requestPermission).not.toHaveBeenCalled();
    expect(getSubscription).not.toHaveBeenCalled();
    expect(subscribe).not.toHaveBeenCalled();
    expect(mocks.subscribePush).not.toHaveBeenCalled();
  });

  it('saves an existing subscription when permission is granted', async () => {
    permission = 'granted';
    const subscription = makeSubscription('https://push.example/existing');
    getSubscription.mockResolvedValue(subscription);

    await expect(ensureRegistered('dmFwaWQ', { prompt: false })).resolves.toBe(true);
    expect(subscribe).not.toHaveBeenCalled();
    expect(mocks.createPushNotificationAPI).toHaveBeenCalledWith({
      baseUrl: 'https://origin.test/api/connect',
      bearerToken: 'origin-token'
    });
    expect(mocks.subscribePush).toHaveBeenCalledWith({
      endpoint: 'https://push.example/existing',
      p256dh: 'p256dh-key',
      auth: 'auth-secret',
      userAgent: 'test-agent'
    });
  });

  it('creates and saves a subscription when permission is granted and none exists', async () => {
    permission = 'granted';
    const subscription = makeSubscription('https://push.example/created');
    getSubscription.mockResolvedValue(null);
    subscribe.mockResolvedValue(subscription);

    await expect(ensureRegistered('dmFwaWQ', { prompt: false })).resolves.toBe(true);
    expect(subscribe).toHaveBeenCalledWith({
      userVisibleOnly: true,
      applicationServerKey: expect.any(Uint8Array)
    });
    expect(mocks.subscribePush).toHaveBeenCalledWith(
      expect.objectContaining({
        endpoint: 'https://push.example/created'
      })
    );
  });

  it('prompts during explicit enable when permission is default', async () => {
    const subscription = makeSubscription('https://push.example/prompted');
    getSubscription.mockResolvedValue(null);
    subscribe.mockResolvedValue(subscription);

    await expect(ensureRegistered('dmFwaWQ', { prompt: true })).resolves.toBe(true);
    expect(requestPermission).toHaveBeenCalledOnce();
    expect(subscribe).toHaveBeenCalledOnce();
    expect(mocks.subscribePush).toHaveBeenCalledOnce();
  });

  it('cleans up only a newly created subscription when server save fails', async () => {
    permission = 'granted';
    const existingSubscription = makeSubscription('https://push.example/existing');
    getSubscription.mockResolvedValueOnce(existingSubscription);
    mocks.subscribePush.mockResolvedValueOnce(false);

    await expect(ensureRegistered('dmFwaWQ', { prompt: false })).resolves.toBe(false);
    expect(existingSubscription.unsubscribe).not.toHaveBeenCalled();

    const createdSubscription = makeSubscription('https://push.example/created');
    getSubscription.mockResolvedValueOnce(null);
    subscribe.mockResolvedValueOnce(createdSubscription);
    mocks.subscribePush.mockResolvedValueOnce(false);

    await expect(ensureRegistered('dmFwaWQ', { prompt: false })).resolves.toBe(false);
    expect(createdSubscription.unsubscribe).toHaveBeenCalledOnce();
  });

  it('unsubscribes from the server before unsubscribing the browser subscription', async () => {
    permission = 'granted';
    const subscription = makeSubscription('https://push.example/existing');
    getSubscription.mockResolvedValue(subscription);

    await expect(unsubscribe()).resolves.toBe(true);

    expect(mocks.unsubscribePush).toHaveBeenCalledWith('https://push.example/existing');
    expect(subscription.unsubscribe).toHaveBeenCalledOnce();
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
