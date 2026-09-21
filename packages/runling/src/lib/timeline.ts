import type { RunlingEvent, TokenUsage } from "runling";
import { mergeUsage } from "./usage.ts";
import type { RunStatus } from "./runs.ts";

export interface TaskMessage {
  id: string;
  timestamp: number;
  direction: "input" | "update";
  payload: string;
  from: string;
  to: string;
  status:
    | "Queued"
    | "Read by task"
    | "Consumed by agent"
    | "Not consumed by agent";
}

export interface Activity {
  id: string;
  label: string;
  kind: "step" | "command" | "agent" | "input";
  parent?: string;
  status: string;
  reason?: "timeout" | "cancelled";
  startedAt: number;
  durationMs?: number;
  logs: string[];
  children: Activity[];
  usage?: TokenUsage;
  preview?: string;
  messages?: TaskMessage[];
  /** Model turns and input waits displayed on this conversation lane. */
  segments?: Activity[];
}

/** Running leaf work is our best signal; parent/child work can still overlap. */
export function isActivityActive(activity: Activity): boolean {
  if (activity.segments?.some(segment => segment.kind === "input" && segment.status === "running")) return false;
  const hasRunningDescendant = (node: Activity): boolean =>
    node.children.some(
      (child) => child.status === "running" || hasRunningDescendant(child),
    );
  return (
    activity.status === "running" &&
    activity.kind !== "input" &&
    !hasRunningDescendant(activity)
  );
}

export function buildTimeline(
  events: RunlingEvent[],
  runStatus: RunStatus,
): Activity[] {
  const nodes = new Map<string, Activity>();
  const conversations = new Set<string>();
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
    const base = {
      parent: event.activityId,
      status: "running",
      startedAt: event.timestamp,
      logs: [],
      children: [],
    };
    if (event.type === "conversation.started" && event.activityId) conversations.add(event.activityId);
    if (event.type === "step.started")
      add({ ...base, id: event.id, kind: "step", label: event.label });
    if (event.type === "command.started")
      add({ ...base, id: event.id, kind: "command", label: event.command });
    if (event.type === "input.requested")
      add({ ...base, id: event.id, kind: "input", label: event.message });
    if (event.type === "agent.started") {
      const id = `${event.agentId}:${index}`;
      agents.set(event.agentId, id);
      add({ ...base, id, kind: "agent", label: event.model });
    }
    if (
      event.type === "step.finished" ||
      event.type === "command.finished" ||
      event.type === "input.finished"
    ) {
      const node = nodes.get(event.id);
      if (node) {
        node.status = event.status === "answered" ? "completed" : event.status;
        node.durationMs = event.durationMs;
        if (event.type === "input.finished" && event.status === "failed")
          node.reason = event.reason;
        if (event.type === "command.finished") {
          if (event.output.stdout) node.logs.push(event.output.stdout);
          if (event.output.stderr) node.logs.push(event.output.stderr);
        }
        if (event.type === "input.finished" && event.status === "answered")
          node.logs.push(event.value);
      }
    }
    if (event.type === "agent.finished") {
      const node = nodes.get(agents.get(event.agentId) ?? "");
      if (node) {
        node.status = event.outcome;
        node.durationMs = event.timestamp - node.startedAt;
      }
    }
    if (event.type === "agent.usage" || event.type === "agent.finished") {
      const node = nodes.get(agents.get(event.agentId) ?? "");
      if (node) node.usage = { ...event.usage };
    }
    if (event.type === "agent.progress") {
      const node = nodes.get(agents.get(event.agentId) ?? "");
      if (node) {
        progressAgents.add(node.id);
        node.preview = event.text;
      }
    }
    if (event.type === "agent.action") {
      const node = nodes.get(agents.get(event.agentId) ?? "");
      if (node) {
        const actions = actionLogs.get(node.id) ?? [];
        actions.push(event.action);
        actionLogs.set(node.id, actions);
        // Old journals and already-running servers have no progress events.
        if (
          !progressAgents.has(node.id) &&
          !/^(?:Agent started|Agent finished|Turn \d+|Tokens:|Token usage:)/.test(
            event.action,
          )
        ) {
          const text = event.action.replace(/\s+/g, " ").trim();
          if (text) node.preview = text.slice(-500);
        }
      }
    }
    if (event.type === "log") {
      const node = nodes.get(
        event.source === "agent"
          ? (agents.get(event.sourceId ?? "") ?? "")
          : (event.sourceId ?? event.activityId ?? ""),
      );
      node?.logs.push(event.message);
    }
  }
  // Resolve links after all events: schema parsing can delay a child's first step.
  const links = new Map(
    events.flatMap((event) =>
      event.type === "task.linked"
        ? [[event.channelId, event.taskId] as const]
        : [],
    ),
  );
  const reads = new Set(
    events.flatMap((event) =>
      event.type === "message.read" ? [event.id] : [],
    ),
  );
  const receipts = new Map(
    events.flatMap((event) =>
      event.type === "message.receipt"
        ? [[event.id, event.consumed] as const]
        : [],
    ),
  );

  for (const event of events) {
    if (event.type !== "message.sent") continue;
    const child = nodes.get(links.get(event.channelId) ?? "");
    const parent = nodes.get(event.activityId ?? "");
    const lane = child ?? parent;
    if (!lane) continue;

    const from = event.direction === "input" ? parent : child;
    const to = event.direction === "input" ? child : parent;
    (lane.messages ??= []).push({
      id: event.id,
      timestamp: event.timestamp,
      direction: event.direction,
      payload: event.payload,
      from: from?.label ?? "Caller",
      to: to?.label ?? "Task",
      status: receipts.has(event.id)
        ? receipts.get(event.id)
          ? "Consumed by agent"
          : "Not consumed by agent"
        : reads.has(event.id)
          ? "Read by task"
          : "Queued",
    });
  }

  // Prefer formatted log events. Older activity instances may only have actions.
  for (const [id, actions] of actionLogs) {
    const node = nodes.get(id)!;
    if (!node.logs.length) node.logs = actions;
  }
  if (runStatus !== "running") {
    for (const node of nodes.values()) {
      if (node.status === "running") node.status = "interrupted";
    }
  }
  // Only explicitly marked conversation tasks combine turns and waits. Ordinary
  // workflows and historical journals retain their existing activity layout.
  for (const id of conversations) {
    const node = nodes.get(id);
    if (!node) continue;
    node.segments = node.children.filter(child => child.kind === "agent" || child.kind === "input");
    node.children = node.children.filter(child => !node.segments!.includes(child));
    for (const segment of node.segments) {
      node.logs.push(`[${segment.startedAt.toFixed(0)}ms] ${segment.label}`, ...segment.logs);
    }
  }

  const aggregate = (node: Activity): void => {
    node.children.forEach(aggregate);
    if (node.kind === "step")
      node.usage = mergeUsage(
        [...node.children, ...(node.segments ?? [])].flatMap((child) => (child.usage ? [child.usage] : [])),
      );
  };
  roots.forEach(aggregate);
  return roots;
}

export function findActivity(
  nodes: Activity[],
  id: string,
): Activity | undefined {
  for (const node of nodes) {
    if (node.id === id) return node;
    const child = findActivity(node.children, id);
    if (child) return child;
  }
}

/** Input waits are activities, not a paused workflow. */
export function activityStatus(activity: Activity): string {
  if (activity.segments?.some(segment => segment.kind === "input" && segment.status === "running")) {
    return "waiting for input";
  }
  if (activity.kind !== "input") return activity.status;
  if (activity.status === "running") return "waiting for input";
  if (activity.status === "completed") return "answered";
  if (activity.reason === "timeout") return "timed out";
  if (activity.reason === "cancelled") return "cancelled";
  return activity.status;
}
