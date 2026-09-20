import type { Activity } from "./timeline.ts";

export interface TimeWindow {
  start: number;
  span: number;
}

/** Event timestamps can include setup time outside the runner's duration clock. */
export function timelineEnd(nodes: Activity[], elapsed: number): number {
  return nodes.reduce((end, node) => Math.max(
    end,
    node.startedAt + (node.durationMs ?? Math.max(0, elapsed - node.startedAt)),
    timelineEnd(node.children, elapsed),
  ), elapsed);
}

function niceCeiling(value: number): number {
  const power = 10 ** Math.floor(Math.log10(Math.max(value, 0.001)));
  const fraction = value / power;
  return (
    (fraction <= 1 ? 1 : fraction <= 2 ? 2 : fraction <= 5 ? 5 : 10) * power
  );
}

/** Fit the actual run duration; only empty runs need a nonzero minimum. */
export function fitWindow(elapsed: number): TimeWindow {
  return { start: 0, span: Math.max(1, elapsed) };
}

/** Keep every navigation path within the full timeline's fit range. */
export function clampWindow(view: TimeWindow, maxSpan: number): TimeWindow {
  const span = Math.max(1, Math.min(maxSpan, view.span));
  return { span, start: Math.max(0, Math.min(maxSpan - span, view.start)) };
}

/** Preserve the time under the pointer while changing scale. */
export function zoomWindow(
  view: TimeWindow,
  factor: number,
  anchor = 0.5,
): TimeWindow {
  const fraction = Math.max(0, Math.min(1, anchor));
  const span = Math.max(1, Math.min(86_400_000, view.span * factor));
  return {
    start: Math.max(0, view.start + fraction * (view.span - span)),
    span,
  };
}

export function panWindow(view: TimeWindow, deltaMs: number): TimeWindow {
  return { ...view, start: Math.max(0, view.start + deltaMs) };
}

/** Negative drag deltas zoom in around the initial pointer. */
export function dragZoomWindow(
  view: TimeWindow,
  delta: number,
  anchor = 0.5,
): TimeWindow {
  return zoomWindow(
    view,
    Math.exp(Math.max(-1000, Math.min(1000, delta)) * 0.006),
    anchor,
  );
}

export function dragRowHeight(initial: number, dy: number): number {
  return Math.max(
    24,
    Math.min(
      100,
      initial * Math.exp(Math.max(-1000, Math.min(1000, dy)) * 0.006),
    ),
  );
}

export function timelineTicks(view: TimeWindow, width: number): number[] {
  const interval = niceCeiling(view.span / Math.max(2, Math.floor(width / 90)));
  const first = Math.ceil(view.start / interval) * interval;
  const ticks: number[] = [];
  for (let value = first; value <= view.start + view.span; value += interval) {
    ticks.push(value);
    if (ticks.length > 30) break;
  }
  return ticks;
}

export function tickLabel(ms: number, step: number): string {
  const seconds = ms >= 1000;
  const unit = seconds ? 1000 : 1;
  const precision = Math.max(
    0,
    Math.min(4, Math.ceil(-Math.log10(step / unit))),
  );
  return `${(ms / unit).toFixed(precision)} ${seconds ? "s" : "ms"}`;
}

export function flattenActivities(
  nodes: Activity[],
  collapsed: ReadonlySet<string>,
  depth = 0,
): { node: Activity; depth: number }[] {
  return nodes.flatMap((node) => [
    { node, depth },
    ...(collapsed.has(node.id)
      ? []
      : flattenActivities(node.children, collapsed, depth + 1)),
  ]);
}

/** Intersect bars with the visible window; short events retain a small hit target. */
export function barPosition(
  start: number,
  end: number,
  view: TimeWindow,
): {
  left: number;
  width: number;
  clippedStart: boolean;
  clippedEnd: boolean;
} | null {
  const right = view.start + view.span;
  if (end < view.start || start > right) return null;
  const visibleStart = Math.max(view.start, start);
  const visibleEnd = Math.min(right, Math.max(start, end));
  return {
    left: ((visibleStart - view.start) / view.span) * 100,
    width: (Math.max(0, visibleEnd - visibleStart) / view.span) * 100,
    clippedStart: start < view.start,
    clippedEnd: end > right,
  };
}
