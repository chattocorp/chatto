import { ansiColor } from "./ansi.ts";
import { stripVTControlCharacters } from "node:util";
import {
  type Focusable,
  Input as TextInput,
  Key,
  ProcessTerminal,
  matchesKey,
  truncateToWidth,
  TuiMainScreen,
  type Component,
  type Terminal,
  visibleWidth,
  wrapTextWithAnsi,
} from "@earendil-works/pi-tui";
import type { RunlingEvent } from "./events.ts";
import type { InputHandler, InputRequest } from "./input.ts";
import { renderMarkdown } from "./markdown.ts";
import type { WorkflowExecution } from "./runner.ts";
import { totalTokens, type TokenUsage } from "./usage.ts";

const SPINNER = ["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"];

const paint = (color: string, text: string): string => {
  const ansi = ansiColor(color);
  return ansi ? `${ansi}${text}\x1b[39m` : text;
};

const bold = (text: string): string => `\x1b[1m${text}\x1b[22m`;
const dim = (text: string): string => `\x1b[2m${text}\x1b[22m`;

interface TreeNodeBase {
  id: string;
  parentId?: string;
}

interface StepNode extends TreeNodeBase {
  type: "step";
  label: string;
  status: "running" | "completed" | "failed";
  durationMs?: number;
}

interface CommandNode extends TreeNodeBase {
  type: "command";
  command: string;
  status: "running" | "completed" | "failed";
  durationMs?: number;
  output?: {
    stdout: string;
    stderr: string;
  };
}

interface AgentNode extends TreeNodeBase {
  type: "agent";
  agentId: string;
  model: string;
  color: string;
  action: string;
  outcome?: "completed" | "blocked" | "failed";
}

interface InputNode extends TreeNodeBase {
  type: "input";
  message: string;
  status: "waiting" | "answered" | "failed";
  reason?: "timeout" | "cancelled";
  value?: string;
  durationMs?: number;
}

interface LogNode extends TreeNodeBase {
  type: "log";
  event: Extract<RunlingEvent, { type: "log" }>;
}

type TreeNode = StepNode | CommandNode | AgentNode | InputNode | LogNode;

export class RunlingDashboard implements Component {
  private readonly nodes: TreeNode[] = [];
  private readonly nodesById = new Map<string, TreeNode>();
  private readonly agents = new Map<string, AgentNode>();
  private readonly inputEditors = new Map<string, Component>();
  private logSequence = 0;
  private usage: TokenUsage = {
    input: 0,
    output: 0,
    cacheRead: 0,
    cacheWrite: 0,
  };
  private execution?: WorkflowExecution;

  constructor(
    private readonly title: string,
    private readonly startedAt = performance.now(),
  ) {}

  handle = (event: RunlingEvent): void => {
    switch (event.type) {
      case "log":
        if (
          event.source !== "step" &&
          event.source !== "agent" &&
          event.source !== "command" &&
          event.source !== "input"
        ) {
          this.addNode({
            type: "log",
            id: `log:${this.logSequence++}`,
            parentId: event.activityId,
            event,
          });
        }
        break;
      case "step.started":
        this.addNode({
          type: "step",
          id: event.id,
          parentId: event.activityId,
          label: event.label,
          status: "running",
        });
        break;
      case "step.finished": {
        const step = this.nodesById.get(event.id);
        if (step?.type === "step") {
          step.status = event.status;
          step.durationMs = event.durationMs;
        }
        break;
      }
      case "command.started":
        this.addNode({
          type: "command",
          id: event.id,
          parentId: event.activityId,
          command: event.command,
          status: "running",
        });
        break;
      case "command.finished": {
        const command = this.nodesById.get(event.id);
        if (command?.type === "command") {
          command.status = event.status;
          command.durationMs = event.durationMs;
          command.output = event.output;
        }
        break;
      }
      case "input.requested":
        this.addNode({
          type: "input",
          id: event.id,
          parentId: event.activityId,
          message: event.message,
          status: "waiting",
        });
        break;
      case "input.finished": {
        const input = this.nodesById.get(event.id);
        if (input?.type === "input") {
          input.status = event.status;
          if (event.status === "failed") input.reason = event.reason;
          if (event.status === "answered") input.value = event.value;
          input.durationMs = event.durationMs;
        }
        break;
      }
      case "agent.started": {
        const existing = this.agents.get(event.agentId);
        if (existing !== undefined) {
          existing.parentId = event.activityId;
          existing.model = event.model;
          existing.color = event.color;
          existing.action = "Starting";
          existing.outcome = undefined;
          break;
        }

        const agent: AgentNode = {
          type: "agent",
          id: `agent:${event.agentId}`,
          parentId: event.activityId,
          agentId: event.agentId,
          model: event.model,
          color: event.color,
          action: "Starting",
        };
        this.agents.set(event.agentId, agent);
        this.addNode(agent);
        break;
      }
      case "agent.action": {
        const agent = this.agents.get(event.agentId);
        if (agent !== undefined) agent.action = event.action;
        break;
      }
      case "agent.finished": {
        const agent = this.agents.get(event.agentId);
        if (agent !== undefined) agent.outcome = event.outcome;
        break;
      }
      case "usage.updated":
        this.usage = event.usage;
        break;
    }
  };

  finish(execution: WorkflowExecution): void {
    this.execution = execution;
    this.usage = execution.usage;
  }

  invalidate(): void {}

  setInputEditor(id: string, editor: Component | undefined): void {
    if (editor === undefined) this.inputEditors.delete(id);
    else this.inputEditors.set(id, editor);
  }

  render(width: number): string[] {
    return [
      ...this.renderHeader(width),
      ...this.renderTranscript(width),
      ...this.renderFooter(width),
    ];
  }

  renderHeader(width: number): string[] {
    const columns = Math.max(1, Math.floor(width));
    const status = this.execution
      ? this.execution.ok
        ? paint("limegreen", "✓")
        : paint("crimson", "✗")
      : paint(
          "dodgerblue",
          this.spinner(),
        );
    return [
      fit(
        `${bold(paint("dodgerblue", "◆ Runling"))}  ${this.title}  ${status}`,
        columns,
      ),
    ];
  }

  renderTranscript(width: number): string[] {
    const columns = Math.max(1, Math.floor(width));
    const lines: string[] = [];

    for (const node of this.nodes) {
      if (!this.hasParent(node)) {
        lines.push(...this.renderNode(node, 1, columns));
      }
    }

    const result = this.execution?.result;
    if (result !== undefined && result !== null) {
      const markdown =
        result.details === undefined
          ? result.summary
          : `${result.summary}\n\n${result.details}`;
      lines.push("", ...renderMarkdown(markdown, columns).split("\n"));
    }

    return lines.map((line) => fit(line, columns));
  }

  renderFooter(width: number): string[] {
    const columns = Math.max(1, Math.floor(width));
    const elapsed = formatElapsed(
      this.execution?.durationMs ?? performance.now() - this.startedAt,
    );
    return [
      dim("─".repeat(columns)),
      fit(this.footer(elapsed), columns),
    ];
  }

  private footer(elapsed: string): string {
    const steps = this.nodes.filter(
      (node): node is StepNode => node.type === "step",
    );
    const completed = steps.filter(
      ({ status }) => status === "completed",
    ).length;
    const failed = steps.filter(
      ({ status }) => status === "failed",
    ).length;
    const waitingForInput = this.nodes.some(
      (node) => node.type === "input" && node.status === "waiting",
    );
    const status = this.execution
      ? this.execution.ok
        ? "Completed"
        : "Failed"
      : waitingForInput
        ? "Waiting for input…"
        : "Working…";
    const tokens = totalTokens(this.usage);
    const parts = [
      status,
      elapsed,
      `${tokens.toLocaleString("en-US")} tokens`,
    ];

    const cacheableInput = this.usage.input + this.usage.cacheRead;
    if (this.usage.cacheRead > 0 && cacheableInput > 0) {
      parts.push(
        `${Math.round((this.usage.cacheRead / cacheableInput) * 100)}% cached`,
      );
    }
    parts.push(formatCost(this.usage.cost ?? 0));

    if (steps.length > 0) {
      parts.push(
        failed > 0
          ? `${failed} failed, ${completed}/${steps.length} steps`
          : `${completed}/${steps.length} steps`,
      );
    }
    return dim(parts.join("  ·  "));
  }

  private addNode(node: TreeNode): void {
    this.nodes.push(node);
    this.nodesById.set(node.id, node);
  }

  private hasParent(node: TreeNode): boolean {
    return node.parentId !== undefined && this.nodesById.has(node.parentId);
  }

  private renderNode(node: TreeNode, depth: number, width: number): string[] {
    const indent = "  ".repeat(depth);
    let lines: string[];

    switch (node.type) {
      case "step": {
        const marker =
          node.status === "completed"
            ? paint("limegreen", "✓")
            : node.status === "failed"
              ? paint("crimson", "✗")
              : paint("dodgerblue", "●");
        const duration =
          node.durationMs === undefined
            ? ""
            : ` ${dim(`· ${formatStepDuration(node.durationMs)}`)}`;
        lines = [fit(`${indent}${marker} ${node.label}${duration}`, width)];
        break;
      }
      case "command": {
        const marker =
          node.status === "completed"
            ? paint("limegreen", "✓")
            : node.status === "failed"
              ? paint("crimson", "✗")
              : paint("#ae3ec9", "●");
        const verb = node.status === "running" ? "Running" : "Ran";
        const duration =
          node.durationMs === undefined
            ? ""
            : ` ${dim(`· ${formatStepDuration(node.durationMs)}`)}`;
        lines = this.wrapNode(
          `${indent}${marker} `,
          `${bold(paint("#ae3ec9", verb))} ${node.command}${duration}`,
          width,
        );
        if (node.status === "failed" && node.output !== undefined) {
          for (const outputLine of failureOutput(node.output)) {
            lines.push(
              ...this.wrapNode(
                `${indent}  ${paint("crimson", "│")} `,
                dim(outputLine),
                width,
              ),
            );
          }
        }
        break;
      }
      case "agent": {
        const marker =
          node.outcome === undefined
            ? paint(node.color, this.spinner())
            : node.outcome === "completed"
              ? paint("limegreen", "✓")
              : node.outcome === "blocked"
                ? paint("#f59f00", "!")
                : paint("crimson", "✗");
        lines = [
          fit(
            `${indent}${marker} ${paint(node.color, `[${node.agentId}]`)} ${dim(`· ${node.model}`)}`,
            width,
          ),
        ];
        if (node.outcome === undefined) {
          lines.push(
            ...this.wrapNode(
              `${indent}  ${paint(node.color, "↳")} `,
              node.action,
              width,
            ),
          );
        }
        break;
      }
      case "input": {
        const marker =
          node.status === "answered"
            ? paint("limegreen", "✓")
            : node.status === "failed"
              ? paint("crimson", "✗")
              : paint("#f59f00", "?");
        const duration =
          node.durationMs === undefined
            ? ""
            : ` ${dim(`· ${formatStepDuration(node.durationMs)}`)}`;
        lines = this.wrapNode(
          `${indent}${marker} `,
          `${node.message}${node.reason ? ` (${node.reason === "timeout" ? "timed out" : "cancelled"})` : ""}${node.value === undefined ? "" : ` ${dim("→")} ${node.value}`}${duration}`,
          width,
        );
        const editor = this.inputEditors.get(node.id);
        if (node.status === "waiting" && editor !== undefined) {
          const prefix = `${indent}  `;
          const continuation = " ".repeat(visibleWidth(prefix));
          const editorLines = editor.render(
            Math.max(1, width - visibleWidth(prefix)),
          );
          lines.push(
            ...editorLines.map((line, index) =>
              fit(`${index === 0 ? prefix : continuation}${line}`, width),
            ),
          );
        }
        break;
      }
      case "log": {
        const { event } = node;
        const marker =
          event.level === "success"
            ? "✓"
            : event.level === "error"
              ? "✗"
              : event.level === "debug"
                ? "·"
                : "●";
        lines = this.wrapNode(
          `${indent}${paint(event.color, marker)} `,
          event.message,
          width,
        );
        break;
      }
    }

    if (node.type !== "step" || node.status !== "completed") {
      for (const child of this.nodes) {
        if (child.parentId === node.id) {
          lines.push(...this.renderNode(child, depth + 1, width));
        }
      }
    }
    return lines;
  }

  private wrapNode(prefix: string, text: string, width: number): string[] {
    const continuation = " ".repeat(visibleWidth(prefix));
    const wrapped = wrapTextWithAnsi(
      text,
      Math.max(1, width - visibleWidth(prefix)),
    );
    return wrapped.map((line, index) =>
      fit(`${index === 0 ? prefix : continuation}${line}`, width),
    );
  }

  private spinner(): string {
    return SPINNER[
      Math.floor((performance.now() - this.startedAt) / 80) % SPINNER.length
    ]!;
  }
}

export class TuiReporter {
  private readonly dashboard: RunlingDashboard;
  private readonly tui: TuiMainScreen;
  private timer?: ReturnType<typeof setInterval>;
  private started = false;
  private stopped = false;
  private readonly pendingInputs = new Set<(error: Error) => void>();
  private readonly pendingEditors = new Set<InlineInput>();

  constructor(
    title: string,
    terminal: Terminal = new ProcessTerminal(),
    private readonly onCancel: () => void = () => {
      process.kill(process.pid, "SIGINT");
    },
  ) {
    this.dashboard = new RunlingDashboard(title);
    this.tui = new TuiMainScreen(terminal);
    this.tui.addChild(this.dashboard);
    this.tui.addInputListener((data) => {
      if (!matchesKey(data, Key.ctrl("c"))) return;

      this.stop();
      this.onCancel();
      return { consume: true };
    });
  }

  handle = (event: RunlingEvent): void => {
    this.dashboard.handle(event);
    if (event.type === "input.finished") this.tui.renderNow();
    else this.tui.requestRender();
  };

  input: InputHandler = (request) => {
    if (!this.started || this.stopped) {
      return Promise.reject(new Error("The TUI cannot request input while stopped"));
    }

    return new Promise<string>((resolve, reject) => {
      let editor: InlineInput;
      const settle = (result: { value: string } | { error: Error }) => {
        request.signal?.removeEventListener("abort", abort);
        this.pendingInputs.delete(cancel);
        this.pendingEditors.delete(editor);
        this.dashboard.setInputEditor(request.id, undefined);
        this.tui.setFocus([...this.pendingEditors].at(-1) ?? null);
        this.tui.requestRender();
        if ("value" in result) resolve(result.value);
        else reject(result.error);
      };
      const cancel = (error: Error) => settle({ error });
      const abort = () =>
        cancel(
          request.signal?.reason instanceof Error
            ? request.signal.reason
            : new Error("Input cancelled"),
        );
      editor = new InlineInput(
        request,
        (value) => settle({ value }),
        () => cancel(new Error(`Input cancelled: ${request.message}`)),
      );

      this.pendingInputs.add(cancel);
      this.pendingEditors.add(editor);
      request.signal?.addEventListener("abort", abort, { once: true });
      this.dashboard.setInputEditor(request.id, editor);
      this.tui.setFocus(editor);
      this.tui.requestRender();
    });
  };

  start(): void {
    this.started = true;
    this.tui.start();
    this.timer = setInterval(() => this.tui.requestRender(), 80);
    this.timer.unref();
  }

  finish(execution: WorkflowExecution): void {
    this.dashboard.finish(execution);
    this.tui.renderNow();
  }

  stop(): void {
    if (!this.started || this.stopped) return;
    this.stopped = true;
    for (const cancel of [...this.pendingInputs]) {
      cancel(new Error("The TUI stopped while waiting for input"));
    }
    if (this.timer !== undefined) clearInterval(this.timer);
    this.tui.stop();
  }
}

class InlineInput implements Component, Focusable {
  private readonly input = new TextInput();

  constructor(
    private readonly request: InputRequest,
    onSubmit: (value: string) => void,
    onCancel: () => void,
  ) {
    if (request.defaultValue !== undefined) {
      for (const character of request.defaultValue) {
        this.input.handleInput(character);
      }
    }
    this.input.onSubmit = onSubmit;
    this.input.onEscape = onCancel;
  }

  get focused(): boolean {
    return this.input.focused;
  }

  set focused(value: boolean) {
    this.input.focused = value;
  }

  handleInput(data: string): void {
    this.input.handleInput(data);
  }

  invalidate(): void {
    this.input.invalidate();
  }

  render(width: number): string[] {
    return this.input.render(Math.max(1, Math.floor(width)));
  }
}

const fit = (line: string, width: number): string =>
  truncateToWidth(line, width, "");

const formatElapsed = (durationMs: number): string => {
  const seconds = Math.max(0, Math.floor(durationMs / 1000));
  const minutes = Math.floor(seconds / 60);
  return minutes > 0
    ? `${minutes}m ${String(seconds % 60).padStart(2, "0")}s`
    : `${seconds}s`;
};

const formatStepDuration = (durationMs: number): string =>
  durationMs < 1000
    ? `${Math.round(durationMs)}ms`
    : `${(durationMs / 1000).toFixed(1)}s`;

const formatCost = (cost: number): string =>
  cost === 0
    ? "$0.00"
    : cost < 0.0001
      ? "<$0.0001"
      : `$${cost.toFixed(cost < 0.01 ? 4 : 2)}`;

const COMMAND_FAILURE_LINES = 8;

const failureOutput = ({
  stdout,
  stderr,
}: {
  stdout: string;
  stderr: string;
}): string[] => {
  const lines = stripVTControlCharacters(
    [stdout.trimEnd(), stderr.trimEnd()].filter(Boolean).join("\n"),
  )
    .split("\n")
    .map((line) => line.trimEnd());

  while (lines[0]?.trim() === "") lines.shift();
  while (lines.at(-1)?.trim() === "") lines.pop();
  if (lines.length <= COMMAND_FAILURE_LINES) return lines;

  const omitted = lines.length - COMMAND_FAILURE_LINES;
  return [
    `… ${omitted.toLocaleString("en-US")} earlier lines omitted`,
    ...lines.slice(-COMMAND_FAILURE_LINES),
  ];
};
