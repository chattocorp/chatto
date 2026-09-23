import type { RunlingEvent } from "runling";

export type LogEvent = Extract<RunlingEvent, { type: "log" }>;

export interface LogRow {
  /** Original event index keeps row identity stable as new records arrive. */
  id: number;
  event: LogEvent;
}

/** Show recorded logs and host-owned task milestones in journal order. */
export function runLogRows(events: RunlingEvent[]): LogRow[] {
  const rows: LogRow[] = [];
  const taskReferences = new Map<string, string>();
  const channelTasks = new Map<string, string>();
  const taskReference = (id: string) => {
    let reference = taskReferences.get(id);
    if (!reference) { reference = `task-${taskReferences.size + 1}`; taskReferences.set(id, reference); }
    return reference;
  };
  const addActivity = (id: number, timestamp: number, message: string, level: LogEvent["level"]) => {
    rows.push({ id, event: { type: "log", timestamp, message, level, depth: 0, color: "dodgerblue", source: "step" } });
  };
  events.forEach((event, id) => {
    switch (event.type) {
      case "log": rows.push({ id, event }); break;
      case "step.started":
        addActivity(id, event.timestamp, `Started ${taskReference(event.id)}`, "info");
        break;
      case "task.linked": channelTasks.set(event.channelId, event.taskId); break;
      case "task.activity": {
        const taskId = channelTasks.get(event.channelId);
        addActivity(id, event.timestamp, `${taskId ? taskReference(taskId) : "Task"} · ${event.message}`, event.level ?? "info");
        break;
      }
      case "step.finished":
        addActivity(id, event.timestamp, `${taskReference(event.id)} ${event.status} · ${Math.round(event.durationMs / 1000)} s`,
          event.status === "completed" ? "success" : "error");
        break;
    }
  });
  return rows;
}

/** Format elapsed run time for the log gutter. */
export function logTime(ms: number): string {
  const seconds = Math.max(0, Math.floor(ms / 1000));
  return `${Math.floor(seconds / 3600).toString().padStart(2, "0")}:${Math.floor(seconds / 60 % 60).toString().padStart(2, "0")}:${(seconds % 60).toString().padStart(2, "0")}`;
}

interface ScrollPosition {
  scrollTop: number;
  scrollHeight: number;
  clientHeight: number;
}

/** Follow new lines until the reader scrolls up; resume at the bottom. */
export function nextLogFollow(
  following: boolean,
  previousTop: number,
  position: ScrollPosition,
): boolean {
  if (position.scrollHeight - position.clientHeight - position.scrollTop <= 8) return true;
  if (position.scrollTop < previousTop - 1) return false;
  return following;
}
