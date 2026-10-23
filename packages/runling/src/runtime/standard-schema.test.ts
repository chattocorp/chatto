import { createTaskContext, type TaskContext } from './context.ts';
import { describe, expect, expectTypeOf, test } from 'vitest';
import { z } from 'zod';
import * as v from 'valibot';
import type { StandardSchemaV1 } from '@standard-schema/spec';
import { Type } from 'typebox';
import { task } from './task.ts';
import { isWorkflowSchema, validateSchema } from './schema.ts';
import { observeRunlingEvents, type RunlingEvent } from './events.ts';
import { runWorkflow } from './runner.ts';

describe('Standard Schema tasks', () => {
  test('infers Zod input and output types and applies both transforms', async () => {
    const length = task(
      {
        name: 'Length',
        input: z.string().transform((value) => value.length),
        result: z.number().transform((value) => ({ length: value }))
      },
      (ctx, input) => {
        expectTypeOf(input).toEqualTypeOf<number>();
        return input;
      }
    );
    expectTypeOf<Parameters<typeof length>>().toEqualTypeOf<[TaskContext<any, any>, string]>();
    expectTypeOf<ReturnType<typeof length>>().toEqualTypeOf<Promise<{ length: number }>>();
    await expect(length(createTaskContext(), 'hello')).resolves.toEqual({ length: 5 });
    const execution = await runWorkflow(length, { input: 'hello' });
    expect(execution.ok).toBe(true);
    expect(execution.output).toEqual({ length: 5 });
  });

  test('accepts Valibot and infers its parsed input', async () => {
    const length = task(
      {
        name: 'Length',
        input: v.pipe(
          v.string(),
          v.transform((value) => value.length)
        ),
        result: v.number()
      },
      (ctx, value) => {
        expectTypeOf(value).toEqualTypeOf<number>();
        return value;
      }
    );
    expectTypeOf<Parameters<typeof length>>().toEqualTypeOf<[TaskContext<any, any>, string]>();
    expectTypeOf<ReturnType<typeof length>>().toEqualTypeOf<Promise<number>>();
    await expect(length(createTaskContext(), 'hello')).resolves.toBe(5);
    await expect(length(createTaskContext(), 42 as never)).rejects.toThrow(
      'Task "Length" input is invalid'
    );
  });

  test('supports mixed Standard Schema and TypeBox boundaries', async () => {
    const first = task(
      { name: 'First', input: z.string(), result: Type.String() },
      (ctx, value) => value
    );
    const second = task(
      { name: 'Second', input: Type.String(), result: z.string() },
      (ctx, value) => value
    );
    expectTypeOf<ReturnType<typeof first>>().toEqualTypeOf<Promise<string>>();
    expectTypeOf<ReturnType<typeof second>>().toEqualTypeOf<Promise<string>>();
    await expect(first(createTaskContext(), 'one')).resolves.toBe('one');
    await expect(second(createTaskContext(), 'two')).resolves.toBe('two');
  });

  test('waits for async input validation before calling the implementation', async () => {
    const events: RunlingEvent[] = [];
    let calls = 0;
    const echo = task(
      {
        name: 'Async',
        input: z.string().refine(async (value) => value === 'ok', 'Expected ok'),
        result: z.string()
      },
      (ctx, value) => {
        calls++;
        return value;
      }
    );
    await observeRunlingEvents(
      (event) => events.push(event),
      async () => {
        await expect(echo(createTaskContext(), 'bad')).rejects.toThrow('Expected ok');
        expect(calls).toBe(0);
        await expect(echo(createTaskContext(), 'ok')).resolves.toBe('ok');
      }
    );
    expect(calls).toBe(1);
    expect(events.filter((event) => event.type === 'step.finished')).toMatchObject([
      { status: 'failed' },
      { status: 'completed' }
    ]);
  });

  test('records async output validation failure inside the task activity', async () => {
    const events: RunlingEvent[] = [];
    const echo = task(
      {
        name: 'Async output',
        input: z.string(),
        result: z.string().refine(async () => false, 'Output rejected')
      },
      async (ctx, value) => value
    );
    await observeRunlingEvents(
      (event) => events.push(event),
      async () => {
        await expect(echo(createTaskContext(), 'hello')).rejects.toThrow(
          'Task "Async output" result is invalid'
        );
      }
    );
    expect(events.filter((event) => event.type === 'step.finished')).toMatchObject([
      { status: 'failed' }
    ]);
  });

  test('preserves implementation errors', async () => {
    const error = new Error('failed');
    const echo = task({ name: 'Echo', input: z.string(), result: z.string() }, (ctx, value) => {
      if (value === 'fail') throw error;
      return value;
    });
    await expect(echo(createTaskContext(), 'fail')).rejects.toBe(error);
    await expect(echo(createTaskContext(), 'hello')).resolves.toBe('hello');
  });

  test('normalizes issue paths and accepts callable schema objects', async () => {
    const schema = Object.assign(() => {}, {
      '~standard': {
        version: 1 as const,
        vendor: 'test',
        validate: () => ({ issues: [{ message: 'Invalid', path: [{ key: 'a/b~c' }, 0] }] })
      }
    }) satisfies StandardSchemaV1;
    expect(isWorkflowSchema(schema)).toBe(true);
    expect(await validateSchema(schema, null)).toEqual({
      issues: [{ message: 'Invalid', path: '/a~1b~0c/0' }]
    });
  });

  test.each([
    { version: 2, vendor: 'test', validate: () => ({ value: 'ok' }) },
    { version: 1, validate: () => ({ value: 'ok' }) },
    { version: 1, vendor: 'test' }
  ])('rejects malformed Standard Schema metadata', (standard) => {
    expect(isWorkflowSchema({ '~standard': standard })).toBe(false);
  });

  test('requires the implementation result to match the output schema input', () => {
    // @ts-expect-error The implementation must return a string for the output parser.
    task({ name: 'Invalid', input: z.string(), result: z.string() }, (ctx) => 42);
  });
});

test('does not call a task aborted during asynchronous schema validation', async () => {
  const ctx = createTaskContext();
  const started = Promise.withResolvers<void>();
  const release = Promise.withResolvers<void>();
  const events: RunlingEvent[] = [];
  let called = false;
  const run = task(
    {
      name: 'Validate',
      input: z.string().refine(async () => {
        started.resolve();
        await release.promise;
        return true;
      }),
      result: z.string()
    },
    (_ctx, input) => {
      called = true;
      return input;
    }
  );
  await observeRunlingEvents(
    (event) => events.push(event),
    async () => {
      const result = run(ctx, 'hello');
      await started.promise;
      try {
        ctx.abort('Stop validation');
      } catch {}
      release.resolve();
      await expect(result).rejects.toBe(ctx.signal.reason);
    }
  );
  expect(called).toBe(false);
  expect(events.filter((event) => event.type === 'step.finished')).toMatchObject([
    { status: 'failed' }
  ]);
});
