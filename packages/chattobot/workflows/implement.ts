/** Implementation for ChattoBot: the supervisor tool, with the implementation task, settings, and
 * helpers re-exported from their modules. */
import { randomUUID } from 'node:crypto';
import { Type, type WorkflowContext } from 'runling';
import {
  defineAgentExtension,
  taskTool,
  type AgentTasks,
  type AgentTaskUpdate
} from 'runling/agents';
import { implementationProcess } from './implementation-process.ts';
import type { InvestigationPlans } from './plan.ts';
import { observePullRequestChecks } from './implementation-ci.ts';
import { implementationInput } from './implementation-artifacts.ts';
import { ownerQuestionPrefix } from './implementation-safety.ts';
import type { ImplementationSettings } from './implementation-settings.ts';
import { createImplementation } from './implementation-task.ts';

export { createImplementation } from './implementation-task.ts';
export {
  implementationCommandEnvKeys,
  implementationSettings,
  matchesRepository,
  normalizeImplementationSettings,
  type ImplementationSettings
} from './implementation-settings.ts';
export { validationDiagnostic, workerStopReason } from './implementation-safety.ts';

/** Use the same background lifecycle, steering, and announcement path as investigation. */
export function implementationExtension(
  ctx: WorkflowContext<string, string>,
  settings: ImplementationSettings,
  announce: (text: string, signal: AbortSignal) => Promise<void>,
  tasks: AgentTasks,
  dependencies: Parameters<typeof createImplementation>[1] & {
    /** Changes with each human message, so one request gets at most one implementation attempt. */
    requestVersion?: () => number;
    /** Original typed plans from this conversation; tool callers select an ID, not replacement content. */
    plans?: InvestigationPlans;
    /** Report a refusal directly so the supervisor cannot describe it as started work. */
    onBlocked?: (summary: string) => Promise<void>;
    /** Report a stopped background implementation directly to the conversation. */
    onStopped?: (message: string) => Promise<void>;
    /** Post the host-verified PR URL before waiting for CI. */
    onPublished?: (message: string) => Promise<void>;
    /** Post the observed CI result after publication. */
    onCiResult?: (message: string) => Promise<void>;
    observeChecks?: typeof observePullRequestChecks;
  } = {}
) {
  const implement = createImplementation(settings, dependencies);
  let attemptedVersion: number | undefined;
  return defineAgentExtension((pi) => {
    pi.registerTool({
      name: 'askImplementation',
      label: 'Ask implementation worker',
      description:
        "Ask a running implementation worker a question on the user's behalf. Queue acceptance is immediate; the worker's answer wakes you later as task.reply. Answer the user when that reply arrives.",
      parameters: Type.Object({
        id: Type.String(),
        question: Type.String({ minLength: 1, maxLength: 4000 })
      }),
      async execute(_id, { id, question }) {
        const current = tasks.get(id);
        if (current.name !== 'Chatto implementation' || current.status !== 'running')
          throw new Error('No running implementation task with that ID');
        const questionId = randomUUID();
        await tasks.send(id, `${ownerQuestionPrefix}${questionId}]\n${question}`);
        return {
          content: [{ type: 'text' as const, text: JSON.stringify({ queued: true, questionId }) }],
          details: {}
        };
      }
    });
    pi.registerTool(
      taskTool(
        ctx,
        {
          name: 'implementChatto',
          label: 'Implement Chatto change',
          description:
            'Implement an explicitly requested fix or feature, run checks, and publish a ready-for-review PR in the configured repository. Returns a background task handle. The final result contains the verified PR URL. Do not invoke for a question or investigation alone. Do not start a duplicate task for the same request.',
          parameters: Type.Object({
            request: implementationInput.properties.request,
            context: implementationInput.properties.context,
            resumeArtifactId: Type.Optional(
              Type.String({
                pattern: '^implementation-[A-Za-z0-9_-]{6,}$',
                description:
                  'Continue the exact unfinished artifact named by the human or a stopped result in this conversation.'
              })
            ),
            investigationId: Type.Optional(
              Type.String({
                description:
                  'ID of a completed investigation whose original plan should be implemented. Use this after an investigation instead of rewriting its plan in context.'
              })
            ),
            announcement: Type.String({ minLength: 1, maxLength: 600 })
          })
        },
        async (context, input) => {
          const version = dependencies.requestVersion ? dependencies.requestVersion() : 0;
          const active = tasks
            .list()
            .find((task) => task.name === 'Chatto implementation' && task.status === 'running');
          const refusal = active
            ? 'An implementation is already running. No second task was started.'
            : attemptedVersion === version
              ? 'An implementation was already attempted for this request. No new task was started. Please review its result before asking for another attempt.'
              : undefined;
          if (refusal) {
            await dependencies.onBlocked?.(refusal);
            return JSON.stringify({ outcome: 'blocked', summary: refusal });
          }
          const announcement = input.announcement.trim();
          if (!announcement || announcement.length > 600)
            throw new Error('A brief implementation announcement is required');
          const plan = input.investigationId
            ? dependencies.plans?.get(input.investigationId)
            : undefined;
          if (input.investigationId && !plan)
            throw new Error(
              'No completed implementation plan exists for this investigation in this conversation'
            );
          const retainedPlan = plan ? structuredClone(plan) : undefined;
          attemptedVersion = version;
          await announce(announcement, context.signal);
          context.signal.throwIfAborted();
          const run = ctx.spawn(async (ctx: WorkflowContext<string, AgentTaskUpdate>) => {
            let result;
            try {
              result = await implement(ctx, {
                request: input.request,
                context: input.context,
                plan: retainedPlan,
                resumeArtifactId: input.resumeArtifactId
              });
            } catch {
              ctx.signal.throwIfAborted();
              result = {
                outcome: 'blocked' as const,
                summary:
                  'The implementation stopped after a worker or host error. Its local artifacts may contain unfinished changes.',
                notes: ['No PR was verified.'],
                branch: '',
                baseCommit: '',
                commit: undefined,
                worktree: '',
                prUrl: undefined,
                ...(input.resumeArtifactId ? { artifactId: input.resumeArtifactId } : {}),
                checks: [],
                workerChecks: []
              };
            }
            if (result.outcome !== 'completed') {
              const failedChecks = result.checks
                .filter((check) => !check.passed)
                .map((check) => check.command);
              const workerCheckCount = result.workerChecks.length;
              const failedWorkerChecks = result.workerChecks.filter(
                (check) => !check.passed
              ).length;
              const message =
                result.outcome === 'publication_unknown'
                  ? 'The implementation finished locally, but I could not verify publication. A branch or PR may exist; check GitHub before retrying.'
                  : `The implementation stopped: ${result.summary}${workerCheckCount ? ` The worker ran ${workerCheckCount} check${workerCheckCount === 1 ? '' : 's'}; ${failedWorkerChecks} failed at the time. These were not final host checks.` : ''}${failedChecks.length ? ` Failed final check: ${failedChecks.join(', ')}. See the workflow result for details.` : ''}${result.worktree ? ` The worktree was kept for review${result.artifactId ? ` as ${result.artifactId}` : ''}.` : ''} Please tell me how you want to proceed.`;
              try {
                if (dependencies.onStopped) {
                  await dependencies.onStopped(message);
                  return { ...result, noticeDelivered: true };
                }
              } catch {
                console.warn('ChattoBot could not post the implementation result.');
              }
              return { ...result, noticeDelivered: false };
            }
            let publicationNoticeDelivered = false;
            try {
              if (dependencies.onPublished && result.prUrl) {
                await dependencies.onPublished(
                  `Opened [the pull request](${result.prUrl}). ${result.checks.length} local checks passed. I will report the CI result when it is available.`
                );
                publicationNoticeDelivered = true;
              }
            } catch {
              console.warn('ChattoBot could not post the verified pull request.');
            }
            await ctx.emit({
              type: 'state',
              value: { phase: 'ci_waiting', prUrl: result.prUrl! },
              activity: 'Waiting for pull request checks'
            });
            let ci;
            try {
              ci = await (dependencies.observeChecks ?? observePullRequestChecks)({
                execute: dependencies.execute ?? implementationProcess,
                repository: settings.repository,
                prUrl: result.prUrl!,
                headCommit: result.commit!,
                cwd: result.worktree,
                signal: ctx.signal,
                onPending: async (checks) => {
                  await ctx.emit({
                    type: 'state',
                    value: { phase: 'ci_waiting', prUrl: result.prUrl!, checks: { ...checks } },
                    activity: 'Waiting for pull request checks'
                  });
                }
              });
            } catch {
              ctx.signal.throwIfAborted();
              ci = { status: 'unavailable' as const, passed: 0, failed: 0, pending: 0, skipped: 0 };
            }
            await ctx.emit({
              type: 'state',
              value: { phase: `ci_${ci.status}`, prUrl: result.prUrl!, checks: { ...ci } },
              activity: `Pull request checks ${ci.status}`,
              activityLevel:
                ci.status === 'passed' ? 'success' : ci.status === 'failed' ? 'error' : undefined
            });
            const ciMessage =
              ci.status === 'passed'
                ? `CI passed for [the pull request](${result.prUrl}): ${ci.passed} checks.`
                : ci.status === 'failed'
                  ? `CI failed for [the pull request](${result.prUrl}): ${ci.failed} checks failed or were cancelled. Please review its Checks tab.`
                  : ci.status === 'pending'
                    ? `CI is still pending for [the pull request](${result.prUrl}) after 30 minutes.${ci.failed ? ` ${ci.failed} checks have already failed or been cancelled.` : ''} Please review its Checks tab.`
                    : ci.status === 'skipped'
                      ? `All reported CI checks were skipped for [the pull request](${result.prUrl}). Please review its Checks tab.`
                      : ci.status === 'head_changed'
                        ? `The head commit changed on [the pull request](${result.prUrl}) before I could report CI for ChattoBot's commit. Please review its Checks tab.`
                        : `I could not read CI for [the pull request](${result.prUrl}). Please review its Checks tab.`;
            let ciNoticeDelivered = false;
            try {
              if (dependencies.onCiResult) {
                await dependencies.onCiResult(ciMessage);
                ciNoticeDelivered = true;
              }
            } catch {
              console.warn('ChattoBot could not post the pull request check result.');
            }
            return {
              ...result,
              ci,
              publicationNoticeDelivered,
              ciNoticeDelivered,
              noticeDelivered: publicationNoticeDelivered && ciNoticeDelivered
            };
          });
          try {
            return JSON.stringify(tasks.observe('Chatto implementation', run));
          } catch (error) {
            await run[Symbol.asyncDispose]();
            throw error;
          }
        }
      )
    );
  });
}
