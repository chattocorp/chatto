import { createTimeout } from './timeout.ts';
import { createChannel } from './channel.ts';
import { createRootContext, type RunIdentity, type WorkflowState } from './context.ts';
import { invoke } from './invocation.ts';
import { withExecutionServices } from './execution.ts';
import { dirname, relative, resolve } from 'node:path';
import { createInterface } from 'node:readline';
import { pathToFileURL } from 'node:url';
import type { RunOptions } from './cli.ts';
import {
  observeRunlingEvents,
  withRunlingEventScope,
  emitRunlingEvent,
  bindRunlingContext,
  type RunlingEventListener
} from './events.ts';
import { log, withRootLog } from './log.ts';
import { renderMarkdown } from './markdown.ts';
import { terminalColors } from './ansi.ts';
import { stripVTControlCharacters } from 'node:util';
import {
  createRunJournal,
  eventRecord,
  newRun,
  runJournalDirectory,
  type RunDetail,
  type RunJournal
} from './run-journal.ts';
import { isTask, type AnyTask, type TaskInput, type TaskResult } from './task.ts';
import { emptyTokenUsage, formatTokenUsage, totalTokens, type TokenUsage } from './usage.ts';

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
  /** The top-level task's validated final result, or null when the run failed.
   * The field keeps its existing name for journal and HTTP compatibility. */
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
  /** Root-owned state, supplied by the host. This function does not persist it. */
  state?: WorkflowState;
  /** Maximum elapsed time in seconds. Omit it to wait without a deadline. Cancellation is
   * cooperative. */
  timeout?: number;
  signal?: AbortSignal;
  verbose?: boolean;
  onEvent?: RunlingEventListener;
  /** Steering from the host to the top-level task. When the iterable ends, steering
   * closes. Without it, steering is closed from the start. */
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
  if (!terminal.isTTY) return output;
  const rendered = renderMarkdown(stripVTControlCharacters(output), terminal.columns ?? 80);
  return terminalColors(terminal) ? rendered : stripVTControlCharacters(rendered);
}

/** Run a workflow: one top-level invocation of a task, with the host as its parent. Its event
 * and log context is independent of the caller. This does not assume a terminal, print, or
 * change process state. */
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
    run: identity,
    state
  }: RunWorkflowOptions<TaskInput<T>>
): Promise<WorkflowExecution<TaskResult<T>>> {
  return withExecutionServices({ verbose }, () =>
    withRunlingEventScope(onEvent, () =>
      withRootLog(() =>
        log.withDestination('silent', () =>
          captureExecution(task, input, { timeout, signal, messages, onUpdate, identity, state })
        )
      )
    )
  ) as Promise<WorkflowExecution<TaskResult<T>>>;
}

async function captureExecution(
  task: AnyTask,
  input: unknown,
  {
    timeout,
    signal,
    messages,
    onUpdate,
    identity,
    state
  }: Pick<RunWorkflowOptions, 'timeout' | 'signal' | 'messages' | 'onUpdate' | 'state'> & {
    identity?: RunIdentity;
  }
): Promise<WorkflowExecution> {
  const deadline = createTimeout(timeout, 'Workflow');
  const lifetime = new AbortController();
  const root = createRootContext(
    bindRunlingContext((usage: TokenUsage) => emitRunlingEvent({ type: 'usage.updated', usage })),
    signal && deadline.signal
      ? AbortSignal.any([signal, deadline.signal])
      : (signal ?? deadline.signal),
    identity,
    lifetime.signal
  );
  const start = performance.now();
  let output: unknown = null;
  let error: string | null = null;
  let invocation: ReturnType<typeof root.start> | undefined;

  try {
    root.signal.throwIfAborted();
    invocation = invoke(root, task, input, { state });
    if (messages) void forward(messages, invocation, root.signal);
    else invocation.closeSteering();
    // Always read updates: an unread buffer would make the task's emit fail.
    const { output: messagesFromTask } = invocation;
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
    lifetime.abort(new Error('Workflow execution finished'));
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
    steer(message: unknown): Promise<void>;
    closeSteering(): void;
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
      await invocation.steer(next.value).catch((cause: unknown) => {
        log.error(
          `Message not delivered: ${cause instanceof Error ? cause.message : String(cause)}`
        );
      });
    }
  } catch {
    // A failed message source ends the input; the task decides what that means.
  } finally {
    invocation.closeSteering();
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

/** `runling run`: show run progress and print the root task's final output once, without
 * indentation. Agent replies and task updates remain in the journal. */
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
        log.withDestination('stderr', () =>
          log.error(`Cannot create a run journal: ${String(cause)}`)
        );
      }
      log.info(
        journal
          ? `Runling starting run ${log.highlight(journal.run.reference!)}`
          : 'Runling starting'
      );
      const base = performance.now();
      const execute = () =>
        withExecutionServices({ verbose, printAgentReplies: false }, () =>
          log.indented(() =>
            captureExecution(task, input, {
              signal: controller.signal,
              messages: terminal,
              identity: journal && {
                id: journal.run.id,
                reference: journal.run.reference,
                filesDirectory: resolve(dirname(journal.writer.path), journal.run.id)
              }
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
        log.withDestination('stderr', () => log.error(execution.error!));
        process.exitCode = 1;
      }
      if (totalTokens(execution.usage) > 0) {
        log.info(`Total token usage: ${formatTokenUsage(execution.usage)}`);
      }
      log.info(`Finished in ${formatDuration(execution.durationMs)}`);
      if (journal) {
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
          log.info(
            `Run ${journal.run.reference} journal: ${relative(process.cwd(), journal.writer.path)}`
          );
        } catch (cause) {
          log.withDestination('stderr', () =>
            log.error(`Cannot save the run journal ${journal.writer.path}: ${String(cause)}`)
          );
        }
      }
      if (json) console.log(JSON.stringify(execution));
      else if (execution.ok && execution.output !== null) {
        console.log('');
        console.log(formatWorkflowOutput(execution.output));
      }
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
