import { createChattoClient, type ConnectAPIConfig } from './connect.js';
import { PushNotificationService } from '@chatto/api-types/api/v1/push_notifications_connect';
import { PushSubscriptionCleanupService } from '@chatto/api-types/chatto/auth/v1/push_subscription_cleanup_connect';

export type SubscribePushInput = {
  endpoint: string;
  p256dh: string;
  auth: string;
  clientHost: string;
  cleanupToken: string;
  userAgent?: string;
};

export type SubscribePushResult = {
  subscribed: boolean;
};

export type PushRequestOptions = {
  signal?: AbortSignal;
};

export function createPushNotificationAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(PushNotificationService, config);
  const cleanupClient = createChattoClient(PushSubscriptionCleanupService, {
    baseUrl: config.baseUrl
  });

  return {
    async subscribe(
      input: SubscribePushInput,
      options: PushRequestOptions = {}
    ): Promise<SubscribePushResult> {
      await client.subscribe(input, { ...(options.signal ? { signal: options.signal } : {}) });
      return {
        subscribed: true
      };
    },

    async unsubscribe(endpoint: string): Promise<boolean> {
      await client.unsubscribe({ endpoint });
      return true;
    },

    async deleteByCapability(
      endpoint: string,
      auth: string,
      cleanupToken: string
    ): Promise<boolean> {
      await cleanupClient.deleteSubscription({ endpoint, auth, cleanupToken });
      return true;
    },

    async sendTestNotification(): Promise<boolean> {
      await client.sendTestNotification({});
      return true;
    }
  };
}

export type PushNotificationAPI = ReturnType<typeof createPushNotificationAPI>;
