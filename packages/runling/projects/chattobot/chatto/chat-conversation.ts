import { Type, type WorkflowContext } from "runling";
import { runConversationTask, type ConversationActivityHandler } from "./conversation-task.ts";
import { sendChattoTyping, type ChattoTyping } from "./typing.ts";
import { createChattoWebhook, postToChatto, type ChattoPost, type Delivery } from "./webhook.ts";

/** Settings supplied to a conversation task by its host. */
export type ConversationOptions<Settings> = Settings & {
  onBusy: ConversationActivityHandler;
  delivery: Delivery;
};

/** Connect a string-in/string-out task to a Chatto thread. */
export function chattoConversation<Settings>({
  name,
  task,
  settings,
  acknowledge,
  post = postToChatto,
  typing = sendChattoTyping,
}: {
  name: string;
  task: (
    ctx: WorkflowContext<string, string>,
    prompt: string,
    options: ConversationOptions<Settings>,
  ) => Promise<string>;
  settings: Settings;
  acknowledge: (delivery: Delivery, signal: AbortSignal) => Promise<void>;
  post?: ChattoPost;
  typing?: ChattoTyping;
}) {
  return createChattoWebhook({
    name,
    output: Type.Object({ reply: Type.String() }),
    post,
    async run(ctx, delivery, destination, inbox) {
      const reply = await runConversationTask(ctx, inbox, (childCtx, onBusy) => {
        return task(childCtx, delivery.message.body, { ...settings, onBusy, delivery });
      }, { destination, post, typing, acknowledge, delivery });

      // Updates already posted the text; this value is only the saved result.
      return { reply };
    },
  });
}
