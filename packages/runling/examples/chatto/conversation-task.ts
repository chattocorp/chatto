import { spawn, type WorkflowContext } from "runling";
import { withChattoTyping, type ChattoTyping } from "./typing.ts";
import { messageSignal, type ChattoInbox, type ChattoPost, type Destination } from "./webhook.ts";

export type ConversationActivityHandler = (busy: boolean) => void;

/** Connect task channels and activity updates to a Chatto thread. */
export async function runConversationTask<Result>(
  ctx: WorkflowContext,
  inbox: ChattoInbox,
  run: (ctx: WorkflowContext<string, string>, onBusy: ConversationActivityHandler) => Promise<Result>,
  { destination, post, typing }: {
    destination: Destination;
    post: ChattoPost;
    typing?: ChattoTyping;
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
    const child = spawn(ctx, run, onBusy);
    const flush = () => {
      for (const message of inbox.drain()) {
        void child.send(message).catch(error => child.cancel(error));
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
      unsubscribe();
      child.cancel();
    }
  });
}
