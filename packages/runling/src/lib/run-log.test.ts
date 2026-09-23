import { expect, test } from "vitest";
import type { RunlingEvent } from "runling";
import { logTime, nextLogFollow, runLogRows } from "./run-log.ts";

test("projects only recorded logs and keeps their order and metadata", () => {
  const events: RunlingEvent[] = [
    { type: "step.started", id: "task", label: "Task", timestamp: 0 },
    { type: "log", level: "info", message: "Starting", depth: 1, color: "blue", timestamp: 20 },
    { type: "agent.action", agentId: "agent", action: "Working", timestamp: 30 },
    { type: "log", level: "debug", message: "Detail", depth: 2, color: "gray", source: "agent", sourceId: "agent", timestamp: 40 },
  ];
  expect(runLogRows(events)).toEqual([
    { id: 1, event: events[1] },
    { id: 3, event: events[3] },
  ]);
  expect(logTime(3_723_999)).toBe("01:02:03");
});

test("follows at the bottom, pauses on upward scroll, and resumes there", () => {
  expect(nextLogFollow(true, 100, { scrollTop: 90, scrollHeight: 500, clientHeight: 100 })).toBe(false);
  expect(nextLogFollow(false, 90, { scrollTop: 120, scrollHeight: 500, clientHeight: 100 })).toBe(false);
  expect(nextLogFollow(false, 120, { scrollTop: 394, scrollHeight: 500, clientHeight: 100 })).toBe(true);
  expect(nextLogFollow(true, 394, { scrollTop: 394, scrollHeight: 600, clientHeight: 100 })).toBe(true);
});
