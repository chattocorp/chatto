import { connectAgent } from "runling/agents";
import {
  createChannel,
  type RunlingAgent,
  type WorkflowContext,
} from "runling";
import { withChattoTyping, type ChattoTyping } from "./typing.ts";
import {
  messageSignal,
  type ChattoInbox,
  type ChattoPost,
  type Destination,
} from "./webhook.ts";

export interface ChattoAgentOptions {
  destination: Destination;
  inbox: ChattoInbox;
  post: ChattoPost;
  typing?: ChattoTyping;
  /** Commands handled by the workflow, never forwarded to the model. */
  reservedCommands?: readonly string[];
}

/** Run an agent with typing, live steering, and ordered replies to its Chatto thread. */
export function runChattoAgent(
  ctx: WorkflowContext,
  agent: Pick<RunlingAgent, "runOutcome" | "steer">,
  prompt: string,
  { destination, inbox, post, typing, reservedCommands }: ChattoAgentOptions,
) {
  return withChattoTyping(ctx, destination, typing, async () => {
    const messages = createChannel<string>();
    const pending: { text: string; consumed: boolean }[] = [];
    const offered: typeof pending = [];

    const flush = () => {
      for (const text of inbox.drain()) {
        const entry = { text, consumed: false };
        pending.push(entry);

        // Commands and overflow stay in the host backlog. Queue acceptance
        // alone must never remove a message from that backlog.
        if (reservedCommands?.includes(text.trim())) continue;
        offered.push(entry);
        void messages.send(text).catch(() => {
          const index = offered.indexOf(entry);
          if (index >= 0) offered.splice(index, 1);
        });
      }
    };

    const unsubscribe = inbox.subscribe(flush);
    try {
      await using connection = connectAgent(ctx, agent, {
        inbox: messages,
        onText: (text) => post(destination, text, messageSignal(ctx)),
        onDelivery: (_text, consumed) => {
          const entry = offered.shift();
          if (entry) entry.consumed = consumed;
        },
      });

      const run = connection.runOutcome(prompt);
      flush();
      return await run;
    } finally {
      unsubscribe();
      messages.close();
      inbox.prepend(
        pending.filter((entry) => !entry.consumed).map((entry) => entry.text),
      );
    }
  });
}
