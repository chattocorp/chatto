import { createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { PushNotificationService } from '@chatto/api-types/api/v1/push_notifications_connect';

export type PushNotificationAPIConfig = {
  baseUrl: string;
  bearerToken: string | null;
};

export type SubscribePushInput = {
  endpoint: string;
  p256dh: string;
  auth: string;
  userAgent?: string;
};

export function createPushNotificationAPI(config: PushNotificationAPIConfig) {
  const transport = createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: true
  });
  const client = createClient(PushNotificationService, transport);
  const headers = () =>
    config.bearerToken ? { Authorization: `Bearer ${config.bearerToken}` } : undefined;

  return {
    async subscribe(input: SubscribePushInput): Promise<boolean> {
      return (await client.subscribe(input, { headers: headers() })).subscribed;
    },

    async unsubscribe(endpoint: string): Promise<boolean> {
      return (await client.unsubscribe({ endpoint }, { headers: headers() })).unsubscribed;
    }
  };
}

export type PushNotificationAPI = ReturnType<typeof createPushNotificationAPI>;
