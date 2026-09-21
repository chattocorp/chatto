import { createChattoClient } from "@chatto/client";
import type { Delivery } from "./chatto/webhook.ts";

export type Acknowledge = (delivery: Delivery, signal?: AbortSignal) => Promise<void>;

/** The eyes reaction is bot policy; transport belongs to the shared client. */
export function createEyesReaction(serverUrl: string, apiKey: string, request = fetch): Acknowledge {
  const client = createChattoClient({ serverUrl, apiKey, fetch: request });
  return (delivery, signal) => client.addReaction(delivery.room_id, delivery.message.id, "eyes", signal);
}

export const acknowledgeChatto: Acknowledge = (delivery, signal) => {
  const url = process.env.CHATTO_URL;
  const key = process.env.CHATTO_API_KEY;
  if (!url || !key) {
    throw new Error("Set CHATTO_URL and CHATTO_API_KEY before adding reactions");
  }
  return createEyesReaction(url, key)(delivery, signal);
};
