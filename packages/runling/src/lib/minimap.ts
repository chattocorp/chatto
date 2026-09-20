import { fitWindow, type TimeWindow } from "./timeline-layout.ts";

/** Navigation must not expand the overview beyond the full run's fit range. */
export function overviewSpan(elapsed: number): number {
  return fitWindow(elapsed).span;
}

export function moveOverview(
  view: TimeWindow,
  total: number,
  time: number,
  offset = view.span / 2,
): TimeWindow {
  return {
    span: view.span,
    start: Math.max(0, Math.min(total - view.span, time - offset)),
  };
}
