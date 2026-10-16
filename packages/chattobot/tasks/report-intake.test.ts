import { expect, test, vi } from 'vitest';
import { createTaskContext, runWorkflow, task, Type, type RunlingEvent } from 'runling';
import { IncomingMessage, ConversationInput } from '../chatto/message.ts';
import { IntakeDecision, IntakeInput } from './classify-intake.ts';
import { createReportIntake } from './report-intake.ts';

const message: IncomingMessage = {
  id: 'post',
  roomId: 'room',
  threadRootId: 'post',
  authorId: 'human',
  body: 'Old report',
  trigger: 'intake'
};
const ping: IncomingMessage = { ...message, id: 'ping', body: 'Please help', trigger: undefined };

function harness(decide: () => Promise<boolean>, readable = true) {
  const received: IncomingMessage[] = [];
  const runIds: (string | undefined)[] = [];
  const classify = vi.fn(decide);
  const read = vi.fn(async () =>
    readable ? { body: 'Current report', attachmentTypes: ['image/png'] } : undefined
  );
  const stopped = vi.fn();
  const classifier = task(
    { name: 'Fixture classifier', input: IntakeInput, output: IntakeDecision },
    async (ctx) => {
      runIds.push(ctx.run?.id);
      const aborted = new Promise<never>((_resolve, reject) =>
        ctx.signal.addEventListener('abort', () => reject(ctx.signal.reason), { once: true })
      );
      try {
        return { engage: await Promise.race([classify(), aborted]), reason: 'fixture' };
      } finally {
        stopped();
      }
    }
  );
  const conversation = task(
    {
      name: 'Fixture conversation',
      input: IncomingMessage,
      output: Type.Null(),
      receives: ConversationInput
    },
    async (ctx, first) => {
      runIds.push(ctx.run?.id);
      received.push(first);
      for await (const next of ctx.inbox) received.push(next);
      return null;
    }
  );
  const workflow = createReportIntake({ classifier, conversation, read });
  return {
    workflow,
    runIds,
    received,
    read,
    classify,
    stopped,
    start: () => createTaskContext().start(workflow, message)
  };
}

test('the host records intake, classifier, and conversation in one run', async () => {
  const h = harness(async () => true);
  const events: RunlingEvent[] = [];
  const result = await runWorkflow(h.workflow, {
    input: message,
    run: { id: 'one-run' },
    onEvent: (event) => events.push(event)
  });
  expect(result.ok).toBe(true);
  expect(h.runIds).toEqual(['one-run', 'one-run']);
  const steps = events.filter((event) => event.type === 'step.started');
  expect(steps.map((event) => event.label)).toEqual([
    'Report intake',
    'Fixture classifier',
    'Fixture conversation'
  ]);
  expect(steps[0]?.activityId).toBeUndefined();
  expect(steps.slice(1).every((event) => event.activityId === steps[0]?.id)).toBe(true);
});

test('a ping queued during metadata lookup is answered even if the report is gone', async () => {
  const h = harness(async () => false);
  const reading = Promise.withResolvers<undefined>();
  h.read.mockImplementation(() => reading.promise);
  await using run = h.start();
  await run.send(ping);
  reading.resolve(undefined);
  run.closeInput();
  await run.result;
  expect(h.received).toEqual([ping]);
  expect(h.classify).not.toHaveBeenCalled();
});

test('accepted intake owns a child conversation and forwards later addressed messages', async () => {
  const h = harness(async () => true);
  await using run = h.start();
  await vi.waitFor(() => expect(h.received).toHaveLength(1));
  expect(h.received[0]).toEqual({ ...message, body: 'Current report' });
  await run.send(ping);
  run.closeInput();
  await expect(run.result).resolves.toBeNull();
  expect(h.received).toEqual([{ ...message, body: 'Current report' }, ping]);
});

test.each(['ignored', 'failed', 'missing'] as const)(
  '%s intake exits without a conversation',
  async (kind) => {
    const h = harness(async () => {
      if (kind === 'failed') throw new Error('provider failed');
      return false;
    }, kind !== 'missing');
    await using run = h.start();
    // The workflow must finish even though its message channel is still open.
    await expect(run.result).resolves.toBeNull();
    expect(h.received).toEqual([]);
    expect(h.classify).toHaveBeenCalledTimes(kind === 'missing' ? 0 : 1);
  }
);

test('a ping during classification cancels the classifier and starts the same workflow conversation', async () => {
  const h = harness(() => new Promise<boolean>(() => {}));
  await using run = h.start();
  await vi.waitFor(() => expect(h.classify).toHaveBeenCalledOnce());
  await run.send(ping);
  await vi.waitFor(() => expect(h.received).toEqual([ping]));
  expect(h.stopped).toHaveBeenCalledOnce();
  const reply = { ...ping, id: 'reply' };
  await run.send(reply);
  run.closeInput();
  await run.result;
  expect(h.received).toEqual([ping, reply]);
});

test('cancelling intake stops classification and prevents a conversation', async () => {
  const h = harness(() => new Promise<boolean>(() => {}));
  await using run = h.start();
  await vi.waitFor(() => expect(h.classify).toHaveBeenCalledOnce());
  run.cancel();
  await expect(run.result).rejects.toThrow();
  expect(h.received).toEqual([]);
});

test('cancelling an accepted workflow stops its child conversation', async () => {
  const h = harness(async () => true);
  await using run = h.start();
  await vi.waitFor(() => expect(h.received).toHaveLength(1));
  run.cancel();
  await expect(run.result).rejects.toThrow();
});
