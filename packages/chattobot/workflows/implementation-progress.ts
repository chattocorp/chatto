/** Progress reports from an implementation to its parent task (Runling ADR-006). The parent
 * decides what reaches the user; the host writes facts, never user-facing messages. */
import type { WorkflowContext } from 'runling';
import type { AgentTaskData, AgentTaskUpdate } from 'runling/agents';
import { workerStopReason } from './implementation-safety.ts';
import type { WorkerCheck } from './implementation-tools.ts';
import type { Git } from './implementation-validation.ts';

/** Progress update timing while the worker works. At most one update is posted per
 * `minIntervalMs`; an earlier worker update waits, and a newer one replaces it. After `quietMs` without any update, the host posts one; it checks every
 * `checkMs`. */
export const PROGRESS_TIMING = { minIntervalMs: 60_000, quietMs: 8 * 60_000, checkMs: 30_000 };

/** What progress reporting needs from the implementation. */
export interface ProgressContext {
  ctx: WorkflowContext<string, AgentTaskUpdate>;
  signal: AbortSignal;
  git: Git;
  worktree: string;
  /** Worker check runs so far, counted in quiet updates. */
  workerChecks: readonly WorkerCheck[];
  timing?: Partial<typeof PROGRESS_TIMING>;
}

/** Create the progress reporter of one implementation. */
export function createProgress({
  ctx,
  signal,
  git,
  worktree,
  workerChecks,
  timing
}: ProgressContext) {
  const { minIntervalMs, quietMs } = { ...PROGRESS_TIMING, ...timing };
  // The supervisor announced the task, so the first update can wait a full interval.
  let lastUpdateAt = Date.now();
  let heldProgress: string | undefined;
  let heldTimer: ReturnType<typeof setTimeout> | undefined;
  let quietUpdateRunning = false;

  /** Send a progress notice to the parent task, which decides what reaches the user. */
  const postProgress = async (message: string) => {
    lastUpdateAt = Date.now();
    await ctx.emit({ type: 'notice', text: message });
  };
  /** Drop a held update, for example when the PR link replaces it. */
  const dropHeldProgress = () => {
    clearTimeout(heldTimer);
    heldTimer = undefined;
    heldProgress = undefined;
  };

  return {
    /** Report a stage of the work to the parent as a notice with its facts in `data`. The
     * parent tells the user in its own words. `description` is for the parent's model, not
     * for the user. */
    async reportMilestone(
      milestone: string,
      description: string,
      facts: { [key: string]: AgentTaskData } = {}
    ) {
      lastUpdateAt = Date.now();
      await ctx.emit({ type: 'notice', text: description, data: { milestone, ...facts } });
    },
    dropHeldProgress,
    /** Send a worker update now, or hold it until the interval ends. Returns the tool result. */
    async reportProgress(message: string) {
      const text = workerStopReason(message, worktree);
      const wait = minIntervalMs - (Date.now() - lastUpdateAt);
      if (wait <= 0) {
        dropHeldProgress();
        await postProgress(text);
        return 'Sent.';
      }
      heldProgress = text;
      heldTimer ??= setTimeout(() => {
        const held = heldProgress;
        heldTimer = undefined;
        heldProgress = undefined;
        if (held && !signal.aborted) void postProgress(held).catch(() => {});
      }, wait);
      return `Queued: the host posts it in about ${Math.ceil(wait / 1000)} seconds. A newer update replaces it until then.`;
    },
    /** After a quiet period, post facts that the host knows about the worker's progress. */
    async quietUpdate() {
      if (quietUpdateRunning || Date.now() - lastUpdateAt < quietMs) return;
      quietUpdateRunning = true;
      try {
        const changed = (
          await git(worktree, ['--no-optional-locks', 'status', '--porcelain', '-uall'])
        )
          .split('\n')
          .filter(Boolean).length;
        const runs = workerChecks.length;
        await postProgress(
          changed
            ? `Still working on the change: ${changed} changed file${changed === 1 ? '' : 's'} so far${runs ? `, ${runs} test and check run${runs === 1 ? '' : 's'}` : ''}.`
            : 'Still reading the code. No files have changed yet.'
        );
      } catch {
        // An update is best effort; the next check tries again.
      } finally {
        quietUpdateRunning = false;
      }
    }
  };
}
export type Progress = ReturnType<typeof createProgress>;
