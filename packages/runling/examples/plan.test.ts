import { createTaskContext } from 'runling';
import { vi, describe, expect, test } from 'vitest';
import type { AgentResult } from 'runling';
import { plan } from './plan.ts';

const usage = { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 };

const runtimeWith = (reports: AgentResult[]) => {
  const agentPrompts: string[] = [];
  let agentOptions: Record<string, unknown> | undefined;
  let disposed = false;
  const f = {
    cwd: '/project',
    agent: async function (this: { cwd: string }, options: Record<string, unknown>) {
      agentOptions = { ...options, cwd: options.cwd };
      return {
        id: 'patient-pandas-1234',
        async runOutcome(_ctx: unknown, agentPrompt: string) {
          agentPrompts.push(agentPrompt);
          const report = reports.shift();
          if (report === undefined) throw new Error('No agent report prepared');
          return report;
        },
        dispose() {
          disposed = true;
        },
        async [Symbol.asyncDispose]() {
          disposed = true;
        }
      };
    },
    step: <T>(_label: string, work: () => T) => work()
  } as Record<string, any>;
  mocks.current = f;

  return {
    f,
    agentPrompts,
    get agentOptions() {
      return agentOptions;
    },
    get disposed() {
      return disposed;
    }
  };
};

describe('plan workflow', () => {
  test('interviews through messages until the agent can complete an implementation plan', async () => {
    const runtime = runtimeWith([
      {
        outcome: 'blocked',
        summary: 'Which hosting provider should we target?',
        usage
      },
      {
        outcome: 'blocked',
        summary: 'Should deployments happen automatically after merge?',
        usage
      },
      {
        outcome: 'completed',
        summary: 'Plan deployment support',
        details: '## Plan\n\n1. Add the deployment adapter.\n2. Test it.',
        usage
      }
    ]);
    const answers = ['Fly.io', 'Yes, for main'];

    const interview = createTaskContext().start(plan, {
      directory: runtime.f.cwd ?? '/project',
      prompt: 'Add deployment support'
    });
    const questions: string[] = [];
    for await (const question of interview.updates) {
      questions.push(question);
      await interview.send(answers.shift()!);
    }

    await expect(interview.result).resolves.toBe(
      '## Plan\n\n1. Add the deployment adapter.\n2. Test it.'
    );
    expect(questions).toEqual([
      'Which hosting provider should we target?',
      'Should deployments happen automatically after merge?'
    ]);
    expect(runtime.agentPrompts).toHaveLength(3);
    expect(runtime.agentPrompts[0]).toContain('Add deployment support');
    expect(runtime.agentPrompts[1]).toContain('Fly.io');
    expect(runtime.agentPrompts[2]).toContain('Yes, for main');
    expect(runtime.agentOptions).toMatchObject({
      cwd: '/project',
      model: 'openai-codex/gpt-5.6-sol',
      thinkingLevel: 'medium',
      tools: ['read', 'grep', 'find', 'ls']
    });
    expect(runtime.disposed).toBe(true);
  });

  test('fails when the input closes before an answer arrives', async () => {
    const runtime = runtimeWith([
      { outcome: 'blocked', summary: 'Which hosting provider should we target?', usage }
    ]);

    // A direct call has no parent that answers: its input is closed.
    await expect(
      plan(createTaskContext(), { directory: '/project', prompt: 'Add deployment support' })
    ).rejects.toThrow('The planning interview ended without an answer');
    expect(runtime.disposed).toBe(true);
  });

  test('fails clearly when the planning agent fails', async () => {
    const runtime = runtimeWith([
      {
        outcome: 'failed',
        summary: 'Could not inspect the repository',
        usage
      }
    ]);

    await expect(
      plan(createTaskContext(), {
        directory: runtime.f.cwd ?? '/project',
        prompt: 'Plan something'
      })
    ).rejects.toThrow('Planning failed: Could not inspect the repository');
    expect(runtime.disposed).toBe(true);
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
