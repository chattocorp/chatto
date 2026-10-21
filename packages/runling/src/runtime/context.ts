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
  /** The server re-entered an open root from a snapshot. No prior input is replayed by the host. */
  resumed?: boolean;
  /** Host-owned files for this run. The server removes this directory with expired history.
   * Absent when the host does not provide persistent run storage. */
  filesDirectory?: string;
}

/**
 * What a running task can do. `Steering` comes from the parent; `Output` streams back to it.
 * The host is the parent of a workflow's top-level task.
 */
export interface TaskContext<Steering = unknown, Output = unknown> {
  /** Mutable JSON data. The server snapshots the root task's object.
   * Children get a fresh, unsaved object; pass selected sub-objects as input to share state. */
  readonly state: WorkflowState;
  /** Start a task as a child. Exchange messages with it through the returned handle.
   * A direct call, `await task(ctx, input)`, starts the task and waits for its result. */
  start<T extends AnyTask>(task: T, input: TaskInput<T>): TaskInvocation<T>;

  /** Steering from the parent. One consumer only. Ends when the parent closes steering. */
  readonly steering: AsyncIterable<Steering>;

  /** Send a message to the parent. Rejects when the message does not match the task's
   * `output` schema, or when the parent does not read its output and the buffer is full. */
  emit(message: Output): Promise<void>;

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

/** Application-owned recovery data. Store JSON-compatible values only. */
export type WorkflowState = Record<string, unknown>;

/** Create a context without a parent, for example to call a task in a test or a script. */
export function createTaskContext(): TaskContext<never, unknown> {
  return createRootContext();
}

/** Internal: the host context of a run. Its steering is empty and its updates are discarded. */
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
    state: {},
    // Use the receiving context, so that children get the signal of their own parent.
    start(task, input) {
      return invoke(this, task, input) as TaskInvocation<typeof task>;
    },
    steering: { async *[Symbol.asyncIterator]() {} },
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
