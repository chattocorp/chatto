/** Application errors stay private while Runling records operation timing and metadata. */
import { expect, test } from 'vitest';
import { runWorkflow, task, Type, type RunlingEvent } from 'runling';
import { safeStep } from './steps.ts';

test('records steps and metadata without private results or external errors', async () => {
  const events: RunlingEvent[] = [];
  const failure = Object.assign(new Error('private token and message'), { code: 'ECONNREFUSED' });
  const root = task(
    { name: 'Safe operations', input: Type.Null(), result: Type.Null() },
    async () => {
      expect(await safeStep('Read context', async () => 'private result', { messages: 3 })).toBe(
        'private result'
      );
      await expect(
        safeStep('Reply post', async () => {
          throw failure;
        })
      ).rejects.toBe(failure);
      return null;
    }
  );
  expect(
    (await runWorkflow(root, { input: null, onEvent: (event) => events.push(event) })).ok
  ).toBe(true);
  expect(events).toContainEqual(
    expect.objectContaining({ type: 'log', message: 'Read context', fields: { messages: 3 } })
  );
  expect(events).toContainEqual(
    expect.objectContaining({
      type: 'step.finished',
      status: 'completed',
      durationMs: expect.any(Number)
    })
  );
  expect(events).toContainEqual(
    expect.objectContaining({
      type: 'step.finished',
      status: 'failed',
      error: 'Error: Reply post failed. Connection refused.'
    })
  );
  expect(JSON.stringify(events)).not.toContain('private');
});
