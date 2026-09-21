import type { InputHandler } from "./input.ts";
import {
  accumulateTokenUsage,
  emptyTokenUsage,
  hasValidTokenCounts,
  type TokenUsage,
  type TokenUsageInput,
} from "./usage.ts";

export class WorkflowAbortError extends Error {
  override readonly name = "WorkflowAbortError";

  constructor(reason = "Workflow aborted") {
    super(reason);
  }
}

export type TextHandler = (text: string) => void | Promise<void>;

export interface WorkflowContext<Incoming = never, Update = unknown> {
  /** Incoming values for a spawned task; empty for a normal context. */
  readonly inbox: AsyncIterable<Incoming>;

  /** Queue an update for the spawning parent; no-op on a normal context. */
  emit(update: Update): Promise<void>;

  /** Deliver user-facing text. Await the handler to wait for delivery. */
  onText?: TextHandler;

  /** Handle questions from this context. Hosts can supply the initial handler. */
  onInput?: InputHandler;

  /** Signals cancellation to agents and other cooperative work. */
  readonly signal: AbortSignal;

  /** Abort this workflow and throw its abort error. The first reason is retained. */
  abort(reason?: string): never;

  /** A shared read-only view of all usage recorded in this execution. */
  readonly usage: Readonly<TokenUsage>;

  /** Add one usage increment, including any reported cost in US dollars. */
  recordUsage(usage: TokenUsageInput): void;
}

/** Create an independent context for direct task calls. */
export function createWorkflowContext(): WorkflowContext {
  return createObservedWorkflowContext();
}

/** Internal bridge from context accounting to runner events. */
export function createObservedWorkflowContext(
  onUsage?: (usage: TokenUsage) => void,
  cancellation?: AbortSignal,
): WorkflowContext {
  const total = emptyTokenUsage();
  const controller = new AbortController();
  const signal = cancellation
    ? AbortSignal.any([controller.signal, cancellation])
    : controller.signal;

  const usage: Readonly<TokenUsage> = Object.freeze({
    get input() {
      return total.input;
    },
    get output() {
      return total.output;
    },
    get cacheRead() {
      return total.cacheRead;
    },
    get cacheWrite() {
      return total.cacheWrite;
    },
    get cost() {
      return total.cost;
    },
    get costIncomplete() {
      return total.costIncomplete;
    },
  });

  return {
    // Direct calls do not allocate channels or retain emitted updates.
    inbox: { async *[Symbol.asyncIterator]() {} },
    async emit() {},
    onInput: undefined,
    onText: undefined,
    signal,
    usage,

    abort(reason) {
      if (!signal.aborted) {
        controller.abort(new WorkflowAbortError(reason));
      }
      throw signal.reason;
    },

    recordUsage(usage) {
      if (!hasValidTokenCounts(usage)) return;
      accumulateTokenUsage(total, usage);
      onUsage?.({ ...total });
    },
  };
}
