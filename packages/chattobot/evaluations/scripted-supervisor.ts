/** Test adapter for existing supervisor scripts: their text becomes an explicit final reply.
 * Contract regressions must call finishTurn directly instead of using this adapter. */
import type { AgentExtensionAPI, AgentOptions, RunlingAgent } from 'runling/agents';
import { createChattoBot } from '../workflows/chat.ts';
import type { TurnCompletion } from '../workflows/turn-completion.ts';

type Supervisor = Pick<RunlingAgent, 'runOutcome' | 'steer' | 'dispose'>;

/** Register the real completion tool without running a model or any external tools. */
export async function completionTool(options: Pick<AgentOptions, 'extensions'>) {
  let finish:
    | ((id: string, input: { action: TurnCompletion }, signal?: AbortSignal) => Promise<unknown>)
    | undefined;
  for (const extension of options.extensions ?? []) {
    await (typeof extension === 'function' ? extension : extension.factory)({
      on() {},
      registerTool(tool: { name: string; execute: NonNullable<typeof finish> }) {
        if (tool.name === 'finishTurn') finish = tool.execute;
      }
    } as unknown as AgentExtensionAPI);
  }
  if (!finish) throw new Error('Supervisor did not register finishTurn');
  const execute = finish;
  return (action: TurnCompletion, signal?: AbortSignal) => execute('finish', { action }, signal);
}

/** Keep routing and authorization test scripts independent of the model output protocol. */
export function scriptedSupervisor(factory: (options: AgentOptions) => Promise<Supervisor>) {
  return async (options: AgentOptions): Promise<Supervisor> => {
    const finish = await completionTool(options);
    const bot = await factory(options);
    return {
      steer: (text) => bot.steer(text),
      dispose: () => bot.dispose(),
      async runOutcome(ctx, prompt, runOptions) {
        let reply = '';
        const result = await bot.runOutcome(ctx, prompt, {
          ...runOptions,
          onText: (text) => {
            reply += text;
          }
        });
        if (result.outcome === 'completed')
          await finish(
            reply && reply !== '[NO_UPDATE]' ? { kind: 'reply', text: reply } : { kind: 'silent' },
            runOptions?.signal
          );
        return result;
      }
    };
  };
}

/** A real Chatto router whose injected agent script uses the completion protocol. */
export function scriptedChattoBot(options: Parameters<typeof createChattoBot>[0]) {
  return createChattoBot({
    ...options,
    ...(options.createAgent && { createAgent: scriptedSupervisor(options.createAgent) })
  });
}
