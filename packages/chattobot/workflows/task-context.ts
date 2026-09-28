/** Project retained task state and select notifications that need a user reply. */
import type { AgentTaskState } from 'runling/agents';
import { implementationResultMessage, isImplementationOutcome } from './implementation-messages.ts';

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
    return JSON.stringify({
      type: value.type,
      taskId: task?.id,
      // A notice is a message for the user that the owner relays in its own words.
      ...(value.type === 'task.notice' && typeof value.text === 'string'
        ? { text: value.text }
        : {})
    });
  } catch {
    return 'Background task changed; use the current backgroundTasks snapshot.';
  }
}

/** Wake the owner for a requested answer, a notice, or a terminal result that still needs a
 * reply. The host posts a finished implementation's result itself with `postResult`, from the
 * owner task; the owner wakes for that result only when the post fails. */
export async function* userFacingTaskNotifications(
  source: AsyncIterable<string>,
  { postResult }: { postResult?: (message: string) => Promise<void> } = {}
): AsyncIterable<string> {
  for await (const message of source) {
    let forward = true;
    try {
      const notice: unknown = JSON.parse(message);
      if (!isRecord(notice)) throw new Error('Invalid task notification');
      if (typeof notice.type !== 'string') throw new Error('Invalid task notification type');
      if (
        !['task.reply', 'task.notice', 'task.completed', 'task.failed', 'task.cancelled'].includes(
          notice.type
        )
      )
        forward = false;
      if (
        postResult &&
        notice.type === 'task.completed' &&
        isRecord(notice.task) &&
        notice.task.name === 'Chatto implementation'
      ) {
        const result: unknown =
          typeof notice.task.result === 'string' ? JSON.parse(notice.task.result) : undefined;
        if (isImplementationOutcome(result)) {
          try {
            await postResult(implementationResultMessage(result));
            forward = false;
          } catch {
            // The owner reports the result instead.
          }
        }
      }
    } catch {
      // An unknown notification can be a terminal result. Let the owner inspect it.
    }
    if (forward) yield message;
  }
}
