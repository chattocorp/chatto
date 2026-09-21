import { createChattoClient, type ChattoPost, type Destination } from "@chatto/client";
export type { ChattoPost, Destination } from "@chatto/client";
import type { WebhookRouter } from "runling/web";
import { task, Type, TimeoutError, type WorkflowContext, type TSchema, type Static } from "runling";

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

export type Delivery = Static<typeof deliverySchema>;

/** Unsolicited messages, consumed explicitly by the workflow or its tasks. */
export interface ChattoInbox {
  drain(): Delivery[];
  subscribe(listener: () => void): () => void;
}

interface Conversation {
  messages: Delivery[];
  listeners: Set<() => void>;
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
  const routeDelivery = (delivery: Delivery): "start" | "ignored" | "duplicate" | "queued" | "cancelled" => {
    if (delivery.message.author_id === delivery.bot_id) return "ignored";
    if (!isDirectMessage(delivery) && !delivery.triggers.includes("mention")) return "ignored";
    const now = Date.now();
    for (const [id, expires] of seen) {
      if (expires <= now) {
        seen.delete(id);
      }
    }
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
        try {
          conversation.ctx?.abort("Cancelled from Chatto");
        } catch {
          // The abort signal also reaches the running task.
        }
        return "cancelled";
      }
      conversation.messages.push(delivery);
      for (const listener of conversation.listeners) {
        listener();
      }
      return "queued";
    }
    if (delivery.message.body.trim() === "/cancel") return "ignored";
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
      subscribe: listener => {
        conversation.listeners.add(listener);
        return () => {
          conversation.listeners.delete(listener);
        };
      },
    };
    const destination = { roomId: delivery.room_id, threadRootId };

    try {
      if (conversation.cancelled) ctx.abort("Cancelled from Chatto");
      return await run(ctx, delivery, destination, inbox);
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
      if (!conversation.cancelled && !(error instanceof TimeoutError)) {
        await post(destination, "I could not finish that reply. Please try again.",
          AbortSignal.timeout(10_000)).catch(() => {});
      }
      throw error;
    } finally {
      conversation.listeners.clear();
      conversations.delete(key);
    }
  });
  Object.assign(route, { label: name, input: deliverySchema });
  return Object.assign(workflow, { route });
}

/** Bind message delivery to the shared client. */
export function createChattoPoster(serverUrl: string, apiKey: string, request = fetch): ChattoPost {
  return createChattoClient({ serverUrl, apiKey, fetch: request }).postMessage;
}

export const isDirectMessage = (delivery: Delivery) => delivery.triggers.includes("direct_message");

function required(name: string): string {
  const value = process.env[name];
  if (!value) throw new Error(`Set ${name} before running ChattoBot`);
  return value;
}

/** Read credentials when a message is sent, so unrelated workflows need no Chatto setup. */
export const postToChatto: ChattoPost = (destination, body, signal) =>
  createChattoPoster(required("CHATTO_URL"), required("CHATTO_API_KEY"))(destination, body, signal);

export function messageSignal(ctx: WorkflowContext): AbortSignal {
  return AbortSignal.any([ctx.signal, AbortSignal.timeout(10_000)]);
}
