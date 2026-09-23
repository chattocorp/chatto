/** Observe checks on a verified PR after publication without changing its implementation result. */
import { setTimeout as delay } from 'node:timers/promises';
import {
  ImplementationCommandError,
  type ImplementationProcess
} from './implementation-process.ts';

export interface PullRequestChecks {
  status: 'passed' | 'failed' | 'pending' | 'skipped' | 'head_changed' | 'unavailable';
  passed: number;
  failed: number;
  pending: number;
  skipped: number;
}

interface CheckOptions {
  execute: ImplementationProcess;
  repository: string;
  prUrl: string;
  /** Commit verified immediately after PR publication. */
  headCommit: string;
  cwd: string;
  signal: AbortSignal;
  /** Defaults to a 30-minute observation window. */
  timeoutMs?: number;
  intervalMs?: number;
  onPending?: (checks: PullRequestChecks) => Promise<void>;
}

/** Read only the check buckets. Never forward check output or names to a chat message. */
function parseChecks(output: string): PullRequestChecks | undefined {
  const end = output.indexOf(']');
  if (end < 0) return;
  let rows: unknown;
  try {
    rows = JSON.parse(output.slice(0, end + 1));
  } catch {
    return;
  }
  if (!Array.isArray(rows) || !rows.every((row) => row && typeof row.bucket === 'string')) return;
  const buckets = rows.map((row) => row.bucket as string);
  if (buckets.some((bucket) => !['pass', 'fail', 'pending', 'skipping', 'cancel'].includes(bucket)))
    return;
  const passed = buckets.filter((bucket) => bucket === 'pass').length;
  const failed = buckets.filter((bucket) => bucket === 'fail' || bucket === 'cancel').length;
  const pending = buckets.filter((bucket) => bucket === 'pending').length;
  const skipped = buckets.filter((bucket) => bucket === 'skipping').length;
  return {
    status:
      pending || !buckets.length ? 'pending' : failed ? 'failed' : passed ? 'passed' : 'skipped',
    passed,
    failed,
    pending,
    skipped
  };
}

/** Poll a verified PR until checks settle or the observation window ends. */
export async function observePullRequestChecks(options: CheckOptions): Promise<PullRequestChecks> {
  const deadline = Date.now() + (options.timeoutMs ?? 30 * 60_000);
  const interval = options.intervalMs ?? 30_000;
  let last: PullRequestChecks = { status: 'pending', passed: 0, failed: 0, pending: 0, skipped: 0 };
  let errors = 0;
  const readHead = async () => {
    const head: unknown = JSON.parse(
      await options.execute(
        'gh',
        ['pr', 'view', options.prUrl, '--repo', options.repository, '--json', 'headRefOid'],
        { cwd: options.cwd, signal: options.signal, timeoutMs: 15_000 }
      )
    );
    if (
      typeof head !== 'object' ||
      head === null ||
      !('headRefOid' in head) ||
      typeof head.headRefOid !== 'string'
    )
      throw new Error('PR head was unavailable');
    return head.headRefOid;
  };
  while (true) {
    options.signal.throwIfAborted();
    try {
      if ((await readHead()) !== options.headCommit) return { ...last, status: 'head_changed' };
      let output: string;
      try {
        output = await options.execute(
          'gh',
          ['pr', 'checks', options.prUrl, '--repo', options.repository, '--json', 'bucket'],
          { cwd: options.cwd, signal: options.signal, timeoutMs: 15_000, captureDiagnostics: true }
        );
      } catch (error) {
        if (!(error instanceof ImplementationCommandError)) throw error;
        output = error.output;
      }
      const checks = parseChecks(output);
      if (!checks) throw new Error('Check response was unavailable');
      errors = 0;
      last = checks;
      if (checks.status !== 'pending') {
        if ((await readHead()) !== options.headCommit) return { ...checks, status: 'head_changed' };
        return checks;
      }
      await options.onPending?.(checks);
    } catch {
      options.signal.throwIfAborted();
      if (++errors >= 3) return { ...last, status: 'unavailable' };
    }
    if (Date.now() >= deadline) return last;
    await delay(Math.min(interval, Math.max(0, deadline - Date.now())), undefined, {
      signal: options.signal
    });
  }
}
