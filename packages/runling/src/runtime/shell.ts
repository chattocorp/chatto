import { requireDirectory } from "./directory.ts";
import { executionServices } from "./execution.ts";
import { emitRunlingEvent } from "./events.ts";
import { log, logCommand } from "./log.ts";
import {
  execa,
  ExecaError,
  type Options,
  type TemplateExpression,
} from "execa";

const COMMAND_COLOR = "#ae3ec9";
const COMMAND_PREVIEW_LENGTH = 200;

export interface ShellOutput {
  stdout: Buffer;
  stderr: Buffer;
  exitCode: number;
}
export { ExecaError as ShellError, ExecaError as CommandError };
type ProcessResult = { stdout?: unknown; stderr?: unknown; exitCode?: number };

type ShellExpression =
  | string
  | number
  | boolean
  | null
  | undefined
  | { raw: string }
  | readonly ShellExpression[];
type ShellArguments = [TemplateStringsArray, ...ShellExpression[]];

/** A configurable, single-execution Execa command with buffered output. */
export class Command implements PromiseLike<ShellOutput> {
  private execution?: Promise<ShellOutput>;
  private directory?: string;
  private silent = true;
  private throwOnError = true;
  constructor(
    private readonly launch: (options: Options) => PromiseLike<ProcessResult>,
  ) {}
  cwd(path: string) {
    this.directory = requireDirectory(path);
    return this;
  }
  quiet(value = true) {
    this.silent = value;
    return this;
  }
  nothrow() {
    this.throwOnError = false;
    return this;
  }
  then<TResult1 = ShellOutput, TResult2 = never>(
    fulfilled?:
      ((value: ShellOutput) => TResult1 | PromiseLike<TResult1>) | null,
    rejected?: ((reason: unknown) => TResult2 | PromiseLike<TResult2>) | null,
  ): Promise<TResult1 | TResult2> {
    return this.run().then(fulfilled, rejected);
  }
  async text() {
    return (await this).stdout.toString();
  }
  async json() {
    return JSON.parse(await this.text());
  }
  private run(): Promise<ShellOutput> {
    return (this.execution ??= Promise.resolve().then(async () => {
      try {
        const output = await this.launch({
          cwd: requireDirectory(this.directory!),
          encoding: "buffer",
          stripFinalNewline: false,
          reject: this.throwOnError,
          stdin: "ignore",
          stdout: this.silent ? "pipe" : ["pipe", "inherit"],
          stderr: this.silent ? "pipe" : ["pipe", "inherit"],
          preferLocal: true,
        });
        return {
          ...output,
          stdout: toBuffer(output.stdout),
          stderr: toBuffer(output.stderr),
          exitCode: output.exitCode ?? 1,
        };
      } catch (error) {
        if (error instanceof ExecaError) {
          Object.assign(error, {
            stdout: toBuffer(error.stdout),
            stderr: toBuffer(error.stderr),
          });
        }
        throw error;
      }
    }));
  }
}

export interface CreateShellOptions {
  verbose?: boolean;
  cwd?: string;
}
export { Command as ShellCommand };

export function createShell(options: CreateShellOptions = {}) {
  return function (...args: ShellArguments) {
    const [strings, ...expressions] = args;
    const command = strings
      .map(
        (part, index) =>
          part +
          (index < expressions.length
            ? formatExpression(expressions[index])
            : ""),
      )
      .join("");
    return track(
      new Command((config) => execa("sh", ["-c", command], config)),
      formatCommand(...args),
      options,
    );
  };
}

/** Execute a program directly. Execa parses the template without a shell. */
export function createExec(options: CreateShellOptions = {}) {
  return function (
    strings: TemplateStringsArray,
    ...expressions: TemplateExpression[]
  ) {
    const preview = strings
      .map(
        (part, index) =>
          part +
          (index < expressions.length
            ? JSON.stringify(expressions[index])
            : ""),
      )
      .join("");
    return track(
      new Command((config) => execa(config)(strings, ...expressions)),
      truncateCommand(preview),
      options,
    );
  };
}
export type Exec = ReturnType<typeof createExec>;
export type CreateExecOptions = CreateShellOptions;

function track(
  command: Command,
  formattedCommand: string,
  options: CreateShellOptions,
) {
  const cwd = options.cwd === undefined ? undefined : requireDirectory(options.cwd);
  const id = crypto.randomUUID();
  const startedAt = performance.now();
  emitRunlingEvent({
    type: "command.started",
    id,
    command: formattedCommand,
  });
  logCommand(
    id,
    `${log.highlight("Running", COMMAND_COLOR)} ${formattedCommand}`,
  );
  command.quiet(!(options.verbose ?? executionServices()?.verbose ?? false));
  const configured = cwd === undefined ? command : command.cwd(cwd);

  queueMicrotask(() => {
    void configured.then(
      (output) =>
        emitRunlingEvent({
          type: "command.finished",
          id,
          status: output.exitCode === 0 ? "completed" : "failed",
          durationMs: performance.now() - startedAt,
          output: shellOutput(output),
        }),
      (error) =>
        emitRunlingEvent({
          type: "command.finished",
          id,
          status: "failed",
          durationMs: performance.now() - startedAt,
          output: shellOutput(error),
        }),
    );
  });

  return configured;
}

export type Shell = ReturnType<typeof createShell>;

const shellOutput = (value: unknown): { stdout: string; stderr: string } => {
  if (typeof value !== "object" || value === null) {
    return { stdout: "", stderr: "" };
  }

  const output = value as { stdout?: unknown; stderr?: unknown };
  return {
    stdout: bufferText(output.stdout),
    stderr: bufferText(output.stderr),
  };
};

const bufferText = (value: unknown): string =>
  value instanceof Uint8Array ? new TextDecoder().decode(value) : "";
const toBuffer = (value: unknown): Buffer =>
  value instanceof Uint8Array
    ? Buffer.from(value)
    : Buffer.from(typeof value === "string" ? value : "");

function formatCommand(...[strings, ...expressions]: ShellArguments) {
  const command = strings
    .map((part, index) => {
      const expression = expressions[index];
      return expression === undefined
        ? part
        : `${part}${formatExpression(expression)}`;
    })
    .join("")
    .trim()
    .replaceAll("\n", " ");

  return truncateCommand(command);
}

function truncateCommand(value: string) {
  const command = value.trim().replaceAll("\n", " ");
  if (command.length <= COMMAND_PREVIEW_LENGTH) return command;

  return `${command.slice(0, COMMAND_PREVIEW_LENGTH)}… (${(command.length - COMMAND_PREVIEW_LENGTH).toLocaleString()} characters omitted)`;
}

function formatExpression(expression: ShellExpression): string {
  if (Array.isArray(expression)) {
    return expression.map(formatExpression).join(" ");
  }
  if (
    typeof expression === "object" &&
    expression !== null &&
    "raw" in expression
  ) {
    return String(expression.raw);
  }

  const value = String(expression ?? "");
  if (/^[a-zA-Z0-9_./:@%+=,-]+$/.test(value)) return value;
  return `'${value.replaceAll("'", "'\\''")}'`;
}

export const exec = createExec();
export const shell = createShell();
