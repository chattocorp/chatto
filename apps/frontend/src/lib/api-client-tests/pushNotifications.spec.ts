import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPushNotificationAPI } from '$lib/api-client/pushNotifications';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  subscribe: vi.fn(),
  subscribeForClient: vi.fn(),
  unsubscribe: vi.fn(),
  deleteSubscriptionByCapability: vi.fn()
}));

vi.mock('@connectrpc/connect', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@connectrpc/connect')>();
  return {
    ...actual,
    createClient: mocks.createClient
  };
});

vi.mock('@connectrpc/connect-web', () => ({
  createConnectTransport: mocks.createConnectTransport
}));

describe('createPushNotificationAPI', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.subscribe.mockReset();
    mocks.subscribeForClient.mockReset();
    mocks.unsubscribe.mockReset();
    mocks.deleteSubscriptionByCapability.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      subscribe: mocks.subscribe,
      subscribeForClient: mocks.subscribeForClient,
      unsubscribe: mocks.unsubscribe,
      deleteSubscriptionByCapability: mocks.deleteSubscriptionByCapability
    });
  });

  it('subscribes and unsubscribes with bearer auth', async () => {
    mocks.subscribe.mockResolvedValue({ subscribed: true });
    mocks.subscribeForClient.mockResolvedValue({ subscribed: true });
    mocks.unsubscribe.mockResolvedValue({ unsubscribed: true });
    mocks.deleteSubscriptionByCapability.mockResolvedValue({ completed: true });

    const api = createPushNotificationAPI({
      baseUrl: 'https://origin.test/api/connect',
      bearerToken: 'token'
    });
    const controller = new AbortController();

    await expect(
      api.subscribe({
        endpoint: 'https://push.example/sub',
        p256dh: 'p256dh-key',
        auth: 'auth-secret',
        userAgent: 'browser'
      })
    ).resolves.toEqual({ subscribed: true });
    await expect(
      api.subscribeForClient(
        {
          endpoint: 'https://push.example/remote',
          p256dh: 'p256dh-key',
          auth: 'auth-secret',
          clientHost: 'app.example'
        },
        { signal: controller.signal }
      )
    ).resolves.toEqual({ subscribed: true });
    await expect(api.unsubscribe('https://push.example/sub')).resolves.toBe(true);
    await expect(
      api.deleteByCapability('https://push.example/stale', 'stale-auth-secret')
    ).resolves.toBe(true);

    expect(mocks.createConnectTransport).toHaveBeenCalledWith({
      baseUrl: 'https://origin.test/api/connect',
      useBinaryFormat: true
    });
    expect(mocks.subscribe).toHaveBeenCalledWith(
      {
        endpoint: 'https://push.example/sub',
        p256dh: 'p256dh-key',
        auth: 'auth-secret',
        userAgent: 'browser'
      },
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(mocks.unsubscribe).toHaveBeenCalledWith(
      { endpoint: 'https://push.example/sub' },
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(mocks.subscribeForClient).toHaveBeenCalledWith(
      {
        endpoint: 'https://push.example/remote',
        p256dh: 'p256dh-key',
        auth: 'auth-secret',
        clientHost: 'app.example'
      },
      { headers: { Authorization: 'Bearer token' }, signal: controller.signal }
    );
    expect(mocks.deleteSubscriptionByCapability).toHaveBeenCalledWith({
      endpoint: 'https://push.example/stale',
      auth: 'stale-auth-secret'
    });
  });

  it('omits auth headers when no bearer token exists', async () => {
    mocks.subscribe.mockResolvedValue({ subscribed: true });

    const api = createPushNotificationAPI({
      baseUrl: '/api/connect',
      bearerToken: null
    });

    await expect(
      api.subscribe({
        endpoint: 'https://push.example/sub',
        p256dh: 'p256dh-key',
        auth: 'auth-secret'
      })
    ).resolves.toEqual({ subscribed: true });

    expect(mocks.subscribe).toHaveBeenCalledWith(
      {
        endpoint: 'https://push.example/sub',
        p256dh: 'p256dh-key',
        auth: 'auth-secret'
      },
      { headers: undefined }
    );
  });
});
