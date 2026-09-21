import { spawn, type WorkflowContext } from "runling";
import { withChattoTyping, type ChattoTyping } from "./typing.ts";
import { messageSignal, type ChattoInbox, type ChattoPost, type Destination, type Delivery } from "./webhook.ts";

export type ConversationActivityHandler = (busy: boolean) => void;

/** Connect task channels and activity updates to a Chatto thread. */
export async function runConversationTask<Result>(
  ctx: WorkflowContext,
  inbox: ChattoInbox,
  run: (ctx: WorkflowContext<string, string>, onBusy: ConversationActivityHandler) => Promise<Result>,
  { destination, post, typing, acknowledge, delivery }: {
    destination: Destination;
    post: ChattoPost;
    typing?: ChattoTyping;
    acknowledge: (delivery: Delivery, signal: AbortSignal) => Promise<void>;
    delivery: Delivery;
  },
): Promise<Result> {
  let busy = true;
  const onBusy: ConversationActivityHandler = value => {
    busy = value;
  };

  // Keep the refresh timer alive while idle, but only send typing while busy.
  const refreshTyping: ChattoTyping | undefined = typing
    ? async (target, signal) => {
      if (busy) {
        await typing(target, signal);
      }
    }
    : undefined;

  return withChattoTyping(ctx, destination, refreshTyping, async () => {
    const closed = new AbortController();
    const signal = AbortSignal.any([ctx.signal, closed.signal]);
    const react = async (message: Delivery) => {
      signal.throwIfAborted();
      try {
        await acknowledge(message, AbortSignal.any([signal, AbortSignal.timeout(10_000)]));
      } catch {
        signal.throwIfAborted();
        // A missing reaction permission must not prevent an otherwise valid reply.
        console.warn("ChattoBot could not add the eyes reaction; continuing the reply.");
      }
      signal.throwIfAborted();
    };
    await react(delivery);
    const child = spawn(ctx, run, onBusy);
    let pending = Promise.resolve();
    const flush = () => {
      for (const message of inbox.drain()) {
        pending = pending.then(async () => {
          signal.throwIfAborted();
          await child.send(message.message.body);
        });
        void pending.catch(error => child.cancel(error));
      }
    };
    const unsubscribe = inbox.subscribe(flush);
    flush();

    try {
      for await (const text of child.updates) {
        await post(destination, text, messageSignal(ctx));
      }

      return await child.result;
    } finally {
      // Release routing and child work even if posting to Chatto fails.
      closed.abort(new Error("Conversation ended"));
      unsubscribe();
      child.cancel();
      await pending.catch(() => {});
    }
  });
}
