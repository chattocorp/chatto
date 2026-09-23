import type { RunlingEvent } from "runling";

export type LogEvent = Extract<RunlingEvent, { type: "log" }>;

export interface LogRow {
  /** Original event index keeps row identity stable as new records arrive. */
  id: number;
  event: LogEvent;
}

/** Keep recorded log events in journal order, including source and debug logs. */
export function runLogRows(events: RunlingEvent[]): LogRow[] {
  const rows: LogRow[] = [];
  events.forEach((event, id) => {
    if (event.type === "log") rows.push({ id, event });
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
