import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ensureRegistered } from './pushNotifications';

const mocks = vi.hoisted(() => ({
  mutation: vi.fn()
}));

vi.mock('$lib/gql', () => ({
  graphql: (source: TemplateStringsArray) => ({ source })
}));

vi.mock('$lib/state/server/graphqlClient.svelte', () => ({
  graphqlClientManager: {
    originClient: {
      client: {
        mutation: mocks.mutation
      }
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

describe('pushNotifications.ensureRegistered', () => {
  beforeEach(() => {
    permission = 'default';
    installPushGlobals();
    mocks.mutation.mockReset();
    mocks.mutation.mockReturnValue({
      toPromise: vi.fn().mockResolvedValue({
        data: { subscribeToPush: true },
        error: null
      })
    });
  });

  it('does not prompt or mutate when permission is default and prompt is false', async () => {
    getSubscription.mockResolvedValue(null);

    await expect(ensureRegistered('dmFwaWQ', { prompt: false })).resolves.toBe(false);
    expect(requestPermission).not.toHaveBeenCalled();
    expect(getSubscription).not.toHaveBeenCalled();
    expect(subscribe).not.toHaveBeenCalled();
    expect(mocks.mutation).not.toHaveBeenCalled();
  });

  it('saves an existing subscription when permission is granted', async () => {
    permission = 'granted';
    const subscription = makeSubscription('https://push.example/existing');
    getSubscription.mockResolvedValue(subscription);

    await expect(ensureRegistered('dmFwaWQ', { prompt: false })).resolves.toBe(true);
    expect(subscribe).not.toHaveBeenCalled();
    expect(mocks.mutation).toHaveBeenCalledWith(
      expect.anything(),
      {
        input: {
          endpoint: 'https://push.example/existing',
          p256dh: 'p256dh-key',
          auth: 'auth-secret',
          userAgent: 'test-agent'
        }
      }
    );
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
    expect(mocks.mutation).toHaveBeenCalledWith(
      expect.anything(),
      expect.objectContaining({
        input: expect.objectContaining({
          endpoint: 'https://push.example/created'
        })
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
    expect(mocks.mutation).toHaveBeenCalledOnce();
  });

  it('cleans up only a newly created subscription when server save fails', async () => {
    permission = 'granted';
    const existingSubscription = makeSubscription('https://push.example/existing');
    getSubscription.mockResolvedValueOnce(existingSubscription);
    mocks.mutation.mockReturnValueOnce({
      toPromise: vi.fn().mockResolvedValue({
        data: null,
        error: new Error('save failed')
      })
    });

    await expect(ensureRegistered('dmFwaWQ', { prompt: false })).resolves.toBe(false);
    expect(existingSubscription.unsubscribe).not.toHaveBeenCalled();

    const createdSubscription = makeSubscription('https://push.example/created');
    getSubscription.mockResolvedValueOnce(null);
    subscribe.mockResolvedValueOnce(createdSubscription);
    mocks.mutation.mockReturnValueOnce({
      toPromise: vi.fn().mockResolvedValue({
        data: null,
        error: new Error('save failed')
      })
    });

    await expect(ensureRegistered('dmFwaWQ', { prompt: false })).resolves.toBe(false);
    expect(createdSubscription.unsubscribe).toHaveBeenCalledOnce();
  });
});
