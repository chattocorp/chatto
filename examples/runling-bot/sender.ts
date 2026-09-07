const ERROR_REPLY = "Sorry, I couldn't generate a reply. Please try again.";

/** One delivery's reply attempt, shared by the agent and the error fallback. */
export interface ReplySender {
  /** Confirmed Chatto message ID; absent until the HTTP request succeeds. */
  readonly id: string | undefined;

  /** Repeated calls share the first attempt, including a failed attempt. */
  send(text: string): Promise<string>;

  /** Best-effort error notification, only if no reply attempt was started. */
  notifyFailure(): Promise<void>;
}

/** Prevent duplicate POSTs within a run, including after ambiguous HTTP failures. */
export function createReplySender(
  post: (text: string, stepName: string) => Promise<string>,
): ReplySender {
  let id: string | undefined;
  let attempt: Promise<string> | undefined;

  function send(text: string, stepName: string): Promise<string> {
    if (!text.trim()) {
      return Promise.reject(new Error("The reply must not be empty"));
    }

    // Store the promise before starting the POST, so concurrent calls share it.
    // Keep failed attempts too: Chatto may have accepted the message even if
    // its response did not reach us. A second POST could send a duplicate.
    return (attempt ??= Promise.resolve().then(async () => {
      id = await post(text, stepName);

      return id;
    }));
  }

  return {
    get id() {
      return id;
    },

    send: (text) => send(text, "Send reply to Chatto"),

    async notifyFailure() {
      // The fallback is safe only when no message send has been attempted.
      if (attempt) {
        return;
      }

      try {
        await send(ERROR_REPLY, "Send error reply");
      } catch {
        // Preserve the original failure; the send step records this one.
      }
    },
  };
}
