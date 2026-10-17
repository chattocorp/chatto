/** Shared execution for isolated, tool-free classifiers. Policy and schemas belong to callers. */
import {
  createAgentSession,
  task,
  validateSchema,
  type AgentSessionFactory,
  type SchemaOutput,
  type WorkflowSchema
} from 'runling';
import { usePrivateTempDirectory } from './private-temp.ts';

/** Define one model turn with a strict JSON result. Errors and cancellation never become
 * decisions. Each invocation has no tools, local instructions, or saved session history. */
export function createClassifier<Input extends WorkflowSchema, Output extends WorkflowSchema>(
  options: {
    name: string;
    label: string;
    model: string;
    input: Input;
    output: Output;
    systemPrompt: string;
  },
  createAgent: AgentSessionFactory = createAgentSession
) {
  return task(
    { name: options.name, input: options.input, output: options.output },
    async (ctx, input) => {
      const signal = AbortSignal.any([ctx.signal, AbortSignal.timeout(30_000)]);
      signal.throwIfAborted();
      await using bot = await createAgent({
        model: options.model,
        label: options.label,
        thinkingLevel: 'low',
        output: 'text',
        cwd: await usePrivateTempDirectory(),
        tools: [],
        resources: {
          extensions: false,
          skills: false,
          promptTemplates: false,
          themes: false,
          contextFiles: false
        },
        systemPrompt: options.systemPrompt
      });
      signal.throwIfAborted();
      const result = await bot.run(ctx, JSON.stringify(input), { signal });
      signal.throwIfAborted();
      let decision: unknown;
      try {
        decision = JSON.parse(result.summary);
      } catch {
        throw new Error('Invalid classification decision');
      }
      const validated = await validateSchema(options.output, decision);
      if (validated.issues) throw new Error('Invalid classification decision');
      return validated.value as SchemaOutput<Output>;
    }
  );
}
