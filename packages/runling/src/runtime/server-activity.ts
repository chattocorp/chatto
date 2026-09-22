import type { RunlingEvent } from "./events.ts";
import { serverLog } from "./server-log.ts";

/** Project execution events into safe operational logs for one live server run.
 * Task numbers are local to the run; full IDs remain in structured records.
 * Never forward prompts, labels, commands, arguments, results, or arbitrary logs.
 * Repeated agent activity is limited to one line per agent per 15 seconds.
 */
export function createServerActivityLog(runId: string, runReference?: string) {
  const tasks = new Map<string, string>();
  const agents = new Map<string, number>();
  let sequence = 0;
  const taskReference = (id: string) => {
    let reference = tasks.get(id);
    if (!reference) { reference = `task-${++sequence}`; tasks.set(id, reference); }
    return reference;
  };
  return (event: RunlingEvent) => {
    let activity: string;
    let level: "info" | "warn" | "error" = "info";
    let taskId = event.activityId;
    let agentId: string | undefined;
    switch (event.type) {
      case "step.started": taskId = event.id; activity = "Task started"; break;
      case "step.finished":
        taskId = event.id;
        activity = `Task ${event.status} · ${Math.round(event.durationMs)} ms`;
        if (event.status === "failed") level = "error";
        break;
      case "agent.started":
        agentId = event.agentId;
        agents.set(agentId, event.timestamp);
        activity = "Agent started";
        break;
      case "agent.finished":
        agentId = event.agentId;
        agents.delete(agentId);
        activity = `Agent ${event.outcome}`;
        if (event.outcome !== "completed") level = event.outcome === "failed" ? "error" : "warn";
        break;
      case "agent.action":
        agentId = event.agentId;
        if (event.timestamp - (agents.get(agentId) ?? -Infinity) < 15_000) return;
        agents.set(agentId, event.timestamp);
        activity = "Agent active";
        break;
      case "agent.tool":
        agentId = event.agentId;
        activity = `Tool ${event.operation} ${event.phase}`;
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
      ...(taskId ? { taskId, taskReference: taskReference(taskId) } : {}),
      ...(agentId ? { agentId } : {}),
    });
  };
}
