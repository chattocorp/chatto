import type { Delivery } from "./chatto/webhook.ts";

export type Acknowledge = (delivery: Delivery, signal?: AbortSignal) => Promise<void>;

export function createEyesReaction(serverUrl: string, apiKey: string, request = fetch): Acknowledge {
  return async (delivery, signal) => {
    const response = await request(new URL(
      "/api/connect/chatto.api.v1.MessageService/AddReaction", serverUrl,
    ), {
      method: "POST",
      headers: {
        Authorization: `Bearer ${apiKey}`,
        "Content-Type": "application/json",
        "Connect-Protocol-Version": "1",
      },
      body: JSON.stringify({
        roomId: delivery.room_id,
        messageEventId: delivery.message.id,
        emoji: "eyes",
      }),
      redirect: "error",
      signal: AbortSignal.any([...(signal ? [signal] : []), AbortSignal.timeout(10_000)]),
    });
    if (!response.ok) {
      throw new Error(`Chatto reaction request failed (${response.status})`);
    }
  };
}

export const acknowledgeChatto: Acknowledge = (delivery, signal) => {
  const url = process.env.CHATTO_URL;
  const key = process.env.CHATTO_API_KEY;
  if (!url || !key) {
    throw new Error("Set CHATTO_URL and CHATTO_API_KEY before adding reactions");
  }
  return createEyesReaction(url, key)(delivery, signal);
};
