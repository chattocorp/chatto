import { expect, test } from "vitest";
import type { RunlingEvent } from "runling";
import { activityFeed, feedTime } from "./activity-feed.ts";

test("maps nested tasks, agent roles and linked output without protocol noise", () => {
  const events: RunlingEvent[] = [
    { type: "step.started", id: "root", label: "Workflow", timestamp: 0 },
    { type: "step.started", id: "child", label: "Task", activityId: "root", timestamp: 1 },
    { type: "task.linked", channelId: "channel", taskId: "child", timestamp: 2 },
    { type: "agent.started", agentId: "agent", label: "Investigate", model: "test/model", color: "#40c057", activityId: "child", timestamp: 3 },
    { type: "agent.tool", agentId: "agent", toolName: "read", operation: "read", phase: "started", timestamp: 4 },
    { type: "agent.tool", agentId: "agent", toolName: "read", operation: "read", phase: "succeeded", timestamp: 5 },
    { type: "agent.progress", agentId: "agent", text: "Partial token", timestamp: 6 },
    { type: "agent.action", agentId: "agent", action: "Reading file", timestamp: 7 },
    { type: "message.sent", id: "m", channelId: "channel", direction: "update", payload: '{"type":"finding","text":"Plan ready"}', activityId: "root", timestamp: 8 },
    { type: "message.read", id: "m", timestamp: 9 },
    { type: "message.sent", id: "t", channelId: "channel", direction: "update", payload: '{"type":"tool","phase":"succeeded"}', timestamp: 10 },
    { type: "log", source: "agent", level: "info", depth: 1, color: "red", message: "duplicate", timestamp: 11 },
  ];
  const rows = activityFeed(events, "running");
  expect(rows.map(row => row.message)).toEqual(["Task started", "Task started", "Agent started", "Reading files · read", "Reading file", "Plan ready"]);
  expect(rows.at(-1)).toMatchObject({ task: "Investigate", depth: 1, reference: "task-2", color: "#40c057", message: "Plan ready" });
  expect(activityFeed([...events, { type: "input.requested", id: "wait", message: "Next?", activityId: "root", timestamp: 12 }], "running").slice(0, -1)).toEqual(rows);
});

test("preserves failure details, neutral waits and terminal states even in old journals", () => {
  const rows = activityFeed([
    { type: "input.requested", id: "i", message: "Waiting", timestamp: 0 },
    { type: "command.finished", id: "c", status: "failed", durationMs: 5, output: { stdout: "", stderr: "diagnostic" }, timestamp: 5 },
    { type: "agent.tool", agentId: "unknown", phase: "failed", operation: "edit", timestamp: 6 },
  ], "interrupted", 10);
  expect(rows[0]).toMatchObject({ tone: "waiting", message: "Waiting for input" });
  expect(rows[1]).toMatchObject({ tone: "error", detail: "diagnostic" });
  expect(rows[2]).toMatchObject({ task: "Run", message: "Tool failed · edit" });
  expect(rows.at(-1)).toMatchObject({ timestamp: 10, message: "Run interrupted" });
  expect(activityFeed([], "completed")).toHaveLength(1);
});

test("bounds large details and formats elapsed time across hours", () => {
  const [row] = activityFeed([{ type: "message.sent", id: "m", direction: "input", channelId: "c", payload: "x".repeat(50000), timestamp: 0 }], "running");
  expect(row!.message.length).toBeLessThan(16100);
  expect(feedTime(3_723_999)).toBe("01:02:03");
});

test("preserves text that happens to be valid JSON", () => {
  const texts = ["42", "true", "null", '"hello"', '{"answer":42}'];
  const events: RunlingEvent[] = texts.map((payload, timestamp) => ({
    type: "message.sent", id: String(timestamp), channelId: "c", direction: "update", payload, timestamp,
  }));
  expect(activityFeed(events, "running").map(row => row.message)).toEqual(texts);
});

test("shows agent text once when the same task forwards it to its owner", () => {
  const events: RunlingEvent[] = [
    { type: "step.started", id: "task", label: "Worker", timestamp: 0 },
    { type: "task.linked", channelId: "c", taskId: "task", timestamp: 1 },
    { type: "agent.started", agentId: "a", activityId: "task", model: "test/model", color: "#60a5fa", timestamp: 2 },
    { type: "agent.action", agentId: "a", action: "Found the cause", timestamp: 3 },
    { type: "message.sent", id: "m", channelId: "c", direction: "update", payload: '{"type":"output","text":"Found the cause"}', timestamp: 4 },
    { type: "message.sent", id: "n", channelId: "c", direction: "update", payload: "Found the cause", timestamp: 5 },
  ];
  // Consume only the mirrored copy; a later independent repeat remains visible.
  expect(activityFeed(events, "running").filter(row => row.message === "Found the cause")).toHaveLength(2);
});
