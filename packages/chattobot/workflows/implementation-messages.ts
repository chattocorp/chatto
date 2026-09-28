/** Host-written implementation messages for the user. The supervisor task posts them. The
 * implementation task only reports facts to its parent (Runling ADR-006). */
import { MAX_CI_REPAIRS, type CiResult } from './implementation-task.ts';

/** Posted when the implementation reports its verified pull request. */
export const publishedMessage = (prUrl: string) =>
  `Opened [the pull request](${prUrl}). Local typecheck and lint passed. I will fix CI failures and report when CI finishes.`;

/** Posted before the worker handles CI failure number `attempt`. */
export const ciRepairMessage = (attempt: number) =>
  `CI failed for the pull request. I am working on it (attempt ${attempt} of ${MAX_CI_REPAIRS}).`;

/** The fields of an implementation result that its final message uses. */
export interface ImplementationOutcome {
  outcome: 'completed' | 'blocked' | 'publication_unknown';
  summary: string;
  prUrl?: string;
  worktree?: string;
  artifactId?: string;
  checks: { command: string; passed: boolean }[];
  workerChecks: { passed: boolean }[];
  ci?: CiResult;
}

/** True for a value with the fields that `implementationResultMessage` reads. */
export function isImplementationOutcome(value: unknown): value is ImplementationOutcome {
  if (typeof value !== 'object' || value === null) return false;
  const result = value as Record<string, unknown>;
  return (
    ['completed', 'blocked', 'publication_unknown'].includes(result.outcome as string) &&
    typeof result.summary === 'string' &&
    Array.isArray(result.checks) &&
    Array.isArray(result.workerChecks)
  );
}

/** The final message for a finished implementation: its stop reason, or its CI result. */
export function implementationResultMessage(result: ImplementationOutcome): string {
  if (result.outcome !== 'completed' || !result.ci || !result.prUrl) {
    if (result.outcome === 'publication_unknown')
      return 'The implementation finished locally, but I could not verify publication. A branch or PR may exist; check GitHub before retrying.';
    const failedChecks = result.checks
      .filter((check) => !check.passed)
      .map((check) => check.command);
    const workerCheckCount = result.workerChecks.length;
    const failedWorkerChecks = result.workerChecks.filter((check) => !check.passed).length;
    return `The implementation stopped: ${result.summary}${workerCheckCount ? ` The worker ran ${workerCheckCount} check${workerCheckCount === 1 ? '' : 's'}; ${failedWorkerChecks} failed at the time. These were not final host checks.` : ''}${failedChecks.length ? ` Failed final check: ${failedChecks.join(', ')}. See the workflow result for details.` : ''}${result.worktree ? ` The worktree was kept for review${result.artifactId ? ` as ${result.artifactId}` : ''}.` : ''} Please tell me how you want to proceed.`;
  }
  const { ci } = result;
  const pr = `[the pull request](${result.prUrl})`;
  const fixes = ci.repairs
    ? ` after ${ci.repairs} repair attempt${ci.repairs === 1 ? '' : 's'}`
    : '';
  return ci.status === 'passed'
    ? `CI passed for ${pr}${fixes}: ${ci.passed} checks.`
    : ci.status === 'failed'
      ? `CI still fails for ${pr} after ${ci.repairs} repair attempts: ${ci.failed} checks failed or were cancelled. Please review its Checks tab.`
      : ci.status === 'unfixed'
        ? `CI failed for ${pr}, and I could not publish a fix: ${ci.reason ?? 'the repair stopped.'} Please review its Checks tab.`
        : ci.status === 'pending'
          ? `CI is still pending for ${pr} after 30 minutes.${ci.failed ? ` ${ci.failed} checks have already failed or been cancelled.` : ''} Please review its Checks tab.`
          : ci.status === 'skipped'
            ? `All reported CI checks were skipped for ${pr}. Please review its Checks tab.`
            : ci.status === 'head_changed'
              ? `Someone else pushed to ${pr}, so I stopped following its CI. Please review its Checks tab.`
              : `I could not read CI for ${pr}. Please review its Checks tab.`;
}
