/** Observe checks on a verified PR, read failed GitHub Actions job logs, and rerun failed jobs. */
import { setTimeout as delay } from 'node:timers/promises';
import {
  ImplementationCommandError,
  type ImplementationProcess
} from './implementation-process.ts';
import { validationDiagnostic } from './implementation-safety.ts';

/** Check counts on the PR head. Only counts may reach chat. */
export interface PullRequestChecks {
  status: 'passed' | 'failed' | 'pending' | 'skipped' | 'head_changed' | 'unavailable';
  passed: number;
  failed: number;
  pending: number;
  skipped: number;
}

/** A failed or cancelled check. Names can reach chat; links and logs are worker context only. */
export interface FailedCheck {
  name: string;
  link: string;
}

/** Observed checks, with the failed checks for repair. */
export type ObservedChecks = PullRequestChecks & { failures: FailedCheck[] };

/** Identify one failed check run. A rerun gets a new link. */
export const failureKey = (check: FailedCheck) => check.link || check.name;

/** GitHub access shared by the functions in this module. */
export interface GitHubAccess {
  execute: ImplementationProcess;
  repository: string;
  cwd: string;
  signal: AbortSignal;
}

interface CheckOptions extends GitHubAccess {
  prUrl: string;
  /** The commit that the host pushed last. Another head stops observation. */
  headCommit: string;
  /** Return as soon as one check fails, before other checks finish. */
  stopOnFailure?: boolean;
  /** Failures, by `failureKey`, that do not end observation early. */
  knownFailures?: ReadonlySet<string>;
  /** Wait before the first poll, so that GitHub can register a rerun. */
  initialDelayMs?: number;
  /** Defaults to a 30-minute observation window. */
  timeoutMs?: number;
  intervalMs?: number;
  onPending?: (checks: PullRequestChecks) => Promise<void>;
}

const emptyChecks = (): ObservedChecks => ({
  status: 'pending',
  passed: 0,
  failed: 0,
  pending: 0,
  skipped: 0,
  failures: []
});

/** Parse `gh pr checks --json bucket,name,link`. Failed commands append stderr after the JSON. */
function parseChecks(output: string): ObservedChecks | undefined {
  if (/no checks reported/i.test(output)) return emptyChecks();
  let rows: unknown;
  for (let end = output.indexOf(']'); end >= 0 && rows === undefined;) {
    try {
      rows = JSON.parse(output.slice(0, end + 1));
    } catch {
      end = output.indexOf(']', end + 1);
    }
  }
  if (!Array.isArray(rows) || !rows.every((row) => row && typeof row.bucket === 'string')) return;
  const buckets = rows.map((row) => row.bucket as string);
  if (buckets.some((bucket) => !['pass', 'fail', 'pending', 'skipping', 'cancel'].includes(bucket)))
    return;
  const failures = rows
    .filter((row) => row.bucket === 'fail' || row.bucket === 'cancel')
    .map((row) => ({
      name: typeof row.name === 'string' ? row.name : 'Unnamed check',
      link: typeof row.link === 'string' ? row.link : ''
    }));
  const passed = buckets.filter((bucket) => bucket === 'pass').length;
  const pending = buckets.filter((bucket) => bucket === 'pending').length;
  const skipped = buckets.filter((bucket) => bucket === 'skipping').length;
  return {
    status:
      pending || !buckets.length
        ? 'pending'
        : failures.length
          ? 'failed'
          : passed
            ? 'passed'
            : 'skipped',
    passed,
    failed: failures.length,
    pending,
    skipped,
    failures
  };
}

/** Poll a verified PR until checks settle, a new check fails with `stopOnFailure`, or the window
 * ends. */
export async function observePullRequestChecks(options: CheckOptions): Promise<ObservedChecks> {
  const deadline = Date.now() + (options.timeoutMs ?? 30 * 60_000);
  const interval = options.intervalMs ?? 30_000;
  let last = emptyChecks();
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
  if (options.initialDelayMs)
    await delay(options.initialDelayMs, undefined, { signal: options.signal });
  while (true) {
    options.signal.throwIfAborted();
    try {
      if ((await readHead()) !== options.headCommit) return { ...last, status: 'head_changed' };
      let output: string;
      try {
        output = await options.execute(
          'gh',
          [
            'pr',
            'checks',
            options.prUrl,
            '--repo',
            options.repository,
            '--json',
            'bucket,name,link'
          ],
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
      const newFailure = checks.failures.some(
        (check) => !options.knownFailures?.has(failureKey(check))
      );
      if (checks.status !== 'pending' || (options.stopOnFailure && newFailure)) {
        if ((await readHead()) !== options.headCommit) return { ...checks, status: 'head_changed' };
        return checks.failed ? { ...checks, status: 'failed' } : checks;
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

/** Run and job IDs of a GitHub Actions check link in the configured repository. */
function actionsJob(repository: string, link: string) {
  const match = /^https:\/\/github\.com\/([^/]+\/[^/]+)\/actions\/runs\/(\d+)\/job\/(\d+)/.exec(
    link
  );
  return match && match[1]!.toLowerCase() === repository.toLowerCase()
    ? { run: match[2]!, job: match[3]! }
    : undefined;
}

/** Failure output of one GitHub Actions job: the log up to its last error annotation, without
 * timestamps, bounded and redacted. Returns a short note when no log is available. */
export async function failedJobLog(
  access: GitHubAccess,
  check: FailedCheck,
  worktree: string
): Promise<string> {
  const ids = actionsJob(access.repository, check.link);
  if (!ids) return 'No GitHub Actions log is available for this check.';
  let log: string;
  try {
    log = await access.execute(
      'gh',
      [
        'api',
        '--allow-escape-sequences',
        `repos/${access.repository}/actions/jobs/${ids.job}/logs`
      ],
      { cwd: access.cwd, signal: access.signal, timeoutMs: 60_000, keepTail: true }
    );
  } catch {
    access.signal.throwIfAborted();
    return 'The job log could not be read.';
  }
  const lines = log.split('\n').map((line) => line.replace(/^\d{4}-\d\d-\d\dT[\d:.]+Z /, ''));
  const lastError = lines.findLastIndex((line) => line.startsWith('##[error]'));
  // Runner cleanup follows the failure. Relative paths are useful; runner paths are not.
  const text = lines
    .slice(0, lastError < 0 ? undefined : lastError + 1)
    .join('\n')
    .replace(/\/home\/runner\/work\/[^/\s]+\/[^/\s]+\//g, '');
  return validationDiagnostic(text, worktree);
}

/** Rerun the failed jobs of the workflow runs behind these checks. Throws when a rerun fails. */
export async function rerunFailedJobs(access: GitHubAccess, failures: FailedCheck[]) {
  const runs = failedWorkflowRunIds(access.repository, failures);
  if (!runs.length) throw new Error('No GitHub Actions run to rerun');
  for (const run of runs)
    await access.execute('gh', ['run', 'rerun', run, '--failed', '--repo', access.repository], {
      cwd: access.cwd,
      signal: access.signal,
      timeoutMs: 30_000
    });
}

/** Exact workflow run IDs eligible for rerun in this repository. Unrelated links are ignored. */
export function failedWorkflowRunIds(repository: string, failures: FailedCheck[]): string[] {
  return [...new Set(failures.flatMap((check) => actionsJob(repository, check.link)?.run ?? []))];
}
