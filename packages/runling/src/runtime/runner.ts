import { createTimeout } from './timeout.ts';
import { createChannel } from './channel.ts';
import { createRootContext, type RunIdentity } from './context.ts';
import { withExecutionServices } from './execution.ts';
import { resolve } from 'node:path';
import { createInterface } from 'node:readline';
import { pathToFileURL } from 'node:url';
import type { RunOptions } from './cli.ts';
import {
  observeRunlingEvents,
  emitRunlingEvent,
  bindRunlingContext,
  type RunlingEventListener
} from './events.ts';
import { log } from './log.ts';
import { renderMarkdown } from './markdown.ts';
import {
  createRunJournal,
  eventRecord,
  newRun,
  runJournalDirectory,
  type RunDetail,
  type RunJournal
} from './run-journal.ts';
import { relative } from 'node:path';
import { isTask, type AnyTask, type TaskInput, type TaskOutput } from './task.ts';
import { emptyTokenUsage, formatTokenUsage, type TokenUsage, totalTokens } from './usage.ts';

export function formatDuration(ms: number): string {
  if (ms < 1000) {
    return `${Math.round(ms)}ms`;
  }

  const tenths = Math.round(ms / 100);
  if (tenths < 600) {
    return `${(tenths / 10).toFixed(1)}s`;
  }

  // Round before splitting the duration so seconds cannot render as `60s`.
  const totalSeconds = Math.round(ms / 1000);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  const parts: string[] = [];

  if (hours > 0) parts.push(`${hours}h`);
  if (minutes > 0) parts.push(`${minutes}m`);
  if (seconds > 0) parts.push(`${seconds}s`);

  return parts.join('') || '0s';
}

export interface WorkflowExecution<Output = unknown> {
  durationMs: number;
  usage: TokenUsage;
  /** The top-level task's validated output, or null when the run failed. */
  output: Output | null;
  error: string | null;
  ok: boolean;
}

export interface TerminalCapabilities {
  isTTY?: boolean;
  columns?: number;
}

export interface RunWorkflowOptions<Input = unknown> {
  input: Input;
  /** Maximum elapsed time in seconds. Omit it to wait without a deadline. Cancellation is
   * cooperative. */
  timeout?: number;
  signal?: AbortSignal;
  verbose?: boolean;
  onEvent?: RunlingEventListener;
  /** Messages from the host to the top-level task. When the iterable ends, the task's input
   * closes. Without it, the task's input is closed from the start. */
  messages?: AsyncIterable<unknown>;
  /** Receives the messages that the top-level task emits. Without it, they are discarded after
   * the journal records them. */
  onUpdate?: (message: unknown) => void | Promise<void>;
  /** The run that `ctx.run` identifies. Hosts that record runs pass it. */
  run?: RunIdentity;
}

/** Show a workflow's output in a terminal: text as Markdown, other values as JSON. */
export function formatWorkflowOutput(
  output: unknown,
  terminal: TerminalCapabilities = process.stdout
): string {
  if (typeof output !== 'string') return JSON.stringify(output, null, 2);
  return terminal.isTTY ? renderMarkdown(output, terminal.columns ?? 80) : output;
}

/** Run a workflow: one top-level invocation of a task, with the host as its parent. This does not
 * assume a terminal, print, or change process state. */
export async function runWorkflow<T extends AnyTask>(
  task: T,
  {
    input,
    verbose = false,
    onEvent = () => {},
    timeout,
    signal,
    messages,
    onUpdate,
    run: identity
  }: RunWorkflowOptions<TaskInput<T>>
): Promise<WorkflowExecution<TaskOutput<T>>> {
  return withExecutionServices({ verbose }, () =>
    observeRunlingEvents(onEvent, () =>
      log.withDestination('silent', () =>
        captureExecution(task, input, { timeout, signal, messages, onUpdate, identity })
      )
    )
  ) as Promise<WorkflowExecution<TaskOutput<T>>>;
}

/** Write a top-level task's updates to the log: text as it is, other values as JSON. */
export function logUpdate(update: unknown): void {
  const text = typeof update === 'string' ? update : JSON.stringify(update);
  if (text?.trim()) log.info(text);
}

async function captureExecution(
  task: AnyTask,
  input: unknown,
  {
    timeout,
    signal,
    messages,
    onUpdate,
    identity
  }: Pick<RunWorkflowOptions, 'timeout' | 'signal' | 'messages' | 'onUpdate'> & {
    identity?: RunIdentity;
  }
): Promise<WorkflowExecution> {
  const deadline = createTimeout(timeout, 'Workflow');
  const root = createRootContext(
    bindRunlingContext((usage: TokenUsage) => emitRunlingEvent({ type: 'usage.updated', usage })),
    signal && deadline.signal
      ? AbortSignal.any([signal, deadline.signal])
      : (signal ?? deadline.signal),
    identity
  );
  const start = performance.now();
  let output: unknown = null;
  let error: string | null = null;
  let invocation: ReturnType<typeof root.start> | undefined;

  try {
    root.signal.throwIfAborted();
    invocation = root.start(task, input);
    if (messages) void forward(messages, invocation, root.signal);
    else invocation.closeInput();
    // Always read updates: an unread buffer would make the task's emit fail.
    const { updates: messagesFromTask } = invocation;
    const updates = (async () => {
      for await (const update of messagesFromTask) await onUpdate?.(update);
    })();
    // A failed task also fails its updates; its result reports the failure.
    void updates.catch(() => {});
    output = await invocation.result;
    await updates;
    root.signal.throwIfAborted();
  } catch (cause) {
    output = null;
    const failure = root.signal.aborted ? root.signal.reason : cause;
    error = failure instanceof Error ? failure.message : String(failure);
  } finally {
    // A cancelled result settles at once. Wait for the task's cooperative cleanup, such as
    // `finally` blocks, so that a host does not stop while it runs.
    await invocation?.[Symbol.asyncDispose]();
    deadline.dispose();
  }

  return {
    durationMs: performance.now() - start,
    usage: { ...root.usage },
    output: output ?? null,
    error,
    ok: error === null
  };
}

/** Forward host messages to the top-level task until either side ends. A message that the task
 * cannot accept is logged and dropped; the host validates messages before it queues them. */
async function forward(
  messages: AsyncIterable<unknown>,
  invocation: {
    send(message: unknown): Promise<void>;
    closeInput(): void;
    result: Promise<unknown>;
  },
  signal: AbortSignal
) {
  const reader = messages[Symbol.asyncIterator]();
  const finished = invocation.result.then(
    () => ({ done: true as const, value: undefined }),
    () => ({ done: true as const, value: undefined })
  );
  try {
    while (!signal.aborted) {
      const next = await Promise.race([reader.next(), finished]);
      if (next.done) break;
      await invocation.send(next.value).catch((cause: unknown) => {
        log.error(
          `Message not delivered: ${cause instanceof Error ? cause.message : String(cause)}`
        );
      });
    }
  } catch {
    // A failed message source ends the input; the task decides what that means.
  } finally {
    invocation.closeInput();
    void reader.return?.();
  }
}

export async function loadWorkflow(path: string): Promise<AnyTask> {
  const resolvedPath = resolve(path);
  const module = await import(/* @vite-ignore */ pathToFileURL(resolvedPath).href);
  if (!isTask(module.default)) {
    throw new Error(`Workflow ${resolvedPath} must have a default task export`);
  }
  return module.default;
}

/** Read lines from an interactive terminal as messages for the top-level task. */
function terminalMessages(signal: AbortSignal): AsyncIterable<string> & { close(): void } {
  // Without terminal handling, the terminal echoes lines and Ctrl-C still signals the process.
  const prompt = createInterface({ input: process.stdin, terminal: false });
  const lines = createChannel<string>({ signal });
  prompt.on('line', (line) => void lines.send(line).catch(() => {}));
  prompt.on('close', () => lines.close());
  return {
    [Symbol.asyncIterator]: () => lines[Symbol.asyncIterator](),
    close: () => prompt.close()
  };
}

/** `runling run`: run a workflow file in this process, log its progress, and write its journal. */
export async function runRunling(workflowPath: string, prompt: string, options: RunOptions) {
  const { json, verbose } = options;
  // The first interrupt cancels the workflow so that its cleanup runs; a second one exits.
  const controller = new AbortController();
  const interrupt = () => {
    if (controller.signal.aborted) process.exit(130);
    log.error('Stopping. Press Ctrl-C again to exit without cleanup.');
    controller.abort(new Error('Interrupted'));
  };
  process.on('SIGINT', interrupt);
  process.on('SIGTERM', interrupt);
  const terminal = process.stdin.isTTY ? terminalMessages(controller.signal) : undefined;
  const started = performance.now();
  try {
    await log.withDestination(json ? 'stderr' : 'stdout', async () => {
      const task = await loadWorkflow(workflowPath);
      const input = options.input === undefined ? prompt : JSON.parse(options.input);
      let journal: { run: RunDetail; writer: RunJournal } | undefined;
      try {
        const run = newRun({ webhook: 'cli', workflow: workflowPath, source: 'cli', input });
        journal = {
          run,
          writer: await createRunJournal(await runJournalDirectory(process.cwd()), run)
        };
      } catch (cause) {
        log.error(`Cannot create a run journal: ${String(cause)}`);
      }
      log.info(
        journal
          ? `Runling starting run ${log.highlight(journal.run.reference!)}`
          : 'Runling starting'
      );
      if (terminal) log.info('Type a line and press Enter to send a message to the workflow.');
      const base = performance.now();
      const execute = () =>
        withExecutionServices({ verbose }, () =>
          log.indented(() =>
            captureExecution(task, input, {
              signal: controller.signal,
              messages: terminal,
              onUpdate: logUpdate,
              identity: journal && { id: journal.run.id, reference: journal.run.reference }
            })
          )
        );
      const execution = await (journal
        ? observeRunlingEvents(
            // A write failure is reported when the finished record is written.
            (event) => void journal.writer.append(eventRecord(event, base)).catch(() => {}),
            execute
          )
        : execute());

      if (execution.error !== null) {
        log.error(execution.error);
        process.exitCode = 1;
      } else if (!json && execution.output !== null) {
        console.log(`\n${formatWorkflowOutput(execution.output)}\n`);
      }
      if (totalTokens(execution.usage) > 0) {
        log.info(`Total token usage: ${formatTokenUsage(execution.usage)}`);
      }
      log.info(`Finished in ${formatDuration(execution.durationMs)}`);
      if (journal) {
        const path = relative(process.cwd(), journal.writer.path);
        try {
          await journal.writer.append({
            type: 'finished',
            status: controller.signal.aborted ? 'cancelled' : execution.ok ? 'completed' : 'failed',
            finishedAt: Date.now(),
            durationMs: execution.durationMs,
            usage: execution.usage,
            output: execution.output,
            error: execution.error
          });
          log.info(`Run ${journal.run.reference} journal: ${path}`);
        } catch (cause) {
          log.error(`Cannot save the run journal ${path}: ${String(cause)}`);
        }
      }
      if (json) console.log(JSON.stringify(execution));
    });
  } catch (cause) {
    const error = cause instanceof Error ? cause.message : String(cause);
    log.error(error);
    process.exitCode = 1;
    // `--json` always writes one execution document, also when the workflow cannot start.
    if (json) {
      const execution: WorkflowExecution = {
        durationMs: performance.now() - started,
        usage: emptyTokenUsage(),
        output: null,
        error,
        ok: false
      };
      console.log(JSON.stringify(execution));
    }
  } finally {
    terminal?.close();
    process.off('SIGINT', interrupt);
    process.off('SIGTERM', interrupt);
  }
}
