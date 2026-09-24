/** Project run journal entries into the console's ordered Log view. */
import type { RunlingEvent } from 'runling';

/** Journal event shape used by a visible log row. */
export type LogEvent = Extract<RunlingEvent, { type: 'log' }>;

/** One stable console row with the original journal event index. */
export interface LogRow {
  /** Original event index keeps row identity stable as new records arrive. */
  id: number;
  event: LogEvent;
}

/** Visual guide data for one row in the chronological log outline. */
export interface TreeLogRow {
  row: LogRow;
  depth: number;
  joinsAbove: boolean;
  continuesBelow: boolean;
}

/** Show recorded logs and host-owned task milestones in journal order. */
export function runLogRows(events: RunlingEvent[]): LogRow[] {
  const rows: LogRow[] = [];
  const taskReferences = new Map<string, string>();
  const channelTasks = new Map<string, string>();
  const taskDepths = new Map<string, number>();
  const childDepth = (parent?: string) =>
    parent === undefined ? 0 : (taskDepths.get(parent) ?? 0) + 1;
  const taskReference = (id: string) => {
    let reference = taskReferences.get(id);
    if (!reference) {
      reference = `task-${taskReferences.size + 1}`;
      taskReferences.set(id, reference);
    }
    return reference;
  };
  const addActivity = (
    id: number,
    timestamp: number,
    message: string,
    level: LogEvent['level'],
    depth: number
  ) => {
    rows.push({
      id,
      event: {
        type: 'log',
        timestamp,
        message,
        level,
        depth,
        color: 'dodgerblue',
        source: 'step'
      }
    });
  };
  events.forEach((event, id) => {
    switch (event.type) {
      case 'log':
        rows.push({ id, event });
        break;
      case 'step.started': {
        const depth = childDepth(event.activityId);
        taskDepths.set(event.id, depth);
        addActivity(id, event.timestamp, `Started ${taskReference(event.id)}`, 'info', depth);
        break;
      }
      case 'task.linked':
        channelTasks.set(event.channelId, event.taskId);
        break;
      case 'task.activity': {
        const taskId = channelTasks.get(event.channelId);
        addActivity(
          id,
          event.timestamp,
          `${taskId ? taskReference(taskId) : 'Task'} · ${event.message}`,
          event.level ?? 'info',
          taskId
            ? (taskDepths.get(taskId) ?? childDepth(event.activityId)) + 1
            : childDepth(event.activityId)
        );
        break;
      }
      case 'step.finished':
        addActivity(
          id,
          event.timestamp,
          `${taskReference(event.id)} ${event.status} · ${Math.round(event.durationMs / 1000)} s`,
          event.status === 'completed' ? 'success' : 'error',
          taskDepths.get(event.id) ?? childDepth(event.activityId)
        );
        break;
    }
  });
  return rows;
}

/** Limit guide width and join adjacent chronological rows at their shared depth. */
export function logTreeRows(rows: LogRow[]): TreeLogRow[] {
  const depth = (row: LogRow) => Math.min(6, Math.max(0, Math.trunc(row.event.depth)));
  return rows.map((row, index) => {
    const current = depth(row);
    return {
      row,
      depth: current,
      joinsAbove: index > 0 && depth(rows[index - 1]!) >= current,
      continuesBelow: index < rows.length - 1 && depth(rows[index + 1]!) >= current
    };
  });
}

/** Format elapsed run time for the log gutter. */
export function logTime(ms: number): string {
  const seconds = Math.max(0, Math.floor(ms / 1000));
  return `${Math.floor(seconds / 3600)
    .toString()
    .padStart(2, '0')}:${Math.floor((seconds / 60) % 60)
    .toString()
    .padStart(2, '0')}:${(seconds % 60).toString().padStart(2, '0')}`;
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
  position: ScrollPosition
): boolean {
  if (position.scrollHeight - position.clientHeight - position.scrollTop <= 8) return true;
  if (position.scrollTop < previousTop - 1) return false;
  return following;
}
