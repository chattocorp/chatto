/** Host-written implementation messages for the user. The supervisor task posts them. The
 * implementation task only reports facts to its parent (Runling ADR-006): milestones as `state`
 * phases, and its result. */
import { MAX_CI_REPAIRS, type CiResult } from './implementation-task.ts';
import { workerChatText } from './implementation-safety.ts';

/** Most failed check names that one message lists. */
const MAX_LISTED_CHECKS = 5;

const checkList = (names: unknown) => {
  if (!Array.isArray(names) || !names.length) return '';
  const listed = names.slice(0, MAX_LISTED_CHECKS).map((name) => `\`${String(name)}\``);
  const more = names.length - listed.length;
  return `${listed.join(', ')}${more > 0 ? ` and ${more} more` : ''}`;
};

/** Milestones that are announced only the first time, although their phase can recur. */
export const ONCE_MILESTONES = new Set(['validating']);

/**
 * The message for an implementation milestone, or undefined when a state update is not one.
 * Each core stage of the work has one entry here; add new stages, such as planning or review,
 * as new phases. The caller posts a message only when the phase changes.
 */
export function milestoneMessage(value: { [key: string]: unknown }): string | undefined {
  const failed = checkList(value.failedChecks);
  switch (value.phase) {
    case 'validating':
      return 'The change is ready. I am running typecheck and lint before I open the pull request.';
    case 'published':
      return typeof value.prUrl === 'string'
        ? `Opened the pull request: ${value.prUrl}\nTypecheck and lint passed. CI is running now; I will fix failures and report the result.`
        : undefined;
    case 'ci_repairing':
      return typeof value.attempt === 'number'
        ? `CI failed${failed ? `: ${failed}` : ''}. I am working on a fix (attempt ${value.attempt} of ${MAX_CI_REPAIRS}).`
        : undefined;
    case 'ci_rerun_pending':
      return `The CI failure${failed ? ` in ${failed}` : ''} looks unrelated to the change, for example a flaky test or an outage. I will rerun the failed jobs when the current CI run finishes.`;
    case 'ci_rerunning':
      return `Rerunning the failed CI jobs${failed ? `: ${failed}` : ''}.`;
    case 'ci_fix_pushed':
      return 'Pushed a fix to the pull request. CI is running again.';
    default:
      return undefined;
  }
}

/** The fields of an implementation result that its final message uses. */
export interface ImplementationOutcome {
  outcome: 'completed' | 'blocked' | 'publication_unknown';
  summary: string;
  notes?: string[];
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

/** The CI result line of a published implementation. */
function ciLine(ci: CiResult, prUrl: string): string {
  const checks = `${ci.passed} check${ci.passed === 1 ? '' : 's'} passed`;
  const repairs = ci.repairs
    ? `, after ${ci.repairs} repair attempt${ci.repairs === 1 ? '' : 's'}`
    : '';
  switch (ci.status) {
    case 'passed':
      return `**CI passed** for the pull request (${checks}${repairs}): ${prUrl}`;
    case 'failed':
      return `**CI still fails** after ${ci.repairs} repair attempts: ${ci.failed} checks failed or were cancelled. Please review its Checks tab: ${prUrl}`;
    case 'unfixed':
      return `**CI failed**, and I could not publish a fix: ${ci.reason ?? 'the repair stopped.'} Please review its Checks tab: ${prUrl}`;
    case 'pending':
      return `**CI is still pending** after 30 minutes.${ci.failed ? ` ${ci.failed} checks have already failed or been cancelled.` : ''} Please review its Checks tab: ${prUrl}`;
    case 'skipped':
      return `**All reported CI checks were skipped.** Please review its Checks tab: ${prUrl}`;
    case 'head_changed':
      return `**Someone else pushed to the pull request**, so I stopped following its CI: ${prUrl}`;
    default:
      return `**I could not read CI** for the pull request. Please review its Checks tab: ${prUrl}`;
  }
}

/** The final message for a finished implementation: its stop reason, or its PR, CI result, and
 * summary of the change. Worker-written text is redacted and bounded. */
export function implementationResultMessage(result: ImplementationOutcome): string {
  const worktree = result.worktree ?? '';
  if (result.outcome !== 'completed' || !result.ci || !result.prUrl) {
    if (result.outcome === 'publication_unknown')
      return 'The implementation finished locally, but I could not verify publication. A branch or PR may exist; check GitHub before retrying.';
    const failedChecks = result.checks
      .filter((check) => !check.passed)
      .map((check) => check.command);
    const workerCheckCount = result.workerChecks.length;
    const failedWorkerChecks = result.workerChecks.filter((check) => !check.passed).length;
    return `The implementation stopped: ${workerChatText(result.summary, worktree, 800)}${workerCheckCount ? ` The worker ran ${workerCheckCount} check${workerCheckCount === 1 ? '' : 's'}; ${failedWorkerChecks} failed at the time. These were not final host checks.` : ''}${failedChecks.length ? ` Failed final check: ${failedChecks.join(', ')}. See the workflow result for details.` : ''}${result.worktree ? ` The worktree was kept for review${result.artifactId ? ` as ${result.artifactId}` : ''}.` : ''} Please tell me how you want to proceed.`;
  }
  const notes = (result.notes ?? [])
    .slice(0, 8)
    .map((note) => `- ${workerChatText(note, worktree, 300).replace(/\n+/g, ' ')}`);
  return [
    ciLine(result.ci, result.prUrl),
    '',
    '**What changed**',
    workerChatText(result.summary, worktree, 2_500),
    ...(notes.length ? ['', '**Notes**', ...notes] : [])
  ].join('\n');
}
