import { createDeliveryTracker, type Destination } from '@chatto/client';
export type { Destination } from '@chatto/client';

/** Send ordered thread messages; workflow adapters supply their cancellation signal. */
export type ChattoPost = (
  destination: Destination,
  body: string,
  signal: AbortSignal
) => Promise<void>;

/** Refresh a thread's typing indicator once. */
export type ChattoTyping = (destination: Destination, signal: AbortSignal) => Promise<void>;
import type { WebhookRouter } from 'runling/web';
import { task, Type, type WorkflowContext, type TSchema, type Static } from 'runling';

export const deliverySchema = Type.Object({
  version: Type.Literal(1),
  id: Type.String(),
  type: Type.Literal('message.created'),
  triggers: Type.Array(Type.String()),
  occurred_at: Type.String(),
  bot_id: Type.String(),
  room_id: Type.String(),
  thread_root_id: Type.Union([Type.String(), Type.Null()]),
  message: Type.Object({ id: Type.String(), author_id: Type.String(), body: Type.String() })
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

/** Process-local conversations shared by successive source generations. */
export function createConversationState() {
  return {
    conversations: new Map<string, Conversation>(),
    seen: new Map<string, number>(),
    reserved: new Set<string>()
  };
}
export type ConversationState = ReturnType<typeof createConversationState>;

/** Scope a conversation to the bot, room, and thread. Everyone who addresses the bot in a
 * thread shares its conversation; permissions are checked per request, not per conversation.
 * Retained implementation metadata stores a hash of this key to authorize resumption,
 * so its value must stay stable for existing conversations. */
export function deliveryConversationKey(delivery: Delivery): string {
  return JSON.stringify([
    delivery.bot_id,
    delivery.room_id,
    delivery.thread_root_id ?? delivery.message.id
  ]);
}

/** The delivery tracker's key for one message to one bot. */
export const deliveryKeyFor = (botId: string, messageId: string) =>
  JSON.stringify([botId, messageId]);

/** True when the bot accepted this message as addressed to it: a direct message, a mention, or a
 * verified reply to one of its messages, from an allowed user. Accepted deliveries are kept for
 * 24 hours in process memory, so a restart forgets older ones. */
export function wasAddressed(state: ConversationState, botId: string, messageId: string) {
  return state.seen.has(deliveryKeyFor(botId, messageId));
}

/** Registration failed before acceptance; the source may retry this delivery. */
export class RegistrationError extends Error {
  constructor(cause: unknown) {
    super('Conversation registration failed', { cause });
  }
}

/** Share delivery routing within one process; task code receives a normal context. */
export function createChattoRouter<Output extends TSchema>({
  name,
  output,
  post,
  run,
  state = createConversationState()
}: {
  name: string;
  output: Output;
  post: ChattoPost;
  state?: ConversationState;
  run: (
    ctx: WorkflowContext,
    delivery: Delivery,
    destination: Destination,
    inbox: ChattoInbox
  ) => Promise<Static<Output>>;
}) {
  const { conversations, seen, reserved } = state;
  const deliveries = createDeliveryTracker({ accepted: seen });
  const deliveryKey = (delivery: Delivery) => deliveryKeyFor(delivery.bot_id, delivery.message.id);
  const routeDelivery = (
    delivery: Delivery
  ): 'start' | 'ignored' | 'duplicate' | 'queued' | 'cancelled' => {
    if (delivery.message.author_id === delivery.bot_id) return 'ignored';
    if (
      !isDirectMessage(delivery) &&
      !delivery.triggers.includes('mention') &&
      !delivery.triggers.includes('reply')
    )
      return 'ignored';
    const id = deliveryKey(delivery);
    if (deliveries.has(id) || reserved.has(id)) return 'duplicate';
    const key = deliveryConversationKey(delivery);
    const conversation = conversations.get(key);
    if (conversation) {
      if (conversation.cancelled) return 'ignored';
      if (delivery.message.body.trim() === '/cancel') {
        conversation.cancelled = true;
        deliveries.accept(id);
        // abort() intentionally throws; the workflow observes the same signal.
        try {
          conversation.ctx?.abort('Cancelled from Chatto');
        } catch {
          // The abort signal also reaches the running task.
        }
        return 'cancelled';
      }
      conversation.messages.push(delivery);
      deliveries.accept(id);
      for (const listener of conversation.listeners) {
        listener();
      }
      return 'queued';
    }
    if (delivery.message.body.trim() === '/cancel') return 'ignored';
    conversations.set(key, { messages: [], listeners: new Set(), cancelled: false });
    return 'start';
  };
  const route: WebhookRouter<Delivery> = async (ctx, delivery) => {
    if (routeDelivery(delivery) !== 'start') return;
    const id = deliveryKey(delivery);
    reserved.add(id);
    try {
      await ctx.start(workflow, { input: delivery });
      deliveries.accept(id);
    } catch (error) {
      // A failed journal creation must leave the delivery retryable.
      if (reserved.delete(id)) {
        conversations.delete(deliveryConversationKey(delivery));
      }
      throw new RegistrationError(error);
    }
    // Registration can finish before execution enters the task. Keep its
    // reservation until the task consumes it, so it is not treated as a duplicate.
  };

  const workflow = task(
    {
      name,
      input: deliverySchema,
      output: Type.Union([Type.String(), output])
    },
    async (ctx, delivery) => {
      if (!reserved.delete(deliveryKey(delivery))) {
        const outcome = routeDelivery(delivery);
        if (outcome !== 'start') return outcome;
      }
      deliveries.accept(deliveryKey(delivery));
      const threadRootId = delivery.thread_root_id ?? delivery.message.id;
      const key = deliveryConversationKey(delivery);
      const conversation = conversations.get(key)!;
      conversation.ctx = ctx;
      const inbox: ChattoInbox = {
        drain: () => conversation.messages.splice(0),
        subscribe: (listener) => {
          conversation.listeners.add(listener);
          return () => {
            conversation.listeners.delete(listener);
          };
        }
      };
      const destination = { roomId: delivery.room_id, threadRootId };

      try {
        if (conversation.cancelled) ctx.abort('Cancelled from Chatto');
        return await run(ctx, delivery, destination, inbox);
      } catch (error) {
        if (conversation.cancelled) {
          await post(destination, 'Conversation cancelled.', AbortSignal.timeout(10_000)).catch(
            () => {}
          );
        }
        if (!conversation.cancelled && !ctx.signal.aborted) {
          await post(
            destination,
            'I could not finish that reply. Please try again.',
            AbortSignal.timeout(10_000)
          ).catch(() => {});
        }
        throw error;
      } finally {
        conversation.listeners.clear();
        conversations.delete(key);
      }
    }
  );
  Object.assign(route, { label: name, input: deliverySchema });
  return Object.assign(workflow, { route });
}

export const isDirectMessage = (delivery: Delivery) => delivery.triggers.includes('direct_message');

export function messageSignal(ctx: WorkflowContext): AbortSignal {
  return AbortSignal.any([ctx.signal, AbortSignal.timeout(10_000)]);
}
