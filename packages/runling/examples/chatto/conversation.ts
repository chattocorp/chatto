import type { InputHandler, WorkflowContext } from "runling";
import type { ChattoAgentOptions } from "./agent.ts";
import { messageSignal, type ChattoPost } from "./webhook.ts";

/** Per-run presentation: ordered replies and no typing while input is pending. */
export function createChattoConversation(
  rootCtx: WorkflowContext,
  options: ChattoAgentOptions,
) {
  let outgoing = Promise.resolve();
  let questions = 0;

  const send: ChattoPost = (target, text, signal) => {
    const sent = outgoing.then(() => options.post(target, text, signal));
    // A failed post belongs to its caller; later replies must still be deliverable.
    outgoing = sent.catch(() => {});
    return sent;
  };

  const onInput: InputHandler = async (request) => {
    await outgoing;
    request.signal?.throwIfAborted();
    questions++;

    try {
      if (!rootCtx.onInput) throw new Error("Chatto conversation requires an input handler");
      return await rootCtx.onInput(request);
    } finally {
      questions--;
    }
  };

  const ctx = { ...rootCtx, onInput };
  const say = (text: string) => send(options.destination, text, messageSignal(rootCtx));
  const agentOptions: ChattoAgentOptions = {
    ...options,
    post: (_target, text) => say(text),
    typing: options.typing
      ? (target, signal) => questions ? Promise.resolve() : options.typing!(target, signal)
      : undefined,
  };

  return { ctx, say, agentOptions };
}
