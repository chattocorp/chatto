/** Write Runling server diagnostics to the terminal and the rotating log file. The file holds
 * only safe summaries; the terminal can also show each run's detailed log. */
import { appendFileSync, mkdirSync, renameSync, statSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { stripVTControlCharacters, styleText } from 'node:util';
import { terminalColors } from './ansi.ts';

type Level = 'info' | 'warn' | 'error';

/** Keep terminal diagnostics on one line; remote text cannot inject terminal controls. */
function plain(value: unknown): string {
  return stripVTControlCharacters(String(value))
    .replace(/[\x00-\x1f\x7f-\x9f]+/g, ' ')
    .trim();
}

function terminalLine(level: Level, event: string, fields: Record<string, unknown>): string {
  const stream = level === 'info' ? process.stdout : process.stderr;
  const color = terminalColors(stream);
  const paint = (style: Parameters<typeof styleText>[0], text: string) =>
    color ? styleText(style, text, { validateStream: false }) : text;
  let message: string;
  switch (event) {
    case 'server.starting':
      message = 'Starting Runling';
      break;
    case 'server.listening':
      message = `Runling ready → ${plain(fields.url)}`;
      break;
    case 'server.stopping':
      message = 'Stopping Runling';
      break;
    case 'server.stopped':
      message = 'Runling stopped';
      break;
    case 'run.interrupted':
      message = 'Previous run interrupted';
      break;
    case 'run.started':
      message = `Started ${plain(fields.workflow)}`;
      break;
    case 'run.finished':
      message = `Run ${plain(fields.status)} · ${Math.round(Number(fields.durationMs))} ms`;
      break;
    case 'run.activity':
      message = plain(fields.activity);
      break;
    case 'http.response':
      message = `${plain(fields.method)} ${plain(fields.route ?? '/')} · ${plain(fields.status)} · ${Math.round(Number(fields.durationMs))} ms`;
      break;
    case 'config.reload_failed':
      message = 'Config reload failed';
      break;
    case 'source.failed':
      message = `Source ${plain(fields.source)} failed`;
      break;
    default:
      message = event.replace(/[._]/g, ' ');
  }
  const prefix = linePrefix(fields, paint);
  const detail = fields.error instanceof Error ? fields.error.message : fields.message;
  if (typeof detail === 'string') message += ` · ${plain(detail)}`;
  const success =
    event === 'server.listening' ||
    (event === 'run.finished' && fields.status === 'completed') ||
    (event === 'run.activity' && fields.activityLevel === 'success');
  const symbol = level === 'error' ? '✗' : level === 'warn' ? '!' : success ? '✓' : '●';
  const tone = level === 'error' ? 'red' : level === 'warn' ? 'yellow' : success ? 'green' : 'cyan';
  return `${paint('dim', new Date().toTimeString().slice(0, 8))} ${prefix}${paint(tone, symbol)} ${paint(success ? 'bold' : tone, plain(message))}`;
}

type Paint = (style: Parameters<typeof styleText>[0], text: string) => string;

/** Time and `[run / task]` identifiers that start every run line. */
function linePrefix(fields: Record<string, unknown>, paint: Paint): string {
  const task = fields.agentLabel
    ? [fields.agentLabel, fields.taskReference].filter(Boolean).map(plain).join(':')
    : (fields.taskReference ?? fields.agentId);
  const identifiers = [
    fields.runReference || (fields.runId ? plain(fields.runId).slice(0, 8) : undefined),
    task
  ]
    .filter(Boolean)
    .map(plain);
  return identifiers.length ? `${paint('magenta', `[${identifiers.join(' / ')}]`)} ` : '';
}

/** Print one line of a run's detailed log to the terminal, never to the log file. Workflow text
 * can come from users or models, so control sequences are removed; line breaks remain, indented
 * under the first line. */
export function terminalRunLog(
  level: 'debug' | 'info' | 'success' | 'error',
  fields: Record<string, unknown>,
  message: string
): void {
  const stream = level === 'error' ? process.stderr : process.stdout;
  const color = terminalColors(stream);
  const paint: Paint = (style, text) =>
    color ? styleText(style, text, { validateStream: false }) : text;
  const symbol =
    level === 'error' ? '✗' : level === 'success' ? '✓' : level === 'debug' ? '·' : '●';
  const tone =
    level === 'error' ? 'red' : level === 'success' ? 'green' : level === 'debug' ? 'dim' : 'cyan';
  const start = `${paint('dim', new Date().toTimeString().slice(0, 8))} ${linePrefix(fields, paint)}${paint(tone, symbol)} `;
  const body = message
    .split('\n')
    .map((line) =>
      stripVTControlCharacters(line)
        .replace(/[\x00-\x08\x0b-\x1f\x7f-\x9f]+/g, ' ')
        .trimEnd()
    )
    .join(`\n${' '.repeat(stripVTControlCharacters(start).length)}`);
  (level === 'error' ? console.error : console.log)(`${start}${body}`);
}

export function serverLogPath(): string {
  return resolve(
    dirname(resolve(process.env.RUNLING_WEB_CONFIG ?? 'runling.config.ts')),
    '.runling/logs/server.jsonl'
  );
}

/** Server diagnostics and safe activity summaries; workflow content stays in the run journal. */
export function serverLog(
  level: Level,
  event: string,
  fields: Record<string, unknown> = {},
  { terminal = true }: { terminal?: boolean } = {}
): void {
  let line: string;
  try {
    line = JSON.stringify(
      { ...fields, time: new Date().toISOString(), level, event },
      (_key, value) =>
        value instanceof Error
          ? { name: value.name, message: value.message, stack: value.stack }
          : value
    );
  } catch {
    line = JSON.stringify({
      time: new Date().toISOString(),
      level,
      event,
      message: 'Log details could not be serialized'
    });
  }
  if (terminal) console[level](terminalLine(level, event, fields));
  try {
    const path = serverLogPath();
    mkdirSync(dirname(path), { recursive: true, mode: 0o700 });
    // Keep one previous file, so a long-running server cannot fill the disk.
    try {
      if (statSync(path).size >= 10 * 1024 * 1024) renameSync(path, `${path}.1`);
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error;
    }
    appendFileSync(path, `${line}\n`, { mode: 0o600 });
  } catch {
    // Diagnostics must not stop a request or workflow when the disk is unavailable.
    console.error(
      terminalLine('error', 'server.log_failed', { message: 'Cannot write Runling server log' })
    );
  }
}
