import type { WorkflowContext } from "runling";
import type { Destination } from "./webhook.ts";

export type ChattoTyping = (destination: Destination, signal: AbortSignal) => Promise<void>;

export function createChattoTyping(serverUrl: string, apiKey: string): ChattoTyping {
  const url = new URL("/api/connect/chatto.api.v1.RoomService/UpdateTypingIndicator", serverUrl);
  return async ({ roomId, threadRootId }, signal) => {
    const response = await fetch(url, {
      method: "POST",
      headers: { Authorization: `Bearer ${apiKey}`, "Content-Type": "application/json", "Connect-Protocol-Version": "1" },
      body: JSON.stringify({ roomId, threadRootEventId: threadRootId }),
      signal,
    });
    if (!response.ok) throw new Error(`Chatto typing request failed (${response.status})`);
  };
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
  if (!typing) return work();
  const controller = new AbortController();
  const signal = AbortSignal.any([ctx.signal, controller.signal]);
  let timer: ReturnType<typeof setTimeout> | undefined;
  const refresh = async () => {
    if (signal.aborted) return;
    try {
      await typing(destination, AbortSignal.any([signal, AbortSignal.timeout(2000)]));
    } catch { /* Typing is best effort; unavailable presence must not fail planning. */ }
    if (!signal.aborted) timer = setTimeout(() => { void refresh(); }, 3000);
  };
  void refresh();
  try {
    return await work();
  } finally {
    controller.abort();
    clearTimeout(timer);
  }
}
