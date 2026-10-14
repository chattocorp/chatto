/**
 * ChattoBot's `investigate` tool: start an investigator subagent in the background, or resume a
 * finished one. The tool returns at once, so the conversation agent can keep talking while the
 * investigator works. The conversation receives the progress and the result through
 * `InvestigationEvents`.
 */
import { defineAgentExtension, Type, type TaskContext } from 'runling';
import { linkCitations } from './citations.ts';
import type { createInvestigator } from './investigate.ts';

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

/** How a background investigation ended: its answer with linked references, or an error. */
export type InvestigationOutcome = { answer: string } | { error: string };

/** What the conversation does with background investigations. */
export interface InvestigationEvents {
  /** The number of investigations that run now. */
  running(): number;
  /** True when the investigation with this ID runs now. */
  isRunning(id: string): boolean;
  started(id: string, question: string): void;
  /** Progress, in batches of updates that arrived close together. */
  progress(id: string, updates: string[]): void;
  /** Called once, after all progress of the investigation. */
  finished(id: string, outcome: InvestigationOutcome): void;
}

export interface InvestigateToolOptions extends InvestigationEvents {
  /** Take one investigation from the turn's budget; false when none are left. */
  take: () => boolean;
  /** The scope of the investigations, such as the thread. */
  scope: string;
}

/** Investigations that can run at the same time in one conversation. */
export const MAX_RUNNING = 3;
/** Updates within this time go into one batch, such as the reads of one model step. */
const BATCH_MS = 1000;

export function investigateExtension(
  ctx: TaskContext<any, any>,
  { directory, investigator, repository }: Investigation,
  { take, scope, ...events }: InvestigateToolOptions
) {
  const investigate = investigator(scope);

  /** Run one investigation to its end and report it. It belongs to the conversation, not to the
   * tool call, so it continues after the turn and stops only with the conversation. */
  async function run(id: string, input: Parameters<typeof investigate>[1]) {
    const investigation = ctx.start(investigate, input);
    investigation.closeInput();
    let batch: string[] = [];
    let timer: ReturnType<typeof setTimeout> | undefined;
    const flush = () => {
      clearTimeout(timer);
      timer = undefined;
      if (batch.length) events.progress(id, batch.splice(0));
    };
    const progress = (async () => {
      for await (const update of investigation.updates) {
        batch.push(update);
        timer ??= setTimeout(flush, BATCH_MS);
      }
    })();
    // A failed investigation also fails its updates; its result reports the failure.
    void progress.catch(() => {});
    let outcome: InvestigationOutcome;
    try {
      const result = await investigation.result;
      await progress;
      const answer =
        repository && result.commit
          ? linkCitations(result.answer, result.citations, { repository, commit: result.commit })
          : result.answer;
      outcome = {
        answer: `${result.commit ? `(Code at commit ${result.commit.slice(0, 9)}.)\n\n` : ''}${answer}`
      };
    } catch (error) {
      outcome = { error: error instanceof Error ? error.message : String(error) };
    }
    flush();
    events.finished(id, outcome);
  }

  return defineAgentExtension((pi) => {
    pi.registerTool({
      name: 'investigate',
      label: 'Investigate the source',
      description: `Start an investigator subagent in the background that answers a question about the Chatto source code. It reads the code, the repository documentation, and the Git history, and answers with file references. It takes a few minutes. Its answer reaches you as a message in a later turn; until then, you can keep talking. For a follow-up on a topic that a finished investigation covered, pass its ID as resume: the investigator remembers what it read. At most ${MAX_RUNNING} investigations run at the same time.`,
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
        if (resume && events.isRunning(resume))
          throw new Error(
            `Investigation ${resume} is still running. Its answer reaches you when it finishes.`
          );
        if (events.running() >= MAX_RUNNING)
          throw new Error(
            `${MAX_RUNNING} investigations are running. Wait until one of them finishes.`
          );
        if (!take())
          throw new Error(
            'You reached the limit of investigations for this turn. Answer with what you found.'
          );
        const id = resume ?? crypto.randomUUID();
        events.started(id, question);
        void run(id, { directory, question, ...(resume ? { session: resume } : { id }) });
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
