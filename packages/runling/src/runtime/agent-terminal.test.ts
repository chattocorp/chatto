/** Real Pi session and tool loop with a local provider stream: no network or credentials. */
import { mkdir, mkdtemp, rm } from 'node:fs/promises';
import { resolve } from 'node:path';
import { expect, test, vi } from 'vitest';
import { ModelRuntime } from '@earendil-works/pi-coding-agent';
import { createAssistantMessageEventStream, type AssistantMessage } from '@earendil-works/pi-ai';
import { Type } from 'typebox';
import { agent, defineAgentExtension } from './agent.ts';
import { createWorkflowContext } from './context.ts';

test('terminal tool ends a real mixed batch and the next interaction still runs', async () => {
  const scratch = resolve('.context');
  await mkdir(scratch, { recursive: true });
  const cwd = await mkdtemp(resolve(scratch, 'terminal-'));
  const runtime = await ModelRuntime.create({
    authPath: resolve(cwd, 'auth.json'),
    modelsPath: null,
    refreshOnCreate: false
  });
  const create = vi.spyOn(ModelRuntime, 'create').mockResolvedValue(runtime);
  const configured = vi.spyOn(runtime, 'hasConfiguredAuth').mockReturnValue(true);
  const auth = vi.spyOn(runtime, 'getAuth').mockResolvedValue({ auth: { apiKey: 'synthetic' } });
  let requests = 0;
  let selected = false;
  const effects: string[] = [];
  const stream = vi.spyOn(runtime, 'streamSimple').mockImplementation((model) => {
    requests++;
    const calls = requests === 1 ? ['work', 'finish', 'work'] : ['finish'];
    if (requests > 2) throw new Error('Unexpected provider continuation');
    const message: AssistantMessage = {
      role: 'assistant',
      api: model.api,
      provider: model.provider,
      model: model.id,
      content: calls.map((name, index) => ({
        type: 'toolCall',
        id: `${requests}-${index}`,
        name,
        arguments: {}
      })),
      stopReason: 'toolUse',
      timestamp: Date.now(),
      usage: {
        input: 1,
        output: 1,
        cacheRead: 0,
        cacheWrite: 0,
        totalTokens: 2,
        cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 }
      }
    };
    const events = createAssistantMessageEventStream();
    events.push({ type: 'start', partial: message });
    events.push({ type: 'done', reason: 'toolUse', message });
    return events;
  });
  let bot: Awaited<ReturnType<typeof agent>> | undefined;
  try {
    bot = await agent({
      cwd,
      model: 'openai/gpt-4o',
      output: 'text',
      tools: ['work', 'finish'],
      terminalTools: ['finish'],
      resources: {
        agentDir: cwd,
        extensions: false,
        skills: false,
        promptTemplates: false,
        themes: false,
        contextFiles: false
      },
      extensions: [
        defineAgentExtension((pi) => {
          pi.on('tool_call', () =>
            selected ? { block: true, reason: 'Already finished' } : undefined
          );
          pi.registerTool({
            name: 'work',
            label: 'Work',
            description: 'Record an effect',
            parameters: Type.Object({}),
            async execute() {
              effects.push('work');
              return { content: [], details: {} };
            }
          });
          pi.registerTool({
            name: 'finish',
            label: 'Finish',
            description: 'Select result',
            parameters: Type.Object({}),
            executionMode: 'sequential',
            exposure: 'model-only',
            async execute() {
              selected = true;
              return { content: [], details: {}, terminate: true };
            }
          });
        })
      ]
    });
    expect((await bot.runOutcome(createWorkflowContext(), 'Run the batch')).outcome).toBe(
      'completed'
    );
    expect(requests).toBe(1);
    expect(effects).toEqual(['work']);
    selected = false;
    expect((await bot.runOutcome(createWorkflowContext(), 'Next interaction')).outcome).toBe(
      'completed'
    );
    expect(requests).toBe(2);
  } finally {
    bot?.dispose();
    stream.mockRestore();
    auth.mockRestore();
    configured.mockRestore();
    create.mockRestore();
    await rm(cwd, { recursive: true, force: true });
  }
});
