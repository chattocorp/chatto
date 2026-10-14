import { afterEach, expect, test, vi } from 'vitest';
import { task, Type } from 'runling';
import type { RunHandle, WebhookContext } from 'runling/web';
import { IncomingMessage } from './message.ts';
import { createChattoRouter, createRouterState } from './router.ts';

const conversation = task(
  { name: 'Conversation', input: IncomingMessage, output: Type.Null(), receives: IncomingMessage },
  async () => null
);

const message = (id: string, body = id, threadRootId = 'root'): IncomingMessage => ({
  id,
  roomId: 'room',
  threadRootId,
  authorId: 'alice',
  body
});

/** A router with fake runs that record their messages. */
function harness({ send }: { send?: (message: unknown) => Promise<void> } = {}) {
  const runs: {
    input: unknown;
    sent: unknown[];
    closed: boolean;
    cancelled: boolean;
    finish: () => void;
  }[] = [];
  const posts: string[] = [];
  const start: WebhookContext['start'] = async (_task, { input }) => {
    const finished = Promise.withResolvers<void>();
    const run = {
      input,
      sent: [] as unknown[],
      closed: false,
      cancelled: false,
      finish: () => finished.resolve()
    };
    runs.push(run);
    const handle: RunHandle = {
      id: `run-${runs.length}`,
      send: async (value) => {
        await send?.(value);
        run.sent.push(value);
      },
      closeInput: () => void (run.closed = true),
      cancel: () => void (run.cancelled = true),
      finished: finished.promise
    };
    return handle as never;
  };
  const route = createChattoRouter({
    botId: 'bot',
    conversation,
    state: createRouterState(),
    post: async (_destination, body) => void posts.push(body),
    idleMs: 1000
  });
  return { route: (incoming: IncomingMessage) => route({ start }, incoming), runs, posts };
}

afterEach(() => vi.useRealTimers());

test('starts one run per thread and sends it later messages', async () => {
  const router = harness();
  await router.route(message('one'));
  await router.route(message('two'));
  await router.route(message('other', 'other', 'elsewhere'));
  expect(router.runs).toHaveLength(2);
  expect(router.runs[0]).toMatchObject({ input: message('one'), sent: [message('two')] });
  expect(router.runs[1]!.input).toEqual(message('other', 'other', 'elsewhere'));
});

test('ignores repeated deliveries of one message', async () => {
  const router = harness();
  await router.route(message('one'));
  await router.route(message('one'));
  expect(router.runs).toHaveLength(1);
  expect(router.runs[0]!.sent).toEqual([]);
});

test('starts a new run when the thread run no longer accepts messages', async () => {
  let accept = true;
  const router = harness({
    send: async () => {
      if (!accept) throw new Error('This run is not running.');
    }
  });
  await router.route(message('one'));
  accept = false;
  await router.route(message('two'));
  expect(router.runs.map((run) => run.input)).toEqual([message('one'), message('two')]);
});

test('forgets a finished run', async () => {
  const router = harness();
  await router.route(message('one'));
  router.runs[0]!.finish();
  await new Promise((resolve) => setTimeout(resolve));
  await router.route(message('two'));
  expect(router.runs).toHaveLength(2);
});

test('/cancel cancels the thread run and confirms it', async () => {
  const router = harness();
  await router.route(message('cancel-before', '/cancel'));
  expect(router.posts).toEqual([]);
  await router.route(message('one'));
  await router.route(message('cancel', ' /cancel '));
  expect(router.runs[0]).toMatchObject({ cancelled: true, sent: [] });
  expect(router.posts).toEqual(['Cancelled.']);
  await router.route(message('two'));
  expect(router.runs).toHaveLength(2);
});

test('closes the input of a thread run after a quiet period', async () => {
  vi.useFakeTimers();
  const router = harness();
  await router.route(message('one'));
  await vi.advanceTimersByTimeAsync(900);
  await router.route(message('two'));
  await vi.advanceTimersByTimeAsync(900);
  expect(router.runs[0]!.closed).toBe(false);
  await vi.advanceTimersByTimeAsync(200);
  expect(router.runs[0]!.closed).toBe(true);
  await router.route(message('three'));
  expect(router.runs).toHaveLength(2);
});
