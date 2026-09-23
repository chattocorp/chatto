import type { RunlingEvent } from "runling";
import type { RunStatus } from "./runs.ts";

const taskColors = ["#60a5fa", "#a78bfa", "#2dd4bf", "#f472b6", "#fb923c", "#a3e635"];

export interface FeedEntry {
  id: string;
  timestamp: number;
  task: string;
  reference?: string;
  depth: number;
  color?: string;
  message: string;
  detail?: string;
  tone: "normal" | "waiting" | "success" | "error";
}

/** Project recorded events into one chronological feed. Protocol traffic and token
 * updates stay out; full raw logs remain available in their own view. */
export function activityFeed(events: RunlingEvent[], status: RunStatus, durationMs?: number): FeedEntry[] {
  const tasks = new Map<string, { label: string; reference: string; depth: number; color?: string }>();
  const agents = new Map<string, { label: string; color?: string; taskId?: string }>();
  const channels = new Map<string, string>();
  // Resolve task ownership before projecting, including links recorded after a start.
  for (const event of events) {
    if (event.type === "step.started") tasks.set(event.id, {
      label: event.label, reference: `task-${tasks.size + 1}`,
      color: taskColors[tasks.size % taskColors.length],
      depth: event.activityId ? (tasks.get(event.activityId)?.depth ?? 0) + 1 : 0,
    });
    if (event.type === "task.linked") channels.set(event.channelId, event.taskId);
    if (event.type === "agent.started") {
      const color = /^#[0-9a-f]{6}$/i.test(event.color) ? event.color : undefined;
      agents.set(event.agentId, { label: event.label ?? "Agent", color, taskId: event.activityId });
      const task = tasks.get(event.activityId ?? "");
      if (task) { if (color) task.color = color; if (event.label) task.label = event.label; }
    }
  }
  const entries: FeedEntry[] = [];
  // Agent text is also forwarded through its task channel. Suppress that one
  // mirrored delivery, without hiding repeated messages from users or tasks.
  const forwardedText = new Map<string, string>();
  for (const [index, event] of events.entries()) {
    const agent = "agentId" in event ? agents.get(event.agentId) : undefined;
    const taskId = event.type === "step.started" || event.type === "step.finished" ? event.id
      : event.type === "task.activity" || event.type === "message.sent" ? channels.get(event.channelId) ?? event.activityId
      : agent?.taskId ?? event.activityId;
    const task = tasks.get(taskId ?? "");
    let message: string;
    let detail: string | undefined;
    let tone: FeedEntry["tone"] = "normal";
    switch (event.type) {
      case "step.started": message = "Task started"; detail = event.label; break;
      case "step.finished": message = `Task ${event.status}`; tone = event.status === "failed" ? "error" : "success"; break;
      case "agent.started": message = "Agent started"; detail = event.model; break;
      case "agent.finished": message = `Agent ${event.outcome}`; tone = event.outcome === "completed" ? "success" : "error"; break;
      case "agent.tool":
        if (event.phase === "succeeded") continue;
        message = event.phase === "failed" ? `Tool failed · ${event.toolName ?? event.operation}`
          : `${{ read: "Reading files", search: "Searching files", edit: "Applying changes", command: "Running command", other: "Using tool" }[event.operation]} · ${event.toolName ?? event.operation}`;
        if (event.phase === "failed") tone = "error";
        break;
      case "task.activity": message = event.message; break;
      case "agent.action":
        message = event.action;
        if (taskId) forwardedText.set(taskId, message);
        break;
      case "command.started": message = "Command started"; detail = event.command; break;
      case "command.finished":
        message = `Command ${event.status}`; tone = event.status === "failed" ? "error" : "success";
        detail = [event.output.stdout, event.output.stderr].filter(Boolean).join("\n"); break;
      case "input.requested": message = "Waiting for input"; detail = event.message; tone = "waiting"; break;
      case "input.finished":
        message = event.status === "answered" ? "Input received" : `Input ${event.reason ?? "failed"}`;
        tone = event.status === "answered" ? "normal" : event.reason === "timeout" ? "waiting" : "error";
        break;
      case "message.sent":
        if (event.direction === "input") { message = `Received: ${event.payload}`; break; }
        message = event.payload;
        try {
          const update = JSON.parse(event.payload);
          if ((update?.type === "finding" || update?.type === "output") && typeof update.text === "string") {
            message = update.text;
          } else if (["state", "tool", "retrying", "blocked", "working"].includes(update?.type)) continue;
        } catch { /* Plain text is a valid task update. */ }
        if (taskId && forwardedText.get(taskId) === message) {
          forwardedText.delete(taskId);
          continue;
        }
        break;
      case "log":
        if (event.source || event.level === "debug") continue;
        message = event.message;
        if (event.level === "error") tone = "error";
        break;
      default: continue;
    }
    entries.push({ id: String(index), timestamp: event.timestamp, task: agent?.label ?? task?.label ?? "Run",
      reference: task?.reference, depth: task?.depth ?? 0, color: agent?.color ?? task?.color,
      message: message.length > 16000 ? `${message.slice(0, 16000)}\n… Text truncated.` : message,
      ...(detail ? { detail: detail.length > 16000 ? `${detail.slice(0, 16000)}\n… Detail truncated.` : detail } : {}), tone });
  }
  if (status !== "running") entries.push({ id: "end", timestamp: durationMs ?? events.at(-1)?.timestamp ?? 0,
    task: "Run", depth: 0, message: `Run ${status}`, tone: status === "completed" ? "success" : status === "failed" ? "error" : "waiting" });
  return entries;
}

/** Elapsed run time, not wall-clock time; keeps historical and live feeds identical. */
export function feedTime(ms: number): string {
  const seconds = Math.max(0, Math.floor(ms / 1000));
  return `${Math.floor(seconds / 3600).toString().padStart(2, "0")}:${Math.floor(seconds / 60 % 60).toString().padStart(2, "0")}:${(seconds % 60).toString().padStart(2, "0")}`;
}
