/** Typed choices through OpenRouter's Decisions API, without an agent session. */
import { Type } from 'typebox';
import { Check } from 'typebox/value';
import { task } from './task.ts';
import type { WorkflowSchema } from './schema.ts';
import { log } from './log.ts';
import { formatTokenUsage } from './usage.ts';

const probability = Type.Number({ minimum: 0, maximum: 1 });
const envelope = Type.Object({
  answers: Type.Object({ decision: Type.Unknown() }),
  usage: Type.Optional(
    Type.Object({
      input_tokens: Type.Integer({ minimum: 0, maximum: Number.MAX_SAFE_INTEGER }),
      output_tokens: Type.Integer({ minimum: 0, maximum: Number.MAX_SAFE_INTEGER }),
      cost: Type.Optional(Type.Number({ minimum: 0 }))
    })
  )
});

/** Define a single choice request to OpenRouter. Input and criteria go to the selected
 * provider. Keys in `choices` become the result's choice type. Probabilities are provider
 * estimates, not authorization or correctness guarantees. No tools, retries, or fallback.
 * Cancellation, a 30-second timeout, HTTP errors, refusals, and invalid answers fail the task.
 * Credentials remain in this closure; provider error bodies are never logged. */
export function createChoiceClassifier<Input extends WorkflowSchema, Choice extends string>(
  options: {
    name: string;
    input: Input;
    model: string;
    apiKey: string;
    instructions: string;
    choices: Record<Choice, string>;
  },
  request: typeof fetch = fetch
) {
  if (!options.apiKey?.trim()) throw new Error('An OpenRouter API key is required');
  if (!options.model.trim()) throw new Error('A decision model is required');
  const keys = Object.keys(options.choices) as Choice[];
  if (keys.length < 2) throw new Error('A choice classifier requires at least two choices');
  const choice = Type.Enum(keys);
  // Object.fromEntries loses literal key types; these keys come from `choices` above.
  const probabilities = Type.Object(
    Object.fromEntries(keys.map((key) => [key, probability])) as Record<Choice, typeof probability>,
    { additionalProperties: false }
  );
  const result = Type.Object({
    type: Type.Literal('choice'),
    choice,
    probabilities,
    confidence: probability
  });
  return task({ name: options.name, input: options.input, result }, async (ctx, input) => {
    const signal = AbortSignal.any([ctx.signal, AbortSignal.timeout(30_000)]);
    signal.throwIfAborted();
    const started = performance.now();
    log.info(`Classifier started (model: ${options.model})`);
    const response = await request('https://openrouter.ai/api/alpha/decisions', {
      method: 'POST',
      headers: { Authorization: `Bearer ${options.apiKey}`, 'Content-Type': 'application/json' },
      redirect: 'error',
      signal,
      body: JSON.stringify({
        model: options.model,
        state: input,
        questions: {
          decision: {
            type: 'choice',
            instructions: options.instructions,
            criteria: options.choices
          }
        },
        provider: { allow_fallbacks: false }
      })
    });
    signal.throwIfAborted();
    if (!response.ok) {
      await response.body?.cancel();
      throw new Error(`OpenRouter decision request failed (HTTP ${response.status})`);
    }
    let data: unknown;
    try {
      data = await response.json();
    } catch {
      signal.throwIfAborted();
      throw new Error('Invalid OpenRouter decision response');
    }
    signal.throwIfAborted();
    if (!Check(envelope, data)) throw new Error('Invalid OpenRouter decision response');
    if (data.usage) {
      const usage = {
        input: data.usage.input_tokens,
        output: data.usage.output_tokens,
        cacheRead: 0,
        cacheWrite: 0,
        ...(data.usage.cost === undefined ? {} : { cost: data.usage.cost })
      };
      ctx.recordUsage(usage);
      log.info(
        `Token usage: ${formatTokenUsage(usage)}${usage.cost === undefined ? '' : ` · $${usage.cost.toFixed(8)}`}`
      );
    } else {
      // A valid answer can omit usage. Keep any earlier cost visibly incomplete.
      ctx.recordUsage({ input: 0, output: 0, cacheRead: 0, cacheWrite: 0, costIncomplete: true });
    }
    const answer = data.answers.decision;
    // Validate here to keep invalid provider content out of task validation errors.
    if (!Check(result, answer)) throw new Error('Invalid OpenRouter choice decision');
    log.info(
      `Decision: ${answer.choice} · probabilities: ${JSON.stringify(answer.probabilities)} · confidence: ${answer.confidence} · ${Math.round(performance.now() - started)} ms`
    );
    return answer;
  });
}
