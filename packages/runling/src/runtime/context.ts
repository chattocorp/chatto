import { invoke } from './invocation.ts';
import type { AnyTask, TaskInput, TaskInvocation } from './task.ts';
import {
  accumulateTokenUsage,
  emptyTokenUsage,
  hasValidTokenCounts,
  type TokenUsage,
  type TokenUsageInput
} from './usage.ts';

export class WorkflowAbortError extends Error {
  override readonly name = 'WorkflowAbortError';

  constructor(reason = 'Workflow aborted') {
    super(reason);
  }
}

/** Identifies the run that a task context belongs to. */
export interface RunIdentity {
  /** The run's stable ID. */
  id: string;
  /** The run's human-readable name, such as `funky-comics-8426`. Older runs have none. */
  reference?: string;
}

/**
 * What a running task can do. `Receives` is the type of messages from the parent and `Emits`
 * is the type of messages to the parent. The host is the parent of a workflow's top-level task.
 */
export interface TaskContext<Receives = unknown, Emits = unknown> {
  /** Start a task as a child. Exchange messages with it through the returned handle.
   * A direct call, `await task(ctx, input)`, starts the task and waits for its result. */
  start<T extends AnyTask>(task: T, input: TaskInput<T>): TaskInvocation<T>;

  /** Messages from the parent. One consumer only. It ends when the parent closes the input. */
  readonly inbox: AsyncIterable<Receives>;

  /** Send a message to the parent. Rejects when the message does not match the task's
   * `emits` schema, or when the parent does not read its updates and the buffer is full. */
  emit(message: Emits): Promise<void>;

  /** Signals cancellation to agents and other cooperative work. */
  readonly signal: AbortSignal;

  /** The run of this context. Child tasks share their parent's run. Absent for direct task
   * calls and for hosts that do not record runs. */
  readonly run?: RunIdentity;

  /** Abort the workflow run and throw its abort error. The first reason is retained. */
  abort(reason?: string): never;

  /** A shared read-only view of all usage recorded in this run. */
  readonly usage: Readonly<TokenUsage>;

  /** Add one usage increment, including any reported cost in US dollars. */
  recordUsage(usage: TokenUsageInput): void;
}

/** Create a context without a parent, for example to call a task in a test or a script. */
export function createTaskContext(): TaskContext<never, unknown> {
  return createRootContext();
}

/** Internal: the host context of a run. Its inbox is empty and its updates are discarded. */
export function createRootContext(
  onUsage?: (usage: TokenUsage) => void,
  cancellation?: AbortSignal,
  run?: RunIdentity
): TaskContext<never, unknown> {
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
    }
  });

  return {
    // Use the receiving context, so that children get the signal of their own parent.
    start(task, input) {
      return invoke(this, task, input) as TaskInvocation<typeof task>;
    },
    inbox: { async *[Symbol.asyncIterator]() {} },
    async emit() {},
    signal,
    ...(run ? { run } : {}),
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
    }
  };
}
