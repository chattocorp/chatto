/** Terminal or JSON logging with host-owned messages and operational metrics. */
import { errorHint } from './diagnostics.ts';

interface Metrics {
  /** Chatto thread root ID; a root message uses its own ID. */
  threadId?: string;
  /** Pi's persisted session identifier, stable when the transcript is reopened. */
  sessionId?: string;
  historyMessages?: number;
  /** Process-local counter, unrelated to any Chatto or user identity. */
  turn?: number;
  messages?: number;
  durationMs?: number;
  waitMs?: number;
  /** MCP tool identifier, never its arguments or returned content. */
  tool?: string;
}

/** Never pass external messages, credentials, URLs, or identities as the message. */
export function log(
  level: 'info' | 'warn' | 'error',
  message: string,
  metrics: Metrics = {}
): void {
  const timestamp = new Date().toISOString();
  const stream = level === 'info' ? process.stdout : process.stderr;
  if (!stream.isTTY) {
    console[level](JSON.stringify({ timestamp, level, message, ...metrics }));
    return;
  }
  const color =
    process.env.NO_COLOR === undefined &&
    process.env.TERM !== 'dumb' &&
    stream.hasColors?.() === true;
  const paint = (code: number, text: string) => (color ? `\u001b[${code}m${text}\u001b[0m` : text);
  const severity = { info: 36, warn: 33, error: 31 }[level];
  const fields = Object.entries(metrics)
    .map(([key, value]) => `${key}=${typeof value === 'string' ? JSON.stringify(value) : value}`)
    .join(' ');
  console[level](
    `${paint(2, timestamp)} ${paint(severity, level.toUpperCase())} ${message}${fields ? ` ${paint(2, fields)}` : ''}`
  );
}

/** Log one outcome with its duration and a safe hint if the operation fails. */
export async function logged<T>(
  stage: string,
  work: () => Promise<T>,
  metrics: Metrics = {}
): Promise<T> {
  const started = performance.now();
  try {
    const result = await work();
    log('info', `${stage} completed`, {
      ...metrics,
      durationMs: Math.round(performance.now() - started)
    });
    return result;
  } catch (error) {
    log('error', `${stage} failed.${errorHint(error)}`, {
      ...metrics,
      durationMs: Math.round(performance.now() - started)
    });
    throw error;
  }
}
