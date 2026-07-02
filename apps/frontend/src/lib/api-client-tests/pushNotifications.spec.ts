import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPushNotificationAPI } from '$lib/api-client/pushNotifications';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  subscribe: vi.fn(),
  unsubscribe: vi.fn()
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
    mocks.unsubscribe.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      subscribe: mocks.subscribe,
      unsubscribe: mocks.unsubscribe
    });
  });

  it('subscribes and unsubscribes with bearer auth', async () => {
    mocks.subscribe.mockResolvedValue({ subscribed: true });
    mocks.unsubscribe.mockResolvedValue({ unsubscribed: true });

    const api = createPushNotificationAPI({
      baseUrl: 'https://origin.test/api/connect',
      bearerToken: 'token'
    });

    await expect(
      api.subscribe({
        endpoint: 'https://push.example/sub',
        p256dh: 'p256dh-key',
        auth: 'auth-secret',
        userAgent: 'browser'
      })
    ).resolves.toBe(true);
    await expect(api.unsubscribe('https://push.example/sub')).resolves.toBe(true);

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
    ).resolves.toBe(true);

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
