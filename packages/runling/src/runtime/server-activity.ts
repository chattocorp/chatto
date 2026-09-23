import type { RunlingEvent } from "./events.ts";
import { serverLog } from "./server-log.ts";

/** Project execution events into safe operational logs for one live server run.
 * Task numbers are local to the run; full IDs remain in structured records.
 * Never forward prompts, task labels, commands, arguments, results, or arbitrary logs.
 * Agent labels and explicit task activity are public host-owned metadata.
 * Successful tool calls are summarized at most every 30 seconds per agent.
 */
export function createServerActivityLog(runId: string, runReference?: string) {
  const tasks = new Map<string, string>();
  const agents = new Map<string, number>();
  const labels = new Map<string, string>();
  const taskLabels = new Map<string, string>();
  const channels = new Map<string, string>();
  const counts = new Map<string, Map<string, number>>();
  const lastAgentEvent = new Map<string, number>();
  const lastReminder = new Map<string, number>();
  const agentTasks = new Map<string, string>();
  let reminder: ReturnType<typeof setInterval> | undefined;
  let sequence = 0;
  const taskReference = (id: string) => {
    let reference = tasks.get(id);
    if (!reference) { reference = `task-${++sequence}`; tasks.set(id, reference); }
    return reference;
  };
  const remind = () => {
    const now = Date.now();
    for (const [agentId, last] of lastAgentEvent) {
      const remindedAt = lastReminder.get(agentId);
      if (now - last < 120_000 || (remindedAt !== undefined && now - remindedAt < 300_000)) continue;
      lastReminder.set(agentId, now);
      const taskId = agentTasks.get(agentId);
      serverLog("warn", "run.activity", {
        runId, runReference, activity: `No recorded agent activity for ${Math.floor((now - last) / 60_000)} min; agent still running`,
        ...(taskId ? { taskId, taskReference: taskReference(taskId) } : {}),
        ...(labels.has(agentId) ? { agentLabel: labels.get(agentId) } : {}),
        agentId,
      });
    }
  };
  const record = (event: RunlingEvent) => {
    if ("agentId" in event && lastAgentEvent.has(event.agentId)) lastAgentEvent.set(event.agentId, Date.now());
    let activity: string;
    let level: "info" | "warn" | "error" = "info";
    let taskId = event.activityId;
    let agentId: string | undefined;
    switch (event.type) {
      case "task.linked": channels.set(event.channelId, event.taskId); return;
      case "task.activity":
        taskId = channels.get(event.channelId);
        activity = event.message;
        if (event.level === "error") level = "error";
        break;
      case "step.started": taskId = event.id; activity = "Task started"; break;
      case "step.finished":
        taskId = event.id;
        activity = `Task ${event.status} · ${Math.round(event.durationMs)} ms`;
        if (event.status === "failed") level = "error";
        break;
      case "agent.started":
        agentId = event.agentId;
        agents.set(agentId, event.timestamp);
        lastAgentEvent.set(agentId, Date.now());
        if (taskId) agentTasks.set(agentId, taskId);
        reminder ??= setInterval(remind, 60_000);
        reminder.unref?.();
        if (event.label) {
          labels.set(agentId, event.label);
          if (taskId) taskLabels.set(taskId, event.label);
        }
        activity = `Agent started · ${event.model}`;
        break;
      case "agent.finished":
        agentId = event.agentId;
        agents.delete(agentId);
        lastAgentEvent.delete(agentId);
        lastReminder.delete(agentId);
        agentTasks.delete(agentId);
        if (!lastAgentEvent.size) { clearInterval(reminder); reminder = undefined; }
        activity = `Agent ${event.outcome}${counts.get(agentId)?.size ? ` · ${[...counts.get(agentId)!].map(([name, count]) => `${count} ${name}`).join(", ")}` : ""}`;
        counts.delete(agentId);
        if (event.outcome !== "completed") level = event.outcome === "failed" ? "error" : "warn";
        break;
      case "agent.action":
        return;
      case "agent.tool":
        agentId = event.agentId;
        if (event.phase === "started") return;
        if (event.phase === "succeeded") {
          const summary = counts.get(agentId) ?? new Map<string, number>();
          summary.set(event.operation, (summary.get(event.operation) ?? 0) + 1);
          counts.set(agentId, summary);
          if (event.timestamp - (agents.get(agentId) ?? -Infinity) < 30_000) return;
          activity = `Activity · ${[...summary].map(([name, count]) => `${count} ${name}`).join(", ")}`;
          counts.delete(agentId);
        } else activity = `Tool ${event.toolName ?? event.operation} failed`;
        if (event.phase === "failed") level = "error";
        agents.set(agentId, event.timestamp);
        break;
      case "command.started": activity = "Command started"; break;
      case "command.finished":
        activity = `Command ${event.status} · ${Math.round(event.durationMs)} ms`;
        if (event.status === "failed") level = "error";
        break;
      case "input.requested": activity = "Waiting for input"; break;
      case "input.finished":
        activity = event.status === "answered" ? "Input received" : `Input ${event.reason ?? "failed"}`;
        if (event.status === "failed") level = "warn";
        break;
      case "conversation.started": activity = "Conversation ready"; break;
      default: return;
    }
    serverLog(level, "run.activity", {
      runId, runReference, activity,
      ...(event.type === "task.activity" && event.level ? { activityLevel: event.level } : {}),
      ...(taskId && taskLabels.has(taskId) ? { agentLabel: taskLabels.get(taskId) } : {}),
      ...(taskId ? { taskId, taskReference: taskReference(taskId) } : {}),
      ...(agentId ? { agentId, ...(labels.has(agentId) ? { agentLabel: labels.get(agentId) } : {}) } : {}),
    });
  };
  return Object.assign(record, { dispose() { clearInterval(reminder); reminder = undefined; } });
}
