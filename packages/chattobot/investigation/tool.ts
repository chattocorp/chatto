/** ChattoBot's `investigate` tool: start an investigator subagent as a background job, or resume
 * a finished investigation. The tool returns at once, so the agent can keep talking while the
 * investigator works. */
import type { Destination } from '@chatto/client';
import { defineAgentExtension, Type, type TaskContext } from 'runling';
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
  /** Take one investigation from the turn's budget; false when none are left. */
  take: () => boolean;
  /** The scope of the investigations, such as the thread. */
  scope: string;
  /** Where the investigation's progress and result go. */
  replyTo: () => Destination;
}

/** Investigations that can run at the same time in one conversation. */
export const MAX_RUNNING = 3;

export function investigateExtension(
  ctx: TaskContext<any, any>,
  { directory, investigator, repository }: Investigation,
  { jobs, take, scope, replyTo }: InvestigateToolOptions
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
      description: `Start an investigator subagent in the background that answers a question about the Chatto source code. It reads the code, the repository documentation, and the Git history, and answers with file references. It takes a few minutes. Its answer reaches you in a later turn; until then, keep talking. For a follow-up on a topic that a finished investigation covered, pass its ID as resume: the investigator remembers what it read. At most ${MAX_RUNNING} investigations run at the same time.`,
      parameters: Type.Object({
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
      async execute(_id, { question, resume }) {
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
        jobs.start(
          {
            id,
            kind: 'investigation',
            label: `Investigation: ${question.slice(0, 300)}`,
            replyTo: replyTo()
          },
          (report) =>
            run(id, { directory, question, ...(resume ? { session: resume } : { id }) }, report)
        );
        return {
          content: [
            {
              type: 'text',
              text: `Investigation ${id} started in the background. Its progress is posted to the thread, and its answer reaches you in a later turn. Tell the people in the thread in one short sentence that you started it; do not wait for it.`
            }
          ],
          details: {}
        };
      }
    });
  });
}
