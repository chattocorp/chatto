/** Project retained task state and select notifications that need a user reply. */
import type { AgentTaskState } from 'runling/agents';

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value);

/** Supervisor-only projection. Runling keeps its original serialized result. */
export type SupervisorTask = Omit<AgentTaskState, 'result'> & { result?: unknown };

/** Present current host state and decoded results without changing retained history. */
export function taskContext(tasks: AgentTaskState[]): SupervisorTask[] {
  return tasks.map((task) => {
    const current: SupervisorTask = { ...task };
    if (task.result !== undefined) {
      try {
        current.result = JSON.parse(task.result);
      } catch {
        /* Preserve plain-text and truncated results without inventing structure. */
      }
    }
    if (task.status !== 'running' || (task.stateAt ?? 0) > (task.progressAt ?? 0)) {
      delete current.progress;
      delete current.progressAt;
      delete current.progressAgeMs;
    }
    // The typed result's outcome describes success/blocked; completed only means the function returned.
    if (task.status !== 'running') {
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
    const value: unknown = JSON.parse(message);
    if (!isRecord(value)) throw new Error('Invalid task notification');
    const task = isRecord(value.task) ? value.task : undefined;
    return JSON.stringify({ type: value.type, taskId: task?.id });
  } catch {
    return 'Background task changed; use the current backgroundTasks snapshot.';
  }
}

/** Wake the owner only for a requested answer or a terminal result that still needs a reply. */
export async function* userFacingTaskNotifications(
  source: AsyncIterable<string>
): AsyncIterable<string> {
  for await (const message of source) {
    let forward = true;
    try {
      const notice: unknown = JSON.parse(message);
      if (!isRecord(notice)) throw new Error('Invalid task notification');
      if (typeof notice.type !== 'string') throw new Error('Invalid task notification type');
      if (
        notice.type !== 'task.reply' &&
        !['task.completed', 'task.failed', 'task.cancelled'].includes(notice.type)
      )
        forward = false;
      if (isRecord(notice.task) && notice.task.name === 'Chatto implementation') {
        const result: unknown =
          typeof notice.task.result === 'string' ? JSON.parse(notice.task.result) : undefined;
        if (isRecord(result) && result.noticeDelivered === true) forward = false;
      }
    } catch {
      // An unknown notification can be a terminal result. Let the owner inspect it.
    }
    if (forward) yield message;
  }
}
