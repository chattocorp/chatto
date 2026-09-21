import { task, validateTimeout, type WorkflowContext } from "runling";
import { runAgentConversation } from "runling/agents";
import { chattoConversation, type ConversationOptions } from "./chatto/chat-conversation.ts";
import { createWebChatAgent } from "./chatto/web-chat-agent.ts";

type Settings = Parameters<typeof createWebChatAgent>[0] & { timeout?: number };

// A task is an async function with a workflow context as its first argument.
// This one owns a whole conversation, so the agent remembers earlier messages.
export const conversation = task(async (
  ctx: WorkflowContext<string, string>,
  prompt: string,
  options: ConversationOptions<Settings>,
) => {
  validateTimeout(options.timeout);
  const bot = await createWebChatAgent(options);

  try {
    // The first message starts the agent. Later messages arrive in ctx.inbox;
    // replies leave through ctx.emit. This helper manages that conversation.
    // After each reply, it waits up to 15 minutes for another message.
    return await runAgentConversation(ctx, bot, prompt, {
      timeout: options.timeout ?? 900,
      onBusy: options.onBusy,
    });
  } finally {
    // Always release the agent, including when the user cancels the run.
    bot.dispose();
  }
});

// New DMs start the task. Thread replies reach its inbox, and emitted text
// is posted back to the thread. The adapter also handles typing and /cancel.
export default chattoConversation({
  name: "Chatto agent demo",
  task: conversation,
  settings: {
    directory: process.cwd(),
    model: process.env.CHATTO_AGENT_MODEL,
  },
});
