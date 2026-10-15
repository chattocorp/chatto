import { createTaskContext } from 'runling';
import { vi, describe, expect, test } from 'vitest';

import { joke } from './joke.ts';

describe('joke workflow', () => {
  test('returns an agent-written joke and has it reviewed', async () => {
    const prompts: string[] = [];
    const steps: string[] = [];
    const options: Record<string, unknown>[] = [];
    const f = {
      runAgent: async (prompt: string, agentOptions: Record<string, unknown>) => {
        prompts.push(prompt);
        options.push(agentOptions);
        return {
          outcome: 'completed' as const,
          summary: 'A joke',
          details: 'TypeScript walked into a bar, but JavaScript let it in anyway.',
          usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 }
        };
      },
      step: <T>(name: string, work: () => T) => {
        steps.push(name);
        return work();
      }
    } as Record<string, any>;
    mocks.current = f;

    await expect(
      joke(createTaskContext(), { directory: f.cwd ?? '/project', prompt: 'TypeScript' })
    ).resolves.toBe('# Joke\n\nTypeScript walked into a bar, but JavaScript let it in anyway.');
    expect(steps).toEqual(['Write joke', 'Review joke for funniness']);
    expect(prompts).toHaveLength(2);
    expect(prompts[1]).toContain('TypeScript walked into a bar, but JavaScript let it in anyway.');
    expect(options[1]).toMatchObject({
      model: 'openai-codex/gpt-5.6-sol',
      thinkingLevel: 'low',
      tools: []
    });
    expect(prompts[0]).toContain('"TypeScript"');
    expect(options[0]).toMatchObject({
      model: 'openai-codex/gpt-5.6-sol',
      thinkingLevel: 'low',
      tools: []
    });
  });

  test('uses the summary when the report has no details', async () => {
    const prompts: string[] = [];
    mocks.current = {
      runAgent: async (prompt: string) => {
        prompts.push(prompt);
        return {
          outcome: 'completed' as const,
          summary: 'A webhook joke',
          usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 }
        };
      },
      step: <T>(_name: string, work: () => T) => work()
    };

    await expect(
      joke(createTaskContext(), { directory: '/project', prompt: 'webhooks' })
    ).resolves.toBe('# Joke\n\nA webhook joke');
    expect(prompts[0]).toContain('"webhooks"');
    expect(prompts).toHaveLength(2);
    expect(prompts[1]).toContain('"A webhook joke"');
  });

  test('requires a topic', async () => {
    const runAgent = vi.fn();
    mocks.current = { runAgent, step: <T>(_name: string, work: () => T) => work() };

    await expect(joke(createTaskContext(), { directory: '/project', prompt: '' })).rejects.toThrow(
      'Task "Tell a joke" input is invalid'
    );
    expect(runAgent).not.toHaveBeenCalled();
  });
});

const mocks = vi.hoisted(() => ({ current: {} as Record<string, any> }));
vi.mock('runling', async (importOriginal) => {
  const actual = await importOriginal<typeof import('runling')>();
  return {
    ...actual,
    createAgentSession: (options: unknown) => mocks.current.agent(options),
    runAgent: (_ctx: unknown, ...args: unknown[]) => mocks.current.runAgent(...args),
    step: (name: string, work: () => unknown) => mocks.current.step(name, work),
    log: { info: (message: string) => mocks.current.log?.info(message) },
    exec: (...args: unknown[]) => {
      const command = mocks.current.exec(...args);
      return Object.assign(command, {
        cwd: (directory: string) => {
          expect(directory).toBe(mocks.current.cwd ?? '/project');
          return command;
        }
      });
    }
  };
});
