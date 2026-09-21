import type { WebhookTask } from "runling/web";
import { serverLog } from "../../runtime/server-log.ts";
import {
  mkdir,
  readdir,
  appendFile,
  writeFile,
  truncate,
  stat,
} from "node:fs/promises";
import { createReadStream } from "node:fs";
import { resolve, dirname } from "node:path";
import { randomUUID } from "node:crypto";
import {
  emptyTokenUsage,
  runWorkflow,
  type WorkflowExecution,
} from "runling";
import {
  type RunDetail,
  type RunRecord,
  type RunSummary,
} from "../runs.ts";

import { summarizeRunActivity } from "../run-activity.ts";

const validId = /^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/;
type Listener = (id: string, record: RunRecord) => void;

function summary(run: RunDetail): RunSummary {
  const { input: _, output: _o, error: _e, events: _v, ...value } = run;
  return value;
}

// Server-owned event arrays can grow in place. Browser state uses applyRecord.
function applyStoredRecord(run: RunDetail, record: RunRecord, includeDetails = true) {
  if (record.type === "event") {
    if (includeDetails) run.events.push(record.event);
    if (record.event.type === "usage.updated") run.usage = record.event.usage;
  } else if (record.type === "finished") {
    const { type: _, output, error, ...result } = record;
    Object.assign(run, result);
    if (includeDetails) Object.assign(run, { output, error });
  }
}

// Ignore an incomplete final line, but preserve its byte offset for recovery.
async function* journalRecords(path: string) {
  let buffer = "";
  for await (const chunk of createReadStream(path, { encoding: "utf8" })) {
    buffer += chunk;
    let end: number;
    while ((end = buffer.indexOf("\n")) !== -1) {
      const line = buffer.slice(0, end);
      buffer = buffer.slice(end + 1);
      yield { record: line ? JSON.parse(line) as RunRecord : undefined, bytes: Buffer.byteLength(line) + 1 };
    }
  }
}

/** One server process owns this journal directory. */
export class RunStore {
  private runs = new Map<string, RunSummary>();
  private details = new Map<string, RunDetail>();
  private pending = new Map<string, Promise<void>>();
  private controllers = new Map<string, AbortController>();
  private listeners = new Set<Listener>();

  constructor(readonly directory: string) {}

  async init(): Promise<void> {
    await mkdir(this.directory, { recursive: true });
    for (const file of await readdir(this.directory)) {
      const id = file.replace(/\.jsonl$/, "");
      if (!file.endsWith(".jsonl") || !validId.test(id)) continue;
      const path = resolve(this.directory, file);
      try {
        const { run, end, lastTimestamp } = await this.read(id, false);
        if (!run) continue;
        if (run.status === "running") {
          // A crash can leave the final JSON line incomplete.
          await truncate(path, end);
          const record: RunRecord = {
            type: "finished",
            status: "interrupted",
            finishedAt: Date.now(),
            durationMs: lastTimestamp,
            output: null,
            usage: run.usage,
            error: "The server stopped before this run finished.",
          };
          await appendFile(path, `${JSON.stringify(record)}\n`);
          applyStoredRecord(run, record, false);
          serverLog("warn", "run.interrupted", { runId: id });
        }
        this.runs.set(id, summary(run));
      } catch (cause) {
        serverLog("error", "run.restore_failed", { runId: id, error: cause });
      }
    }
  }

  list(): RunSummary[] {
    return [...this.runs.values()]
      .sort((a, b) => b.startedAt - a.startedAt)
      .slice(0, 100)
      .map((run) => {
        const detail = this.details.get(run.id);
        return { ...run, activity: detail ? summarizeRunActivity(detail) : null };
      });
  }

  async get(id: string): Promise<RunDetail | undefined> {
    if (!validId.test(id) || !this.runs.has(id)) return undefined;
    return this.details.get(id) ?? (await this.read(id, true)).run;
  }

  cancel(id: string): boolean {
    const controller = this.controllers.get(id);
    if (!controller) return false;
    controller.abort(new Error("Workflow cancelled by user."));
    return true;
  }

  private async read(id: string, includeDetails: boolean) {
    let run: RunDetail | undefined;
    let end = 0;
    let lastTimestamp = 0;
    for await (const line of journalRecords(resolve(this.directory, `${id}.jsonl`))) {
      end += line.bytes;
      const record = line.record;
      if (!record) continue;
      if (!run) {
        if (record.type !== "started" || record.run.id !== id) break;
        run = includeDetails ? record.run : {
          ...record.run, input: null, output: null, error: null, events: [],
        };
      } else {
        applyStoredRecord(run, record, includeDetails);
        if (record.type === "event") lastTimestamp = record.event.timestamp;
      }
    }
    return { run, end, lastTimestamp };
  }

  subscribe(listener: Listener): () => void {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  }

  private publish(id: string, record: RunRecord): void {
    for (const listener of this.listeners) listener(id, record);
  }

  private append(id: string, record: RunRecord): Promise<void> {
    const next = (this.pending.get(id) ?? Promise.resolve()).then(async () => {
      await appendFile(
        resolve(this.directory, `${id}.jsonl`),
        `${JSON.stringify(record)}\n`,
      );
      const run = this.details.get(id)!;
      applyStoredRecord(run, record);
      this.runs.set(id, summary(run));
      this.publish(id, record);
      if (record.type === "finished") this.details.delete(id);
    });
    this.pending.set(id, next);
    return next;
  }

  async start<Input, Output>(
    webhook: string,
    workflow: WebhookTask<Input, Output>,
    input: Input,
    source: "webhook" | "web",
  ) {
    const id = randomUUID();
    const run: RunDetail = {
      id,
      webhook,
      workflow: workflow.name,
      source,
      input: input === undefined ? null : JSON.parse(JSON.stringify(input)),
      status: "running",
      startedAt: Date.now(),
      output: null,
      error: null,
      events: [],
      usage: emptyTokenUsage(),
    };
    const started: RunRecord = { type: "started", run };
    await writeFile(
      resolve(this.directory, `${id}.jsonl`),
      `${JSON.stringify(started)}\n`,
      { flag: "wx", mode: 0o600 },
    );
    this.runs.set(id, summary(run));
    this.details.set(id, run);
    const controller = new AbortController();
    this.controllers.set(id, controller);
    this.publish(id, started);
    serverLog("info", "run.started", { runId: id, webhook, workflow: workflow.name, source });
    const completion = this.execute(id, workflow, input, controller.signal);
    // Background runs must always have a rejection handler, even after the HTTP client leaves.
    void completion.catch((cause) => serverLog("error", "run.error", { runId: id, error: cause }));
    return { id, completion };
  }

  private async execute<Input, Output>(
    id: string,
    workflow: WebhookTask<Input, Output>,
    input: Input,
    signal: AbortSignal,
  ): Promise<WorkflowExecution> {
    const base = performance.now();
    const execution = await runWorkflow(workflow, {
      input,
      signal,
      onEvent: (event) => {
        void this.append(id, {
          type: "event",
          event: { ...event, timestamp: Math.max(0, event.timestamp - base) },
        }).catch(() => {}); // The same write failure is handled when completion flushes the queue.
      },
    });
    this.controllers.delete(id);
    try {
      await this.append(id, {
        type: "finished",
        status: signal.aborted ? "cancelled" : execution.ok ? "completed" : "failed",
        finishedAt: Date.now(),
        durationMs: execution.durationMs,
        usage: execution.usage,
        output: execution.output,
        error: execution.error,
      });
    } catch (cause) {
      const record: RunRecord = {
        type: "finished",
        status: "failed",
        finishedAt: Date.now(),
        durationMs: execution.durationMs,
        usage: execution.usage,
        output: null,
        error: `Cannot save run history: ${cause instanceof Error ? cause.message : String(cause)}`,
      };
      const run = this.details.get(id)!;
      applyStoredRecord(run, record);
      this.runs.set(id, summary(run));
      this.publish(id, record);
      throw new Error(record.error!);
    } finally {
      this.pending.delete(id);
    }
    serverLog(execution.ok ? "info" : "error", "run.finished", {
      runId: id, status: execution.ok ? "completed" : "failed",
      durationMs: execution.durationMs, usage: execution.usage, error: execution.error,
    });
    return execution;
  }
}

// Preserve active executions through Vite module reloads.
const state = globalThis as typeof globalThis & {
  __runlingRunStore?: Promise<RunStore>;
};
export async function historyDirectory(cwd: string): Promise<string> {
  const current = resolve(cwd, ".runling/runs");
  const legacy = resolve(cwd, ".factory/runs");
  for (const path of [current, legacy]) {
    try {
      if ((await stat(path)).isDirectory()) return path;
      throw new Error(`Run history path is not a directory: ${path}`);
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw error;
    }
  }
  return current;
}

export function getRunStore(): Promise<RunStore> {
  state.__runlingRunStore ??= (async () => {
    const configPath = process.env.RUNLING_WEB_CONFIG;
    if (!configPath) throw new Error("RUNLING_WEB_CONFIG is required for run history");
    const cwd = dirname(configPath);
    const store = new RunStore(await historyDirectory(cwd));
    await store.init();
    return store;
  })();
  return state.__runlingRunStore;
}
