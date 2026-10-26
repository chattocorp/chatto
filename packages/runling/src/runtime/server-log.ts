/** Write Runling server diagnostics to the terminal and the rotating log file. The file holds
 * only safe summaries; the terminal can also show each run's detailed log. */
import { appendFileSync, mkdirSync, renameSync, statSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { stripVTControlCharacters } from 'node:util';
import { terminalLogLine, type TerminalLogLevel } from './terminal-log.ts';
import { formatLogFields, type LogFields } from './log-fields.ts';
import { log } from './log.ts';

type Level = TerminalLogLevel;

/** Keep terminal diagnostics on one line; remote text cannot inject terminal controls. */
function plain(value: unknown): string {
  return stripVTControlCharacters(String(value))
    .replace(/[\x00-\x1f\x7f-\x9f]+/g, ' ')
    .trim();
}

function terminalLine(level: Level, event: string, fields: Record<string, unknown>): string {
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
    case 'source.log':
      return terminalLogLine(
        level,
        String(fields.message) + formatLogFields(fields.fields as LogFields | undefined),
        {
          context: `source:${plain(fields.source)}`,
          timestamp: true,
          stderr: level === 'warn' || level === 'error'
        }
      );
    default:
      message = event.replace(/[._]/g, ' ');
  }
  const detail = fields.error instanceof Error ? fields.error.message : fields.message;
  if (typeof detail === 'string') message += ` · ${plain(detail)}`;
  const success =
    level === 'success' ||
    event === 'server.listening' ||
    (event === 'run.finished' && fields.status === 'completed') ||
    (event === 'run.activity' && fields.activityLevel === 'success');
  return terminalLogLine(success ? 'success' : level, plain(message), {
    context: lineContext(fields),
    timestamp: true,
    stderr: level === 'warn' || level === 'error'
  });
}

/** Time and `[run / task]` identifiers that start every run line. */
function lineContext(fields: Record<string, unknown>): string {
  const task = fields.agentLabel
    ? [fields.agentLabel, fields.taskReference].filter(Boolean).map(plain).join(':')
    : (fields.taskReference ?? fields.agentId);
  const identifiers = [
    fields.runReference || (fields.runId ? plain(fields.runId).slice(0, 8) : undefined),
    task
  ]
    .filter(Boolean)
    .map(plain);
  return identifiers.join(' / ');
}

/** Print one line of a run's detailed log to the terminal, never to the log file. Workflow text
 * can come from users or models, so control sequences are removed; line breaks remain, indented
 * under the first line. */
export function terminalRunLog(
  level: TerminalLogLevel,
  fields: Record<string, unknown>,
  message: string,
  metadata?: LogFields
): void {
  const stderr = level === 'error' || level === 'warn';
  (stderr ? console.error : console.log)(
    terminalLogLine(level, message + formatLogFields(metadata), {
      context: lineContext(fields),
      timestamp: true,
      stderr
    })
  );
}

export function serverLogPath(): string {
  return resolve(
    dirname(resolve(process.env.RUNLING_WEB_CONFIG ?? 'runling.config.ts')),
    process.env.RUNLING_DATA_DIRECTORY || '.runling',
    'logs/server.jsonl'
  );
}

/** Server diagnostics and safe activity summaries; workflow content stays in the run journal. */
export function serverLog(
  level: Level,
  event: string,
  fields: Record<string, unknown> = {},
  { terminal = true }: { terminal?: boolean } = {}
): void {
  // Retain routine HTTP diagnostics only when explicitly requested.
  if (level === 'debug' && log.level !== 'debug') return;
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
  if (terminal) console[level === 'success' ? 'info' : level](terminalLine(level, event, fields));
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
