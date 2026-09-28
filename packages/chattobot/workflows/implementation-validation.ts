/** Host-owned validation of an implementation worktree, with comparison against the base commit. */
import type { WorkflowContext } from 'runling';
import type { AgentTaskUpdate } from 'runling/agents';
import {
  ImplementationCommandError,
  type ImplementationProcess
} from './implementation-process.ts';
import { validationDiagnostic } from './implementation-safety.ts';

/** Run Git with hooks disabled in one directory. */
export type Git = (cwd: string, args: string[], signal?: AbortSignal) => Promise<string>;

/** One final host check, recorded with the source tree it ran on. */
export interface Check {
  command: string;
  passed: boolean;
  tree?: string;
  diagnostic?: string;
}

/** Everything validation needs from the running implementation. */
export interface ValidationContext {
  ctx: WorkflowContext<string, AgentTaskUpdate>;
  signal: AbortSignal;
  execute: ImplementationProcess;
  git: Git;
  /** The operator's checkout, used to add the baseline worktree. */
  directory: string;
  worktree: string;
  /** Detached worktree at the base commit, created only when a check fails. */
  baselineWorktree: string;
  baseCommit: string;
  /** Environment variables removed from repository commands. */
  unsetEnv: string[];
}

/** Create validation state for one implementation. `checks` holds the latest final host checks. */
export function createValidation({
  ctx,
  signal,
  execute,
  git,
  directory,
  worktree,
  baselineWorktree,
  baseCommit,
  unsetEnv
}: ValidationContext) {
  const checks = new Map<string, Check>();
  const baselineChecks = new Map<string, 'passed' | 'failed' | 'unknown'>();
  let baselineReady = false;
  let baselineUnavailable = false;
  const stageTree = async () => {
    await git(worktree, ['add', '-A']);
    return (await git(worktree, ['write-tree'])).trim();
  };
  /** Compare a failed worker or final check with the pristine base commit. */
  const checkBaseline = async (command: string, args: string[], comparisonSignal = signal) => {
    const cached = baselineChecks.get(command);
    if (cached) return cached;
    if (!baselineReady && !baselineUnavailable) {
      try {
        await git(
          directory,
          ['worktree', 'add', '--detach', baselineWorktree, baseCommit],
          comparisonSignal
        );
        await execute('mise', ['x', '--', 'pnpm', 'install', '--frozen-lockfile'], {
          cwd: baselineWorktree,
          signal: comparisonSignal,
          timeoutMs: 10 * 60_000,
          unsetEnv
        });
        if ((await git(baselineWorktree, ['status', '--porcelain'], comparisonSignal)).trim())
          throw new Error('Baseline setup changed source files');
        baselineReady = true;
      } catch {
        comparisonSignal.throwIfAborted();
        baselineUnavailable = true;
      }
    }
    let comparison: 'passed' | 'failed' | 'unknown' = 'unknown';
    if (baselineReady) {
      try {
        await execute('mise', args, {
          cwd: baselineWorktree,
          signal: comparisonSignal,
          timeoutMs: 10 * 60_000,
          captureDiagnostics: true,
          unsetEnv
        });
        comparison = 'passed';
      } catch (error) {
        comparisonSignal.throwIfAborted();
        if (error instanceof ImplementationCommandError) comparison = 'failed';
      }
    }
    baselineChecks.set(command, comparison);
    return comparison;
  };
  // Commands are host-owned. The worker receives failure output, but cannot
  // substitute an easier command or declare its own checks successful.
  const validate = async (paths: string[]) => {
    const frontendOnly = paths.every((path) => path.startsWith('apps/frontend/'));
    const commands = [
      ['x', '--', 'pnpm', 'run', frontendOnly ? 'check:frontend' : 'check'],
      ['x', '--', 'pnpm', 'run', frontendOnly ? 'test:frontend' : 'test'],
      ...(paths.some((path) => path.endsWith('.go') || /(^|\/)go\.(mod|sum)$/.test(path))
        ? [['run', 'test-cli']]
        : [])
    ];
    checks.clear();
    const completed: string[] = [];
    const pending = commands.map((args) => `mise ${args.join(' ')}`);
    const before = await stageTree();
    for (const args of commands) {
      const command = `mise ${args.join(' ')}`;
      await ctx.emit({
        type: 'state',
        value: {
          phase: 'validating',
          currentCheck: command,
          completedChecks: [...completed],
          pendingChecks: [...pending]
        },
        activity: `Validating · ${command}`
      });
      let output = '';
      let passed = false;
      try {
        output = await execute('mise', args, {
          cwd: worktree,
          signal,
          timeoutMs: 10 * 60_000,
          captureDiagnostics: true,
          unsetEnv
        });
        passed = true;
      } catch (error) {
        signal.throwIfAborted();
        if (error instanceof ImplementationCommandError) output = error.output;
      }
      // Retain a bounded diagnostic in the private result, never the server log.
      const diagnostic = passed ? undefined : validationDiagnostic(output, worktree);
      checks.set(command, { command, passed, tree: before, diagnostic });
      await ctx.emit({
        type: 'finding',
        text: `Host validation ${passed ? 'passed' : 'failed'}: ${command}.`
      });
      if (!passed) {
        const baseline = await checkBaseline(command, args);
        await ctx.emit({
          type: 'state',
          value: {
            phase: 'validation_failed',
            failedCheck: command,
            baseline,
            completedChecks: [...completed],
            pendingChecks: [...pending]
          },
          activity: `Validation failed · ${command}`,
          activityLevel: 'error'
        });
        if (baseline === 'failed') {
          await ctx.emit({
            type: 'finding',
            text: `The same check also failed on the base commit: ${command}. Cause is not established.`
          });
          return {
            blocked: `The check ${command} also failed on the base commit. The cause is not established; review the check before continuing.`
          };
        }
        return `Validation failed: ${command}\nBase comparison: ${baseline}.\n${diagnostic}`;
      }
      completed.push(command);
      pending.shift();
      await ctx.emit({
        type: 'state',
        value: {
          phase: 'validating',
          completedChecks: [...completed],
          pendingChecks: [...pending]
        },
        activity: `Validation passed · ${command}`,
        activityLevel: 'success'
      });
    }
    if ((await stageTree()) !== before)
      return 'Validation changed source files. Review those changes; all checks must run again on the final tree.';
    return undefined;
  };
  return { checks, stageTree, checkBaseline, validate };
}
