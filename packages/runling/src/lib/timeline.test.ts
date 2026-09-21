import { expect, test } from "vitest";
import { buildTimeline, isActivityActive, activityStatus, type Activity } from "./timeline.ts";
import type { RunlingEvent } from "runling";

const task = (overrides: Partial<Activity> = {}): Activity => ({
  id: "task", label: "Task", kind: "step", status: "running", startedAt: 0,
  logs: [], children: [], ...overrides,
});

test("indicates running leaf work but not input waits or finished tasks", () => {
  for (const kind of ["step", "agent", "command"] as const)
    expect(isActivityActive(task({ kind }))).toBe(true);
  expect(isActivityActive(task({ kind: "input" }))).toBe(false);
  for (const status of ["completed", "failed", "blocked", "interrupted"])
    expect(isActivityActive(task({ status }))).toBe(false);
});

test("keeps waiting ancestors quiet and marks parallel children active", () => {
  const first = task({ id: "first", kind: "agent" });
  const second = task({ id: "second", kind: "command" });
  const parent = task({ children: [first, second] });
  expect(isActivityActive(parent)).toBe(false);
  expect(isActivityActive(task({ children: [parent] }))).toBe(false);
  expect(isActivityActive(first)).toBe(true);
  expect(isActivityActive(second)).toBe(true);
  first.status = "completed";
  expect(isActivityActive(parent)).toBe(false);
  second.status = "completed";
  expect(isActivityActive(parent)).toBe(true);
});

test("does not animate a step waiting for user input", () => {
  expect(isActivityActive(task({ children: [task({ kind: "input" })] }))).toBe(false);
});

test("uses formatted logs once without deduplicating genuine repeated messages", () => {
  const events: RunlingEvent[] = [
    {
      type: "agent.started",
      agentId: "a",
      model: "model",
      color: "blue",
      timestamp: 0,
    },
  ];
  const message = "\u001b[33m[a]\u001b[0m Reading file.ts";
  for (let i = 0; i < 2; i++) {
    events.push(
      {
        type: "agent.action",
        agentId: "a",
        action: "Reading file.ts",
        timestamp: i * 2 + 1,
      },
      {
        type: "log",
        source: "agent",
        sourceId: "a",
        message,
        level: "info",
        depth: 0,
        color: "blue",
        timestamp: i * 2 + 2,
      },
    );
  }
  const node = buildTimeline(events, "running")[0]!;
  expect(node.logs).toEqual([message, message]);
  expect(node.preview).toBe("Reading file.ts");
  // A later invocation of the same agent can still have action-only history.
  events.push(
    {
      type: "agent.started",
      agentId: "a",
      model: "model",
      color: "blue",
      timestamp: 5,
    },
    {
      type: "agent.action",
      agentId: "a",
      action: "Legacy message",
      timestamp: 6,
    },
  );
  expect(buildTimeline(events, "completed")[1]?.logs).toEqual([
    "Legacy message",
  ]);
});

test("shows legacy tool actions without letting debug or token summaries replace them", () => {
  const events: RunlingEvent[] = [
    {
      type: "agent.started",
      agentId: "a",
      model: "model",
      color: "blue",
      timestamp: 0,
    },
    ...[
      "Agent started (model: model)",
      "Reading\n src/file.ts",
      "Turn 2 started",
      "Token usage: in 10, out 2",
    ].map((action, index) => ({
      type: "agent.action" as const,
      agentId: "a",
      action,
      timestamp: index + 1,
    })),
  ];
  expect(buildTimeline(events, "completed")[0]?.preview).toBe(
    "Reading src/file.ts",
  );
  expect(buildTimeline(events, "running")[0]?.logs).toHaveLength(4);
});

test("keeps agent previews separate from logs and retains them on completion", () => {
  const nodes = buildTimeline(
    [
      {
        type: "agent.started",
        agentId: "a",
        model: "model",
        color: "blue",
        timestamp: 0,
      },
      {
        type: "agent.started",
        agentId: "b",
        model: "model",
        color: "blue",
        timestamp: 1,
      },
      {
        type: "agent.progress",
        agentId: "a",
        text: "Reading file.ts",
        timestamp: 2,
      },
      {
        type: "agent.progress",
        agentId: "b",
        text: "Running tests",
        timestamp: 3,
      },
      {
        type: "agent.action",
        agentId: "a",
        action: "Turn 2 started",
        timestamp: 4,
      },
      {
        type: "agent.finished",
        agentId: "a",
        outcome: "completed",
        usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
        timestamp: 5,
      },
    ],
    "running",
  );
  expect(nodes[0]?.preview).toBe("Reading file.ts");
  expect(nodes[0]?.logs).toEqual(["Turn 2 started"]);
  expect(nodes[1]?.preview).toBe("Running tests");
});

test("rolls live snapshots through nested steps without double counting finishes or parallel agents", () => {
  const usage = {
    input: 10,
    output: 2,
    cacheRead: 30,
    cacheWrite: 1,
    cost: 0.01,
  };
  const events: RunlingEvent[] = [
    { type: "step.started", id: "root", label: "Root", timestamp: 0 },
    {
      type: "step.started",
      id: "child",
      label: "Child",
      activityId: "root",
      timestamp: 1,
    },
    {
      type: "agent.started",
      agentId: "a",
      model: "model",
      color: "blue",
      activityId: "child",
      timestamp: 2,
    },
    {
      type: "agent.started",
      agentId: "b",
      model: "model",
      color: "blue",
      activityId: "root",
      timestamp: 3,
    },
    { type: "agent.usage", agentId: "a", usage, timestamp: 4 },
    {
      type: "agent.usage",
      agentId: "a",
      usage: { ...usage, input: 20 },
      timestamp: 5,
    },
    { type: "agent.usage", agentId: "b", usage, timestamp: 6 },
  ];
  const live = buildTimeline(events, "running")[0]!;
  expect(live.usage).toEqual({
    ...usage,
    input: 30,
    output: 4,
    cacheRead: 60,
    cacheWrite: 2,
    cost: 0.02,
  });
  expect(live.children[0]?.usage?.input).toBe(20);
  events.push({
    type: "agent.finished",
    agentId: "a",
    outcome: "failed",
    usage: { ...usage, input: 20 },
    timestamp: 7,
  });
  expect(buildTimeline(events, "interrupted")[0]?.usage).toEqual(live.usage);
  // Older journals have only agent.finished events.
  expect(
    buildTimeline(
      events.filter((e) => e.type !== "agent.usage"),
      "failed",
    )[0]?.usage?.input,
  ).toBe(20);
});

test("keeps repeated turns of one agent distinct and interrupts unfinished blocks", () => {
  const tree = buildTimeline(
    [
      { type: "step.started", id: "parent", label: "Parent", timestamp: 0 },
      {
        type: "agent.started",
        agentId: "agent",
        model: "model",
        color: "blue",
        activityId: "parent",
        timestamp: 1,
      },
      {
        type: "agent.finished",
        agentId: "agent",
        outcome: "completed",
        timestamp: 5,
        usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
      },
      {
        type: "agent.started",
        agentId: "agent",
        model: "model",
        color: "blue",
        activityId: "parent",
        timestamp: 6,
      },
    ],
    "interrupted",
  );
  expect(tree[0]?.status).toBe("interrupted");
  expect(tree[0]?.children.map((node) => node.status)).toEqual([
    "completed",
    "interrupted",
  ]);
});

test("input outcomes retain failure reasons and old journals still render", () => {
  const events: RunlingEvent[] = [
    { type: "input.requested", id: "q", message: "Question", timestamp: 0 },
  ];
  expect(activityStatus(buildTimeline(events, "running")[0]!)).toBe("waiting for input");
  for (const reason of [undefined, "timeout", "cancelled"] as const) {
    const node = buildTimeline([...events, { type: "input.finished", id: "q", status: "failed", reason, durationMs: 500, timestamp: 500 }], "completed")[0]!;
    expect(node.durationMs).toBe(500);
    expect(activityStatus(node)).toBe(reason === "timeout" ? "timed out" : reason ?? "failed");
  }
  const answered = buildTimeline([...events, { type: "input.finished", id: "q", status: "answered", value: "Yes", durationMs: 400, timestamp: 400 }], "completed")[0]!;
  expect(activityStatus(answered)).toBe("answered");
  expect(answered.logs).toContain("Yes");
});

test("message markers resolve delayed task links and distinguish queue, read, and agent receipts", () => {
  const nodes = buildTimeline([
    { type: "step.started", id: "parent", label: "Coordinator", timestamp: 0 },
    { type: "message.sent", id: "q", channelId: "channel", direction: "input", payload: "queued", activityId: "parent", timestamp: 1 },
    { type: "step.started", id: "child", label: "Investigator", activityId: "parent", timestamp: 2 },
    { type: "task.linked", channelId: "channel", taskId: "child", timestamp: 2 },
    { type: "message.sent", id: "r", channelId: "channel", direction: "input", payload: "read", activityId: "parent", timestamp: 3 },
    { type: "message.read", id: "r", timestamp: 4 },
    { type: "message.sent", id: "a", channelId: "channel", direction: "input", payload: "agent", activityId: "parent", timestamp: 5 },
    { type: "message.receipt", id: "a", consumed: true, timestamp: 6 },
    { type: "message.sent", id: "u", channelId: "channel", direction: "update", payload: "result", activityId: "parent", timestamp: 7 },
    { type: "message.read", id: "u", timestamp: 8 },
  ], "running");
  const messages = nodes[0]!.children[0]!.messages!;
  expect(messages.map(message => message.status)).toEqual(["Queued", "Read by task", "Consumed by agent", "Read by task"]);
  expect(messages[0]).toMatchObject({ from: "Coordinator", to: "Investigator" });
  expect(messages[3]).toMatchObject({ from: "Investigator", to: "Coordinator" });
});

test("conversation turns and waits share one lane without doubling usage", () => {
  const usage = { input: 10, output: 2, cacheRead: 0, cacheWrite: 0 };
  const events: RunlingEvent[] = [
    { type: "step.started", id: "chat", label: "Conversation", timestamp: 0 },
    { type: "conversation.started", activityId: "chat", timestamp: 0 },
    { type: "agent.started", agentId: "bot", model: "luna", color: "purple", activityId: "chat", timestamp: 1 },
    { type: "agent.finished", agentId: "bot", outcome: "completed", usage, activityId: "chat", timestamp: 5 },
    { type: "input.requested", id: "wait", message: "Next message", activityId: "chat", timestamp: 5 },
    { type: "input.finished", id: "wait", status: "answered", value: "Hello", durationMs: 5, activityId: "chat", timestamp: 10 },
    { type: "agent.started", agentId: "bot", model: "luna", color: "purple", activityId: "chat", timestamp: 10 },
    { type: "agent.finished", agentId: "bot", outcome: "completed", usage, activityId: "chat", timestamp: 15 },
    { type: "input.requested", id: "waiting", message: "Next message", activityId: "chat", timestamp: 15 },
  ];
  const [chat] = buildTimeline(events, "running");
  expect(chat!.children).toEqual([]);
  expect(chat!.segments?.map(segment => segment.kind)).toEqual(["agent", "input", "agent", "input"]);
  expect(chat!.usage?.input).toBe(20);
  expect(isActivityActive(chat!)).toBe(false);
  expect(chat!.logs).toContain("Hello");

  const ordinary = buildTimeline(events.filter(event => event.type !== "conversation.started"), "running")[0]!;
  expect(ordinary.children).toHaveLength(4);
  expect(ordinary.segments).toBeUndefined();
});
