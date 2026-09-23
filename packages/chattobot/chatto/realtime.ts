import { createChattoClient, type RealtimeCheckpoint } from "@chatto/client";
import { createBotClient, type AddressedMessage } from "@chatto/bot-client";
import { threadLocation } from "../thread.ts";
import type { EventSource } from "runling/web";
import { log } from "runling";
import { setTimeout as delay } from "node:timers/promises";
import { createChattoBot } from "../workflows/chat.ts";
import { investigationSettings } from "../workflows/investigate.ts";
import { implementationSettings } from "../workflows/implement.ts";
import { createConversationState, RegistrationError, type ConversationState, type Delivery } from "./routing.ts";

interface Session {
  identity: string;
  apiKey: string;
  checkpoint: RealtimeCheckpoint;
  conversations: ConversationState;
}

/** Convert authorized public message events into the bot's existing conversation input. */
export function messageDelivery(message: AddressedMessage, botId: string): Delivery {
  return {
    version: 1, id: message.id, type: "message.created", triggers: message.reasons,
    occurred_at: message.occurredAt ?? "",
    bot_id: botId, room_id: message.roomId, thread_root_id: message.threadRootId ?? null,
    message: { id: message.id, author_id: message.authorId, body: message.body },
  };
}

/** Outbound-only bot source. Conversation state survives reloads for the same bot identity. */
export const chattoSource: EventSource = async ctx => {
  const serverUrl = process.env.CHATTO_URL;
  const apiKey = process.env.CHATTO_API_KEY;
  // Capture policy for this source generation. Match the server-authenticated
  // actor ID, never a display name or user-supplied message field.
  const allowedUserId = process.env.CHATTO_ALLOWED_USER_ID?.trim() || undefined;
  if (!serverUrl || !apiKey) throw new Error("Set CHATTO_URL and CHATTO_API_KEY");
  const client = createChattoClient({ serverUrl, apiKey });
  const botClient = await createBotClient(client, { signal: ctx.signal });
  const botId = botClient.viewerId;
  const identity = JSON.stringify([new URL(serverUrl).origin, botId]);
  let session = ctx.state.get("chatto") as Session | undefined;
  if (!session || session.identity !== identity) {
    session = { identity, apiKey, checkpoint: {}, conversations: createConversationState() };
    ctx.state.set("chatto", session);
  } else if (session.apiKey !== apiKey) {
    session.apiKey = apiKey;
    session.checkpoint = {};
  }
  // Active runs keep this generation's client even if a reload changes credentials.
  const bot = createChattoBot({
    state: session.conversations, model: process.env.CHATTO_AGENT_MODEL,
    investigation: investigationSettings(),
    implementation: implementationSettings(),
    post: client.postMessage, typing: client.refreshTyping,
    readThread: (delivery, signal) => botClient.readThread(threadLocation(delivery), signal),
    acknowledge: (delivery, signal) => client.addReaction(delivery.room_id, delivery.message.id, "eyes", signal),
  });
  await client.consumeRealtime({
    signal: ctx.signal, checkpoint: session.checkpoint,
    onStatus(status) {
      // Only fixed local status fields are logged, never connection details or payloads.
      if (status.state === "ready" && status.gap) console.warn("Chatto realtime recovery gap: some messages may have been missed.");
      else if (status.state === "ready") log.success("Chatto realtime connected");
      else log.info(`Chatto realtime: ${status.state}`);
    },
    async onEvent(event) {
      // Ignore before routing: disallowed senders cannot start, steer, cancel,
      // or trigger acknowledgements for an existing conversation.
      if (allowedUserId && event.actorId !== allowedUserId) return;
      const message = await botClient.addressedMessage(event, { signal: ctx.signal }).catch(() => {
        ctx.signal.throwIfAborted();
        // Missing reply targets do not stop this bot's realtime source.
        console.warn("ChattoBot could not verify a reply target; ignoring the unmentioned message.");
        return undefined;
      });
      if (!message) return;
      const delivery = messageDelivery(message, botId);
      for (let attempt = 0; ; attempt++) {
        ctx.signal.throwIfAborted();
        try { await ctx.dispatch(bot.route, delivery); return; }
        catch (error) {
          // Runling wraps routing failures with their cause. Only failed run
          // registration is retryable; accepted inbox deliveries are never retried.
          const cause = error instanceof Error ? error.cause : undefined;
          if (!(cause instanceof RegistrationError) || attempt >= 2) throw error;
          await delay(250 * 2 ** attempt, undefined, { signal: ctx.signal });
        }
      }
    },
  });
};
