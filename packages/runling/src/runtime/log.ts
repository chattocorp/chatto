import { executionServices } from './execution.ts';
import { ansiColor, terminalColors } from './ansi.ts';
import { stripVTControlCharacters } from 'node:util';
import { AsyncLocalStorage } from 'node:async_hooks';
import { emitRunlingEvent } from './events.ts';
import { renderMarkdown } from './markdown.ts';

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

function indent(): string {
  return INDENT_UNIT.repeat(depthStorage.getStore() ?? 0);
}

function write(
  level: 'debug' | 'info' | 'success' | 'error',
  message: string,
  defaultColor: string,
  source?: 'step',
  markdown?: { label: string; text: string }
): void {
  const color = colorStorage.getStore() ?? defaultColor;
  const contextualSource = sourceStorage.getStore();
  emitRunlingEvent({
    type: 'log',
    level,
    message,
    depth: depthStorage.getStore() ?? 0,
    color,
    source: source ?? contextualSource?.type,
    sourceId: contextualSource?.id
  });

  const destination = destinationStorage.getStore();
  if (destination === 'silent') return;

  const symbol =
    level === 'success' ? '✓' : level === 'error' ? '✗' : level === 'debug' ? '·' : '●';
  const stderr = level === 'error' || destination === 'stderr';
  const stream = stderr ? process.stderr : process.stdout;
  const colored = terminalColors(stream);
  const ansi = ansiColor(color);
  const marker = colored && ansi ? `${ansi}${symbol}\x1b[0m` : symbol;
  // Render only for a terminal. Journals and pipes retain the original Markdown.
  // The label gets its own line so it does not consume the answer's first-line width.
  const rendered =
    markdown && stream.isTTY
      ? `${markdown.label ? `${markdown.label}\n\n` : ''}${renderMarkdown(
          stripVTControlCharacters(markdown.text),
          Math.max(1, (stream.columns || 80) - indent().length - 2)
        )}\n`
      : message;
  const body = colored ? rendered : stripVTControlCharacters(rendered);
  // Continuation lines align with the first line's text, under the marker.
  const line = `${indent()}${marker} ${body.replace(/\n/g, `\n${indent()}  `)}`;
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
  get level(): LogLevel {
    const services = executionServices();
    return services ? (services.verbose ? 'debug' : 'info') : defaultLevel;
  },
  set level(value: LogLevel) {
    defaultLevel = value;
  },
  debug: (message: string) => {
    if (log.level === 'debug') {
      write('debug', message, 'gray');
    }
  },
  info: (message: string) => write('info', message, 'dodgerblue'),
  success: (message: string) => write('success', message, 'limegreen'),
  error: (message: string) => write('error', message, 'crimson'),
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
