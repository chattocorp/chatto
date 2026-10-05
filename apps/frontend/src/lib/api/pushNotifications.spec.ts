import { Code } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPushNotificationAPI } from './pushNotifications';
import { PushNotificationService } from '@chatto/api-types/api/v1/push_notifications_connect';
import { PushSubscriptionCleanupService } from '@chatto/api-types/chatto/auth/v1/push_subscription_cleanup_connect';
import {
  fakeServer,
  mockService,
  receivedContext,
  receivedRequest
} from '@chatto/client/testing/fakeServer';

const push = mockService(PushNotificationService);
const cleanup = mockService(PushSubscriptionCleanupService);

function pushAPI() {
  return createPushNotificationAPI(
    fakeServer(
      (router) =>
        router
          .service(PushNotificationService, push)
          .service(PushSubscriptionCleanupService, cleanup),
      { bearerToken: 'token' }
    )
  );
}

const subscription = {
  endpoint: 'https://push.example/sub',
  p256dh: 'p256dh-key',
  auth: 'auth-secret',
  clientHost: 'app.example',
  cleanupToken: '0123456789abcdef0123456789abcdef'
};

describe('createPushNotificationAPI', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    push.subscribe.mockReturnValue({});
    push.unsubscribe.mockReturnValue({});
    cleanup.deleteSubscription.mockReturnValue({});
  });

  it('subscribes and unsubscribes with the session credential', async () => {
    const api = pushAPI();

    await expect(api.subscribe({ ...subscription, userAgent: 'browser' })).resolves.toEqual({
      subscribed: true
    });
    await expect(api.unsubscribe('https://push.example/sub')).resolves.toBe(true);

    expect(receivedRequest(push.subscribe)).toMatchObject({
      ...subscription,
      userAgent: 'browser'
    });
    expect(receivedContext(push.subscribe)?.requestHeader.get('Authorization')).toBe(
      'Bearer token'
    );
    expect(receivedRequest(push.unsubscribe)).toMatchObject({
      endpoint: 'https://push.example/sub'
    });
  });

  it('deletes a stale subscription by capability, without the session credential', async () => {
    await expect(
      pushAPI().deleteByCapability(
        'https://push.example/stale',
        'stale-auth-secret',
        'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
      )
    ).resolves.toBe(true);

    expect(receivedRequest(cleanup.deleteSubscription)).toMatchObject({
      endpoint: 'https://push.example/stale',
      auth: 'stale-auth-secret',
      cleanupToken: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
    });
    expect(receivedContext(cleanup.deleteSubscription)?.requestHeader.has('Authorization')).toBe(
      false
    );
  });

  it('cancels a subscription with the caller signal', async () => {
    await expect(
      pushAPI().subscribe(subscription, { signal: AbortSignal.abort() })
    ).rejects.toMatchObject({ code: Code.Canceled });
  });
});
