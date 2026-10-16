/** ChattoBot's `investigate` tool: start an investigator subagent as a background job, or resume
 * a finished investigation. The tool returns at once, so the agent can keep talking while the
 * investigator works. */
import { defineAgentExtension, log, Type, type TaskContext } from 'runling';
import { replyTo, type IncomingMessage } from '../chatto/message.ts';
import type { Jobs } from '../harness/jobs.ts';
import { linkCitations } from './citations.ts';
import type { createInvestigator } from '../tasks/investigate.ts';

/** What the conversation needs for code investigations. */
export interface Investigation {
  /** The checkout to investigate. The operator keeps it current. */
  directory: string;
  /** The investigator for one conversation scope, such as a thread. An investigation can be
   * resumed only in the scope that started it, so its questions stay in that thread. */
  investigator: (scope: string) => ReturnType<typeof createInvestigator>;
  /** The GitHub repository of the checkout, `owner/repo`, for links to cited lines. */
  repository?: string;
}

export interface InvestigateToolOptions {
  jobs: Jobs;
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
  { jobs, maintainers, messages, take, scope }: InvestigateToolOptions
) {
  const investigate = investigator(scope);

  /** Run one investigation and return its answer for the agent. */
  async function run(
    id: string,
    input: Parameters<typeof investigate>[1],
    report: (update: string) => void
  ) {
    const investigation = ctx.start(investigate, input);
    investigation.closeInput();
    const progress = (async () => {
      for await (const update of investigation.updates) report(update);
    })();
    // A failed investigation also fails its updates; its result reports the failure.
    void progress.catch(() => {});
    const result = await investigation.result;
    await progress;
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
        const request = messages().find((message) => message.id === requestId);
        if (request?.trigger === 'intake') {
          log.info(
            `Investigation authorization denied: request=${request.id}; reason=not-addressed`
          );
          throw new Error('Investigation requires a maintainer message addressed to the bot.');
        }
        if (!request || !maintainers.has(request.authorId)) {
          // Unknown model input must not enter authorization logs.
          log.info(
            `Investigation authorization denied: request=${request?.id ?? 'not-current'}; reason=${request ? 'not-maintainer' : 'no-current-message'}`
          );
          throw new Error('Investigation requires a message from a maintainer in this turn.');
        }
        ctx.signal.throwIfAborted();
        if (resume && jobs.has(resume))
          throw new Error(
            `Investigation ${resume} is still running. Its answer reaches you when it finishes.`
          );
        if (jobs.list().filter((job) => job.kind === 'investigation').length >= MAX_RUNNING)
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
        jobs.start(
          {
            id,
            kind: 'investigation',
            label: `Investigation: ${question.slice(0, 300)}`,
            replyTo: replyTo(request)
          },
          (report) =>
            run(id, { directory, question, ...(resume ? { session: resume } : { id }) }, report)
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
