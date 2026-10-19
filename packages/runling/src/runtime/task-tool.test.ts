import { expect, test, vi } from 'vitest';
import { Type } from 'typebox';
import { z } from 'zod';
import type { AgentToolResult, ExtensionToolContext } from '@earendil-works/pi-coding-agent';
import { createTaskContext } from './context.ts';
import { observeRunlingEvents, type RunlingEvent } from './events.ts';
import { task } from './task.ts';
import { taskTool } from './task-tool.ts';

// `taskTool` does not use the extension context that Pi passes to a tool.
const extensionContext = {} as ExtensionToolContext;
const Name = Type.Object({ name: Type.String({ description: 'Who to greet' }) });

const greet = task(
  {
    name: 'Greet',
    input: Name,
    result: Type.String(),
    output: Type.Object({ progress: Type.Number() })
  },
  async (ctx, { name }) => {
    await ctx.emit({ progress: 50 });
    return `Hello, ${name}`;
  }
);

const text = (result: AgentToolResult<unknown>) =>
  result.content.map((part) => (part.type === 'text' ? part.text : '')).join('');

test('uses the task input schema as the tool parameters and the label for logs', () => {
  const tool = taskTool(createTaskContext(), greet, {
    name: 'greet',
    description: 'Greet a person'
  });
  expect(tool).toMatchObject({ name: 'greet', label: 'Greet', description: 'Greet a person' });
  expect(tool.parameters).toBe(Name);
  expect(
    taskTool(createTaskContext(), greet, { name: 'g', description: 'd', label: 'Hi' })
  ).toMatchObject({ label: 'Hi' });
});

test('returns text as it is and reports updates as partial results', async () => {
  const tool = taskTool(createTaskContext(), greet, { name: 'greet', description: 'Greet' });
  const onUpdate = vi.fn();
  const result = await tool.execute('call', { name: 'Ada' }, undefined, onUpdate, extensionContext);
  expect(text(result)).toBe('Hello, Ada');
  expect(onUpdate.mock.calls.map(([update]) => text(update))).toEqual(['{"progress":50}']);
});

test('returns other outputs as JSON', async () => {
  const count = task(
    { name: 'Count', input: Name, result: Type.Object({ length: Type.Number() }) },
    (_ctx, { name }) => ({ length: name.length })
  );
  const tool = taskTool(createTaskContext(), count, { name: 'count', description: 'Count' });
  const result = await tool.execute(
    'call',
    { name: 'Ada' },
    undefined,
    undefined,
    extensionContext
  );
  expect(text(result)).toBe('{"length":3}');
});

test('starts the task as a child with a closed input', async () => {
  const events: RunlingEvent[] = [];
  const drain = task(
    { name: 'Drain', input: Type.Object({}), result: Type.Number() },
    async (ctx) => {
      let count = 0;
      for await (const _ of ctx.steering) count++;
      return count;
    }
  );
  const ctx = createTaskContext();
  const tool = taskTool(ctx, drain, { name: 'drain', description: 'Drain' });
  await observeRunlingEvents(
    (event) => events.push(event),
    async () => {
      const result = await tool.execute('call', {}, undefined, undefined, extensionContext);
      expect(text(result)).toBe('0');
    }
  );
  expect(events.filter((event) => event.type === 'step.started').map((e) => e.label)).toEqual([
    'Drain'
  ]);
});

test('validates the input and propagates task errors', async () => {
  const error = new Error('failed');
  const failing = task({ name: 'Fail', input: Name, result: Type.String() }, () => {
    throw error;
  });
  const ctx = createTaskContext();
  await expect(
    taskTool(ctx, greet, { name: 'greet', description: 'Greet' }).execute(
      'call',
      { name: 1 } as never,
      undefined,
      undefined,
      extensionContext
    )
  ).rejects.toThrow('Task "Greet" input is invalid');
  await expect(
    taskTool(ctx, failing, { name: 'fail', description: 'Fail' }).execute(
      'call',
      { name: 'Ada' },
      undefined,
      undefined,
      extensionContext
    )
  ).rejects.toBe(error);
});

test('cancelling the tool call cancels the task but not the parent or other calls', async () => {
  const ctx = createTaskContext();
  const started = Promise.withResolvers<void>();
  const signals: AbortSignal[] = [];
  const release = Promise.withResolvers<void>();
  const slow = task(
    { name: 'Slow', input: Name, result: Type.String() },
    async (child, { name }) => {
      signals.push(child.signal);
      if (signals.length === 2) started.resolve();
      await release.promise;
      child.signal.throwIfAborted();
      return name;
    }
  );
  const tool = taskTool(ctx, slow, { name: 'slow', description: 'Slow' });
  const turn = new AbortController();
  const a = tool.execute('a', { name: 'a' }, turn.signal, undefined, extensionContext);
  const b = tool.execute(
    'b',
    { name: 'b' },
    new AbortController().signal,
    undefined,
    extensionContext
  );
  await started.promise;
  const reason = new Error('cancel a');
  turn.abort(reason);

  await expect(a).rejects.toBe(reason);
  expect(signals[0]?.aborted).toBe(true);
  release.resolve();
  expect(text(await b)).toBe('b');
  expect(signals[1]?.aborted).toBe(false);
  expect(ctx.signal.aborted).toBe(false);
});

test('rejects a pre-cancelled tool call before the task runs', async () => {
  const run = vi.fn(() => 'unused');
  const unused = task({ name: 'Unused', input: Name, result: Type.String() }, run);
  const tool = taskTool(createTaskContext(), unused, { name: 'unused', description: 'Unused' });
  const reason = new Error('cancelled');
  await expect(
    tool.execute('call', { name: 'Ada' }, AbortSignal.abort(reason), undefined, extensionContext)
  ).rejects.toBe(reason);
  expect(run).not.toHaveBeenCalled();
});

test('requires a TypeBox input schema', () => {
  const standard = task(
    { name: 'Standard', input: z.object({ name: z.string() }), result: z.string() },
    (_ctx, { name }) => name
  );
  expect(() =>
    taskTool(createTaskContext(), standard, { name: 'standard', description: 'Standard' })
  ).toThrow(new TypeError('Task "Standard" needs a TypeBox input schema for a tool'));
});
