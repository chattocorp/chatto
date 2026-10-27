/** Project run journal entries into the console's ordered Log view. */
import type { RunlingEvent } from 'runling';
import { formatLogFields } from '../runtime/log-fields.ts';
import { ansiTokens } from './ansi.ts';
import { buildTimeline, type Activity } from './timeline.ts';
import type { RunStatus } from './runs.ts';

/** Journal event shape used by a visible log row. */
export type LogEvent = Extract<RunlingEvent, { type: 'log' }>;

/** One stable console row with the original journal event index. */
export interface LogRow {
  /** Original event index keeps row identity stable as new records arrive. */
  id: number;
  event: LogEvent;
  /** Activity that this line can open in the details pane. */
  activity?: { id: string; reference: string; embedded: boolean };
  /** Compact activity row; its identity and position stay stable as work completes. */
  summary?: { node: Activity; fields?: LogEvent['fields']; logs: string[] };
}

/** Project task lifecycles into single rows, preserving messages and diagnostics between them. */
export function activityLogRows(
  events: RunlingEvent[],
  status: RunStatus,
  activities = buildTimeline(events, status)
): LogRow[] {
  const nodes = new Map<string, Activity>();
  const depths = new Map<string, number>();
  const visit = (items: Activity[], depth: number) => {
    for (const node of items) {
      nodes.set(node.id, node);
      depths.set(node.id, depth);
      visit(node.children, depth + 1);
    }
  };
  visit(activities, 0);
  const detailed = new Map(runLogRows(events).map((row) => [row.id, row]));
  const rows: LogRow[] = [];
  const summaries = new Map<string, NonNullable<LogRow['summary']>>();
  const agents = new Map<string, string>();
  const pending = new Map<string | undefined, Map<string, string>>();
  const plain = (text: string) =>
    ansiTokens(text)
      .map((token) => token.text)
      .join('');

  events.forEach((event, index) => {
    if (event.type === 'execution.finished') {
      agents.clear();
      pending.clear();
    }
    const activityId =
      event.type === 'agent.started'
        ? `${event.agentId}:${index}`
        : event.type === 'step.started' ||
            event.type === 'command.started' ||
            event.type === 'wait.started'
          ? event.id
          : undefined;
    if (activityId) {
      const node = nodes.get(activityId)!;
      const summary = { node, logs: [] as string[] };
      summaries.set(activityId, summary);
      rows.push({
        id: index,
        summary,
        event: {
          type: 'log',
          message: node.kind === 'agent' ? 'Agent' : node.label,
          timestamp: node.startedAt,
          depth: depths.get(activityId) ?? 0,
          color: 'dodgerblue',
          level:
            node.status === 'running'
              ? 'info'
              : node.status === 'completed' || node.status === 'received'
                ? 'success'
                : 'error'
        }
      });
      if (event.type === 'step.started') {
        const labels = pending.get(event.activityId) ?? new Map<string, string>();
        labels.set(event.label, event.id);
        pending.set(event.activityId, labels);
      }
      if (event.type === 'agent.started') agents.set(event.agentId, activityId);
      return;
    }
    if (event.type === 'log') {
      let message = plain(event.message);
      if (event.sourceId && (event.source === 'agent' || event.source === 'step')) {
        const prefix = `[${event.sourceId}] `;
        if (message.startsWith(prefix)) message = message.slice(prefix.length);
      }
      const stepId =
        event.source === 'step' ? pending.get(event.activityId)?.get(message) : undefined;
      const owner =
        stepId ??
        (event.source === 'agent'
          ? agents.get(event.sourceId ?? '')
          : (event.sourceId ?? event.activityId));
      const summary = owner ? summaries.get(owner) : undefined;
      if (summary) {
        if (stepId) summary.fields = event.fields;
        else summary.logs.push(message + formatLogFields(event.fields));
      }
      const diagnostic = event.level === 'warn' || event.level === 'error';
      if (!diagnostic) {
        if (stepId) {
          pending.get(event.activityId)?.delete(message);
          return;
        }
        // Match lifecycle positions as well as text, so replies discussing usage stay visible.
        const next = events[index + 1];
        if (
          summary &&
          event.source === 'agent' &&
          ((summary.logs.length === 1 &&
            message === `Agent started (model: ${summary.node.label})`) ||
            (message.startsWith('Token usage: ') &&
              next?.type === 'agent.finished' &&
              next.agentId === event.sourceId))
        )
          return;
        if (event.level === 'debug') return;
      }
      rows.push({ id: index, event: { ...event, message } });
      return;
    }
    if (event.type === 'step.finished' && event.error) {
      summaries.get(event.id)?.logs.push(event.error);
      rows.push({
        id: index,
        event: {
          type: 'log',
          level: 'error',
          message: event.error,
          timestamp: event.timestamp,
          depth: (depths.get(event.id) ?? 0) + 1,
          color: 'crimson'
        }
      });
    }
    // Host messages remain visible. Lifecycle rows above replace synthetic start/finish lines.
    if (event.type === 'message.sent') {
      const row = detailed.get(index);
      if (row) rows.push(row);
    }
  });
  for (const summary of summaries.values()) {
    if (!summary.logs.length) summary.logs = summary.node.logs;
  }
  return rows;
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
  const activityTasks = new Map<string, string>();
  const agents = new Map<string, string>();
  const pendingStepLogs = new Map<string | undefined, Map<string, string>>();
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
  const task = (id: string | undefined, embedded: boolean) =>
    id ? { id, reference: taskReference(id), embedded } : undefined;
  const addActivity = (
    id: number,
    timestamp: number,
    message: string,
    level: LogEvent['level'],
    depth: number,
    taskId?: string
  ) => {
    rows.push({
      id,
      activity: task(taskId, true),
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
      case 'log': {
        // A step writes its label before its own activity ID is active. Link that
        // visible name to the step even when another event arrives between them.
        const pending = event.source === 'step' ? pendingStepLogs.get(event.activityId) : undefined;
        const startedId = pending?.get(event.message);
        if (startedId) pending?.delete(event.message);
        rows.push({
          id,
          event: { ...event, message: event.message + formatLogFields(event.fields) },
          activity: startedId
            ? { id: startedId, reference: event.message, embedded: true }
            : event.source === 'agent' && event.sourceId && agents.has(event.sourceId)
              ? {
                  id: agents.get(event.sourceId)!,
                  reference: event.sourceId,
                  embedded: event.message.includes(`[${event.sourceId}]`)
                }
              : task(
                  activityTasks.get(event.activityId ?? '') ??
                    activityTasks.get(event.sourceId ?? ''),
                  false
                )
        });
        break;
      }
      case 'agent.started':
        agents.set(event.agentId, `${event.agentId}:${id}`);
        break;
      case 'step.started': {
        const depth = childDepth(event.activityId);
        taskDepths.set(event.id, depth);
        activityTasks.set(event.id, event.id);
        const pending = pendingStepLogs.get(event.activityId) ?? new Map<string, string>();
        pending.set(event.label, event.id);
        pendingStepLogs.set(event.activityId, pending);
        addActivity(
          id,
          event.timestamp,
          `Started ${taskReference(event.id)}`,
          'info',
          depth,
          event.id
        );
        break;
      }
      case 'command.started':
      case 'wait.started': {
        const parentTask = activityTasks.get(event.activityId ?? '');
        if (parentTask) activityTasks.set(event.id, parentTask);
        break;
      }
      case 'task.linked':
        channelTasks.set(event.channelId, event.taskId);
        break;
      case 'message.sent': {
        // Show the messages between the host and the top-level task. Messages between tasks
        // stay on the timeline.
        if (event.activityId !== undefined) break;
        const taskId = channelTasks.get(event.channelId);
        const payload =
          event.payload.length > 2000 ? `${event.payload.slice(0, 2000)}…` : event.payload;
        const target = taskId ? taskReference(taskId) : 'workflow';
        addActivity(
          id,
          event.timestamp,
          `${event.direction === 'input' ? 'Message to' : 'Update from'} ${target} · ${payload}`,
          'info',
          0,
          taskId
        );
        break;
      }
      case 'step.finished':
        addActivity(
          id,
          event.timestamp,
          `${taskReference(event.id)} ${event.status} · ${Math.round(event.durationMs / 1000)} s${event.error ? ` · ${event.error}` : ''}`,
          event.status === 'completed' ? 'success' : 'error',
          taskDepths.get(event.id) ?? childDepth(event.activityId),
          event.id
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
