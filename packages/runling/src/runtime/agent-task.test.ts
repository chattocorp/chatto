import { expect, expectTypeOf, test, vi } from 'vitest';
import { Type } from 'typebox';
import { z } from 'zod';
import { agent } from './agent-task.ts';
import { AgentOutcomeError, type AgentOptions, type RunlingAgent } from './agent.ts';
import { createTaskContext } from './context.ts';
import { isTask } from './task.ts';
import { taskTool } from './task-tool.ts';
import { runWorkflow } from './runner.ts';
import { emptyTokenUsage } from './usage.ts';

const settings = { model: 'test/model', cwd: '.' };
const answer = (summary: string) => ({
  outcome: 'completed' as const,
  summary,
  usage: emptyTokenUsage()
});

function session(run: RunlingAgent['runOutcome']): RunlingAgent {
  return {
    id: 'test-agent',
    runOutcome: vi.fn(run),
    run: async () => {
      throw new Error('unused');
    },
    steer: vi.fn(async () => false),
    fork: async () => {
      throw new Error('unused');
    },
    dispose: vi.fn(async () => {}),
    async [Symbol.asyncDispose]() {
      await this.dispose();
    }
  };
}

test('an agent is a lazy reusable task with string input, output, and messages', async () => {
  const instances: RunlingAgent[] = [];
  const factory = vi.fn(async (_options: AgentOptions) => {
    const instance = session(async (_ctx, prompt, options) => {
      options?.onText?.(`Answer: ${prompt}`, { hasToolCalls: false });
      return answer(`Answer: ${prompt}`);
    });
    instances.push(instance);
    return instance;
  });
  const worker = agent({ name: 'Worker', ...settings }, factory);
  expect(isTask(worker)).toBe(true);
  expectTypeOf<ReturnType<typeof worker>>().toEqualTypeOf<Promise<string>>();
  expect(factory).not.toHaveBeenCalled();
  const outputs: unknown[] = [];
  const run = await runWorkflow(worker, {
    input: 'Hello',
    onUpdate: (text) => {
      outputs.push(text);
    }
  });
  expect(run).toMatchObject({ ok: true, output: 'Answer: Hello' });
  expect(outputs).toEqual(['Answer: Hello']);
  expect(await worker(createTaskContext(), 'Again')).toBe('Answer: Again');
  expect(factory).toHaveBeenCalledTimes(2);
  expect(factory).toHaveBeenCalledWith(expect.objectContaining({ ...settings, output: 'text' }));
  expect(instances[0]).not.toBe(instances[1]);
  for (const instance of instances) expect(instance.dispose).toHaveBeenCalledOnce();
});

test('typed input is parsed before setup and prompt, including Standard Schema transforms', async () => {
  const setup = vi.fn(async (_ctx, value: number) => ({
    ...settings,
    instructions: [`Count ${value}`]
  }));
  const instance = session(async (_ctx, prompt) => answer(prompt));
  const factory = vi.fn(async () => instance);
  const worker = agent(
    {
      name: 'Typed',
      input: z.string().transform((value) => value.length),
      setup,
      prompt: (length) => {
        expectTypeOf(length).toEqualTypeOf<number>();
        return `Length: ${length}`;
      }
    },
    factory
  );
  await expect(worker(createTaskContext(), 42 as never)).rejects.toThrow('input is invalid');
  expect(setup).not.toHaveBeenCalled();
  expect(factory).not.toHaveBeenCalled();
  expect(await worker(createTaskContext(), 'hello')).toBe('Length: 5');
  expect(setup).toHaveBeenCalledWith(expect.anything(), 5);
});

test('agent tasks work as tools and forward completed replies as partial results', async () => {
  const instance = session(async (_ctx, prompt, options) => {
    options?.onText?.('Checking', { hasToolCalls: true });
    options?.onText?.('Done', { hasToolCalls: false });
    return answer(prompt);
  });
  const worker = agent(
    {
      name: 'Tool worker',
      input: Type.Object({ question: Type.String() }),
      prompt: ({ question }) => question,
      ...settings
    },
    async () => instance
  );
  const tool = taskTool(createTaskContext(), worker, {
    name: 'worker',
    description: 'Answer a question'
  });
  const update = vi.fn();
  const result = await tool.execute('call', { question: 'Hello' }, undefined, update, {} as never);
  expect(result.content).toEqual([{ type: 'text', text: 'Hello' }]);
  expect(update.mock.calls.map(([value]) => value.content[0].text)).toEqual(['Checking', 'Done']);
  expect(instance.dispose).toHaveBeenCalledOnce();
});

test('open input does not prevent completion and active steering is delivered once', async () => {
  const entered = Promise.withResolvers<void>();
  const finish = Promise.withResolvers<void>();
  const instance = session(async () => {
    entered.resolve();
    await finish.promise;
    return answer('Done');
  });
  vi.mocked(instance.steer).mockImplementation(async () => {
    finish.resolve();
    return true;
  });
  const worker = agent({ name: 'Steered', ...settings }, async () => instance);
  await using invocation = createTaskContext().start(worker, 'Start');
  await entered.promise;
  await expect(invocation.steer(12 as never)).rejects.toThrow('steering is invalid');
  await invocation.steer('Also check closed issues');
  expect(await invocation.result).toBe('Done');
  expect(instance.steer).toHaveBeenCalledExactlyOnceWith('Also check closed issues');
  expect(instance.runOutcome).toHaveBeenCalledTimes(1);
  expect(instance.dispose).toHaveBeenCalledOnce();
});

test('undelivered steering becomes a new turn before the session is disposed', async () => {
  const entered = Promise.withResolvers<void>();
  const finish = Promise.withResolvers<void>();
  const received = Promise.withResolvers<void>();
  const instance = session(async (_ctx, prompt) => {
    if (prompt === 'Start') {
      entered.resolve();
      await finish.promise;
    }
    return answer(prompt);
  });
  vi.mocked(instance.steer).mockImplementation(async () => {
    received.resolve();
    await finish.promise;
    return false;
  });
  const worker = agent({ name: 'Late steering', ...settings }, async () => instance);
  await using invocation = createTaskContext().start(worker, 'Start');
  await entered.promise;
  await invocation.steer('Follow up');
  await received.promise;
  finish.resolve();
  expect(await invocation.result).toBe('Follow up');
  expect(vi.mocked(instance.runOutcome).mock.calls.map(([, prompt]) => prompt)).toEqual([
    'Start',
    'Follow up'
  ]);
  expect(instance.dispose).toHaveBeenCalledOnce();
});

test('failure throws AgentOutcomeError and disposes the session', async () => {
  const instance = session(async () => ({ ...answer('Provider failed'), outcome: 'failed' }));
  const worker = agent({ name: 'Failure', ...settings }, async () => instance);
  await expect(worker(createTaskContext(), 'Start')).rejects.toBeInstanceOf(AgentOutcomeError);
  expect(instance.dispose).toHaveBeenCalledOnce();
});

test('steering queued during setup is retained when the initial turn finishes immediately', async () => {
  const entered = Promise.withResolvers<void>();
  const ready = Promise.withResolvers<void>();
  const instance = session(async (_ctx, prompt) => answer(prompt));
  const worker = agent(
    {
      name: 'Queued steering',
      setup: async () => {
        entered.resolve();
        await ready.promise;
        return settings;
      }
    },
    async () => instance
  );
  await using invocation = createTaskContext().start(worker, 'Start');
  await entered.promise;
  await invocation.steer('One');
  await invocation.steer('Two');
  await invocation.steer('Three');
  ready.resolve();
  expect(await invocation.result).toBe('Three');
  expect(vi.mocked(instance.runOutcome).mock.calls.map(([, prompt]) => prompt)).toEqual([
    'Start',
    'One',
    'Two',
    'Three'
  ]);
});

test('concurrent invocations have independent sessions and cleanup', async () => {
  const entered = Promise.withResolvers<void>();
  const finish = Promise.withResolvers<void>();
  const instances: RunlingAgent[] = [];
  const worker = agent({ name: 'Concurrent', ...settings }, async () => {
    const instance = session(async (_ctx, prompt) => {
      if (prompt === 'Slow') {
        entered.resolve();
        await finish.promise;
      }
      return answer(prompt);
    });
    instances.push(instance);
    return instance;
  });
  const slow = worker(createTaskContext(), 'Slow');
  await entered.promise;
  expect(await worker(createTaskContext(), 'Fast')).toBe('Fast');
  expect(instances[0]!.dispose).not.toHaveBeenCalled();
  expect(instances[1]!.dispose).toHaveBeenCalledOnce();
  finish.resolve();
  expect(await slow).toBe('Slow');
  expect(instances[0]!.dispose).toHaveBeenCalledOnce();
});

test('cancellation during setup does not create a session', async () => {
  const entered = Promise.withResolvers<void>();
  const finish = Promise.withResolvers<void>();
  const factory = vi.fn(async () => session(async () => answer('unused')));
  const worker = agent(
    {
      name: 'Setup',
      setup: async () => {
        entered.resolve();
        await finish.promise;
        return settings;
      }
    },
    factory
  );
  const invocation = createTaskContext().start(worker, 'Start');
  await entered.promise;
  invocation.cancel(new Error('Stop'));
  finish.resolve();
  await invocation[Symbol.asyncDispose]();
  await expect(invocation.result).rejects.toThrow('Stop');
  expect(factory).not.toHaveBeenCalled();
});

test('cancellation during session creation still disposes the created session', async () => {
  const entered = Promise.withResolvers<void>();
  const finish = Promise.withResolvers<void>();
  const instance = session(async () => answer('unused'));
  const worker = agent({ name: 'Session setup', ...settings }, async () => {
    entered.resolve();
    await finish.promise;
    return instance;
  });
  const invocation = createTaskContext().start(worker, 'Start');
  await entered.promise;
  invocation.cancel(new Error('Stop'));
  finish.resolve();
  await invocation[Symbol.asyncDispose]();
  await expect(invocation.result).rejects.toThrow('Stop');
  expect(instance.runOutcome).not.toHaveBeenCalled();
  expect(instance.dispose).toHaveBeenCalledOnce();
});

test('cancellation reaches a running turn and waits for cleanup', async () => {
  const entered = Promise.withResolvers<void>();
  const instance = session(async (_ctx, _prompt, options) => {
    entered.resolve();
    await new Promise((_, reject) =>
      options!.signal!.addEventListener('abort', () => reject(options!.signal!.reason), {
        once: true
      })
    );
    return answer('unused');
  });
  const worker = agent({ name: 'Cancellation', ...settings }, async () => instance);
  const invocation = createTaskContext().start(worker, 'Start');
  await entered.promise;
  invocation.cancel(new Error('Stop'));
  await invocation[Symbol.asyncDispose]();
  await expect(invocation.result).rejects.toThrow('Stop');
  expect(instance.dispose).toHaveBeenCalledOnce();
});

test('update delivery failure fails the task instead of silently losing replies', async () => {
  const instance = session(async (_ctx, _prompt, options) => {
    for (let i = 0; i < 65; i++) options?.onText?.(`Reply ${i}`, { hasToolCalls: false });
    return answer('Done');
  });
  const worker = agent({ name: 'Overflow', ...settings }, async () => instance);
  await using invocation = createTaskContext().start(worker, 'Start');
  await expect(invocation.result).rejects.toThrow('Channel buffer is full');
  expect(instance.dispose).toHaveBeenCalledOnce();
});
