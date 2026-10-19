import { expect, test, vi } from 'vitest';
import {
  createChannel,
  createTaskContext,
  runWorkflow,
  task,
  Type,
  type RunlingEvent
} from 'runling';
import { IncomingMessage, ConversationInput, ConversationStart } from '../chatto/message.ts';
import { IntakeDecision, IntakeInput } from './classify-intake.ts';
import { createThreadWorkflow } from './thread.ts';
import type { FollowUpSnapshot, ReadFollowUp } from '../chatto/follow-up.ts';
import { createFollowUpReader } from '../chatto/follow-up.ts';

const message: IncomingMessage = {
  id: 'post',
  roomId: 'room',
  threadRootId: 'post',
  authorId: 'human',
  body: 'Old report',
  trigger: 'intake'
};
const ping: IncomingMessage = { ...message, id: 'ping', body: 'Please help', trigger: undefined };

function harness(
  decide: () => Promise<boolean>,
  readable = true,
  options: {
    readFollowUp?: ReadFollowUp;
    intervalMs?: number;
    batchMs?: number;
    idleMs?: number;
    ready?: Promise<void>;
    state?: 'waiting' | 'idle';
  } = {}
) {
  const received: IncomingMessage[] = [];
  const batches: IncomingMessage[][] = [];
  const runIds: (string | undefined)[] = [];
  const classify = vi.fn((_input: unknown) => decide());
  const read = vi.fn(async () =>
    readable ? { body: 'Current report', attachmentTypes: ['image/png'] } : undefined
  );
  const stopped = vi.fn();
  const classifier = task(
    { name: 'Fixture classifier', input: IntakeInput, result: IntakeDecision },
    async (ctx, input) => {
      runIds.push(ctx.run?.id);
      const aborted = new Promise<never>((_resolve, reject) =>
        ctx.signal.addEventListener('abort', () => reject(ctx.signal.reason), { once: true })
      );
      try {
        return { engage: await Promise.race([classify(input), aborted]), reason: 'fixture' };
      } finally {
        stopped();
      }
    }
  );
  const conversation = task(
    {
      name: 'Fixture conversation',
      input: ConversationStart,
      result: Type.Null(),
      steering: ConversationInput,
      output: Type.Union([Type.Literal('busy'), Type.Literal('waiting'), Type.Literal('idle')])
    },
    async (ctx, first) => {
      runIds.push(ctx.run?.id);
      if (!first.resume) received.push(first);
      await options.ready;
      await ctx.emit(options.state ?? 'idle');
      for await (const next of ctx.steering) {
        if (Array.isArray(next)) batches.push(next);
        received.push(...(Array.isArray(next) ? next : [next]));
        await ctx.emit(options.state ?? 'idle');
      }
      return null;
    }
  );
  const workflow = createThreadWorkflow({
    classifier,
    conversation,
    read,
    readFollowUp: async () => undefined,
    ...options
  });
  return {
    workflow,
    runIds,
    received,
    batches,
    read,
    classify,
    stopped,
    start: (first = message) => createTaskContext().start(workflow, first)
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
    'Chatto thread',
    'Fixture classifier',
    'Fixture conversation'
  ]);
  expect(steps[0]?.activityId).toBeUndefined();
  expect(steps.slice(1).every((event) => event.activityId === steps[0]?.id)).toBe(true);
});

test('an empty recovery snapshot never repeats the original addressed request', async () => {
  const h = harness(async () => true);
  const messages = createChannel<IncomingMessage>();
  const run = runWorkflow(h.workflow, {
    input: ping,
    state: {},
    run: { id: 'recovered', resumed: true },
    messages
  });
  const next = { ...ping, id: 'fresh' };
  messages.send(next);
  messages.close();
  expect((await run).ok).toBe(true);
  expect(h.received).toEqual([next]);
  expect(h.classify).not.toHaveBeenCalled();
});

test('an expired saved conversation closes on the polling tick without another turn or read', async () => {
  const readFollowUp = vi.fn(async () => undefined);
  const h = harness(async () => true, true, { readFollowUp, intervalMs: 5 });
  const messages = createChannel<IncomingMessage>();
  const result = await runWorkflow(h.workflow, {
    input: message,
    state: { first: message, delivered: [message.id], lastActivity: 0 },
    run: { id: 'expired', resumed: true },
    messages
  });
  messages.close();
  expect(result.ok).toBe(true);
  expect(h.received).toEqual([]);
  expect(h.classify).not.toHaveBeenCalled();
  expect(readFollowUp).not.toHaveBeenCalled();
});

test('a fresh reply queued during recovery renews an expired conversation', async () => {
  const h = harness(async () => true, true, { intervalMs: 5, idleMs: 1000 });
  const messages = createChannel<IncomingMessage>();
  messages.send({ ...ping, id: 'new' });
  const state = { first: message, delivered: [message.id], lastActivity: 0 };
  const run = runWorkflow(h.workflow, {
    input: message,
    state,
    run: { id: 'expired', resumed: true },
    messages
  });
  await vi.waitFor(() => expect(h.received.map((item) => item.id)).toEqual(['new']));
  expect(state.lastActivity).toBeGreaterThan(0);
  messages.close();
  expect((await run).ok).toBe(true);
});

test('a ping queued during metadata lookup is answered even if the report is gone', async () => {
  const h = harness(async () => false);
  const reading = Promise.withResolvers<undefined>();
  h.read.mockImplementation(() => reading.promise);
  await using run = h.start();
  await run.steer(ping);
  reading.resolve(undefined);
  run.closeSteering();
  await run.result;
  expect(h.received).toEqual([ping]);
  expect(h.classify).not.toHaveBeenCalled();
});

test('accepted intake owns a child conversation and forwards later addressed messages', async () => {
  const h = harness(async () => true);
  await using run = h.start();
  await vi.waitFor(() => expect(h.received).toHaveLength(1));
  expect(h.received[0]).toEqual({ ...message, body: 'Current report' });
  await run.steer(ping);
  run.closeSteering();
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
  await run.steer(ping);
  await vi.waitFor(() => expect(h.received).toEqual([ping]));
  expect(h.stopped).toHaveBeenCalledOnce();
  const reply = { ...ping, id: 'reply' };
  await run.steer(reply);
  run.closeSteering();
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

const snapshot = (...ids: string[]): FollowUpSnapshot => ({
  boundary: 'bot-last',
  input: {
    context: [{ role: 'bot', body: 'What version and deployment method do you use?' }]
  },
  messages: ids.map((id) => ({
    message: {
      ...ping,
      id,
      authorId: `author-${id}`,
      body: id,
      trigger: 'follow-up'
    },
    attachmentTypes: []
  }))
});

test('replies received during a turn remain one batch after the bot posts its answer', async () => {
  const ready = Promise.withResolvers<void>();
  const thread = [
    { id: 'question', authorId: 'bot', body: 'What browser and version?', fromViewer: true }
  ];
  const api = {
    readThread: async () => ({ messages: thread, olderOmitted: false }),
    service: () => ({ batchGetUsers: async () => ({ users: [{ user: { id: 'human' } }] }) })
  } as unknown as Parameters<typeof createFollowUpReader>[0];
  const h = harness(async () => true, true, {
    ready: ready.promise,
    readFollowUp: createFollowUpReader(api),
    intervalMs: 60_000,
    batchMs: 5
  });
  await using run = h.start(ping);
  await vi.waitFor(() => expect(h.received).toEqual([ping]));
  for (const [id, body] of [
    ['one', 'Firefox'],
    ['two', '142']
  ] as const) {
    thread.push({ id, body, authorId: 'human', fromViewer: false });
    await run.steer({ ...ping, id, body: 'Untrusted event copy', trigger: 'follow-up' });
  }
  thread.push({ id: 'new-bot-answer', body: 'Still checking.', authorId: 'bot', fromViewer: true });
  ready.resolve();
  await vi.waitFor(() => expect(h.batches).toHaveLength(1));
  expect(h.batches[0]!.map(({ id, body }) => ({ id, body }))).toEqual([
    { id: 'one', body: 'Firefox' },
    { id: 'two', body: '142' }
  ]);
  expect(h.classify).toHaveBeenCalledOnce();
  run.closeSteering();
  await run.result;
});

test('saved pending reply IDs are checked after recovery without another realtime event', async () => {
  const readFollowUp = vi.fn<ReadFollowUp>(async (_message, _signal, pendingIds) =>
    pendingIds?.includes('one') ? snapshot('one') : undefined
  );
  const h = harness(async () => true, true, { readFollowUp, intervalMs: 60_000, batchMs: 5 });
  const state = {
    first: ping,
    delivered: [ping.id],
    pendingFollowUps: ['one'],
    lastActivity: Date.now()
  };
  const messages = createChannel<IncomingMessage>();
  const run = runWorkflow(h.workflow, {
    input: ping,
    state,
    run: { id: 'recovered', resumed: true },
    messages
  });
  try {
    await vi.waitFor(() => expect(h.batches).toHaveLength(1));
    expect(h.received.map(({ id }) => id)).toEqual(['one']);
    expect(state.pendingFollowUps).toEqual([]);
    expect(state.delivered).toContain('one');
  } finally {
    messages.close();
    expect((await run).ok).toBe(true);
  }
});

test('a direct ping cancels a check without consuming the pending ordinary replies', async () => {
  let decisions = 0;
  const h = harness(async () => (++decisions === 1 ? new Promise<boolean>(() => {}) : true), true, {
    readFollowUp: async (_message, _signal, pendingIds) =>
      pendingIds?.includes('one') ? snapshot('one') : undefined,
    intervalMs: 60_000,
    batchMs: 5
  });
  await using run = h.start(ping);
  await run.steer({ ...ping, id: 'one', trigger: 'follow-up' });
  await vi.waitFor(() => expect(h.classify).toHaveBeenCalledOnce());
  await run.steer({ ...ping, id: 'direct' });
  await vi.waitFor(() => expect(h.batches).toHaveLength(1));
  expect(h.received.map(({ id }) => id)).toEqual([ping.id, 'direct', 'one']);
  expect(h.classify).toHaveBeenCalledTimes(2);
  run.closeSteering();
  await run.result;
});

test('realtime replies batch promptly, use a fresh snapshot, and do not go directly to the conversation', async () => {
  const readFollowUp = vi.fn(async () => snapshot('one', 'two'));
  const h = harness(async () => true, true, { readFollowUp, intervalMs: 60_000, batchMs: 10 });
  await using run = h.start(ping);
  await vi.waitFor(() => expect(h.received).toEqual([ping]));
  await run.steer({ ...ping, id: 'one', body: 'unverified event body', trigger: 'follow-up' });
  await run.steer({ ...ping, id: 'two', trigger: 'follow-up' });
  await vi.waitFor(() => expect(h.batches).toHaveLength(1));
  expect(readFollowUp).toHaveBeenCalledOnce();
  expect(h.classify).toHaveBeenCalledExactlyOnceWith({
    context: snapshot().input.context,
    messages: [
      { body: 'one', attachmentTypes: [] },
      { body: 'two', attachmentTypes: [] }
    ]
  });
  expect(h.batches[0]).toEqual(snapshot('one', 'two').messages.map(({ message }) => message));
  await run.steer({ ...ping, id: 'one', trigger: 'follow-up' });
  await new Promise((resolve) => setTimeout(resolve, 25));
  expect(readFollowUp).toHaveBeenCalledOnce();
  expect(h.classify).toHaveBeenCalledOnce();
  run.closeSteering();
  await run.result;
});

test('forwarding an accepted batch does not cancel its completed check', async () => {
  const aborted = vi.fn();
  const h = harness(async () => true, true, {
    readFollowUp: async (_message, signal) => {
      signal.addEventListener('abort', aborted, { once: true });
      return snapshot('one');
    },
    intervalMs: 60_000,
    batchMs: 5
  });
  await using run = h.start(ping);
  await run.steer({ ...ping, id: 'one', trigger: 'follow-up' });
  await vi.waitFor(() => expect(h.batches).toHaveLength(1));
  expect(h.stopped).toHaveBeenCalledOnce();
  expect(aborted).not.toHaveBeenCalled();
  run.closeSteering();
  await run.result;
});

test('realtime replies wait for a busy conversation and then wake the check without polling', async () => {
  const ready = Promise.withResolvers<void>();
  const readFollowUp = vi.fn(async () => snapshot('one'));
  const h = harness(async () => false, true, {
    readFollowUp,
    ready: ready.promise,
    intervalMs: 60_000,
    batchMs: 5
  });
  await using run = h.start(ping);
  await run.steer({ ...ping, id: 'one', trigger: 'follow-up' });
  await new Promise((resolve) => setTimeout(resolve, 20));
  expect(readFollowUp).not.toHaveBeenCalled();
  ready.resolve();
  await vi.waitFor(() => expect(h.classify).toHaveBeenCalledOnce());
  expect(h.received).toEqual([ping]);
  run.closeSteering();
  await run.result;
});

test('ordinary replies cannot bypass pending intake, but an addressed request still can', async () => {
  const h = harness(() => new Promise<boolean>(() => {}));
  await using run = h.start();
  await vi.waitFor(() => expect(h.classify).toHaveBeenCalledOnce());
  await run.steer({ ...ping, id: 'ordinary', trigger: 'follow-up' });
  await new Promise((resolve) => setTimeout(resolve, 20));
  expect(h.received).toEqual([]);
  expect(h.stopped).not.toHaveBeenCalled();
  await run.steer(ping);
  await vi.waitFor(() => expect(h.received).toEqual([ping]));
  run.closeSteering();
  await run.result;
});

test('replies arriving during classification are reconsidered once the current check finishes', async () => {
  let current = snapshot('one');
  const decision = Promise.withResolvers<boolean>();
  const readFollowUp = vi.fn(async () => current);
  const h = harness(() => decision.promise, true, { readFollowUp, intervalMs: 60_000, batchMs: 5 });
  await using run = h.start(ping);
  await run.steer({ ...ping, id: 'one', trigger: 'follow-up' });
  await vi.waitFor(() => expect(h.classify).toHaveBeenCalledOnce());
  current = snapshot('one', 'two');
  await run.steer({ ...ping, id: 'two', trigger: 'follow-up' });
  decision.resolve(false);
  await vi.waitFor(() => expect(h.classify).toHaveBeenCalledTimes(2));
  expect(h.classify).toHaveBeenLastCalledWith({
    context: current.input.context,
    messages: [
      { body: 'one', attachmentTypes: [] },
      { body: 'two', attachmentTypes: [] }
    ]
  });
  expect(h.received).toEqual([ping]);
  run.closeSteering();
  await run.result;
});

test('addressed entry bypasses intake; empty checks never call the model and stop on input close', async () => {
  const readFollowUp = vi.fn(async () => undefined);
  const h = harness(async () => true, true, { readFollowUp, intervalMs: 5 });
  await using run = h.start(ping);
  await vi.waitFor(() => expect(readFollowUp.mock.calls.length).toBeGreaterThan(1));
  expect(h.read).not.toHaveBeenCalled();
  expect(h.classify).not.toHaveBeenCalled();
  run.closeSteering();
  await run.result;
  const reads = readFollowUp.mock.calls.length;
  await new Promise((resolve) => setTimeout(resolve, 25));
  expect(readFollowUp).toHaveBeenCalledTimes(reads);
});

test('unchanged replies are checked once; new replies reconsider the full batch since the bot post', async () => {
  let current = snapshot('one');
  const readFollowUp = vi.fn(async () => current);
  const h = harness(async () => current.messages.length > 1, true, { readFollowUp, intervalMs: 5 });
  await using run = h.start(ping);
  await vi.waitFor(() => expect(readFollowUp.mock.calls.length).toBeGreaterThan(2));
  expect(h.classify).toHaveBeenCalledOnce();
  expect(h.received).toEqual([ping]);
  current = snapshot('one', 'two');
  await vi.waitFor(() =>
    expect(h.batches).toEqual([current.messages.map(({ message }) => message)])
  );
  expect(h.classify).toHaveBeenCalledTimes(2);
  await new Promise((resolve) => setTimeout(resolve, 25));
  expect(h.batches).toHaveLength(1);
  // A new post boundary must not redeliver messages the conversation already received.
  current = { ...current, boundary: 'new-bot-post' };
  const reads = readFollowUp.mock.calls.length;
  await vi.waitFor(() => expect(readFollowUp.mock.calls.length).toBeGreaterThan(reads + 1));
  expect(h.batches).toHaveLength(1);
  expect(h.classify).toHaveBeenCalledTimes(2);
  run.closeSteering();
  await run.result;
});

test('a directly delivered resolution is not polled again; a new reply can reopen the thread', async () => {
  let current: FollowUpSnapshot | undefined;
  const readFollowUp = vi.fn(async () => current);
  const h = harness(async () => true, true, { readFollowUp, intervalMs: 5 });
  await using run = h.start(ping);
  await vi.waitFor(() => expect(h.received).toEqual([ping]));
  const resolution = { ...ping, id: 'resolved', body: 'That issue covers it, thanks' };
  await run.steer(resolution);
  await vi.waitFor(() => expect(h.received).toEqual([ping, resolution]));
  // No new bot post: this is the reaction-only completion from seven-bats-3145.
  current = snapshot('resolved');
  const reads = readFollowUp.mock.calls.length;
  await vi.waitFor(() => expect(readFollowUp.mock.calls.length).toBeGreaterThan(reads + 2));
  expect(h.classify).not.toHaveBeenCalled();
  expect(h.batches).toEqual([]);
  current = snapshot('resolved', 'reopened');
  current.messages[0]!.attachmentTypes = ['image/png'];
  current.messages[1]!.attachmentTypes = ['application/pdf'];
  await vi.waitFor(() => expect(h.batches).toEqual([[current!.messages[1]!.message]]));
  expect(h.classify).toHaveBeenLastCalledWith({
    context: [...current.input.context, { role: 'human', body: 'resolved' }],
    messages: [{ body: 'reopened', attachmentTypes: ['application/pdf'] }]
  });
  current = snapshot('resolved', 'reopened', 'more');
  await vi.waitFor(() => expect(h.batches).toHaveLength(2));
  expect(h.batches[1]).toEqual([current.messages[2]!.message]);
  run.closeSteering();
  await run.result;
});

test('no polling occurs while the conversation is answering', async () => {
  const ready = Promise.withResolvers<void>();
  const readFollowUp = vi.fn(async () => undefined);
  const h = harness(async () => true, true, { readFollowUp, intervalMs: 5, ready: ready.promise });
  await using run = h.start(ping);
  await vi.waitFor(() => expect(h.received).toEqual([ping]));
  await new Promise((resolve) => setTimeout(resolve, 25));
  expect(readFollowUp).not.toHaveBeenCalled();
  ready.resolve();
  await vi.waitFor(() => expect(readFollowUp).toHaveBeenCalled());
  run.closeSteering();
  await run.result;
});

test('a ping cancels an in-flight follow-up classification and reaches the same child immediately', async () => {
  const h = harness(() => new Promise<boolean>(() => {}), true, {
    readFollowUp: async () => snapshot('one'),
    intervalMs: 5
  });
  await using run = h.start(ping);
  await vi.waitFor(() => expect(h.classify).toHaveBeenCalledOnce());
  const direct = { ...ping, id: 'direct', authorId: 'maintainer' };
  await run.steer(direct);
  await vi.waitFor(() => expect(h.received).toEqual([ping, direct]));
  expect(h.stopped).toHaveBeenCalledOnce();
  expect(h.batches).toEqual([]);
  run.closeSteering();
  await run.result;
});

test('failed follow-up classification stays silent and is not retried for unchanged replies', async () => {
  const readFollowUp = vi.fn(async () => snapshot('one'));
  const h = harness(
    async () => {
      throw new Error('provider failed');
    },
    true,
    { readFollowUp, intervalMs: 5 }
  );
  await using run = h.start(ping);
  await vi.waitFor(() => expect(readFollowUp.mock.calls.length).toBeGreaterThan(2));
  expect(h.classify).toHaveBeenCalledOnce();
  expect(h.received).toEqual([ping]);
  run.closeSteering();
  await run.result;
});

test('ignored checks do not keep a conversation alive', async () => {
  const readFollowUp = vi.fn(async () => snapshot('one'));
  const h = harness(async () => false, true, { readFollowUp, intervalMs: 5, idleMs: 100 });
  await using run = h.start(ping);
  await run.result;
  expect(readFollowUp).toHaveBeenCalled();
  expect(h.received).toEqual([ping]);
});

test('the next polling tick expires an inactive conversation before another thread read', async () => {
  const clock = vi.spyOn(Date, 'now').mockReturnValue(0);
  try {
    const readFollowUp = vi.fn(async () => undefined);
    const h = harness(async () => true, true, { readFollowUp, intervalMs: 10, idleMs: 1000 });
    await using run = h.start(ping);
    await vi.waitFor(() => expect(readFollowUp).toHaveBeenCalled());
    const reads = readFollowUp.mock.calls.length;
    clock.mockReturnValue(1000);
    expect(run.status).toBe('running');
    await run.result;
    expect(readFollowUp).toHaveBeenCalledTimes(reads);
    expect(h.classify).not.toHaveBeenCalled();
  } finally {
    clock.mockRestore();
  }
});

test('waiting for a background job permits follow-ups but prevents inactivity expiry', async () => {
  const readFollowUp = vi.fn(async () => snapshot('one'));
  const h = harness(async () => true, true, {
    readFollowUp,
    intervalMs: 5,
    idleMs: 10,
    state: 'waiting'
  });
  await using run = h.start(ping);
  await vi.waitFor(() => expect(readFollowUp.mock.calls.length).toBeGreaterThan(3));
  expect(run.status).toBe('running');
  expect(h.batches).toEqual([snapshot('one').messages.map(({ message }) => message)]);
  const direct = { ...ping, id: 'direct' };
  await run.steer(direct);
  await vi.waitFor(() => expect(h.received.at(-1)).toEqual(direct));
  run.closeSteering();
  await run.result;
});

test('accepted follow-ups reset inactivity; the parent stops checks when the conversation expires', async () => {
  let current: FollowUpSnapshot | undefined;
  const readFollowUp = vi.fn(async () => current);
  const h = harness(async () => true, true, { readFollowUp, intervalMs: 5, idleMs: 200 });
  await using run = h.start(ping);
  await vi.waitFor(() => expect(h.received).toEqual([ping]));
  await new Promise((resolve) => setTimeout(resolve, 100));
  current = snapshot('one');
  await vi.waitFor(() => expect(h.batches).toHaveLength(1), { interval: 5 });
  await new Promise((resolve) => setTimeout(resolve, 100));
  expect(run.status).toBe('running');
  await run.result;
  const reads = readFollowUp.mock.calls.length;
  await new Promise((resolve) => setTimeout(resolve, 25));
  expect(readFollowUp).toHaveBeenCalledTimes(reads);
});

test('cancellation stops a pending follow-up classifier', async () => {
  const h = harness(() => new Promise<boolean>(() => {}), true, {
    readFollowUp: async () => snapshot('one'),
    intervalMs: 5
  });
  await using run = h.start(ping);
  await vi.waitFor(() => expect(h.classify).toHaveBeenCalledOnce());
  run.cancel();
  await expect(run.result).rejects.toThrow();
  await vi.waitFor(() => expect(h.stopped).toHaveBeenCalledOnce());
  expect(h.batches).toEqual([]);
});

test('an inactivity deadline does not close delivery while the child is still working', async () => {
  const ready = Promise.withResolvers<void>();
  const h = harness(async () => true, true, { ready: ready.promise, idleMs: 40, intervalMs: 5 });
  await using run = h.start(ping);
  await vi.waitFor(() => expect(h.received).toEqual([ping]));
  await new Promise((resolve) => setTimeout(resolve, 60));
  expect(run.status).toBe('running');
  const reply = { ...ping, id: 'while-working' };
  await run.steer(reply);
  ready.resolve();
  await vi.waitFor(() => expect(h.received).toEqual([ping, reply]));
  run.closeSteering();
  await run.result;
});
