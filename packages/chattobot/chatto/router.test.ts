import { expect, test } from 'vitest';
import { task, Type } from 'runling';
import type { RunHandle, WebhookContext } from 'runling/web';
import { IncomingMessage } from './message.ts';
import { createChattoRouter, createRouterState } from './router.ts';

const conversation = task(
  {
    name: 'Conversation',
    input: IncomingMessage,
    result: Type.Null(),
    steering: IncomingMessage
  },
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
function harness({ steer }: { steer?: (message: unknown) => Promise<void> } = {}) {
  const runs: {
    workflow: string;
    input: unknown;
    sent: unknown[];
    closed: boolean;
    finish: () => void;
  }[] = [];
  const start: WebhookContext['start'] = async (workflow, { input }) => {
    const finished = Promise.withResolvers<void>();
    const run = {
      workflow: workflow.name,
      input,
      sent: [] as unknown[],
      closed: false,
      finish: () => finished.resolve()
    };
    runs.push(run);
    const handle: RunHandle = {
      id: `run-${runs.length}`,
      steer: async (value) => {
        await steer?.(value);
        run.sent.push(value);
      },
      closeSteering: () => void (run.closed = true),
      cancel: () => {},
      finished: finished.promise
    };
    return handle as never;
  };
  const state = createRouterState();
  const route = createChattoRouter({ botId: 'bot', workflow: conversation, state });
  return { route: (incoming: IncomingMessage) => route({ start }, incoming), runs, state };
}

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

test('an existing thread run does not receive a second unsolicited root', async () => {
  const router = harness();
  await router.route(message('ping'));
  await router.route({ ...message('root'), trigger: 'intake' });
  await router.route(message('reply'));
  expect(router.runs).toHaveLength(1);
  expect(router.runs[0]!.sent).toEqual([message('reply')]);
});

test('concurrent deliveries keep one run per thread', async () => {
  const router = harness();
  await Promise.all([
    router.route(message('ping')),
    router.route({ ...message('root'), trigger: 'intake' }),
    router.route(message('reply'))
  ]);
  expect(router.runs).toHaveLength(1);
  expect(router.runs[0]!.sent).toEqual([message('reply')]);
});

test('starts intake once and forwards later pings to that workflow', async () => {
  const router = harness();
  const root = { ...message('root'), trigger: 'intake' as const };
  await router.route(root);
  await router.route(root);
  await router.route(message('ping'));
  expect(router.runs).toHaveLength(1);
  expect(router.runs[0]).toMatchObject({
    workflow: 'Conversation',
    input: root,
    sent: [message('ping')]
  });
});

test('starts a new run when the thread run no longer accepts messages', async () => {
  let accept = true;
  const router = harness({
    steer: async () => {
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

test('ordinary replies only wake active workflows and never create or reopen a run', async () => {
  const router = harness();
  const reply = (id: string) => ({ ...message(id), trigger: 'follow-up' as const });
  await router.route(reply('before'));
  expect(router.runs).toHaveLength(0);
  expect(router.state.accepted.size).toBe(0);
  await router.route(message('ping'));
  await router.route(reply('during'));
  await router.route(reply('during'));
  expect(router.runs[0]!.sent).toEqual([reply('during')]);
  router.runs[0]!.finish();
  await new Promise((resolve) => setTimeout(resolve));
  await router.route(reply('after'));
  expect(router.runs).toHaveLength(1);
});

test('a reply racing a closed workflow does not start another one', async () => {
  const router = harness({
    steer: async () => {
      throw new Error('Closed');
    }
  });
  await router.route(message('ping'));
  await router.route({ ...message('reply'), trigger: 'follow-up' });
  expect(router.runs).toHaveLength(1);
});
