import { authHeaders, createChattoClient } from "./connect.js";
import { PushNotificationService } from "@chatto/api-types/api/v1/push_notifications_connect";

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

export function createPushNotificationAPI(config: PushNotificationAPIConfig) {
  const client = createChattoClient(PushNotificationService, config);
  const headers = () => authHeaders(config);

  return {
    async subscribe(input: SubscribePushInput): Promise<boolean> {
      return (await client.subscribe(input, { headers: headers() })).subscribed;
    },

    async unsubscribe(endpoint: string): Promise<boolean> {
      return (await client.unsubscribe({ endpoint }, { headers: headers() }))
        .unsubscribed;
    },
  };
}

export type PushNotificationAPI = ReturnType<typeof createPushNotificationAPI>;
