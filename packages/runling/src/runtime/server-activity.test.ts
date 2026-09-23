import { afterEach, expect, test, vi } from "vitest";
import { serverLog } from "./server-log.ts";
import { createServerActivityLog } from "./server-activity.ts";

vi.mock("./server-log.ts", () => ({ serverLog: vi.fn() }));
afterEach(() => vi.clearAllMocks());

test("concurrent runs and nested tasks retain distinct stable prefixes without payloads", () => {
  const first = createServerActivityLog("run-a", "bright-waves-1234");
  const second = createServerActivityLog("run-b", "happy-toes-5678");
  first({ type: "step.started", id: "outer", label: "private label", timestamp: 0 });
  first({ type: "step.started", id: "inner", activityId: "outer", label: "private name", timestamp: 1 });
  second({ type: "step.started", id: "other", label: "secret", timestamp: 2 });
  first({ type: "command.started", id: "cmd", activityId: "inner", command: "secret command", timestamp: 3 });
  first({ type: "input.requested", id: "input", activityId: "outer", message: "private question", timestamp: 4 });
  first({ type: "step.finished", id: "inner", activityId: "outer", status: "completed", durationMs: 5, timestamp: 6 });
  expect(vi.mocked(serverLog).mock.calls.map(call => [call[2]?.runReference, call[2]?.taskReference])).toEqual([
    ["bright-waves-1234", "task-1"], ["bright-waves-1234", "task-2"],
    ["happy-toes-5678", "task-1"], ["bright-waves-1234", "task-2"],
    ["bright-waves-1234", "task-1"], ["bright-waves-1234", "task-2"],
  ]);
  expect(JSON.stringify(vi.mocked(serverLog).mock.calls)).not.toMatch(/private|secret/);
});

test("throttles agent activity but always reports tool failures and completion", () => {
  const write = createServerActivityLog("run");
  write({ type: "agent.started", agentId: "agent", model: "model", color: "blue", timestamp: 0 });
  for (const timestamp of [1, 2, 14_999, 15_000, 15_001]) {
    write({ type: "agent.action", agentId: "agent", action: "secret text", timestamp });
  }
  write({ type: "agent.tool", agentId: "agent", operation: "read", phase: "failed", timestamp: 15_002 });
  write({ type: "step.finished", id: "task", status: "failed", durationMs: 5, timestamp: 15_003 });
  expect(vi.mocked(serverLog).mock.calls.map(call => [call[0], call[2]?.activity])).toEqual([
    ["info", "Agent started · model"], ["error", "Tool read failed"], ["error", "Task failed · 5 ms"],
  ]);
  expect(JSON.stringify(vi.mocked(serverLog).mock.calls)).not.toContain("secret");
});

test("summarizes successful calls, reports failures immediately, and associates host milestones", () => {
  const write = createServerActivityLog("run", "sunny-poems-5431");
  write({ type: "task.linked", taskId: "step", channelId: "channel", timestamp: 0 });
  write({ type: "agent.started", activityId: "step", agentId: "worker", label: "implement", model: "test/model", color: "blue", timestamp: 0 });
  for (let timestamp = 1; timestamp <= 100; timestamp++) {
    write({ type: "agent.tool", activityId: "step", agentId: "worker", operation: "read", phase: "started", timestamp });
    write({ type: "agent.tool", activityId: "step", agentId: "worker", operation: "read", phase: "succeeded", timestamp });
  }
  expect(serverLog).toHaveBeenCalledOnce();
  write({ type: "agent.tool", activityId: "step", agentId: "worker", operation: "search", phase: "succeeded", timestamp: 30_000 });
  write({ type: "agent.tool", activityId: "step", agentId: "worker", operation: "other", toolName: "preparePullRequest", phase: "failed", timestamp: 30_001 });
  write({ type: "task.activity", channelId: "channel", message: "Validation failed", timestamp: 30_002 });
  expect(vi.mocked(serverLog).mock.calls.map(call => call[2]?.activity)).toEqual([
    "Agent started · test/model", "Activity · 100 read, 1 search", "Tool preparePullRequest failed", "Validation failed",
  ]);
  expect(vi.mocked(serverLog).mock.calls[1]?.[2]).toMatchObject({ agentLabel: "implement", taskReference: "task-1" });
  expect(vi.mocked(serverLog).mock.calls[3]?.[2]).toMatchObject({ taskReference: "task-1" });
});
