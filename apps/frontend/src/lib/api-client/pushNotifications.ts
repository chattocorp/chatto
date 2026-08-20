import { authHeaders, createChattoClient } from './connect.js';
import { PushNotificationService } from '@chatto/api-types/api/v1/push_notifications_connect';

export type PushNotificationAPIConfig = {
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

export type SubscribePushInput = {
  endpoint: string;
  p256dh: string;
  auth: string;
  userAgent?: string;
};

export type SubscribeForClientPushInput = SubscribePushInput & {
  clientHost: string;
};

export type SubscribePushResult = {
  subscribed: boolean;
};

export type PushRequestOptions = {
  signal?: AbortSignal;
};

export function createPushNotificationAPI(config: PushNotificationAPIConfig) {
  const client = createChattoClient(PushNotificationService, config);
  const headers = () => authHeaders(config);

  return {
    async subscribe(
      input: SubscribePushInput,
      options: PushRequestOptions = {}
    ): Promise<SubscribePushResult> {
      const response = await client.subscribe(input, {
        headers: headers(),
        ...(options.signal ? { signal: options.signal } : {})
      });
      return {
        subscribed: response.subscribed
      };
    },

    async subscribeForClient(
      input: SubscribeForClientPushInput,
      options: PushRequestOptions = {}
    ): Promise<SubscribePushResult> {
      const response = await client.subscribeForClient(input, {
        headers: headers(),
        ...(options.signal ? { signal: options.signal } : {})
      });
      return {
        subscribed: response.subscribed
      };
    },

    async unsubscribe(endpoint: string): Promise<boolean> {
      return (await client.unsubscribe({ endpoint }, { headers: headers() })).unsubscribed;
    },

    async deleteByCapability(endpoint: string, auth: string): Promise<boolean> {
      return (await client.deleteSubscriptionByCapability({ endpoint, auth })).completed;
    },

    async sendTestNotification(): Promise<boolean> {
      return (await client.sendTestNotification({}, { headers: headers() })).sent;
    }
  };
}

export type PushNotificationAPI = ReturnType<typeof createPushNotificationAPI>;
