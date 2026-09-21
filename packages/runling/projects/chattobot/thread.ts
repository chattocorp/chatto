import type { Delivery } from "./chatto/webhook.ts";

interface ThreadEvent {
  id?: string;
  messagePosted?: { message?: { actorId?: string; body?: string } };
}

export interface ThreadMessage {
  id: string;
  authorId?: string;
  role: "bot" | "human";
  body: string;
}

export type ReadThread = (delivery: Delivery, signal: AbortSignal) => Promise<ThreadMessage[]>;

/** Read all pages, preserving the root before replies and removing page overlap. */
export function createThreadReader(serverUrl: string, apiKey: string, request = fetch): ReadThread {
  return async (delivery, signal) => {
    const rootId = delivery.thread_root_id ?? delivery.message.id;
    let root: ThreadEvent | undefined;
    let replies: ThreadEvent[] = [];
    let before: string | undefined;
    const cursors = new Set<string>();
    const readSignal = AbortSignal.any([signal, AbortSignal.timeout(30_000)]);

    do {
      const response = await request(new URL(
        "/api/connect/chatto.api.v1.ThreadService/GetThreadEvents", serverUrl,
      ), {
        method: "POST",
        headers: {
          Authorization: `Bearer ${apiKey}`,
          "Content-Type": "application/json",
          "Connect-Protocol-Version": "1",
        },
        body: JSON.stringify({
          roomId: delivery.room_id,
          threadRootEventId: rootId,
          limit: 100,
          ...(before ? { before } : {}),
        }),
        redirect: "error",
        signal: readSignal,
      });
      if (!response.ok) {
        throw new Error(`Chatto thread request failed (${response.status})`);
      }

      const { page } = await response.json() as {
        page?: { events?: ThreadEvent[]; hasOlder?: boolean; startCursor?: string };
      };
      if (!page) {
        throw new Error("Chatto did not return the thread page");
      }

      const events = page.events ?? [];
      root ??= events.find(event => event.id === rootId);
      replies = [...events.filter(event => event.id !== rootId), ...replies];
      if (!page.hasOlder) {
        break;
      }

      before = page.startCursor;
      if (!before || cursors.has(before)) {
        throw new Error("Thread pagination did not advance");
      }
      cursors.add(before);
    } while (true);

    const seen = new Set<string>();
    return [...(root ? [root] : []), ...replies].flatMap(event => {
      if (!event.id || seen.has(event.id)) {
        return [];
      }
      seen.add(event.id);
      const message = event.messagePosted?.message;
      if (!message?.body) {
        return [];
      }

      return [{
        id: event.id,
        authorId: message.actorId,
        role: message.actorId === delivery.bot_id ? "bot" as const : "human" as const,
        body: message.body,
      }];
    });
  };
}

export const readChattoThread: ReadThread = (delivery, signal) => {
  const url = process.env.CHATTO_URL;
  const key = process.env.CHATTO_API_KEY;
  if (!url || !key) {
    throw new Error("Set CHATTO_URL and CHATTO_API_KEY before reading threads");
  }
  return createThreadReader(url, key)(delivery, signal);
};
