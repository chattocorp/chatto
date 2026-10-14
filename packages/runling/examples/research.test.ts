import { createTaskContext } from 'runling';
import { vi, describe, expect, test } from 'vitest';

import { research } from './research.ts';

const completedReport = {
  outcome: 'completed' as const,
  summary: 'Bun is a JavaScript runtime and toolkit',
  details: '# Bun\n\nBun is a JavaScript runtime and toolkit. [Source](https://bun.com/docs)',
  usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 }
};

describe('research workflow', () => {
  test('researches the CLI topic with only the web tool', async () => {
    const prompts: string[] = [];
    const options: Record<string, unknown>[] = [];
    const steps: string[] = [];
    const f = {
      prompt: 'The Bun JavaScript runtime',
      runAgent: async (prompt: string, agentOptions: Record<string, unknown>) => {
        prompts.push(prompt);
        options.push(agentOptions);
        return completedReport;
      },
      step: <T>(name: string, work: () => T) => {
        steps.push(name);
        return work();
      }
    } as Record<string, any>;
    mocks.current = f;

    await expect(
      research(createTaskContext(), {
        directory: f.cwd ?? '/project',
        prompt: 'The Bun JavaScript runtime'
      })
    ).resolves.toBe(completedReport.details);
    expect(steps).toEqual(['Researching topic']);
    expect(prompts[0]).toContain('The Bun JavaScript runtime');
    expect(prompts[0]).toContain('multiple relevant sources');
    expect(options[0]).toMatchObject({
      model: 'openai-codex/gpt-5.6-sol',
      thinkingLevel: 'medium',
      tools: ['web_fetch']
    });
  });

  test('requires a topic', async () => {
    const runAgent = vi.fn();
    mocks.current = { runAgent, step: <T>(_name: string, work: () => T) => work() };

    await expect(
      research(createTaskContext(), { directory: '/project', prompt: '' })
    ).rejects.toThrow('Task "Research topic" input is invalid');
    expect(runAgent).not.toHaveBeenCalled();
  });

  test('returns the summary when the report has no details', async () => {
    mocks.current = {
      runAgent: async () => ({ ...completedReport, details: undefined }),
      step: <T>(_name: string, work: () => T) => work()
    };

    await expect(
      research(createTaskContext(), { directory: '/project', prompt: 'Bun' })
    ).resolves.toBe(completedReport.summary);
  });
});

const mocks = vi.hoisted(() => ({ current: {} as Record<string, any> }));
vi.mock('runling', async (importOriginal) => {
  const actual = await importOriginal<typeof import('runling')>();
  return {
    ...actual,
    agent: (options: unknown) => mocks.current.agent(options),
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
