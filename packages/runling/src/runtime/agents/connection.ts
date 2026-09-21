import { reportMessageReceipt } from "../message-observation.ts";
import type { AgentResult, AgentRunOptions, RunlingAgent } from "../agent.ts";
import type { WorkflowContext } from "../context.ts";

export interface AgentConnectionOptions {
  /** One consumer for this connection's lifetime. The caller retains missed messages. */
  inbox?: AsyncIterable<string>;
  /** Ordered delivery of completed assistant text, awaited before a turn returns. */
  onText?: (text: string) => void | Promise<void>;
  /** True means the agent consumed the message, not merely that it was queued. */
  onDelivery?: (text: string, consumed: boolean) => void | Promise<void>;
}

export interface AgentConnection extends AsyncDisposable {
  /** Cancellation or background inbox failure for the connection lifetime. */
  readonly signal: AbortSignal;
  runOutcome(
    prompt: string,
    options?: Pick<AgentRunOptions, "signal">,
  ): Promise<AgentResult>;
  /** Disconnect and cancel pending work. Does not dispose the supplied agent. */
  dispose(): Promise<void>;
}

type ConnectableAgent = Pick<RunlingAgent, "runOutcome"> &
  Partial<Pick<RunlingAgent, "steer">>;

/**
 * Connect an agent to an explicit input stream and asynchronous output handlers.
 * Turns are sequential. Idle or rejected messages are reported as undelivered.
 */
export function connectAgent(
  ctx: WorkflowContext<unknown>,
  agent: ConnectableAgent,
  options: AgentConnectionOptions = {},
): AgentConnection {
  const controller = new AbortController();
  const signal = AbortSignal.any([ctx.signal, controller.signal]);
  const inbox = options.inbox?.[Symbol.asyncIterator]();
  let active = false;
  let disposed = false;
  let disposal: Promise<void> | undefined;
  let receiving: Promise<void> | undefined;
  let delivery = Promise.resolve();

  // Race I/O with cancellation so a pending read or user callback cannot hold
  // the connection open. The underlying operation must still cooperate to stop.
  async function interruptible<T>(
    work: Promise<T>,
    currentSignal = signal,
  ): Promise<T> {
    if (currentSignal.aborted) {
      void work.catch(() => {});
      throw currentSignal.reason;
    }

    let onAbort!: () => void;
    const cancelled = new Promise<never>((_resolve, reject) => {
      onAbort = () => reject(currentSignal.reason);
      if (currentSignal.aborted) onAbort();
      else currentSignal.addEventListener("abort", onAbort, { once: true });
    });

    try {
      return await Promise.race([work, cancelled]);
    } finally {
      currentSignal.removeEventListener("abort", onAbort);
    }
  }

  async function receive() {
    if (!inbox) return;

    while (!disposed && !signal.aborted) {
      const message = await interruptible(Promise.resolve(inbox.next()));
      if (message.done || disposed || signal.aborted) return;

      // Queue steering immediately. A receipt may wait for the next agent turn;
      // it must not prevent later user messages from reaching that same turn.
      const consumed =
        active && agent.steer
          ? Promise.resolve()
              .then(() => agent.steer!(message.value))
              .catch(() => false)
          : Promise.resolve(false);

      // Keep acknowledgement callbacks ordered even if receipts settle out of order.
      delivery = delivery.then(async () => {
        const delivered = await interruptible(consumed);
        signal.throwIfAborted();
        reportMessageReceipt(message, delivered);
        await options.onDelivery?.(message.value, delivered);
      });
      void delivery.catch((reason) => controller.abort(reason));
    }
  }

  function startReceiving() {
    if (receiving || !inbox) return;
    receiving = receive();
    void receiving.catch((reason) => controller.abort(reason));
  }

  async function runOutcome(
    prompt: string,
    runOptions: Pick<AgentRunOptions, "signal"> = {},
  ): Promise<AgentResult> {
    signal.throwIfAborted();
    if (disposed) throw new Error("Agent connection is disposed");
    if (active) throw new Error("Agent connection already has an active turn");

    const turnSignal = runOptions.signal
      ? AbortSignal.any([signal, runOptions.signal])
      : signal;
    turnSignal.throwIfAborted();
    active = true;
    let output = Promise.resolve();
    let acceptingText = true;

    try {
      // Start the interaction before draining queued input: steer requires an
      // active agent. No workflow context or transport is retained by the agent.
      const work = agent.runOutcome(ctx, prompt, {
        signal: turnSignal,
        onText(text) {
          if (!acceptingText) return;
          output = output.then(() => {
            turnSignal.throwIfAborted();
            return options.onText?.(text);
          });
          void output.catch((reason) => controller.abort(reason));
        },
      });
      startReceiving();

      const result = await interruptible(work, turnSignal);
      await interruptible(delivery, turnSignal);
      await interruptible(output, turnSignal);
      return result;
    } finally {
      acceptingText = false;
      // Preserve already queued replies when the agent itself fails.
      await interruptible(output, turnSignal).catch(() => {});
      active = false;
    }
  }

  function dispose(): Promise<void> {
    if (disposal) return disposal;
    disposed = true;
    controller.abort(new Error("Agent connection is disposed"));

    disposal = (async () => {
      // Observe return() even if an arbitrary iterable cannot finish its pending
      // read. Cancellation has already released our own receiver.
      if (inbox?.return) {
        await interruptible(
          Promise.resolve().then(() => inbox.return!()),
        ).catch(() => {});
      }
      await receiving?.catch(() => {});
    })();
    return disposal;
  }

  return { signal, runOutcome, dispose, [Symbol.asyncDispose]: dispose };
}
