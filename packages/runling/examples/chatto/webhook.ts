import type { WebhookRouter } from "runling/web";
import { task, Type, TimeoutError, type InputHandler, type WorkflowContext, type TSchema, type Static } from "runling";

export const deliverySchema = Type.Object({
  version: Type.Literal(1),
  id: Type.String(),
  type: Type.Literal("message.created"),
  triggers: Type.Array(Type.String()),
  occurred_at: Type.String(),
  bot_id: Type.String(),
  room_id: Type.String(),
  thread_root_id: Type.Union([Type.String(), Type.Null()]),
  message: Type.Object({ id: Type.String(), author_id: Type.String(), body: Type.String() }),
});

export interface Destination {
  roomId: string;
  threadRootId: string;
}

export type ChattoPost = (destination: Destination, body: string, signal: AbortSignal) => Promise<void>;
export type Delivery = Static<typeof deliverySchema>;

/** Unsolicited messages, consumed explicitly by the workflow or its tasks. */
export interface ChattoInbox {
  drain(): string[];
  prepend(messages: string[]): void;
  subscribe(listener: () => void): () => void;
}

interface Conversation {
  messages: string[];
  listeners: Set<() => void>;
  answer?: (answer: string) => void;
  ctx?: WorkflowContext;
  cancelled: boolean;
}


/** Share delivery routing within one process; task code receives a normal context. */
export function createChattoWebhook<Output extends TSchema>({ name, output, post, run }: {
  name: string;
  output: Output;
  post: ChattoPost;
  run: (ctx: WorkflowContext, delivery: Delivery, destination: Destination, inbox: ChattoInbox) => Promise<Static<Output>>;
}) {
  const conversations = new Map<string, Conversation>();
  const seen = new Map<string, number>();

  const reserved = new Set<string>();
  const deliveryKey = (delivery: Delivery) => JSON.stringify([delivery.bot_id, delivery.message.id]);
  const conversationKey = (delivery: Delivery) => JSON.stringify([
    delivery.bot_id, delivery.room_id, delivery.thread_root_id ?? delivery.message.id, delivery.message.author_id,
  ]);
  const routeDelivery = (delivery: Delivery): "start" | "ignored" | "answered" | "duplicate" | "queued" | "cancelled" => {
    if (!delivery.triggers.includes("direct_message")) return "ignored";
    const now = Date.now();
    for (const [id, expires] of seen) if (expires <= now) seen.delete(id);
    const id = deliveryKey(delivery);
    if (seen.has(id)) return "duplicate";
    seen.set(id, now + 86_400_000);
    const key = conversationKey(delivery);
    const conversation = conversations.get(key);
    if (conversation) {
      if (conversation.cancelled) return "ignored";
      if (delivery.message.body.trim() === "/cancel") {
        conversation.cancelled = true;
        // abort() intentionally throws; the workflow observes the same signal.
        try { conversation.ctx?.abort("Cancelled from Chatto"); } catch { /* delivered */ }
        return "cancelled";
      }
      if (conversation.answer) {
        const answer = conversation.answer;
        conversation.answer = undefined;
        answer(delivery.message.body);
        return "answered";
      }
      conversation.messages.push(delivery.message.body);
      for (const listener of conversation.listeners) listener();
      return "queued";
    }
    if (delivery.thread_root_id !== null) return "ignored";
    conversations.set(key, { messages: [], listeners: new Set(), cancelled: false });
    return "start";
  };
  const route: WebhookRouter<Delivery> = async (ctx, delivery) => {
    if (routeDelivery(delivery) !== "start") return;
    const id = deliveryKey(delivery);
    reserved.add(id);
    try {
      await ctx.start(workflow, { input: delivery });
    } catch (error) {
      // A failed journal creation must leave the delivery retryable.
      if (reserved.delete(id)) {
        seen.delete(id);
        conversations.delete(conversationKey(delivery));
      }
      throw error;
    }
    // Registration can finish before execution enters the task. Keep its
    // reservation until the task consumes it, so it is not treated as a duplicate.
  };

  const workflow = task({
    name,
    input: deliverySchema,
    output: Type.Union([Type.String(), output]),
  }, async (ctx, delivery) => {
    if (!reserved.delete(deliveryKey(delivery))) {
      const outcome = routeDelivery(delivery);
      if (outcome !== "start") return outcome;
    }
    const threadRootId = delivery.thread_root_id ?? delivery.message.id;
    const key = conversationKey(delivery);
    const conversation = conversations.get(key)!;
    conversation.ctx = ctx;
    const inbox: ChattoInbox = {
      drain: () => conversation.messages.splice(0),
      prepend: messages => { conversation.messages.unshift(...messages); },
      subscribe: listener => {
        conversation.listeners.add(listener);
        return () => { conversation.listeners.delete(listener); };
      },
    };
    const destination = { roomId: delivery.room_id, threadRootId };
    const closed = new AbortController();
    let previous: Promise<unknown> = Promise.resolve();
    const onInput: InputHandler = (request) => {
      const questionSignal = AbortSignal.any([request.signal ?? ctx.signal, closed.signal]);
      // Queue presentation, not answers. Deadlines include time spent in this queue.
      const response = previous.then(async () => {
        questionSignal.throwIfAborted();
        let resolveAnswer!: (answer: string) => void;
        let rejectAnswer!: (error: unknown) => void;
        const answer = new Promise<string>((resolve, reject) => {
          resolveAnswer = resolve;
          rejectAnswer = reject;
        });
        void answer.catch(() => {});
        const abort = () => {
          if (conversation.answer === resolveAnswer) conversation.answer = undefined;
          rejectAnswer(questionSignal.reason);
        };
        // Register immediately before posting so a quick reply cannot be lost.
        conversation.answer = resolveAnswer;
        questionSignal.addEventListener("abort", abort, { once: true });
        try {
          await post(destination, request.message, questionSignal);
          return await answer;
        } finally {
          if (conversation.answer === resolveAnswer) conversation.answer = undefined;
          questionSignal.removeEventListener("abort", abort);
        }
      });
      previous = response.catch(() => {});
      return response;
    };

    try {
      if (conversation.cancelled) ctx.abort("Cancelled from Chatto");
      return await run({ ...ctx, onInput }, delivery, destination, inbox);
    } catch (error) {
      if (conversation.cancelled) {
        await post(destination, "Conversation cancelled.", AbortSignal.timeout(10_000)).catch(() => {});
      }
      if (error instanceof TimeoutError && !ctx.signal.aborted) {
        // The question signal has expired. Use a fresh, bounded signal for the notice.
        await post(destination, "The question timed out. Send me a new DM to try again.",
          AbortSignal.any([ctx.signal, AbortSignal.timeout(10_000)]),
        ).catch(() => {}); // Preserve the original timeout if the notice cannot be delivered.
      }
      throw error;
    } finally {
      closed.abort(new Error("Chatto conversation ended"));
      conversation.listeners.clear();
      conversations.delete(key);
    }
  });
  Object.assign(route, { label: name, input: deliverySchema });
  return Object.assign(workflow, { route });
}

/** ConnectRPC's JSON transport needs no generated client for this single call. */
export function createChattoPoster(serverUrl: string, apiKey: string): ChattoPost {
  const url = new URL("/api/connect/chatto.api.v1.MessageService/CreateMessage", serverUrl);
  return async ({ roomId, threadRootId }, body, signal) => {
    // Keep long plans readable and within message limits; preserve Unicode characters.
    const characters = Array.from(body);
    for (let offset = 0; offset < Math.max(1, characters.length); offset += 8000) {
      const response = await fetch(url, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${apiKey}`,
          "Content-Type": "application/json",
          "Connect-Protocol-Version": "1",
        },
        body: JSON.stringify({ roomId, body: characters.slice(offset, offset + 8000).join(""), threadRootEventId: threadRootId }),
        signal,
      });
      if (!response.ok) throw new Error(`Chatto message request failed (${response.status})`);
    }
  };
}

function required(name: string): string {
  const value = process.env[name];
  if (!value) throw new Error(`Set ${name} before running the Chatto demo`);
  return value;
}

/** Read credentials when a message is sent, so unrelated workflows need no Chatto setup. */
export const postToChatto: ChattoPost = (destination, body, signal) =>
  createChattoPoster(required("CHATTO_URL"), required("CHATTO_API_KEY"))(destination, body, signal);

export function messageSignal(ctx: WorkflowContext): AbortSignal {
  return AbortSignal.any([ctx.signal, AbortSignal.timeout(10_000)]);
}
