import { setTimeout as sleep } from "node:timers/promises";
import { visibleWidth } from "@earendil-works/pi-tui";
import { stripVTControlCharacters } from "node:util";
import { expect, test } from "vitest";
import type { Terminal } from "@earendil-works/pi-tui";
import type { RunlingEvent, RunlingEventPayload } from "./events.ts";
import { observeRunlingEvents } from "./events.ts";
import { createInput } from "./input.ts";
import type { WorkflowExecution } from "./runner.ts";
import { RunlingDashboard, TuiReporter } from "./tui.ts";

const event = (
  payload: RunlingEventPayload,
  activityId?: string,
): RunlingEvent => ({
  ...payload,
  activityId,
  timestamp: 0,
}) as RunlingEvent;

test("renders activity as one semantic execution tree", () => {
  const dashboard = new RunlingDashboard("examples/review.ts", 0);
  dashboard.handle(
    event({ type: "step.started", id: "review", label: "Review" }),
  );
  dashboard.handle(
    event(
      { type: "step.started", id: "investigate", label: "Investigate change" },
      "review",
    ),
  );
  dashboard.handle(
    event(
      { type: "command.started", id: "command", command: "git status --short" },
      "investigate",
    ),
  );
  dashboard.handle(
    event(
      {
        type: "command.finished",
        id: "command",
        status: "completed",
        durationMs: 24,
        output: { stdout: "", stderr: "" },
      },
      "investigate",
    ),
  );
  dashboard.handle(
    event(
      {
        type: "agent.started",
        agentId: "bright-otters-1234",
        model: "openai/gpt-5",
        color: "orange",
      },
      "investigate",
    ),
  );
  dashboard.handle(
    event({
      type: "agent.action",
      agentId: "bright-otters-1234",
      action: "Reading src/runner.ts",
    }),
  );
  dashboard.handle(
    event({
      type: "log",
      level: "info",
      message: "[bright-otters-1234] Reading src/runner.ts",
      depth: 1,
      color: "orange",
      source: "agent",
      sourceId: "bright-otters-1234",
    }),
  );
  dashboard.handle(
    event({
      type: "agent.action",
      agentId: "bright-otters-1234",
      action: "Using grep",
    }),
  );
  dashboard.handle(
    event({
      type: "log",
      level: "error",
      message: "[bright-otters-1234] read failed",
      depth: 1,
      color: "orange",
      source: "agent",
      sourceId: "bright-otters-1234",
    }),
  );
  dashboard.handle(
    event(
      {
        type: "log",
        level: "info",
        message: "Review correctness",
        depth: 1,
        color: "dodgerblue",
      },
      "investigate",
    ),
  );
  dashboard.handle(
    event({
      type: "usage.updated",
      usage: {
        input: 100,
        output: 20,
        cacheRead: 500,
        cacheWrite: 0,
        cost: 0.042,
      },
    }),
  );

  const output = stripVTControlCharacters(dashboard.render(100).join("\n"));
  const lines = output.split("\n");
  expect(output).toContain("Runling  examples/review.ts");
  expect(lines).toContain("  ● Review");
  expect(lines).toContain("    ● Investigate change");
  expect(lines).toContain("      ✓ Ran git status --short · 24ms");
  expect(
    lines.some((line) =>
      /^      [⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏] \[bright-otters-1234\]/.test(line),
    ),
  ).toBe(true);
  expect(lines).toContain("        ↳ Using grep");
  expect(lines).toContain("      ● Review correctness");
  expect(output).not.toContain("Active agents");
  expect(output).not.toContain("Workflow\n");
  expect(output).not.toContain("Activity");
  expect(output).not.toContain("Reading src/runner.ts");
  expect(output).not.toContain("read failed");
  expect(output).toMatch(/Working…  ·  \d+(?:m \d{2}s|s)  ·  620 tokens/);
  expect(output).toContain("83% cached");
  expect(output).toContain("$0.04");
  expect(output).toContain("0/2 steps");

  dashboard.handle(
    event(
      {
        type: "agent.finished",
        agentId: "bright-otters-1234",
        outcome: "completed",
        usage: { input: 100, output: 20, cacheRead: 500, cacheWrite: 0 },
      },
      "investigate",
    ),
  );
  const completed = stripVTControlCharacters(dashboard.render(100).join("\n"));
  expect(completed).toContain("      ✓ [bright-otters-1234]");
  expect(completed).not.toContain("↳ Using grep");

  dashboard.handle(
    event({
      type: "step.finished",
      id: "investigate",
      status: "completed",
      durationMs: 1_500,
    }),
  );
  const collapsedStep = stripVTControlCharacters(dashboard.render(100).join("\n"));
  expect(collapsedStep).toContain("    ✓ Investigate change · 1.5s");
  expect(collapsedStep).not.toContain("git status --short");
  expect(collapsedStep).not.toContain("bright-otters-1234");
  expect(collapsedStep).not.toContain("Review correctness");

  dashboard.handle(
    event({
      type: "step.finished",
      id: "review",
      status: "completed",
      durationMs: 2_000,
    }),
  );
  const collapsedWorkflow = stripVTControlCharacters(dashboard.render(100).join("\n"));
  expect(collapsedWorkflow).toContain("  ✓ Review · 2.0s");
  expect(collapsedWorkflow).not.toContain("Investigate change");

  dashboard.handle(
    event({
      type: "step.finished",
      id: "review",
      status: "failed",
      durationMs: 2_100,
    }),
  );
  const failedWorkflow = stripVTControlCharacters(dashboard.render(100).join("\n"));
  expect(failedWorkflow).toContain("  ✗ Review · 2.1s");
  expect(failedWorkflow).toContain("    ✓ Investigate change · 1.5s");
  expect(failedWorkflow).not.toContain("git status --short");
});

test("renders the final Markdown report and completion state", () => {
  const dashboard = new RunlingDashboard("examples/review.ts", 0);
  const execution: WorkflowExecution = {
    ok: true,
    error: null,
    durationMs: 1_000,
    usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
    output: {
      summary: "Review complete",
      details: "## Findings\n\nEverything looks **good**.",
    },
    result: {
      summary: "Review complete",
      details: "## Findings\n\nEverything looks **good**.",
    },
  };

  dashboard.finish(execution);
  const output = stripVTControlCharacters(dashboard.render(80).join("\n"));

  expect(output).toContain("Completed  ·  1s  ·  0 tokens  ·  $0.00");
  expect(output).toContain("\nReview complete\n");
  expect(output).not.toContain("✓ Review complete");
  expect(output).toContain("Findings");
  expect(output).toContain("Everything looks good.");
  expect(output).not.toContain("##");
  expect(output).not.toContain("**");
});

test("wraps and Markdown-renders the final summary when details are present", () => {
  const dashboard = new RunlingDashboard("examples/review.ts", 0);
  dashboard.finish({
    ok: true,
    error: null,
    durationMs: 1_000,
    usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
    output: {
      summary:
        "Chatto **deliberately** keeps servers independent, according to the documentation.",
      details: "Supporting details.",
    },
    result: {
      summary:
        "Chatto **deliberately** keeps servers independent, according to the documentation.",
      details: "Supporting details.",
    },
  });

  const output = stripVTControlCharacters(dashboard.render(40).join("\n"));
  expect(output).toContain(
    "\nChatto deliberately keeps servers\nindependent, according to the\ndocumentation.\n",
  );
  expect(output).toContain("deliberately");
  expect(output).toContain("documentation.");
  expect(output).not.toContain("**");
});

test("renders a Markdown result supplied as the workflow summary", () => {
  const dashboard = new RunlingDashboard("examples/joke.ts", 0);

  dashboard.finish({
    ok: true,
    error: null,
    durationMs: 1_000,
    usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
    output: "# Joke\n\nA runling walks into a **bar**.",
    result: {
      summary: "# Joke\n\nA runling walks into a **bar**.",
    },
  });

  const output = stripVTControlCharacters(dashboard.render(80).join("\n"));
  expect(output).toContain("Joke");
  expect(output).toContain("A runling walks into a bar.");
  expect(output).not.toContain("# Joke");
  expect(output).not.toContain("**");
});

test("shows a bounded, ANSI-free tail for failed commands", () => {
  const dashboard = new RunlingDashboard("workflow.ts", performance.now());
  dashboard.handle(
    event({ type: "step.started", id: "test", label: "Run tests" }),
  );
  dashboard.handle(
    event(
      { type: "command.started", id: "bun-test", command: "pnpm test" },
      "test",
    ),
  );
  dashboard.handle(
    event(
      {
        type: "command.finished",
        id: "bun-test",
        status: "failed",
        durationMs: 1_800,
        output: {
          stdout: "Preparing tests\n",
          stderr: `${Array.from({ length: 9 }, (_, index) => `failure ${index + 1}`).join("\n")}\n\x1b[31mfinal failure\x1b[0m\n`,
        },
      },
      "test",
    ),
  );

  const output = stripVTControlCharacters(dashboard.render(80).join("\n"));
  expect(output).toContain("✗ Ran pnpm test · 1.8s");
  expect(output).toContain("… 3 earlier lines omitted");
  expect(output).not.toContain("Preparing tests");
  expect(output).not.toContain("failure 2");
  expect(output).toContain("failure 3");
  expect(output).toContain("final failure");
  expect(dashboard.render(80).join("\n")).not.toContain("\x1b[31m");
});

test("never renders beyond the available width", () => {
  const dashboard = new RunlingDashboard("a-very-long-workflow-name.ts", 0);
  dashboard.handle(
    event({
      type: "log",
      level: "info",
      message: "A very long message that needs to be truncated safely",
      depth: 3,
      color: "dodgerblue",
    }),
  );

  expect(
    dashboard.render(20).every((line) => visibleWidth(line) <= 20),
  ).toBe(true);
});

test("drives pi-tui through the main-screen reporter lifecycle", () => {
  const terminal = new FakeTerminal();
  const reporter = new TuiReporter("workflow.ts", terminal);

  reporter.start();
  reporter.handle(
    event({
      type: "log",
      level: "info",
      message: "Doing useful work",
      depth: 0,
      color: "dodgerblue",
    }),
  );
  reporter.finish({
    ok: true,
    error: null,
    durationMs: 100,
    usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
    output: "Done",
    result: { summary: "Done" },
  });
  reporter.stop();

  expect(terminal.started).toBe(true);
  expect(terminal.stopped).toBe(true);
  expect(terminal.output).not.toContain("\x1b[?1049h");
  expect(terminal.output).not.toContain("\x1b[?1049l");
  expect(terminal.output).not.toContain("\x1b[2J");
  expect(terminal.output).not.toContain("\x1b[3J");
  expect(stripVTControlCharacters(terminal.output)).toContain("Doing useful work");
  expect(stripVTControlCharacters(terminal.output)).toContain("Done");
});

test("collects workflow input inline in the execution tree", async () => {
  const terminal = new FakeTerminal();
  const reporter = new TuiReporter("workflow.ts", terminal);
  const input = createInput(reporter.input);

  reporter.start();
  const answer = observeRunlingEvents(reporter.handle, () =>
    input("What should we call this release?", { defaultValue: "Runling " }),
  );
  await sleep(10);

  expect(stripVTControlCharacters(terminal.output)).toContain(
    "What should we call this release?",
  );
  expect(stripVTControlCharacters(terminal.output)).toContain("> Runling ");
  expect(stripVTControlCharacters(terminal.output)).not.toContain("Interview");
  expect(terminal.output).not.toContain("\x1b[2J");
  expect(terminal.output).not.toContain("\x1b[3J");

  for (const character of "Manbot") terminal.send(character);
  terminal.send("\r");

  await expect(answer).resolves.toBe("Runling Manbot");
  await sleep(10);
  expect(stripVTControlCharacters(terminal.output)).toContain(
    "✓ What should we call this release? → Runling Manbot",
  );
  expect(terminal.output).not.toContain("\x1b[2J");
  expect(terminal.output).not.toContain("\x1b[3J");
  reporter.stop();
});

test("restores the terminal before handling Ctrl-C", () => {
  const terminal = new FakeTerminal();
  let terminalWasStopped = false;
  const reporter = new TuiReporter("workflow.ts", terminal, () => {
    terminalWasStopped = terminal.stopped;
  });

  reporter.start();
  terminal.send("\x03");

  expect(terminalWasStopped).toBe(true);
});

class FakeTerminal implements Terminal {
  columns = 80;
  rows = 24;
  kittyProtocolActive = false;
  started = false;
  stopped = false;
  output = "";
  private onInput?: (data: string) => void;

  start(onInput: (data: string) => void): void {
    this.started = true;
    this.onInput = onInput;
  }

  stop(): void {
    this.stopped = true;
  }

  async drainInput(): Promise<void> {}

  write(data: string): void {
    this.output += data;
  }

  moveBy(): void {}
  hideCursor(): void {}
  showCursor(): void {}
  clearLine(): void {}
  clearFromCursor(): void {}
  clearScreen(): void {}
  setTitle(): void {}
  setProgress(): void {}

  send(data: string): void {
    this.onInput?.(data);
  }
}

test("renders timeout and cancellation reasons for questions", () => {
  const dashboard = new RunlingDashboard("Questions", 0);
  for (const reason of ["timeout", "cancelled"] as const) {
    dashboard.handle(event({ type: "input.requested", id: reason, message: reason }));
    dashboard.handle(event({ type: "input.finished", id: reason, status: "failed", reason, durationMs: 1000 }));
  }
  const text = stripVTControlCharacters(dashboard.render(100).join("\n"));
  expect(text).toContain("timed out");
  expect(text).toContain("cancelled");
});
