/** ChattoBot owns turn delivery. Runling owns agent execution and tool validation. */
import { Type, emptyTokenUsage, type Static } from 'runling';
import { defineAgentExtension, type RunlingAgent } from 'runling/agents';

/** The supervisor selects exactly one final action; ordinary assistant text is private. */
export const turnCompletionSchema = Type.Object({
  action: Type.Union([
    Type.Object({ kind: Type.Literal('reply'), text: Type.String({ minLength: 1 }) }),
    Type.Object({
      kind: Type.Literal('react'),
      emoji: Type.String({ minLength: 1, maxLength: 100 })
    }),
    Type.Object({ kind: Type.Literal('silent') })
  ])
});
export type TurnCompletion = Static<typeof turnCompletionSchema>['action'];

/** One instance per conversation. A failed reaction leaves the turn open for a text fallback. */
export function createTurnCompletion(options: {
  react: (emoji: string, signal: AbortSignal) => Promise<void>;
}) {
  let selected: TurnCompletion | undefined;
  const selection = () => selected;
  let selecting = false;
  let repairing = false;
  const extension = defineAgentExtension((pi) => {
    pi.registerTool({
      name: 'finishTurn',
      label: 'Finish conversation turn',
      exposure: 'model-only',
      description:
        'Finish this turn with one action: reply with your complete text, react to a simple acknowledgement, or stay silent. Questions and task results need a reply. After success, stop; further text is not delivered. If a reaction fails, finish with a brief text reply instead.',
      parameters: turnCompletionSchema,
      async execute(_id, { action: completion }, signal) {
        if (selected || selecting) throw new Error('This turn already has a final action.');
        if (completion.kind === 'reply' && !completion.text.trim())
          throw new Error('A reply must contain text. Use silent to send nothing.');
        selecting = true;
        try {
          signal?.throwIfAborted();
          if (completion.kind === 'react')
            await options.react(completion.emoji, signal ?? AbortSignal.timeout(10_000));
          selected = completion;
          return {
            content: [{ type: 'text' as const, text: 'Final action accepted. End this turn now.' }],
            details: { kind: completion.kind }
          };
        } finally {
          selecting = false;
        }
      }
    });
  });

  return {
    extension,
    /** Blocks tools after selection, including while a reaction is being delivered. */
    get finished() {
      return !!selected || selecting;
    },
    /** A repair may only select delivery; it must not repeat external work. */
    get repairing() {
      return repairing;
    },
    /** Use Runling's normal conversation lifecycle, but deliver only the selected reply.
     * One corrective turn repairs a missing selection. Cancellation never starts a repair. */
    wrap(
      bot: Pick<RunlingAgent, 'runOutcome' | 'steer'>
    ): Pick<RunlingAgent, 'runOutcome' | 'steer'> {
      return {
        steer: (text) => bot.steer(text),
        async runOutcome(ctx, prompt, runOptions) {
          selected = undefined;
          repairing = false;
          const quietOptions = { ...runOptions, onText: undefined };
          const run = async (input: string) => {
            try {
              return await bot.runOutcome(ctx, input, quietOptions);
            } catch (error) {
              ctx.signal.throwIfAborted();
              runOptions?.signal?.throwIfAborted();
              if (!selected) throw error;
              // Delivery was selected before an unnecessary continuation threw. Usage is
              // already recorded by Runling; the rejected call has no usage result to return.
              return { outcome: 'completed' as const, summary: '', usage: emptyTokenUsage() };
            }
          };
          let result = await run(prompt);
          ctx.signal.throwIfAborted();
          runOptions?.signal?.throwIfAborted();
          if (!selected && result.outcome === 'completed') {
            repairing = true;
            const firstUsage = result.usage;
            result = await run(
              'Your turn has no final action. Call finishTurn now with reply, react, or silent. Do not repeat work or an acknowledgement. Ordinary assistant text is not delivered.'
            );
            result = {
              ...result,
              usage: {
                input: firstUsage.input + result.usage.input,
                output: firstUsage.output + result.usage.output,
                cacheRead: firstUsage.cacheRead + result.usage.cacheRead,
                cacheWrite: firstUsage.cacheWrite + result.usage.cacheWrite,
                ...((firstUsage.costIncomplete || result.usage.costIncomplete) && {
                  costIncomplete: true
                }),
                ...((firstUsage.cost !== undefined || result.usage.cost !== undefined) && {
                  cost: (firstUsage.cost ?? 0) + (result.usage.cost ?? 0)
                })
              }
            };
          }
          ctx.signal.throwIfAborted();
          runOptions?.signal?.throwIfAborted();
          // A successful final action remains valid even if an unnecessary model continuation
          // fails. In particular, do not post a failure after a reaction already succeeded.
          const completion = selection();
          if (completion) {
            const text = completion.kind === 'reply' ? completion.text : '';
            runOptions?.onText?.(text);
            return { outcome: 'completed', summary: text, usage: result.usage };
          }
          if (result.outcome !== 'completed') return result;
          return {
            outcome: 'failed',
            summary: 'The supervisor did not select a final action.',
            usage: result.usage
          };
        }
      };
    }
  };
}
