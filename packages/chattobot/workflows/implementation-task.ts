/** One implementation run, composed from its stages: preflight, worktree setup, worker turns with
 * host validation, publication, and CI follow-up. Each stage lives in its own module. */
import { randomUUID } from 'node:crypto';
import { mkdir, mkdtemp, stat, writeFile } from 'node:fs/promises';
import { basename, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { task, Type, type WorkflowContext } from 'runling';
import {
  agent,
  connectAgent,
  type AgentOptions,
  type AgentTaskUpdate,
  type RunlingAgent
} from 'runling/agents';
import {
  HostCommandError,
  implementationProcess,
  type ImplementationProcess
} from './implementation-process.ts';
import {
  implementationInput,
  MAX_FOLLOW_UPS,
  WORKER_SESSION_FILE,
  type ImplementationMetadata
} from './implementation-artifacts.ts';
import { protectedPath } from './implementation-safety.ts';
import {
  implementationCommandEnvKeys,
  normalizeImplementationSettings,
  type ImplementationSettings
} from './implementation-settings.ts';
import {
  WORKER_INSTRUCTIONS,
  WORKER_TOOLS,
  workerToolsExtension,
  type WorkerCheck,
  type WorkerState
} from './implementation-tools.ts';
import { createValidation } from './implementation-validation.ts';
import { publishPullRequest } from './implementation-publication.ts';
import type { observePullRequestChecks } from './implementation-ci.ts';
import { createGit, createRemoteCheck, preflight } from './implementation-preflight.ts';
import { createProgress, PROGRESS_TIMING } from './implementation-progress.ts';
import { createWorkLoop, treeQueries, type RunState } from './implementation-work.ts';
import { ciResult, followCi, type CiResult } from './implementation-follow-ci.ts';

export { PROGRESS_TIMING } from './implementation-progress.ts';
export { MAX_CI_REPAIRS, type CiResult } from './implementation-follow-ci.ts';

type Worker = Pick<RunlingAgent, 'runOutcome' | 'dispose'> & Partial<Pick<RunlingAgent, 'steer'>>;

/** Builds the workspace packages that the frontend imports, such as generated API types. */
export const FRONTEND_DEPENDENCY_BUILD = [
  'x',
  '--',
  'pnpm',
  'turbo',
  'run',
  'build',
  '--filter=chatto-frontend^...',
  '--output-logs=errors-only'
];

/** Where implementation artifacts, their worktrees, and worker sessions are kept. */
export function implementationArtifactsDirectory(settings: ImplementationSettings): string {
  return resolve(
    settings.artifactsDirectory ??
      fileURLToPath(new URL('../.runling/implementations/', import.meta.url))
  );
}

/** Install locked dependencies and build the generated packages that the frontend imports.
 * Returns a blocked summary when setup fails or changes source files. */
async function setupWorktree({
  ctx,
  signal,
  execute,
  worktree,
  unsetEnv,
  stageTree
}: {
  ctx: WorkflowContext<string, AgentTaskUpdate>;
  signal: AbortSignal;
  execute: ImplementationProcess;
  worktree: string;
  unsetEnv: string[];
  stageTree: () => Promise<string>;
}): Promise<string | undefined> {
  await ctx.emit({
    type: 'finding',
    text: 'Preparing the implementation worktree and installing locked dependencies.'
  });
  const beforeSetupTree = await stageTree();
  try {
    await execute('mise', ['x', '--', 'pnpm', 'install', '--frozen-lockfile'], {
      cwd: worktree,
      signal,
      timeoutMs: 10 * 60_000,
      unsetEnv
    });
  } catch {
    signal.throwIfAborted();
    return 'Worktree dependency setup failed. Check mise, pnpm, and package registry access on the bot host. No coding agent started or PR was created.';
  }
  // The frontend imports generated workspace packages, such as API types, from their build
  // output. Build them so that the worker's focused tests can load. Host checks build them too.
  try {
    await execute('mise', FRONTEND_DEPENDENCY_BUILD, {
      cwd: worktree,
      signal,
      timeoutMs: 10 * 60_000,
      unsetEnv
    });
  } catch {
    signal.throwIfAborted();
    await ctx.emit({
      type: 'finding',
      text: 'Could not build the generated workspace packages. Focused frontend tests may not start.'
    });
  }
  // Setup must not silently change either a new base or retained edits.
  if ((await stageTree()) !== beforeSetupTree)
    return 'Dependency setup changed source files. Review the worktree before retrying.';
}

/** Edit in a new or verified retained worktree, validate, then publish through host-owned Git/gh calls.
 * After publication, the same worker stays available until CI settles: the host returns CI failures
 * and later user messages to it and pushes its validated fixes to the pull request.
 * The task communicates only with its parent: progress and milestones as `notice` updates (a
 * milestone has `data.milestone` and its facts), stages as `state` phases, and its result.
 * Worktrees are not a shell sandbox. Only run with trusted users on an isolated host.
 * Cancellation retains local artifacts; a push or PR already accepted remotely is not undone.
 */
export function createImplementation(
  settings: ImplementationSettings,
  dependencies: {
    createAgent?: (options: AgentOptions) => Promise<Worker>;
    execute?: ImplementationProcess;
    /** Opaque conversation identity; only its unfinished work can be resumed. */
    ownerKey?: string;
    observeChecks?: typeof observePullRequestChecks;
    /** Wait after a rerun before CI is read again. Defaults to 30 seconds. */
    rerunDelayMs?: number;
    progressTiming?: Partial<typeof PROGRESS_TIMING>;
  } = {}
) {
  const execute = dependencies.execute ?? implementationProcess;
  const createAgent = dependencies.createAgent ?? agent;
  const { baseBranch } = normalizeImplementationSettings(settings);
  const directory = resolve(settings.directory);
  const artifacts = implementationArtifactsDirectory(settings);

  return task(
    {
      name: 'Implement Chatto change',
      input: implementationInput,
      output: Type.Object({
        outcome: Type.Union([
          Type.Literal('completed'),
          Type.Literal('blocked'),
          Type.Literal('publication_unknown')
        ]),
        summary: Type.String(),
        notes: Type.Array(Type.String()),
        prUrl: Type.Optional(Type.String()),
        branch: Type.String(),
        baseCommit: Type.String(),
        commit: Type.Optional(Type.String()),
        worktree: Type.String(),
        artifactId: Type.Optional(Type.String()),
        checks: Type.Array(
          Type.Object({
            command: Type.String(),
            passed: Type.Boolean(),
            diagnostic: Type.Optional(Type.String())
          })
        ),
        workerChecks: Type.Array(
          Type.Object({
            command: Type.String(),
            passed: Type.Boolean()
          })
        ),
        ci: Type.Optional(ciResult)
      })
    },
    async (ctx: WorkflowContext<string, AgentTaskUpdate>, input) => {
      const signal = ctx.signal;
      const unsetEnv = implementationCommandEnvKeys(process.env);
      const git = createGit(execute, signal);
      const verifyRemote = createRemoteCheck(git, settings.repository);
      const checked = await preflight(
        {
          signal,
          execute,
          git,
          verifyRemote,
          directory,
          artifacts,
          repository: settings.repository,
          baseBranch,
          ownerKey: dependencies.ownerKey,
          resumeArtifactId: input.resumeArtifactId
        },
        `chattobot/${randomUUID()}`
      );
      if (!checked.ok)
        return {
          outcome: 'blocked' as const,
          summary: `Implementation stopped before editing. ${checked.obstacle}`,
          notes: ['No coding agent started and no PR was created.'],
          branch: checked.branch,
          baseCommit: '',
          commit: undefined,
          worktree: '',
          prUrl: undefined,
          ...(input.resumeArtifactId ? { artifactId: input.resumeArtifactId } : {}),
          checks: [],
          workerChecks: []
        };
      const { branch, baseCommit, resumed } = checked;
      await mkdir(artifacts, { recursive: true, mode: 0o700 });
      const folder = resumed?.folder ?? (await mkdtemp(resolve(artifacts, 'implementation-')));
      const artifactId = basename(folder);
      // Every state update names the artifact, so the parent can offer to continue the work
      // even when the task is cancelled and returns no result.
      const parentEmit = ctx.emit.bind(ctx);
      ctx = {
        ...ctx,
        emit: (update) =>
          parentEmit(
            typeof update !== 'string' && update.type === 'state'
              ? { ...update, value: { ...update.value, artifactId } }
              : update
          )
      };
      const worktree = resolve(folder, 'worktree');
      // The worker's conversation, so that a continuation keeps its full context.
      const sessionFile = resolve(folder, WORKER_SESSION_FILE);
      const sessionSaved = await stat(sessionFile).then(
        (file) => file.size > 0,
        () => false
      );
      const metadata: ImplementationMetadata = resumed?.metadata ?? {
        branch,
        baseBranch,
        baseCommit,
        repository: settings.repository,
        stage: 'editing',
        ownerKey: dependencies.ownerKey,
        input: {
          request: input.request,
          context: input.context,
          plan: input.plan,
          ...(input.issue ? { issue: input.issue } : {})
        }
      };
      const save = () =>
        writeFile(resolve(folder, 'metadata.json'), JSON.stringify(metadata, null, 2), {
          mode: 0o600
        });
      // A resume request carries the user's newest instructions. Keep them with the artifact so
      // this and later workers follow them, not only the original request.
      if (resumed)
        metadata.followUps = [
          ...(metadata.followUps ?? []),
          { request: input.request, ...(input.context ? { context: input.context } : {}) }
        ].slice(-MAX_FOLLOW_UPS);
      await save();
      if (!resumed) await git(directory, ['worktree', 'add', '-b', branch, worktree, baseCommit]);
      const { checks, stageTree, validate } = createValidation({
        ctx,
        signal,
        execute,
        git,
        worktree,
        unsetEnv
      });
      const workerChecks: WorkerCheck[] = [];
      const state: WorkerState = {
        checkpointRequested: false,
        announcedChanges: false,
        ciFailure: false,
        rerunRequested: false
      };
      const run: RunState = {
        head: baseCommit,
        lastReportSummary: '',
        validationAnnounced: false,
        missedClarification: false,
        queueMessages: false,
        waiting: []
      };
      const tree = treeQueries({ git, worktree, baseCommit, branch, run });
      const progress = createProgress({
        ctx,
        signal,
        git,
        worktree,
        workerChecks,
        timing: dependencies.progressTiming
      });
      let worker: Worker | undefined;
      const result = async (
        outcome: 'completed' | 'blocked' | 'publication_unknown',
        summary: string,
        notes: string[] = [],
        ci?: CiResult
      ) => {
        if (outcome === 'blocked') {
          metadata.stage = 'blocked';
          await save();
        }
        await ctx.emit({
          type: 'state',
          value: { phase: outcome },
          activity: `Implementation ${outcome}`,
          activityLevel: outcome === 'completed' ? 'success' : 'error'
        });
        return {
          outcome,
          summary,
          notes,
          branch,
          baseCommit,
          ...(metadata.commit ? { commit: metadata.commit } : {}),
          worktree,
          artifactId,
          ...(metadata.prUrl ? { prUrl: metadata.prUrl } : {}),
          checks: [...checks.values()].map(({ command, passed, diagnostic }) => ({
            command,
            passed,
            ...(diagnostic ? { diagnostic } : {})
          })),
          workerChecks: [...workerChecks],
          ...(ci ? { ci } : {})
        };
      };
      const tools = workerToolsExtension({
        ctx,
        signal,
        execute,
        folder,
        worktree,
        baseCommit,
        unsetEnv,
        metadata,
        save,
        workerChecks,
        state,
        reportProgress: progress.reportProgress
      });
      try {
        metadata.stage = 'setup';
        await ctx.emit({ type: 'state', value: { phase: 'setup' } });
        await save();
        const setupFailure = await setupWorktree({
          ctx,
          signal,
          execute,
          worktree,
          unsetEnv,
          stageTree
        });
        if (setupFailure) return result('blocked', setupFailure);
        metadata.stage = 'editing';
        await ctx.emit({
          type: 'state',
          value: { phase: 'editing', attempt: 1 },
          activity: 'Implementation editing · attempt 1'
        });
        await save();
        worker = await createAgent({
          cwd: worktree,
          sessionFile,
          model: settings.model ?? 'openai-codex/gpt-5.6-sol',
          thinkingLevel: settings.thinkingLevel ?? 'medium',
          label: 'implement',
          tools: WORKER_TOOLS,
          // A repository check alone can take ten minutes.
          codemode: { timeoutMs: 20 * 60_000 },
          extensions: [tools],
          textDelivery: 'final',
          resources: {
            extensions: false,
            skills: false,
            promptTemplates: false,
            themes: false,
            contextFiles: true
          },
          onStatus: (status) => {
            void ctx.emit(status).catch(() => {});
          },
          onActivity: (activity) => {
            void ctx.emit(activity).catch(() => {});
          },
          instructions: WORKER_INSTRUCTIONS
        });
        signal.throwIfAborted();
        const connection = connectAgent({ ...ctx, signal }, worker, {
          inbox: ctx.inbox,
          onText: (text) => ctx.emit({ type: 'output', text }),
          onDelivery: async (text, consumed) => {
            if (consumed) return;
            if (!run.queueMessages) {
              run.missedClarification = true;
              return;
            }
            run.waiting.push(text);
            run.wake?.abort();
          }
        });
        const work = createWorkLoop({
          ctx,
          signal,
          worktree,
          baseCommit,
          metadata,
          state,
          run,
          progress,
          tree,
          stageTree,
          validate,
          connection,
          checkMs: { ...PROGRESS_TIMING, ...dependencies.progressTiming }.checkMs
        });
        // Give the worker the actual checkout revision so plan drift is visible without shell access.
        // A saved conversation already holds the request, plan, and the worker's own progress.
        const prompt = sessionSaved
          ? JSON.stringify({
              resumeArtifactId: artifactId,
              followUps: metadata.followUps,
              continuation:
                'Your work in this worktree was interrupted, for example by a cancellation or a restart. Your conversation so far is above. Your last step may not have finished, so check the current diff and the files you were changing first. Then continue where you stopped. followUps are later user instructions for this request, oldest first; where they differ from earlier instructions, follow the latest one. Prepare the full PR proposal again when the change is ready; all host checks will run again.'
            })
          : JSON.stringify({
              ...(resumed?.metadata.input ?? input),
              baseCommit,
              ...(resumed
                ? {
                    resumeArtifactId: basename(resumed.folder),
                    handoff: resumed.metadata.handoff,
                    followUps: metadata.followUps,
                    continuation:
                      'Review the retained worktree diff and verify the saved handoff against current source. Continue this implementation. followUps are later user instructions for this request, oldest first; where they differ from the original request or your handoff, follow the latest one. Recreate the PR proposal; all host checks will run again.'
                  }
                : {})
            });
        try {
          // Before publication, rerunFailedChecks is refused, so work cannot return `rerun`.
          const outcome = await work(prompt);
          if (typeof outcome === 'object') return result('blocked', outcome.stopped, outcome.notes);
          signal.throwIfAborted();
          if (run.missedClarification)
            return result(
              'blocked',
              'A user clarification was not consumed by the worker. Publication was stopped; review the request before continuing.'
            );
          // From here on, the worker waits between turns. Later messages go to its next turn.
          run.queueMessages = true;
          const proposal = state.proposal;
          if (!proposal)
            return result('blocked', 'Implementation did not produce a prepared change.');
          if (!(await tree.sameGitState()))
            return result(
              'blocked',
              'The worker changed Git history or branches; publication was stopped.'
            );
          const finalTree = await stageTree();
          const paths = await tree.changedPaths();
          if (!paths.length) return result('blocked', 'No source changes were produced.');
          if (paths.some(protectedPath))
            return result(
              'blocked',
              'Protected instructions or environment files changed; publication was stopped.'
            );
          if (
            !checks.size ||
            [...checks.values()].some((check) => !check.passed || check.tree !== finalTree)
          )
            return result(
              'blocked',
              'All recorded checks must pass on the final source tree before publication.'
            );
          const publication = await publishPullRequest({
            ctx,
            signal,
            execute,
            git,
            verifyRemote,
            folder,
            worktree,
            repository: settings.repository,
            branch,
            baseBranch,
            proposal,
            checks,
            metadata,
            save
          });
          if (publication === 'unknown')
            return result(
              'publication_unknown',
              'Could not verify publication. A branch or PR may already exist; check GitHub before retrying.',
              proposal.notes
            );
          run.head = metadata.commit!;
          // The PR milestone supersedes a progress update that is still waiting.
          progress.dropHeldProgress();
          await progress.reportMilestone(
            'published',
            'The pull request is open. Typecheck and lint passed, and CI is running now.',
            { prUrl: metadata.prUrl! }
          );
          const ci = await followCi(
            {
              ctx,
              signal,
              execute,
              git,
              verifyRemote,
              repository: settings.repository,
              worktree,
              branch,
              metadata,
              save,
              state,
              run,
              progress,
              tree,
              stageTree,
              observeChecks: dependencies.observeChecks,
              rerunDelayMs: dependencies.rerunDelayMs
            },
            work
          );
          return result('completed', proposal.summary, proposal.notes, ci);
        } finally {
          await connection.dispose();
        }
      } catch (error) {
        signal.throwIfAborted();
        // After publication, the parent's generic handling applies; the PR already exists.
        if (metadata.prUrl) throw error;
        // Name the failed stage and host command, so the user learns what went wrong. Worker and
        // provider errors can contain private details and stay generic.
        return result(
          'blocked',
          `The implementation stopped during ${metadata.stage} because ${
            error instanceof HostCommandError ? error.message : 'of an unexpected error'
          }. The work so far is kept.`,
          ['No PR was created.']
        );
      } finally {
        progress.dropHeldProgress();
        worker?.dispose();
        if (!['published', 'blocked', 'publication_unknown'].includes(metadata.stage)) {
          metadata.stage = ['publishing', 'pushed'].includes(metadata.stage)
            ? 'publication_unknown'
            : 'interrupted';
        }
        // Preserve the diff after cancellation, including staged new files.
        try {
          await writeFile(
            resolve(folder, 'changes.patch'),
            await git(
              worktree,
              ['diff', '--binary', '--no-ext-diff', '--no-textconv', baseCommit],
              AbortSignal.timeout(30_000)
            ),
            { mode: 0o600 }
          );
        } finally {
          await save();
        }
      }
    }
  );
}
