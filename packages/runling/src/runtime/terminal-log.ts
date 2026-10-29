/** One terminal renderer for standalone workflows, sources, and server diagnostics. */
import { stripVTControlCharacters } from 'node:util';
import { ansiColor, terminalColors } from './ansi.ts';

export type TerminalLogLevel = 'debug' | 'info' | 'success' | 'warn' | 'error';

/** Format a log line. Hosts choose context and timestamps; formatting stays in Runling. */
export function terminalLogLine(
  level: TerminalLogLevel,
  message: string,
  {
    context = '',
    timestamp = false,
    depth = 0,
    color,
    stderr = false,
    preserveStyles = false
  }: {
    context?: string;
    timestamp?: boolean;
    depth?: number;
    color?: string;
    stderr?: boolean;
    preserveStyles?: boolean;
  } = {}
): string {
  const colored = terminalColors(stderr ? process.stderr : process.stdout);
  const paint = (value: string, shade: string) =>
    colored ? `${ansiColor(shade)}${value}\x1b[0m` : value;
  const clean = (value: string) =>
    stripVTControlCharacters(value).replace(/[\x00-\x1f\x7f-\x9f]+/g, ' ');
  const symbol = { debug: '·', info: '●', success: '✓', warn: '!', error: '✗' }[level];
  const tone =
    color ??
    { debug: 'gray', info: 'dodgerblue', success: 'limegreen', warn: 'gold', error: 'crimson' }[
      level
    ];
  const prefix = `${'  '.repeat(depth)}${timestamp ? `${paint(new Date().toTimeString().slice(0, 8), 'gray')} ` : ''}${context ? `${paint(`[${clean(context)}]`, 'magenta')} ` : ''}${paint(symbol, tone)} `;
  const body = message
    .split('\n')
    .map((line) =>
      preserveStyles && colored
        ? line
        : stripVTControlCharacters(line)
            .replace(/[\x00-\x08\x0b-\x1f\x7f-\x9f]+/g, '')
            .trimEnd()
    )
    .join(`\n${' '.repeat(stripVTControlCharacters(prefix).length)}`);
  return `${prefix}${body}`;
}
