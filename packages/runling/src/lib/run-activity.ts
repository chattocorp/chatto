import type { RunActivity, RunDetail } from "./runs.ts";
import { buildTimeline, isActivityActive } from "./timeline.ts";
import { flattenActivities } from "./timeline-layout.ts";

export function summarizeRunActivity(run: RunDetail): RunActivity | null {
  if (run.status !== "running") return null;
  const rows = flattenActivities(buildTimeline(run.events, run.status), new Set());
  const current = rows.map(({ node }) => node).filter((node) =>
    node.status === "running" && (isActivityActive(node) || node.kind === "input"),
  );
  // Prefer an input wait, then the activity with the latest update.
  let selected = current.find((node) => node.kind === "input");
  if (!selected) {
    for (let i = run.events.length - 1; i >= 0; i--) {
      const event = run.events[i]!;
      selected = current.find((node) => {
        if ("agentId" in event || (event.type === "log" && event.source === "agent")) {
          const id = "agentId" in event ? event.agentId : event.sourceId;
          return node.kind === "agent" && node.id.slice(0, node.id.lastIndexOf(":")) === id;
        }
        return node.id === ("id" in event ? event.id : event.type === "log" ? event.sourceId ?? event.activityId : event.activityId);
      });
      if (selected) break;
    }
  }
  selected ??= current.at(-1);
  let parent = rows.find(({ node }) => node.id === selected?.parent)?.node;
  while (parent && parent.kind !== "step") {
    parent = rows.find(({ node }) => node.id === parent?.parent)?.node;
  }
  return selected ? {
    label: selected.label,
    step: parent?.label,
    preview: selected.preview?.slice(-500),
    waiting: selected.kind === "input",
    pendingInputs: current.filter(node => node.kind === "input").length,
    parallel: current.filter(node => node.kind !== "input" && node !== selected).length,
  } : null;
}
