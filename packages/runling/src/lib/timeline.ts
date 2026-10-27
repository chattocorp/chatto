import type { RunlingEvent, TokenUsage } from 'runling';
import { mergeUsage } from './usage.ts';
import type { RunStatus } from './runs.ts';

export interface TaskMessage {
  id: string;
  timestamp: number;
  direction: 'input' | 'update';
  payload: string;
  from: string;
  to: string;
  status: 'Queued' | 'Read';
}

export interface Activity {
  id: string;
  label: string;
  /** A `wait` is a task waiting for a message from its parent. */
  kind: 'step' | 'command' | 'agent' | 'wait';
  parent?: string;
  status: string;
  startedAt: number;
  durationMs?: number;
  logs: string[];
  children: Activity[];
  usage?: TokenUsage;
  preview?: string;
  messages?: TaskMessage[];
}

/** Running leaf work is our best signal; parent/child work can still overlap. */
export function isActivityActive(activity: Activity): boolean {
  const hasRunningDescendant = (node: Activity): boolean =>
    node.children.some((child) => child.status === 'running' || hasRunningDescendant(child));
  return (
    activity.status === 'running' && activity.kind !== 'wait' && !hasRunningDescendant(activity)
  );
}

export function buildTimeline(events: RunlingEvent[], runStatus: RunStatus): Activity[] {
  const nodes = new Map<string, Activity>();
  const agents = new Map<string, string>();
  const progressAgents = new Set<string>();
  const actionLogs = new Map<string, string[]>();
  const roots: Activity[] = [];
  const add = (node: Activity) => {
    nodes.set(node.id, node);
    const parent = node.parent ? nodes.get(node.parent) : undefined;
    (parent?.children ?? roots).push(node);
  };
  for (const [index, event] of events.entries()) {
    if (event.type === 'execution.finished') {
      for (const node of nodes.values()) {
        if (node.status !== 'running') continue;
        node.status = 'interrupted';
        node.durationMs = Math.max(0, event.timestamp - node.startedAt);
      }
    }
    const base = {
      parent: event.activityId,
      status: 'running',
      startedAt: event.timestamp,
      logs: [],
      children: []
    };
    if (event.type === 'step.started')
      add({ ...base, id: event.id, kind: 'step', label: event.label });
    if (event.type === 'command.started')
      add({ ...base, id: event.id, kind: 'command', label: event.command });
    if (event.type === 'wait.started')
      add({ ...base, id: event.id, kind: 'wait', label: 'Waiting for a message' });
    if (event.type === 'agent.started') {
      const id = `${event.agentId}:${index}`;
      agents.set(event.agentId, id);
      add({ ...base, id, kind: 'agent', label: event.model });
    }
    if (
      event.type === 'step.finished' ||
      event.type === 'command.finished' ||
      event.type === 'wait.finished'
    ) {
      const node = nodes.get(event.id);
      if (node) {
        node.status = event.status;
        node.durationMs = event.durationMs;
        if (event.type === 'command.finished') {
          if (event.output.stdout) node.logs.push(event.output.stdout);
          if (event.output.stderr) node.logs.push(event.output.stderr);
        }
      }
    }
    if (event.type === 'agent.finished') {
      const node = nodes.get(agents.get(event.agentId) ?? '');
      if (node) {
        node.status = event.outcome;
        node.durationMs = event.timestamp - node.startedAt;
      }
    }
    if (event.type === 'agent.usage' || event.type === 'agent.finished') {
      const node = nodes.get(agents.get(event.agentId) ?? '');
      if (node) node.usage = { ...event.usage };
    }
    if (event.type === 'agent.progress') {
      const node = nodes.get(agents.get(event.agentId) ?? '');
      if (node) {
        progressAgents.add(node.id);
        node.preview = event.text;
      }
    }
    if (event.type === 'agent.action') {
      const node = nodes.get(agents.get(event.agentId) ?? '');
      if (node) {
        const actions = actionLogs.get(node.id) ?? [];
        actions.push(event.action);
        actionLogs.set(node.id, actions);
        // Old journals and already-running servers have no progress events.
        if (
          !progressAgents.has(node.id) &&
          !/^(?:Agent started|Agent finished|Turn \d+|Tokens:|Token usage:)/.test(event.action)
        ) {
          const text = event.action.replace(/\s+/g, ' ').trim();
          if (text) node.preview = text.slice(-500);
        }
      }
    }
    if (event.type === 'log') {
      const node = nodes.get(
        event.source === 'agent'
          ? (agents.get(event.sourceId ?? '') ?? '')
          : (event.sourceId ?? event.activityId ?? '')
      );
      node?.logs.push(event.message);
    }
  }
  // Resolve links after all events: schema parsing can delay a child's first step.
  const links = new Map(
    events.flatMap((event) =>
      event.type === 'task.linked' ? [[event.channelId, event.taskId] as const] : []
    )
  );
  const reads = new Set(
    events.flatMap((event) => (event.type === 'message.read' ? [event.id] : []))
  );

  for (const event of events) {
    if (event.type !== 'message.sent') continue;
    const child = nodes.get(links.get(event.channelId) ?? '');
    const parent = nodes.get(event.activityId ?? '');
    const lane = child ?? parent;
    if (!lane) continue;

    // A message without a parent activity is between the host and the top-level task.
    const parentLabel = parent?.label ?? 'Host';
    const childLabel = child?.label ?? 'Task';
    (lane.messages ??= []).push({
      id: event.id,
      timestamp: event.timestamp,
      direction: event.direction,
      payload: event.payload,
      from: event.direction === 'input' ? parentLabel : childLabel,
      to: event.direction === 'input' ? childLabel : parentLabel,
      status: reads.has(event.id) ? 'Read' : 'Queued'
    });
  }

  // Prefer formatted log events. Older activity instances may only have actions.
  for (const [id, actions] of actionLogs) {
    const node = nodes.get(id)!;
    if (!node.logs.length) node.logs = actions;
  }
  if (runStatus !== 'running') {
    for (const node of nodes.values()) {
      if (node.status === 'running') node.status = 'interrupted';
    }
  }
  const aggregate = (node: Activity): void => {
    node.children.forEach(aggregate);
    if (node.kind === 'step')
      node.usage = mergeUsage(node.children.flatMap((child) => (child.usage ? [child.usage] : [])));
  };
  roots.forEach(aggregate);
  return roots;
}

export function findActivity(nodes: Activity[], id: string): Activity | undefined {
  for (const node of nodes) {
    if (node.id === id) return node;
    const child = findActivity(node.children, id);
    if (child) return child;
  }
}

/** A wait is an activity, not a paused workflow. */
export function activityStatus(activity: Activity): string {
  if (activity.kind !== 'wait') return activity.status;
  if (activity.status === 'running') return 'waiting for a message';
  if (activity.status === 'received') return 'message received';
  return activity.status;
}
