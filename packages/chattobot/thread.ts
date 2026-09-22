import { createChattoClient, type ThreadMessage } from "@chatto/client";
import type { Delivery } from "./chatto/routing.ts";

export type { ThreadMessage } from "@chatto/client";
export type ReadThread = (delivery: Delivery, signal: AbortSignal) => Promise<ThreadMessage[]>;

/** Bind the shared history reader to this bot's connection. */
export function createThreadReader(serverUrl: string, apiKey: string, request = fetch): ReadThread {
  return createChattoClient({ serverUrl, apiKey, fetch: request }).readThread;
}

export const readChattoThread: ReadThread = (delivery, signal) => {
  const url = process.env.CHATTO_URL;
  const key = process.env.CHATTO_API_KEY;
  if (!url || !key) {
    throw new Error("Set CHATTO_URL and CHATTO_API_KEY before reading threads");
  }
  return createThreadReader(url, key)(delivery, signal);
};
