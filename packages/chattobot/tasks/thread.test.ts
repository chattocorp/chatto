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
import { createThreadWorkflow, type ThreadOptions } from './thread.ts';
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
  decide: () => Promise<boolean | IntakeDecision>,
  readable = true,
  options: {
    addressed?: (input: unknown) => Promise<IntakeDecision>;
    react?: ThreadOptions['react'];
    readFollowUp?: ReadFollowUp;
    readContext?: ThreadOptions['readContext'];
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
  const read = vi.fn(async (item: IncomingMessage, _signal: AbortSignal) =>
    readable
      ? {
          body: item.trigger === 'intake' ? 'Current report' : item.body,
          attachmentTypes: ['image/png']
        }
      : undefined
  );
  const react = vi.fn(options.react ?? (async () => {}));
  const stopped = vi.fn();
  const classifier = task(
    { name: 'Fixture classifier', input: IntakeInput, result: IntakeDecision },
    async (ctx, input) => {
      runIds.push(ctx.run?.id);
      if ('addressed' in input && input.addressed)
        return options.addressed ? await options.addressed(input) : { action: 'engage' as const };
      const aborted = new Promise<never>((_resolve, reject) =>
        ctx.signal.addEventListener('abort', () => reject(ctx.signal.reason), { once: true })
      );
      try {
        const result = await Promise.race([classify(input), aborted]);
        return typeof result === 'boolean'
          ? { action: result ? ('engage' as const) : ('ignore' as const), reason: 'fixture' }
          : result;
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
      if (!first.resume) {
        received.push(...(first.messages ?? [first]));
        if (first.messages) batches.push(first.messages);
      }
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
    readContext: async () => [],
    readFollowUp: async () => undefined,
    batchMs: 0,
    ...options,
    react
  });
  return {
    workflow,
    runIds,
    received,
    batches,
    read,
    classify,
    stopped,
    react,
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
  h.read.mockImplementation((item) =>
    item.trigger === 'intake'
      ? reading.promise
      : Promise.resolve({ body: item.body, attachmentTypes: [] })
  );
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

test.each([false, true])(
  'a ping followed by no bot text still classifies ordinary replies (engage=%s)',
  async (engage) => {
    const thread = [{ id: ping.id, authorId: 'human', body: ping.body, fromViewer: false }];
    const api = {
      readThread: async () => ({ messages: thread, olderOmitted: false }),
      service: () => ({ batchGetUsers: async () => ({ users: [{ user: { id: 'human' } }] }) })
    } as unknown as Parameters<typeof createFollowUpReader>[0];
    const h = harness(async () => engage, true, {
      readFollowUp: createFollowUpReader(api),
      intervalMs: 60_000,
      batchMs: 5
    });
    await using run = h.start(ping);
    await vi.waitFor(() => expect(h.received).toEqual([ping]));
    thread.push({ id: 'cool', authorId: 'human', body: 'cool :D', fromViewer: false });
    await run.steer({ ...ping, id: 'cool', body: 'Untrusted event copy', trigger: 'follow-up' });
    await vi.waitFor(() => expect(h.classify).toHaveBeenCalledOnce());
    expect(h.classify.mock.calls[0]![0]).toMatchObject({
      context: [{ role: 'human', body: ping.body }],
      messages: [{ body: 'cool :D', attachmentTypes: [] }]
    });
    if (engage) {
      await vi.waitFor(() => expect(h.received).toHaveLength(2));
      expect(h.received[1]).toMatchObject({ id: 'cool', body: 'cool :D', trigger: 'follow-up' });
    } else expect(h.received).toEqual([ping]);
    run.closeSteering();
    await run.result;
  }
);

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

test('the first ping and another addressed reply start one conversation batch', async () => {
  const h = harness(async () => true, true, { batchMs: 40 });
  await using run = h.start(ping);
  const reply = { ...ping, id: 'more', authorId: 'another-user' };
  await run.steer(reply);
  expect(h.received).toEqual([]);
  await vi.waitFor(() => expect(h.batches).toEqual([[ping, reply]]));
  expect(h.classify).not.toHaveBeenCalled();
  run.closeSteering();
  await run.result;
});

test('later pings and direct replies reach the existing conversation as one batch', async () => {
  const h = harness(async () => true, true, { batchMs: 40 });
  await using run = h.start(ping);
  await vi.waitFor(() => expect(h.received).toEqual([ping]));
  const one = { ...ping, id: 'one' };
  const two = { ...ping, id: 'two', authorId: 'another-user' };
  await run.steer(one);
  await run.steer(two);
  expect(h.received).toEqual([ping]);
  await vi.waitFor(() => expect(h.batches).toEqual([[one, two]]));
  expect(h.classify).not.toHaveBeenCalled();
  run.closeSteering();
  await run.result;
});

test('ordinary replies classify at the end of the inbox window without another delay', async () => {
  vi.useFakeTimers();
  try {
    const h = harness(async () => true, true, {
      batchMs: 3_000,
      intervalMs: 1_000,
      readFollowUp: async () => snapshot('one')
    });
    await using run = h.start(ping);
    await vi.advanceTimersByTimeAsync(3_000);
    expect(h.received).toEqual([ping]);
    await run.steer({ ...ping, id: 'one', trigger: 'follow-up' });
    await vi.advanceTimersByTimeAsync(2_999);
    expect(h.classify).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    expect(h.batches).toHaveLength(1);
    expect(h.classify).toHaveBeenCalledOnce();
    run.closeSteering();
    await run.result;
  } finally {
    vi.useRealTimers();
  }
});

test('a negative intake decision waits for a ping already in its batching window', async () => {
  const decision = Promise.withResolvers<boolean>();
  const h = harness(() => decision.promise, true, { batchMs: 40 });
  await using run = h.start();
  await vi.waitFor(() => expect(h.classify).toHaveBeenCalledOnce());
  await run.steer(ping);
  decision.resolve(false);
  await vi.waitFor(() => expect(h.received).toEqual([ping]));
  run.closeSteering();
  await run.result;
});

test('a reply arriving during a thread read waits for its window and wakes a fresh check', async () => {
  vi.useFakeTimers();
  try {
    const read = Promise.withResolvers<FollowUpSnapshot>();
    const readFollowUp = vi
      .fn<ReadFollowUp>()
      .mockImplementationOnce(() => read.promise)
      .mockResolvedValue(snapshot('one', 'two'));
    const h = harness(async () => true, true, { batchMs: 3_000, intervalMs: 60_000, readFollowUp });
    await using run = h.start(ping);
    await vi.advanceTimersByTimeAsync(3_000);
    await run.steer({ ...ping, id: 'one', trigger: 'follow-up' });
    await vi.advanceTimersByTimeAsync(3_000);
    expect(readFollowUp).toHaveBeenCalledOnce();
    await run.steer({ ...ping, id: 'two', trigger: 'follow-up' });
    read.resolve(snapshot('one', 'two'));
    await vi.advanceTimersByTimeAsync(2_999);
    expect(h.classify).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    expect(h.batches).toEqual([snapshot('one', 'two').messages.map(({ message }) => message)]);
    expect(h.classify).toHaveBeenCalledOnce();
    run.closeSteering();
    await run.result;
  } finally {
    vi.useRealTimers();
  }
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

test('addressed entry is read once; empty follow-up checks do not call the model', async () => {
  const readFollowUp = vi.fn(async () => undefined);
  const h = harness(async () => true, true, { readFollowUp, intervalMs: 5 });
  await using run = h.start(ping);
  await vi.waitFor(() => expect(readFollowUp.mock.calls.length).toBeGreaterThan(1));
  expect(h.read).toHaveBeenCalledOnce();
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

test('a batched ping cancels an in-flight follow-up classification and reaches the same child', async () => {
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

/** The reacted-to text is a completed answer, not a new request from its author. */
const appreciation: IncomingMessage = {
  ...ping,
  id: 'reaction-thanks',
  body: '+1',
  reaction: {
    emoji: '+1',
    messageId: 'bot-answer',
    body: '#1013 — Invite server member to room covers this request.'
  }
};

test('appreciation on a completed answer does not open a conversation', async () => {
  const context = [{ role: 'bot' as const, body: appreciation.reaction!.body }];
  const h = harness(async () => false, true, { readContext: async () => context });
  await using run = h.start(appreciation);
  await run.result;
  expect(h.received).toEqual([]);
  expect(h.read).not.toHaveBeenCalled();
  expect(h.classify).toHaveBeenCalledExactlyOnceWith({ context, reaction: appreciation.reaction });
});

test.each(['maintainer', 'human'])(
  'accepted proposal reactions retain the %s identity, event ID, and exact target',
  async (authorId) => {
    const approval = {
      ...appreciation,
      authorId,
      reaction: { ...appreciation.reaction!, body: 'Should I file the image upload issue?' }
    };
    const h = harness(async () => true);
    await using run = h.start(approval);
    await vi.waitFor(() => expect(h.received).toEqual([approval]));
    run.closeSteering();
    await run.result;
  }
);

test.each([false, true])(
  'mixed opening batches keep text and only accepted reactions (%s)',
  async (engage) => {
    const h = harness(async () => engage, true, { batchMs: 20 });
    await using run = h.start(appreciation);
    await run.steer(ping);
    await vi.waitFor(() => expect(h.received).toEqual(engage ? [appreciation, ping] : [ping]));
    expect(h.classify).toHaveBeenCalledOnce();
    run.closeSteering();
    await run.result;
  }
);

test('an active thread checks each reaction once and does not forward ignored reactions in mixed batches', async () => {
  let engage = false;
  const h = harness(async () => engage, true, { batchMs: 10 });
  await using run = h.start(ping);
  await vi.waitFor(() => expect(h.received).toEqual([ping]));
  const next = { ...ping, id: 'next' };
  await run.steer(appreciation);
  await run.steer(appreciation);
  await run.steer(next);
  await vi.waitFor(() => expect(h.received).toEqual([ping, next]));
  expect(h.classify).toHaveBeenCalledOnce();
  engage = true;
  const approval = { ...appreciation, id: 'approval' };
  await run.steer(appreciation);
  await run.steer(approval);
  await run.steer(approval);
  await vi.waitFor(() => expect(h.received).toEqual([ping, next, approval]));
  expect(h.classify).toHaveBeenCalledTimes(2);
  run.closeSteering();
  await run.result;
});

test.each(['context', 'classifier'])(
  'reaction %s failure preserves explicit messages and consumes the reaction',
  async (failure) => {
    const fail = async (): Promise<never> => {
      throw new Error('fixture unavailable');
    };
    const h = harness(failure === 'classifier' ? fail : async () => true, true, {
      batchMs: 10,
      readContext: failure === 'context' ? fail : async () => []
    });
    await using run = h.start(appreciation);
    await run.steer(ping);
    await vi.waitFor(() => expect(h.received).toEqual([ping]));
    await run.steer(appreciation);
    run.closeSteering();
    await run.result;
    expect(h.received).toEqual([ping]);
    expect(h.classify).toHaveBeenCalledTimes(failure === 'classifier' ? 1 : 0);
  }
);

test('a ping received during a rejected opening reaction check is not lost', async () => {
  const decision = Promise.withResolvers<boolean>();
  const h = harness(() => decision.promise);
  await using run = h.start(appreciation);
  await vi.waitFor(() => expect(h.classify).toHaveBeenCalledOnce());
  await run.steer(ping);
  decision.resolve(false);
  await vi.waitFor(() => expect(h.received).toEqual([ping]));
  run.closeSteering();
  await run.result;
});

test('recovery does not repeat checked reactions and still classifies fresh ones', async () => {
  const h = harness(async () => true);
  const messages = createChannel<IncomingMessage>();
  const run = runWorkflow(h.workflow, {
    input: ping,
    state: {
      first: ping,
      delivered: [ping.id],
      checkedReactions: [appreciation.id],
      lastActivity: Date.now()
    },
    run: { id: 'reaction-recovery', resumed: true },
    messages
  });
  const fresh = { ...appreciation, id: 'fresh-reaction' };
  messages.send(appreciation);
  messages.send(fresh);
  await vi.waitFor(() => expect(h.received).toEqual([fresh]));
  messages.close();
  expect((await run).ok).toBe(true);
  expect(h.classify).toHaveBeenCalledOnce();
});

test('one accepted reaction does not admit appreciation from the same window', async () => {
  const decisions = [false, true];
  const h = harness(async () => decisions.shift()!, true, { batchMs: 20 });
  await using run = h.start(ping);
  await vi.waitFor(() => expect(h.received).toEqual([ping]));
  const approval = {
    ...appreciation,
    id: 'proposal-approval',
    reaction: { ...appreciation.reaction!, body: 'Should I file the image issue?' }
  };
  const next = { ...ping, id: 'new-question' };
  await run.steer(appreciation);
  await run.steer(approval);
  await run.steer(next);
  await vi.waitFor(() => expect(h.received).toEqual([ping, approval, next]));
  expect(h.batches).toEqual([[approval, next]]);
  run.closeSteering();
  await run.result;
});

test('an ignored reaction cannot suppress the report whose intake it interrupted', async () => {
  const decision = Promise.withResolvers<boolean>();
  let calls = 0;
  const h = harness(async () => {
    calls++;
    if (calls === 1) return decision.promise;
    return calls === 3;
  });
  await using run = h.start();
  await vi.waitFor(() => expect(h.classify).toHaveBeenCalledOnce());
  await run.steer(appreciation);
  await vi.waitFor(() => expect(h.received).toEqual([{ ...message, body: 'Current report' }]));
  expect(h.classify).toHaveBeenCalledTimes(3);
  decision.resolve(false);
  run.closeSteering();
  await run.result;
});

test('a ping during retried intake after an ignored reaction is not discarded', async () => {
  const retry = Promise.withResolvers<boolean>();
  let calls = 0;
  const h = harness(async () => {
    calls++;
    if (calls === 1) return new Promise<boolean>(() => {});
    if (calls === 2) return false;
    return retry.promise;
  });
  await using run = h.start();
  await vi.waitFor(() => expect(h.classify).toHaveBeenCalledOnce());
  await run.steer(appreciation);
  await vi.waitFor(() => expect(h.classify).toHaveBeenCalledTimes(3));
  await run.steer(ping);
  retry.resolve(false);
  await vi.waitFor(() => expect(h.received).toEqual([ping]));
  run.closeSteering();
  await run.result;
});

test('an ignored reaction resumes a pending ordinary reply check without waiting for reconciliation', async () => {
  const reactionDecision = Promise.withResolvers<boolean>();
  const reading = Promise.withResolvers<FollowUpSnapshot>();
  let calls = 0;
  const h = harness(async () => (++calls === 1 ? reactionDecision.promise : true), true, {
    readFollowUp: async () => reading.promise,
    batchMs: 10,
    intervalMs: 60_000
  });
  await using run = h.start(ping);
  await vi.waitFor(() => expect(h.received).toEqual([ping]));
  await run.steer(appreciation);
  await run.steer(snapshot('one').messages[0]!.message);
  await vi.waitFor(() => expect(h.classify).toHaveBeenCalledOnce());
  reading.resolve(snapshot('one'));
  await new Promise<void>((resolve) => setImmediate(resolve));
  reactionDecision.resolve(false);
  await vi.waitFor(() => expect(h.received.map((item) => item.id)).toEqual(['ping', 'one']));
  run.closeSteering();
  await run.result;
});

test.each(['like', 'love', 'smile'] as const)(
  'addressed %s reacts without starting the conversation',
  async (action) => {
    const addressed = vi.fn(async () => ({ action }));
    const h = harness(async () => true, true, { addressed });
    const run = h.start({ ...ping, body: 'Thanks!' });
    run.closeSteering();
    await run.result;
    expect(addressed).toHaveBeenCalledWith(
      expect.objectContaining({
        addressed: true,
        messages: [{ body: 'Thanks!', attachmentTypes: ['image/png'] }]
      })
    );
    expect(h.received).toEqual([]);
    expect(h.react).toHaveBeenCalledExactlyOnceWith(
      expect.objectContaining({ id: 'ping' }),
      { like: '+1', love: 'heart', smile: 'smile' }[action],
      expect.any(AbortSignal)
    );
  }
);

test('an unrelated direct message is ignored', async () => {
  const h = harness(async () => true, true, { addressed: async () => ({ action: 'ignore' }) });
  const run = h.start(ping);
  run.closeSteering();
  await run.result;
  expect(h.received).toEqual([]);
  expect(h.react).not.toHaveBeenCalled();
});

test('an addressed classifier outage forwards verified text', async () => {
  const h = harness(async () => true, true, {
    addressed: async () => {
      throw new Error('outage');
    }
  });
  const run = h.start(ping);
  run.closeSteering();
  await run.result;
  expect(h.received).toEqual([ping]);
});

test('a reaction write failure does not start an agent or retry', async () => {
  const h = harness(async () => true, true, {
    addressed: async () => ({ action: 'love' }),
    react: async () => {
      throw new Error('uncertain write');
    }
  });
  const run = h.start(ping);
  await run.steer(ping);
  run.closeSteering();
  await run.result;
  expect(h.react).toHaveBeenCalledOnce();
  expect(h.received).toEqual([]);
});

test('a substantive addressed batch keeps each message and author', async () => {
  const addressed = vi.fn(async () => ({ action: 'engage' as const }));
  const h = harness(async () => true, true, { addressed, batchMs: 5 });
  const thanks = { ...ping, body: 'Thanks!' };
  const question = {
    ...ping,
    id: 'question',
    authorId: 'other',
    body: 'Can you explain how to upgrade?'
  };
  const run = h.start(thanks);
  await run.steer(question);
  run.closeSteering();
  await run.result;
  expect(addressed).toHaveBeenCalledOnce();
  expect(h.received).toEqual([thanks, question]);
  expect(h.react).not.toHaveBeenCalled();
});

test('a consumed acknowledgment is not reacted to after recovery', async () => {
  const h = harness(async () => true, true, { addressed: async () => ({ action: 'love' }) });
  const messages = createChannel<IncomingMessage>();
  const run = runWorkflow(h.workflow, {
    input: ping,
    state: { first: ping, delivered: [ping.id], consumed: ['thanks'], lastActivity: Date.now() },
    run: { id: 'ack-recovery', resumed: true },
    messages
  });
  messages.send({ ...ping, id: 'thanks' });
  messages.close();
  expect((await run).ok).toBe(true);
  expect(h.react).not.toHaveBeenCalled();
  expect(h.received).toEqual([]);
});

test('ordinary thanks reacts once and does not steer the existing agent', async () => {
  const readFollowUp = vi.fn(async () => snapshot('thanks'));
  const h = harness(async () => ({ action: 'like' }), true, { readFollowUp, intervalMs: 5 });
  await using run = h.start(ping);
  await vi.waitFor(() => expect(h.react).toHaveBeenCalledOnce());
  expect(h.react).toHaveBeenCalledWith(
    expect.objectContaining({ id: 'thanks', trigger: 'follow-up' }),
    '+1',
    expect.any(AbortSignal)
  );
  expect(h.received).toEqual([ping]);
  await run.steer({ ...ping, id: 'thanks', trigger: 'follow-up' });
  await vi.waitFor(() => expect(readFollowUp.mock.calls.length).toBeGreaterThan(2));
  expect(h.react).toHaveBeenCalledOnce();
  expect(h.classify).toHaveBeenCalledOnce();
  run.closeSteering();
  await run.result;
});

test('an ignored direct message cancels an obsolete follow-up and permits a fresh check', async () => {
  const addressed = Promise.withResolvers<IntakeDecision>();
  let calls = 0;
  const h = harness(async () => (++calls === 1 ? new Promise<boolean>(() => {}) : true), true, {
    addressed: async () => (h.received.length ? addressed.promise : { action: 'engage' }),
    readFollowUp: async (_first, _signal, pending) =>
      pending?.includes('answer') ? snapshot('answer') : undefined,
    intervalMs: 60_000
  });
  await using run = h.start(ping);
  await vi.waitFor(() => expect(h.received).toEqual([ping]));
  await run.steer({ ...ping, id: 'answer', trigger: 'follow-up' });
  await vi.waitFor(() => expect(h.classify).toHaveBeenCalledOnce());
  await run.steer({ ...ping, id: 'irrelevant' });
  await vi.waitFor(() => expect(h.stopped).toHaveBeenCalledOnce());
  addressed.resolve({ action: 'ignore' });
  await vi.waitFor(() => expect(h.received.map((item) => item.id)).toEqual(['ping', 'answer']));
  expect(h.classify).toHaveBeenCalledTimes(2);
  run.closeSteering();
  await run.result;
});

test('a completed follow-up decision waits for an unfinished direct window without consuming replies', async () => {
  const decision = Promise.withResolvers<boolean>();
  let checks = 0;
  const h = harness(async () => (++checks === 1 ? decision.promise : true), true, {
    addressed: async () => ({ action: h.received.length ? 'ignore' : 'engage' }),
    readFollowUp: async (_first, _signal, pending) =>
      pending?.includes('answer') ? snapshot('answer') : undefined,
    intervalMs: 60_000,
    batchMs: 40
  });
  await using run = h.start(ping);
  await vi.waitFor(() => expect(h.received).toEqual([ping]));
  await run.steer({ ...ping, id: 'answer', trigger: 'follow-up' });
  await vi.waitFor(() => expect(h.classify).toHaveBeenCalledOnce());
  await run.steer({ ...ping, id: 'unrelated' });
  decision.resolve(true);
  await vi.waitFor(() => expect(h.received.map((item) => item.id)).toEqual(['ping', 'answer']));
  expect(h.classify).toHaveBeenCalledTimes(2);
  run.closeSteering();
  await run.result;
});

test.each(['first', 'middle'])(
  'a %s metadata read failure does not discard later addressed messages',
  async (position) => {
    const h = harness(async () => true, true, { batchMs: 5 });
    h.read.mockImplementation(async (item) => {
      if (item.id === 'failed') throw new Error('transient read failure');
      return { body: item.body, attachmentTypes: [] };
    });
    const before = { ...ping, id: 'before' };
    const after = { ...ping, id: 'after' };
    const failed = { ...ping, id: 'failed' };
    const batch = position === 'first' ? [failed, after] : [before, failed, after];
    const run = h.start(batch[0]!);
    for (const item of batch.slice(1)) await run.steer(item);
    run.closeSteering();
    await run.result;
    expect(h.read.mock.calls.map(([item]) => item.id)).toEqual(batch.map((item) => item.id));
    expect(h.received).toEqual(batch.filter((item) => item.id !== 'failed'));
  }
);

test('cancellation during message verification stops the rest of the batch', async () => {
  const h = harness(async () => true, true, { batchMs: 5 });
  h.read.mockImplementation(
    async (_item, signal) =>
      new Promise((_resolve, reject) => {
        signal.addEventListener('abort', () => reject(signal.reason), { once: true });
      })
  );
  const run = h.start(ping);
  await run.steer({ ...ping, id: 'next' });
  await vi.waitFor(() => expect(h.read).toHaveBeenCalledOnce());
  run.cancel();
  await expect(run.result).rejects.toThrow();
  expect(h.read).toHaveBeenCalledOnce();
  expect(h.received).toEqual([]);
});
