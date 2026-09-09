const ERROR_REPLY = "Sorry, I couldn't generate a reply. Please try again.";

/** One final answer or error notification per webhook run. */
export interface ReplySender {
  /** Confirmed final-answer ID; error notifications do not complete delivery. */
  readonly id: string | undefined;

  /** Send the final answer. Repeated calls share the first POST attempt. */
  sendFinal(text: string): Promise<string>;

  /** Send a fixed error only if no answer or notification POST was attempted. */
  notifyFailure(): Promise<void>;
}

/** Share one POST attempt, including failures whose delivery may be uncertain. */
export function createReplySender(
  post: (text: string, stepName: string) => Promise<string>,
): ReplySender {
  let id: string | undefined;
  let attempt: Promise<string> | undefined;

  function send(text: string, finalAnswer: boolean): Promise<string> {
    const body = text.trim();
    if (!body) return Promise.reject(new Error("The reply must not be empty"));

    // Store the attempt before starting I/O. Never retry an uncertain POST.
    return (attempt ??= Promise.resolve().then(async () => {
      const messageId = await post(
        body,
        finalAnswer ? "Send final answer" : "Send error reply",
      );
      if (finalAnswer) id = messageId;
      return messageId;
    }));
  }

  return {
    get id() {
      return id;
    },
    sendFinal: (text) => send(text, true),
    async notifyFailure() {
      if (attempt) return;
      try {
        await send(ERROR_REPLY, false);
      } catch {
        // Preserve the original failure. Notifications are best effort.
      }
    },
  };
}
