import type { WorkflowContext } from "../context.ts";
import type { Run } from "../spawn.ts";
import { Type } from "typebox";
import { defineAgentExtension } from "../agent.ts";
import type { AgentStatus, AgentActivity } from "../agent.ts";
import { emitRunlingEvent } from "../events.ts";

/** JSON state published by a workflow, independent of its transport or domain. */
export type AgentTaskData = null | boolean | number | string | AgentTaskData[] | { [key: string]: AgentTaskData };

/** Published text, not private reasoning or raw tool output. */
export interface AgentTaskOutput {
  sequence: number;
  at: number;
  kind: "output" | "finding";
  text: string;
  /** True when the retained text is only a prefix of the published message. */
  truncated?: boolean;
}

/** Child-authored findings, workflow state, or safe provider lifecycle updates. */
export type AgentTaskUpdate = string | AgentStatus | AgentActivity | {
  /** Buffer published agent commentary without sending a notification or replacing current progress. */
  type: "output";
  text: string;
} | {
  /** Replaces the previous workflow state. Must be JSON and at most 16,000 characters. Does not wake the owner. */
  type: "state";
  value: { [key: string]: AgentTaskData };
  /** Optional public operational message for the server console. Supply static,
   * host-owned text only: no prompts, model output, paths, credentials, or personal data. */
  activity?: string;
} | {
  /** Explicit substantive progress, retained across later tool activity.
   * The application must validate this text; Runling does not verify evidence. */
  type: "finding";
  text: string;
};

/** A conversation-local task handle. Results and handles are not durable across restart. */
export interface AgentTaskState {
  id: string;
  name: string;
  status: "running" | "completed" | "failed" | "cancelled";
  result?: string;
  /** Latest workflow-owned state. Task status still decides whether work is running. */
  state?: { [key: string]: AgentTaskData };
  stateAt?: number;
  stateAgeMs?: number;
  /** Last 16 published messages, each bounded to 4,000 characters; separate from current state. */
  output: AgentTaskOutput[];
  /** Number of earlier messages removed from the bounded buffer. */
  droppedOutput: number;
  /** Latest child-authored finding, with its receipt time and current age. */
  progress?: string;
  progressAt?: number;
  progressAgeMs?: number;
  /** Provider state is separate from task completion and substantive findings. */
  provider?: AgentStatus;
  /** Latest observed tool lifecycle; never contains arguments or raw output. */
  activity?: AgentActivity;
  activityAt?: number;
  activityAgeMs?: number;
  /** Most recent unresolved tool failure; unrelated successful tools do not clear it. */
  lastToolFailure?: AgentActivity;
  /** Host-enforced failure budget, separate from provider retries. */
  failureReason?: "tool_failure_limit";
}

/** Observe child runs for an agent: retain snapshots and coalesce notifications.
 * Execution, identity, channels, and cancellation belong to the ordinary run.
 * Once observed, this adapter owns cancellation and cleanup on disposal. */
export function observeAgentTasks(ctx: WorkflowContext<any, any>, {
  progressIntervalMs = 30_000, maxTasks = 32, maxToolFailures, notifyActivity = true, toolFailureNoticeThreshold = 2,
}: { progressIntervalMs?: number; maxTasks?: number; /** Notify after repeated failures; permission errors notify immediately. Defaults to 2. */ toolFailureNoticeThreshold?: number; /** Keep routine activity in snapshots without waking the owner when false. */ notifyActivity?: boolean; /** Stop a child after this many failures of an operation without success. Disabled by default. */ maxToolFailures?: number } = {}) {
  if (!Number.isSafeInteger(maxTasks) || maxTasks < 1 || !Number.isFinite(progressIntervalMs) || progressIntervalMs < 0) {
    throw new Error("Invalid agent task limits");
  }
  if (maxToolFailures !== undefined && (!Number.isSafeInteger(maxToolFailures) || maxToolFailures < 1)) throw new Error("Invalid tool failure limit");
  if (!Number.isSafeInteger(toolFailureNoticeThreshold) || toolFailureNoticeThreshold < 1) throw new Error("Invalid tool failure notification threshold");
  type Entry = { state: AgentTaskState; handle: Run<string, AgentTaskUpdate, unknown>;
    settled: Promise<void>; timer?: ReturnType<typeof setTimeout>; progress?: string; finding?: boolean };
  const tasks = new Map<string, Entry>();
  // At most one pending notification per task. Completion replaces stale progress.
  const notices = new Map<string, { entry: Entry; type: string; text?: string }>();
  let wake: (() => void) | undefined;
  let closed = false;
  let claimed = false;
  let closing: Promise<void> | undefined;
  const snapshot = (entry: Entry): AgentTaskState => ({ ...entry.state,
    output: entry.state.output.map(item => ({ ...item })),
    ...(entry.state.state ? { state: structuredClone(entry.state.state), stateAgeMs: Math.max(0, Date.now() - entry.state.stateAt!) } : {}),
    ...(entry.state.provider ? { provider: { ...entry.state.provider } } : {}),
    ...(entry.state.progressAt !== undefined ? { progressAgeMs: Math.max(0, Date.now() - entry.state.progressAt) } : {}),
    ...(entry.state.activity ? { activity: { ...entry.state.activity }, activityAgeMs: Math.max(0, Date.now() - entry.state.activityAt!) } : {}),
    ...(entry.state.lastToolFailure ? { lastToolFailure: { ...entry.state.lastToolFailure } } : {}),
  });
  const notify = (entry: Entry, type: string, text?: string) => {
    if (closed) return;
    notices.set(entry.state.id, { entry, type, text });
    if (type === "task.progress") entry.progress = undefined;
    wake?.();
  };
  const lookup = (id: string) => {
    const entry = tasks.get(id);
    if (!entry) throw new Error("Unknown task handle");
    return entry;
  };
  const abort = () => { void dispose(); };

  /** Cancel children and await their cooperative cleanup, not just handle cancellation. */
  function dispose(): Promise<void> {
    if (closing) return closing;
    closed = true;
    ctx.signal.removeEventListener("abort", abort);
    for (const entry of tasks.values()) {
      clearTimeout(entry.timer);
      entry.handle.cancel();
    }
    notices.clear();
    wake?.();
    closing = Promise.all([...tasks.values()].map(entry => entry.settled)).then(() => {});
    return closing;
  }
  ctx.signal.addEventListener("abort", abort, { once: true });
  if (ctx.signal.aborted) void dispose();

  const observer = {
    /** Merge these notifications into the owning agent's conversation. One consumer only. */
    notifications: {
      [Symbol.asyncIterator]() {
        if (claimed) throw new Error("Task notifications already have a consumer");
        claimed = true;
        return {
          async next(): Promise<IteratorResult<string>> {
            while (!closed && notices.size === 0) await new Promise<void>(resolve => { wake = resolve; });
            wake = undefined;
            const first = notices.entries().next();
            if (closed || first.done) return { done: true, value: undefined };
            notices.delete(first.value[0]);
            const { entry, type, text } = first.value[1];
            const { output: _output, ...task } = snapshot(entry);
            return { done: false, value: JSON.stringify({ type, task, ...(text ? { progress: text } : {}) }) };
          },
          async return(): Promise<IteratorResult<string>> { await dispose(); return { done: true, value: undefined }; },
        };
      },
    } satisfies AsyncIterable<string>,
    /** Compatibility convenience. Prefer observing a run created by ctx.spawn(). */
    start<Input, Update extends AgentTaskUpdate = AgentTaskUpdate>(name: string, run: (ctx: WorkflowContext<string, Update>, input: Input) => Promise<string>, input: Input): AgentTaskState {
      ctx.signal.throwIfAborted();
      if (closed) throw new Error("Agent tasks are closed");
      if (tasks.size >= maxTasks) throw new Error("Conversation task limit reached");
      return observer.observe(name, ctx.spawn((ctx: WorkflowContext<string, Update>) => {
        return Promise.resolve().then(() => run({ ...ctx, emit: async update => {
          if (typeof update !== "string" && update.type === "state") {
            const encoded = JSON.stringify(update.value);
            if (!encoded || encoded.length > 16_000) throw new Error("Task state exceeds the JSON size limit");
            // Copy before queuing so subsequent producer mutations cannot change the published state.
            await ctx.emit({ type: "state", value: JSON.parse(encoded) } as Update);
          } else await ctx.emit(update);
        } }, input));
      }));
    },
    /** Adopt a run and consume its output. Do not also iterate run.output.
     * Rejected adoption leaves ownership with the caller. */
    observe<Result>(name: string, handle: Run<string, AgentTaskUpdate, Result>): AgentTaskState {
      ctx.signal.throwIfAborted();
      if (closed) throw new Error("Agent tasks are closed");
      if (tasks.has(handle.id)) return snapshot(lookup(handle.id));
      if (tasks.size >= maxTasks) throw new Error("Conversation task limit reached");
      const state: AgentTaskState = { id: handle.id, name: name.slice(0, 200), status: handle.status, output: [], droppedOutput: 0 };
      const entry: Entry = { state, handle, settled: Promise.resolve() };
      tasks.set(state.id, entry);
      const remember = (kind: AgentTaskOutput["kind"], text: string) => {
        state.output.push({ sequence: state.droppedOutput + state.output.length + 1, at: Date.now(), kind, text: text.slice(0, 4_000),
          ...(text.length > 4_000 ? { truncated: true } : {}) });
        if (state.output.length > 16) { state.output.shift(); state.droppedOutput++; }
      };
      const updates = (async () => {
        try {
          for await (const text of handle.output) {
            if (state.status === "failed" || state.status === "cancelled") continue;
            if (typeof text !== "string" && text.type === "output") {
              remember("output", text.text);
              continue;
            }
            if (typeof text !== "string" && text.type === "state") {
              const encoded = JSON.stringify(text.value);
              if (!encoded || encoded.length > 16_000) throw new Error("Task state exceeds the JSON size limit");
              state.state = JSON.parse(encoded);
              state.stateAt = Date.now();
              if (text.activity) emitRunlingEvent({ type: "task.activity", channelId: handle.id, message: text.activity.slice(0, 200) });
              continue;
            }
            if (typeof text !== "string" && text.type !== "finding") {
              if (text.type === "tool") {
                state.activity = { type: "tool", operation: text.operation, phase: text.phase, failures: text.failures,
                  ...(text.toolName ? { toolName: text.toolName } : {}),
                  ...(text.error ? { error: text.error } : {}),
                };
                state.activityAt = Date.now();
                // Observed tool activity supersedes earlier prose, which may
                // only describe an intention rather than an actual finding.
                if (!entry.finding) {
                  delete state.progress;
                  delete state.progressAt;
                  entry.progress = undefined;
                  if (notices.get(state.id)?.type === "task.progress") notices.delete(state.id);
                }
                if (text.phase === "succeeded" && state.lastToolFailure?.operation === text.operation) {
                  delete state.lastToolFailure;
                  if (notices.get(state.id)?.type === "task.tool_failed") notices.delete(state.id);
                }
                if (text.phase === "failed") {
                  state.lastToolFailure = { ...state.activity };
                  clearTimeout(entry.timer);
                  entry.timer = undefined;
                  if (maxToolFailures !== undefined && text.failures >= maxToolFailures) {
                    state.failureReason = "tool_failure_limit";
                    state.status = "failed";
                    entry.handle.cancel(new Error("Task stopped after repeated tool failures"));
                  } else if (text.failures >= toolFailureNoticeThreshold || text.error === "permission_denied") notify(entry, "task.tool_failed");
                } else if (!entry.timer) entry.timer = setTimeout(() => {
                  entry.timer = undefined;
                  if (state.status === "running" && (entry.progress || notifyActivity)) notify(entry, entry.progress ? "task.progress" : "task.activity", entry.progress);
                }, progressIntervalMs);
                continue;
              }
              const previousProvider = state.provider?.type;
              state.provider = text.type === "retrying"
                ? { type: "retrying", attempt: text.attempt, maxAttempts: text.maxAttempts, delayMs: text.delayMs }
                : text.type === "blocked" ? { type: "blocked", reason: "provider_error" } : { type: "working" };
              if (text.type === "working" && notices.get(state.id)?.type === "task.retrying") notices.delete(state.id);
              clearTimeout(entry.timer);
              entry.timer = undefined;
              // Recovery updates the snapshot, but does not wake the owner to
              // announce progress that has not happened. Repeated retries coalesce.
              if (state.provider.type !== "working" && previousProvider !== state.provider.type) notify(entry, `task.${state.provider.type}`);
              continue;
            }
            entry.finding = typeof text !== "string";
            entry.progress = (typeof text === "string" ? text : text.text).slice(0, 4_000);
            remember(entry.finding ? "finding" : "output", typeof text === "string" ? text : text.text);
            state.progress = entry.progress;
            state.progressAt = Date.now();
            if (!entry.timer) entry.timer = setTimeout(() => {
              entry.timer = undefined;
              if (state.status === "running" && (entry.progress || notifyActivity)) notify(entry, entry.progress ? "task.progress" : "task.activity", entry.progress);
            }, progressIntervalMs);
          }
        } catch {
          // Invalid observations stop the child without exposing raw update/error data.
          // Run failures and cancellation are reported from the result below.
          if (handle.status === "running" || handle.status === "completed") {
            state.status = "failed";
            handle.cancel();
          }
        }
      })();
      entry.settled = (async () => {
        try {
          const result = await handle.result;
          // Agent snapshots use text, but the run retains its original typed result.
          let text: string;
          try { text = typeof result === "string" ? result : JSON.stringify(result) ?? ""; }
          catch { text = "[Result cannot be represented as JSON]"; }
          state.result = text.length > 64_000 ? `${text.slice(0, 64_000)}\n[Task result truncated]` : text;
          if (state.status !== "failed") state.status = handle.status;
        } catch {
          if (state.status !== "failed") state.status = handle.status;
        } finally {
          await updates;
          clearTimeout(entry.timer);
          await handle.settled;
          notify(entry, `task.${state.status}`);
        }
      })();
      return snapshot(entry);
    },
    get(id: string): AgentTaskState { return snapshot(lookup(id)); },
    /** Fresh snapshots for owner prompts; no model polling tool is required. */
    list(): AgentTaskState[] { return [...tasks.values()].map(snapshot); },
    /** Forward a clarification to a child's bounded inbox without waiting for consumption. */
    async send(id: string, text: string) {
      const entry = lookup(id);
      if (closed || entry.state.status !== "running") throw new Error("Task is not running");
      if (!text.trim() || text.length > 16_000) throw new Error("Invalid task message");
      await entry.handle.send(text);
    },
    cancel(id: string) {
      const entry = lookup(id);
      if (entry.state.status !== "running") return;
      entry.state.status = "cancelled";
      entry.handle.cancel();
    },
    get active() { return notices.size > 0 || [...tasks.values()].some(entry => entry.state.status === "running"); },
    dispose,
  };
  return observer;
}

/** Compatibility factory name. This adapter observes ordinary child runs. */
export const createAgentTasks = observeAgentTasks;

export type AgentTasks = ReturnType<typeof observeAgentTasks>;

/** Steering and cancellation tools. Status arrives through notifications and host snapshots. */
export function agentTasksExtension(tasks: AgentTasks) {
  const result = (value: unknown) => ({ content: [{ type: "text" as const, text: JSON.stringify(value) }], details: {} });
  return defineAgentExtension(pi => {
    pi.registerTool({ name: "task_send", label: "Steer task", description: "Send a relevant user clarification to a running task's inbox.",
      parameters: Type.Object({ id: Type.String(), message: Type.String({ minLength: 1, maxLength: 16_000 }) }),
      async execute(_id, input) { await tasks.send(input.id, input.message); return result({ queued: true }); },
    });
    pi.registerTool({ name: "task_cancel", label: "Cancel task", description: "Cancel one owned task without ending the conversation.",
      parameters: Type.Object({ id: Type.String() }),
      async execute(_id, input) { tasks.cancel(input.id); return result(tasks.get(input.id)); },
    });
  });
}
