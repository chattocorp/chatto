/** ChattoBot's `investigate` tool: start an investigator subagent, or resume one. */
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

export interface InvestigateToolOptions {
  /** Take one investigation from the turn's budget; false when none are left. */
  take: () => boolean;
  /** The scope of the investigations, such as the thread. */
  scope: string;
  /** Receives the investigator's progress, in batches of updates that arrived close together.
   * All updates of an investigation arrive before its tool call returns. */
  onProgress?: (updates: string[]) => void;
}

/** Updates within this time go into one batch, such as the reads of one model step. */
const BATCH_MS = 1000;

export function investigateExtension(
  ctx: TaskContext<any, any>,
  { directory, investigator, repository }: Investigation,
  { take, scope, onProgress }: InvestigateToolOptions
) {
  const investigate = investigator(scope);
  return defineAgentExtension((pi) => {
    pi.registerTool({
      name: 'investigate',
      label: 'Investigate the source',
      description:
        'Ask an investigator subagent a question about the Chatto source code. It reads the code, the repository documentation, and the Git history, and answers with file references. It takes up to a few minutes. For a follow-up on a topic that an earlier investigation covered, pass its ID as resume instead of starting a new one: the investigator remembers what it read, so it answers faster and with more context.',
      parameters: Type.Object({
        question: Type.String({
          minLength: 1,
          maxLength: 20_000,
          description:
            'For a new investigation, a complete, self-contained question. With resume, ask the follow-up as you would in a conversation.'
        }),
        resume: Type.Optional(
          Type.String({ description: 'The ID of an earlier investigation to continue' })
        )
      }),
      async execute(_id, { question, resume }, signal) {
        if (!take())
          throw new Error(
            'You reached the limit of investigations for this turn. Answer with what you found.'
          );
        const toolCtx = signal ? { ...ctx, signal: AbortSignal.any([ctx.signal, signal]) } : ctx;
        const investigation = toolCtx.start(investigate, {
          directory,
          question,
          ...(resume ? { session: resume } : {})
        });
        investigation.closeInput();
        let batch: string[] = [];
        let timer: ReturnType<typeof setTimeout> | undefined;
        const flush = () => {
          clearTimeout(timer);
          timer = undefined;
          if (batch.length) onProgress?.(batch.splice(0));
        };
        const progress = (async () => {
          for await (const update of investigation.updates) {
            batch.push(update);
            timer ??= setTimeout(flush, BATCH_MS);
          }
        })();
        // A failed investigation also fails its updates; its result reports the failure.
        void progress.catch(() => {});
        let result: Awaited<typeof investigation.result>;
        try {
          result = await investigation.result;
          await progress;
        } finally {
          flush();
        }
        const answer =
          repository && result.commit
            ? linkCitations(result.answer, result.citations, { repository, commit: result.commit })
            : result.answer;
        return {
          content: [
            {
              type: 'text',
              text: `Investigation ${result.session}${result.commit ? ` (code at commit ${result.commit.slice(0, 9)})` : ''}:\n\n${answer}\n\nFor a follow-up question on this topic, call investigate with resume: "${result.session}". This investigator remembers what it read.`
            }
          ],
          details: {}
        };
      }
    });
  });
}
