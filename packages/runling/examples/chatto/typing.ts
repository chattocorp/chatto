import { createChattoClient, withTyping, type ChattoTyping } from "@chatto/client";
export type { ChattoTyping } from "@chatto/client";
import type { WorkflowContext } from "runling";
import type { Destination } from "./webhook.ts";

export function createChattoTyping(serverUrl: string, apiKey: string): ChattoTyping {
  return createChattoClient({ serverUrl, apiKey }).refreshTyping;
}

export const sendChattoTyping: ChattoTyping = async (destination, signal) => {
  const url = process.env.CHATTO_URL;
  const key = process.env.CHATTO_API_KEY;
  if (!url || !key) throw new Error("Set CHATTO_URL and CHATTO_API_KEY before sending typing indicators");
  await createChattoTyping(url, key)(destination, signal);
};

/** Refresh typing only during work. Requests never overlap or block the work. */
export async function withChattoTyping<Result>(
  ctx: WorkflowContext,
  destination: Destination,
  typing: ChattoTyping | undefined,
  work: () => Promise<Result>,
): Promise<Result> {
  return withTyping(ctx.signal, typing ? signal => typing(destination, signal) : undefined, work);
}
