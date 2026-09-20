import { expect, test } from "vitest";
import {
  barPosition,
  clampWindow,
  dragZoomWindow,
  dragRowHeight,
  fitWindow,
  timelineEnd,
  flattenActivities,
  panWindow,
  tickLabel,
  timelineTicks,
  zoomWindow,
} from "./timeline-layout.ts";
import type { Activity } from "./timeline.ts";

test("full range includes bar endpoints on a different duration clock", () => {
  const child: Activity = {
    id: "child", label: "Child", kind: "step", status: "completed",
    startedAt: 6400, durationMs: 4701, children: [], logs: [],
  };
  const parent: Activity = { ...child, id: "parent", startedAt: 0, durationMs: 11100, children: [child] };
  const end = timelineEnd([parent], 11099);
  expect(end).toBe(11101);
  expect(barPosition(6400, 11101, fitWindow(end))?.clippedEnd).toBe(false);
  expect(timelineEnd([parent], 12000)).toBe(12000);
  expect(timelineEnd([], 0)).toBe(0);
});

test("middle drag down enlarges rows, up shrinks them, and respects row limits", () => {
  expect(dragRowHeight(48, 80)).toBeGreaterThan(48);
  expect(dragRowHeight(48, -80)).toBeLessThan(48);
  expect(dragRowHeight(48, 0)).toBe(48);
  expect(dragRowHeight(48, 1000)).toBe(100);
  expect(dragRowHeight(48, -1000)).toBe(24);
});

test("signed drag delta zooms time around its anchor", () => {
  const view = { start: 300, span: 400 };
  const zoomed = dragZoomWindow(view, -80, 0.25);
  expect(zoomed.span).toBeLessThan(view.span);
  expect(zoomed.start + zoomed.span * 0.25).toBeCloseTo(400);
  expect(dragZoomWindow(view, 80).span).toBeGreaterThan(view.span);
  expect(dragZoomWindow(view, 0)).toEqual(view);
  expect(clampWindow(dragZoomWindow(view, 1000), 1000)).toEqual({
    start: 0,
    span: 1000,
  });
});

test("clamps zoom-out and panning to the full timeline", () => {
  expect(clampWindow({ start: 400, span: 1200 }, 1000)).toEqual({
    start: 0,
    span: 1000,
  });
  expect(clampWindow({ start: 950, span: 200 }, 1000)).toEqual({
    start: 800,
    span: 200,
  });
  expect(clampWindow({ start: -20, span: 200 }, 1000)).toEqual({
    start: 0,
    span: 200,
  });
  expect(clampWindow({ start: 300, span: 200 }, 1000)).toEqual({
    start: 300,
    span: 200,
  });
  for (const elapsed of [0, 1000, 120000]) {
    const full = fitWindow(elapsed);
    expect(clampWindow(zoomWindow(full, 2), full.span)).toEqual(full);
    expect(clampWindow(panWindow(full, 10000), full.span)).toEqual(full);
  }
});

test("fit ends at the actual run duration without padding or rounding", () => {
  expect(fitWindow(0)).toEqual({ start: 0, span: 1 });
  for (const elapsed of [5, 90, 1000, 9700, 12345, 3600000]) {
    expect(fitWindow(elapsed)).toEqual({ start: 0, span: elapsed });
  }
});

test("all navigation clamps to the actual end, including a stale rounded view", () => {
  const full = fitWindow(9700);
  expect(clampWindow({ start: 0, span: 20000 }, full.span)).toEqual(full);
  expect(clampWindow(zoomWindow(full, 2), full.span)).toEqual(full);
  expect(clampWindow(dragZoomWindow(full, 1000), full.span)).toEqual(full);
  expect(clampWindow(panWindow({ start: 0, span: 2000 }, 20000), full.span))
    .toEqual({ start: 7700, span: 2000 });
});

test("zoom anchors to the pointer and pan preserves scale", () => {
  const view = { start: 1000, span: 10000 };
  const next = zoomWindow(view, 0.5, 0.3);
  expect(next.start + next.span * 0.3).toBe(view.start + view.span * 0.3);
  expect(next.span).toBe(5000);
  expect(panWindow(next, 750)).toEqual({ start: next.start + 750, span: 5000 });
  expect(panWindow(view, -10000).start).toBe(0);
  expect(zoomWindow(view, 0.00000001).span).toBeGreaterThan(0);
});

test("manual windows and completed bar positions do not depend on the live clock", () => {
  const manual = zoomWindow(fitWindow(1500), 0.5);
  const before = barPosition(500, 750, manual);
  fitWindow(30000);
  expect(barPosition(500, 750, manual)).toEqual(before);
});

test("clips partially visible bars and excludes offscreen activities", () => {
  const view = { start: 100, span: 100 };
  expect(barPosition(50, 150, view)).toEqual({
    left: 0,
    width: 50,
    clippedStart: true,
    clippedEnd: false,
  });
  expect(barPosition(150, 250, view)).toEqual({
    left: 50,
    width: 50,
    clippedStart: false,
    clippedEnd: true,
  });
  expect(barPosition(0, 90, view)).toBeNull();
  expect(barPosition(201, 250, view)).toBeNull();
  expect(barPosition(150, 150, view)).toEqual({
    left: 50,
    width: 0,
    clippedStart: false,
    clippedEnd: false,
  });
});

test("distinguishes viewport cuts from actual endpoints when zoomed in", () => {
  const view = { start: 2500, span: 100 };
  expect(barPosition(0, 4300, view)).toEqual({
    left: 0,
    width: 100,
    clippedStart: true,
    clippedEnd: true,
  });
  expect(barPosition(2500, 2600, view)).toEqual({
    left: 0,
    width: 100,
    clippedStart: false,
    clippedEnd: false,
  });
  expect(barPosition(0, 4300, fitWindow(4300))).toMatchObject({
    clippedStart: false,
    clippedEnd: false,
  });
});

test("ticks remain readable and bounded at different zoom levels", () => {
  for (const view of [
    { start: 0, span: 1 },
    { start: 2300, span: 10000 },
    { start: 0, span: 1000000 },
  ]) {
    const ticks = timelineTicks(view, 600);
    expect(ticks.length).toBeGreaterThan(1);
    expect(ticks.length).toBeLessThan(12);
    expect(
      ticks.every(
        (tick) => tick >= view.start && tick <= view.start + view.span,
      ),
    ).toBe(true);
  }
  expect(tickLabel(1200.2, 0.2)).toBe("1.2002 s");
  expect(tickLabel(0.2, 0.2)).toBe("0.2 ms");
});

test("collapsing a parent hides descendants without changing sibling order", () => {
  const node = (id: string, children: Activity[] = []): Activity => ({
    id,
    label: id,
    kind: "step",
    status: "completed",
    startedAt: 0,
    logs: [],
    children,
  });
  const roots = [node("parent", [node("child")]), node("sibling")];
  expect(
    flattenActivities(roots, new Set()).map(({ node, depth }) => [
      node.id,
      depth,
    ]),
  ).toEqual([
    ["parent", 0],
    ["child", 1],
    ["sibling", 0],
  ]);
  expect(
    flattenActivities(roots, new Set(["parent"])).map(({ node }) => node.id),
  ).toEqual(["parent", "sibling"]);
});
