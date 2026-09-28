/** Host-owned validation of an implementation worktree before each push. CI runs the tests. */
import type { WorkflowContext } from 'runling';
import type { AgentTaskUpdate } from 'runling/agents';
import {
  ImplementationCommandError,
  type ImplementationProcess
} from './implementation-process.ts';
import { lstat } from 'node:fs/promises';
import { resolve } from 'node:path';
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
  worktree: string;
  /** Environment variables removed from repository commands. */
  unsetEnv: string[];
}

/** Create validation state for one implementation. `checks` holds the latest final host checks. */
export function createValidation({
  ctx,
  signal,
  execute,
  git,
  worktree,
  unsetEnv
}: ValidationContext) {
  const checks = new Map<string, Check>();
  const stageTree = async () => {
    await git(worktree, ['add', '-A']);
    return (await git(worktree, ['write-tree'])).trim();
  };
  // Commands are host-owned. The worker receives failure output, but cannot
  // substitute an easier command or declare its own checks successful.
  /** Prepare the tree the way the repository expects before checks run: regenerate protobuf
   * code, then format the changed files. Returns a failure message for the worker, if any. */
  const prepareTree = async (paths: string[]) => {
    const run = async (args: string[]) => {
      try {
        await execute('mise', args, {
          cwd: worktree,
          signal,
          timeoutMs: 10 * 60_000,
          captureDiagnostics: true,
          unsetEnv
        });
        return undefined;
      } catch (error) {
        signal.throwIfAborted();
        return error instanceof ImplementationCommandError ? error.output : '';
      }
    };
    if (paths.some((path) => path.startsWith('proto/'))) {
      await ctx.emit({
        type: 'state',
        value: { phase: 'generating' },
        activity: 'Generating protobuf code'
      });
      const failure = await run(['run', 'codegen-proto']);
      if (failure !== undefined)
        return `Protobuf code generation failed. Fix the .proto sources.\n${validationDiagnostic(failure, worktree)}`;
    }
    // Deleted paths are in the diff too; format only files that still exist.
    const existing: string[] = [];
    for (const path of paths) {
      try {
        if ((await lstat(resolve(worktree, path))).isFile()) existing.push(path);
      } catch {
        // Deleted in this change.
      }
    }
    const goFiles = existing.filter((path) => path.endsWith('.go'));
    // Formatting is best effort. A file that cannot be parsed fails the checks that follow.
    if (existing.length)
      await run([
        'x',
        '--',
        'pnpm',
        'exec',
        'prettier',
        '--write',
        '--ignore-unknown',
        '--',
        ...existing
      ]);
    if (goFiles.length) await run(['x', '--', 'gofmt', '-w', ...goFiles]);
    return undefined;
  };

  /** Host checks for a change: the repository's typecheck and lint for the affected area. They are
   * fast and deterministic. Tests run in CI on the pull request, where failures return to the worker. */
  const commandsFor = (paths: string[]) => {
    const frontendOnly = paths.every((path) => path.startsWith('apps/frontend/'));
    const goChanged = paths.some(
      (path) => path.endsWith('.go') || /(^|\/)go\.(mod|sum)$/.test(path)
    );
    return [
      ['x', '--', 'pnpm', 'run', frontendOnly ? 'check:frontend' : 'check'],
      ['x', '--', 'pnpm', 'run', frontendOnly ? 'lint:frontend' : 'lint'],
      ...(paths.some((path) => path.startsWith('proto/')) ? [['run', 'lint-proto']] : []),
      ...(goChanged ? [['run', 'lint-cli']] : [])
    ];
  };

  const validate = async (paths: string[]) => {
    const preparation = await prepareTree(paths);
    if (preparation) return preparation;
    const commands = commandsFor(paths);
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
        await ctx.emit({
          type: 'state',
          value: {
            phase: 'validation_failed',
            failedCheck: command,
            completedChecks: [...completed],
            pendingChecks: [...pending]
          },
          activity: `Validation failed · ${command}`,
          activityLevel: 'error'
        });
        return `Validation failed: ${command}\n${diagnostic}`;
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
  return { checks, stageTree, validate };
}
