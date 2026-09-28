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
      // A cancelled or failed task returns no result; its artifact ID still allows continuing.
      const artifactId = task.state?.artifactId;
      delete current.state;
      delete current.stateAt;
      delete current.stateAgeMs;
      if (typeof artifactId === 'string' && task.status !== 'completed')
        current.state = { artifactId };
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
      ...(value.type === 'task.completed' ? finalImplementationReport(task) : {}),
      // A notice reports progress or a milestone that the owner relays in its own words.
      ...(value.type === 'task.notice' && typeof value.text === 'string'
        ? { text: value.text }
        : {}),
      ...(value.type === 'task.notice' && isRecord(value.data) ? { data: value.data } : {})
    });
  } catch {
    return 'Background task changed; use the current backgroundTasks snapshot.';
  }
}

/** The final result of a finished implementation, with the request to report it in full. The
 * result is final, so it is not a stale snapshot. The supervisor writes the report itself. */
function finalImplementationReport(task: Record<string, unknown> | undefined) {
  if (task?.name !== 'Chatto implementation' || typeof task.result !== 'string') return {};
  let result: unknown;
  try {
    result = JSON.parse(task.result);
  } catch {
    return {};
  }
  if (!isRecord(result)) return {};
  const { outcome, summary, notes, prUrl, ci, artifactId, checks } = result;
  return {
    result: {
      outcome,
      summary,
      notes,
      prUrl,
      ci,
      artifactId,
      failedChecks: Array.isArray(checks)
        ? checks.filter((check) => isRecord(check) && !check.passed).map((check) => check.command)
        : []
    },
    report:
      'This is the final report of the implementation. Write it in full, not briefly: the outcome and CI result with the pull request URL exactly as result.prUrl, then a short section on what changed with a few points from result.summary, then notable result.notes. For a stopped implementation, explain why it stopped, the failed checks, and result.artifactId for continuing.'
  };
}

/** Wake the owner for a requested answer, a notice, or a terminal result. */
export async function* userFacingTaskNotifications(
  source: AsyncIterable<string>
): AsyncIterable<string> {
  for await (const message of source) {
    let forward = true;
    try {
      const notice: unknown = JSON.parse(message);
      if (!isRecord(notice)) throw new Error('Invalid task notification');
      if (typeof notice.type !== 'string') throw new Error('Invalid task notification type');
      forward = [
        'task.reply',
        'task.notice',
        'task.completed',
        'task.failed',
        'task.cancelled'
      ].includes(notice.type);
    } catch {
      // An unknown notification can be a terminal result. Let the owner inspect it.
    }
    if (forward) yield message;
  }
}

/** Pull request URLs in a task notification that the user must receive exactly: the `prUrl` of
 * a `published` milestone notice, or of a finished implementation's result. Other milestones
 * refer to a pull request that the user already knows. */
export function notificationUrls(message: string): string[] {
  try {
    const notice: unknown = JSON.parse(message);
    if (!isRecord(notice)) return [];
    const urls: unknown[] = [];
    if (
      notice.type === 'task.notice' &&
      isRecord(notice.data) &&
      notice.data.milestone === 'published'
    )
      urls.push(notice.data.prUrl);
    if (notice.type === 'task.completed' && isRecord(notice.task)) {
      const result: unknown =
        typeof notice.task.result === 'string' ? JSON.parse(notice.task.result) : undefined;
      if (isRecord(result)) urls.push(result.prUrl);
    }
    return urls.filter(
      (url): url is string => typeof url === 'string' && /^https:\/\/github\.com\/\S+$/.test(url)
    );
  } catch {
    return [];
  }
}
