import { executionServices } from './execution.ts';
import { ansiColor, terminalColors } from './ansi.ts';
import { stripVTControlCharacters } from 'node:util';
import { AsyncLocalStorage } from 'node:async_hooks';
import { emitRunlingEvent } from './events.ts';
import { renderMarkdown } from './markdown.ts';
import { terminalLogLine, type TerminalLogLevel } from './terminal-log.ts';
import { formatLogFields, type LogFields } from './log-fields.ts';

function paint(color: string, text: string) {
  if (!terminalColors(destinationStorage.getStore() === 'stderr' ? process.stderr : process.stdout))
    return text;
  const ansi = ansiColor(color);
  return ansi ? `${ansi}${text}\x1b[0m` : text;
}

function highlight(text: string, color = 'white') {
  if (!terminalColors(destinationStorage.getStore() === 'stderr' ? process.stderr : process.stdout))
    return text;
  const ansi = ansiColor(color) ?? '';
  return `\x1b[1m${ansi}${text}\x1b[0m`;
}

export type LogLevel = 'info' | 'debug';

const INDENT_UNIT = '  ';

/**
 * Tracks how deeply nested the currently running work is. Using
 * `AsyncLocalStorage` keeps concurrent steps (e.g. two agents running at the
 * same time) from corrupting each other's indentation level.
 */
const depthStorage = new AsyncLocalStorage<number>();
const colorStorage = new AsyncLocalStorage<string>();
type LogDestination = 'stdout' | 'stderr' | 'silent';
interface LogSource {
  type: 'agent' | 'command';
  id: string;
}

const destinationStorage = new AsyncLocalStorage<LogDestination>();
const sourceStorage = new AsyncLocalStorage<LogSource>();
const fieldsStorage = new AsyncLocalStorage<LogFields>();

/** Start host-run logs at the root, without the dispatching task's indentation, color, or agent. */
export const withRootLog = <T>(work: () => T): T =>
  depthStorage.exit(() =>
    colorStorage.exit(() => sourceStorage.exit(() => fieldsStorage.exit(work)))
  );

function indent(): string {
  return INDENT_UNIT.repeat(depthStorage.getStore() ?? 0);
}

function write(
  level: TerminalLogLevel,
  message: string,
  defaultColor: string,
  source?: 'step',
  markdown?: { label: string; text: string },
  fields?: LogFields
): void {
  const color = colorStorage.getStore() ?? defaultColor;
  const contextualSource = sourceStorage.getStore();
  const metadata = { ...fieldsStorage.getStore(), ...fields };
  emitRunlingEvent({
    type: 'log',
    level,
    message,
    ...(Object.keys(metadata).length ? { fields: metadata } : {}),
    depth: depthStorage.getStore() ?? 0,
    color,
    source: source ?? contextualSource?.type,
    sourceId: contextualSource?.id
  });

  const destination = destinationStorage.getStore();
  if (destination === 'silent') return;

  const stderr = level === 'error' || level === 'warn' || destination === 'stderr';
  const stream = stderr ? process.stderr : process.stdout;
  // Render only for a terminal. Journals and pipes retain the original Markdown.
  // The label gets its own line so it does not consume the answer's first-line width.
  const rendered =
    markdown && stream.isTTY
      ? `${markdown.label ? `${markdown.label}\n\n` : ''}${renderMarkdown(
          stripVTControlCharacters(markdown.text),
          Math.max(1, (stream.columns || 80) - indent().length - 2)
        )}\n`
      : message;
  const line = terminalLogLine(level, rendered + formatLogFields(metadata), {
    depth: depthStorage.getStore() ?? 0,
    color,
    stderr,
    preserveStyles: true
  });
  if (stderr) {
    console.error(line);
  } else {
    console.log(line);
  }
}

/** Steps inside agent tools retain the agent's label in terminal output. */
export const logStep = (message: string): void => {
  const source = sourceStorage.getStore();
  const prefix = source?.type === 'agent' ? `${log.colorize(`[${source.id}]`)} ` : '';
  write('info', `${prefix}${message}`, 'dodgerblue', 'step');
};

/** Show a labeled Markdown block on terminals, retaining its source text in log events. */
export const logMarkdown = (label: string, text: string): void =>
  write('info', label ? `${label} ${text}` : text, 'dodgerblue', undefined, { label, text });

export const logCommand = (id: string, message: string): void =>
  withLogSource({ type: 'command', id }, () => log.info(message));

export const withLogSource = <T>(source: LogSource, work: LoggedWork<T>): T =>
  sourceStorage.run(source, work);

/** A chunk of work whose log output is indented one level deeper. */
export type LoggedWork<T> = () => T;

let defaultLevel: LogLevel = 'info';

export const log = {
  /** Debug is enabled globally by the environment or level setter, or for a verbose run. */
  get level(): LogLevel {
    return process.env.RUNLING_LOG_LEVEL === 'debug' || executionServices()?.verbose
      ? 'debug'
      : defaultLevel;
  },
  set level(value: LogLevel) {
    defaultLevel = value;
  },
  debug: (message: string, fields?: LogFields) => {
    if (log.level === 'debug') {
      write('debug', message, 'gray', undefined, undefined, fields);
    }
  },
  info: (message: string, fields?: LogFields) =>
    write('info', message, 'dodgerblue', undefined, undefined, fields),
  success: (message: string, fields?: LogFields) =>
    write('success', message, 'limegreen', undefined, undefined, fields),
  warn: (message: string, fields?: LogFields) =>
    write('warn', message, 'gold', undefined, undefined, fields),
  error: (message: string, fields?: LogFields) =>
    write('error', message, 'crimson', undefined, undefined, fields),
  /** Attach safe metadata to logs for this asynchronous operation. Inner fields take precedence. */
  withFields<T>(fields: LogFields, work: () => T): T {
    return fieldsStorage.run({ ...fieldsStorage.getStore(), ...fields }, work);
  },
  /** Renders `text` in the active contextual color, if there is one. */
  colorize(text: string): string {
    const color = colorStorage.getStore();
    return color === undefined ? text : paint(color, text);
  },
  /** Renders `text` in a bold, bright color. */
  highlight,
  /** Runs `work` with log markers rendered in `color`. */
  withColor<T>(color: string, work: LoggedWork<T>): T {
    return colorStorage.run(color, work);
  },
  /** Runs `work` with informational output written to `destination`. */
  withDestination<T>(destination: LogDestination, work: LoggedWork<T>): T {
    return destinationStorage.run(destination, work);
  },
  /**
   * Runs `work` with all of its log output indented one level deeper than the
   * surrounding log output. The indentation level is restored once `work`
   * finishes, even if it throws.
   */
  indented<T>(work: LoggedWork<T>): T {
    return depthStorage.run((depthStorage.getStore() ?? 0) + 1, work);
  }
};
