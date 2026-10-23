/** ChattoBot's `investigate` tool: start an investigator subagent as a background job, or resume
 * a finished investigation. The tool returns at once, so the agent can keep talking while the
 * investigator works. */
import { defineAgentExtension, log, Type, type TaskContext } from 'runling';
import { replyTo, type IncomingMessage } from '../chatto/message.ts';
import type { WorkOwner } from '../harness/work.ts';
import { requireMaintainerRequest } from '../harness/authorization.ts';
import { linkCitations } from './citations.ts';
import type { createInvestigator } from '../tasks/investigate.ts';

/** What the conversation needs for code investigations. */
export interface Investigation {
  /** The checkout to investigate. Conversation starts refresh the configured reference copy. */
  directory: string;
  /** The investigator for one conversation scope, such as a thread. An investigation can be
   * resumed only in the scope that started it, so its questions stay in that thread. */
  investigator: (scope: string) => ReturnType<typeof createInvestigator>;
  /** The GitHub repository of the checkout, `owner/repo`, for links to cited lines. */
  repository?: string;
}

export interface InvestigateToolOptions {
  work: WorkOwner;
  maintainers: ReadonlySet<string>;
  /** Only host-owned addressed messages in the current turn can authorize starts or resumes. */
  messages: () => readonly IncomingMessage[];
  /** Take one investigation from the turn's budget; false when none are left. */
  take: () => boolean;
  /** The scope of the investigations, such as the thread. */
  scope: string;
}

/** Investigations that can run at the same time in one conversation. */
export const MAX_RUNNING = 3;

export function investigateExtension(
  ctx: TaskContext<any, any>,
  { directory, investigator, repository }: Investigation,
  { work, maintainers, messages, take, scope }: InvestigateToolOptions
) {
  const investigate = investigator(scope);

  /** Present the investigator's result for the next conversation turn. */
  function formatResult(id: string, result: Awaited<ReturnType<typeof investigate>>) {
    const answer =
      repository && result.commit
        ? linkCitations(result.answer, result.citations, { repository, commit: result.commit })
        : result.answer;
    return `Investigation ${id}${result.commit ? `, code at commit ${result.commit.slice(0, 9)}` : ''}. Pass on what it found, with its links. Resume it with its ID for a follow-up on this topic.\n\n${answer}`;
  }

  return defineAgentExtension((pi) => {
    pi.registerTool({
      name: 'investigate',
      label: 'Investigate the source',
      description: `Start or resume a source investigation only for a maintainer request in this turn. Pass that message's ID; never use an unrelated maintainer message to authorize someone else's request. It reads the code, repository documentation, and Git history, and answers with file references. Its answer reaches you in a later turn. For a follow-up on a finished investigation, pass its ID as resume. At most ${MAX_RUNNING} investigations run at the same time.`,
      parameters: Type.Object({
        requestId: Type.String({ description: 'The message ID of the current maintainer request' }),
        question: Type.String({
          minLength: 1,
          maxLength: 20_000,
          description:
            'For a new investigation, a complete, self-contained question. With resume, ask the follow-up as you would in a conversation.'
        }),
        resume: Type.Optional(
          Type.String({ description: 'The ID of a finished investigation to continue' })
        )
      }),
      async execute(_id, { requestId, question, resume }) {
        const request = requireMaintainerRequest(
          'Investigation',
          requestId,
          messages(),
          maintainers
        );
        ctx.signal.throwIfAborted();
        if (resume && [...work.pending.values()].some((job) => job.sessionId === resume))
          throw new Error(
            `Investigation ${resume} is still running. Its answer reaches you when it finishes.`
          );
        if (
          [...work.pending.values()].filter((job) => job.kind === 'investigation').length >=
          MAX_RUNNING
        )
          throw new Error(
            `${MAX_RUNNING} investigations are running. Wait until one of them finishes.`
          );
        if (!take())
          throw new Error(
            'You reached the limit of investigations for this turn. Answer with what you found.'
          );
        const id = resume ?? crypto.randomUUID();
        log.info(
          `Investigation authorization allowed: request=${request.id}; reason=current-maintainer`
        );
        work.track(
          ctx.start(investigate, {
            directory,
            question,
            ...(resume ? { session: resume } : { id })
          }),
          {
            sessionId: id,
            label: `Investigation: ${question.slice(0, 300)}`,
            kind: 'investigation',
            replyTo: replyTo(request)
          },
          (result) => formatResult(id, result)
        );
        return {
          content: [
            {
              type: 'text',
              text: `Investigation ${id} started. It has not finished. Its result reaches you in a later turn. Send exactly one short acknowledgment such as "I'm investigating", without job details or a completion claim, then end this turn.`
            }
          ],
          details: {}
        };
      }
    });
  });
}
