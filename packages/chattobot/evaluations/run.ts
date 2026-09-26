import { fileURLToPath } from 'node:url';
import { task, Type } from 'runling';
import { agent } from 'runling/agents';
import { responsePolicy } from '../workflows/response-policy.ts';
import { checkReply, replyCases } from './cases.ts';

/** Opt-in policy evaluation. Each case gets a fresh, tool-free agent session. */
export default task(
  {
    name: 'Evaluate ChattoBot replies',
    input: Type.Object({
      model: Type.String({ minLength: 1 }),
      repeats: Type.Optional(Type.Integer({ minimum: 1, maximum: 5 })),
      dryRun: Type.Optional(Type.Boolean())
    }),
    output: Type.Unknown()
  },
  async (ctx, input) => {
    if (input.dryRun) return { cases: replyCases, instructions: responsePolicy, modelCalls: 0 };
    const results = [];
    for (let repetition = 0; repetition < (input.repeats ?? 1); repetition++) {
      for (const example of replyCases) {
        ctx.signal.throwIfAborted();
        const bot = await agent({
          cwd: fileURLToPath(new URL('..', import.meta.url)),
          model: input.model,
          thinkingLevel: 'low',
          output: 'text',
          allowEmptyResponse: true,
          tools: [],
          resources: {
            extensions: false,
            skills: false,
            promptTemplates: false,
            themes: false,
            contextFiles: false
          },
          instructions: [
            'You are ChattoBot. Your text is posted to the conversation. Reply briefly to the supplied conversation data.',
            ...responsePolicy
          ]
        });
        let reply = '';
        try {
          await bot.runOutcome(ctx, JSON.stringify(example.prompt), {
            signal: AbortSignal.any([ctx.signal, AbortSignal.timeout(60_000)]),
            onText: (text) => {
              reply += text;
            }
          });
          results.push({
            id: example.id,
            repetition,
            reply,
            failures: checkReply(example.id, reply),
            rubric: example.rubric
          });
        } catch {
          ctx.signal.throwIfAborted();
          results.push({
            id: example.id,
            repetition,
            reply,
            failures: ['Model call failed or timed out'],
            rubric: example.rubric
          });
        } finally {
          bot.dispose();
        }
      }
    }
    return {
      model: input.model,
      results,
      failed: results.filter((result) => result.failures.length).length,
      limitation:
        'Heuristic policy checks only. Review every reply against its rubric. This does not evaluate tool selection or a full conversation.'
    };
  }
);
