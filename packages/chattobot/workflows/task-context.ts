import type { AgentTaskState } from "runling/agents";

/** Supervisor-only projection. Runling keeps its original serialized result. */
export type SupervisorTask = Omit<AgentTaskState, "result"> & { result?: unknown };

/** Present current host state and decoded results without changing retained history. */
export function taskContext(tasks: AgentTaskState[]): SupervisorTask[] {
  return tasks.map(task => {
    const current: SupervisorTask = { ...task };
    if (task.result !== undefined) {
      try { current.result = JSON.parse(task.result); }
      catch { /* Preserve plain-text and truncated results without inventing structure. */ }
    }
    if (task.status !== "running" || (task.stateAt ?? 0) > (task.progressAt ?? 0)) {
      delete current.progress;
      delete current.progressAt;
      delete current.progressAgeMs;
    }
    // The typed result's outcome describes success/blocked; completed only means the function returned.
    if (task.status !== "running") {
      delete current.state;
      delete current.stateAt;
      delete current.stateAgeMs;
    }
    return current;
  });
}

/** Notifications are wake-up signals. Never duplicate their stale task snapshots in a prompt. */
export function taskNotification(message: string): string {
  try {
    const value = JSON.parse(message);
    return JSON.stringify({ type: value.type, taskId: value.task?.id });
  } catch { return "Background task changed; use the current backgroundTasks snapshot."; }
}
