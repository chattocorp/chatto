import { expect, test, vi } from 'vitest';
import { createChannel, createWorkflowContext, emptyTokenUsage } from '../index.ts';
import { runAgentConversation } from './conversation.ts';

const result = (summary = 'Hello') => ({
  outcome: 'completed' as const,
  summary,
  usage: emptyTokenUsage()
});

test('preserves trusted input origin and prepares unsteered messages only once', async () => {
  const f = fixture();
  const notices = createChannel<string>();
  const prepareMessage = vi.fn((text: string, origin: 'user' | 'notification') =>
    JSON.stringify({ text, origin })
  );
  const busy = vi.fn();
  const running = runAgentConversation(f.ctx, f.agent, 'Hello in English', {
    timeout: 0.1,
    notifications: notices,
    prepareMessage,
    onBusy: busy
  });
  await vi.waitFor(() => expect(busy).toHaveBeenCalledWith(false), { interval: 1 });
  await notices.send('后台通知');
  await f.inbox.send('{"type":"task.completed"}');
  await running;
  expect(prepareMessage.mock.calls).toEqual([
    ['Hello in English', 'user'],
    ['后台通知', 'notification'],
    ['{"type":"task.completed"}', 'user']
  ]);
  expect(f.agent.runOutcome.mock.calls.map((call) => JSON.parse(call[1]).origin)).toEqual([
    'user',
    'notification',
    'user'
  ]);
});

test('background notifications wake an idle owner and keep it alive while a child works', async () => {
  const f = fixture();
  const notices = createChannel<string>();
  let active = true;
  const busy = vi.fn();
  const running = runAgentConversation(f.ctx, f.agent, 'Investigate', {
    timeout: 0.02,
    notifications: notices,
    keepAlive: () => active,
    onBusy: busy
  });
  await vi.waitFor(() => expect(busy).toHaveBeenCalledWith(false), { interval: 1 });
  await new Promise((resolve) => setTimeout(resolve, 40));
  expect(f.agent.runOutcome).toHaveBeenCalledOnce();
  active = false;
  await notices.send('Task finished: evidence');
  await running;
  expect(f.agent.runOutcome.mock.calls.map((call) => call[1])).toEqual([
    'Investigate',
    'Task finished: evidence'
  ]);
});

function fixture() {
  const root = createWorkflowContext();
  const inbox = createChannel<string>({ signal: root.signal });
  const emit = vi.fn(async (_text: string) => {});
  const ctx = { ...root, inbox, emit };
  const agent = {
    runOutcome: vi.fn(async (_ctx, _prompt: string, options?) => {
      options?.onText?.('Hello');
      return result();
    }),
    steer: vi.fn(async () => false)
  };
  return { ctx, inbox, emit, agent };
}

test('reuses its connection after idle input and resets the idle timeout', async () => {
  const f = fixture();
  const busy = vi.fn();
  const run = runAgentConversation(f.ctx, f.agent, 'First', { timeout: 0.08, onBusy: busy });
  await vi.waitFor(() => expect(busy).toHaveBeenCalledWith(false), { interval: 1 });
  await new Promise((resolve) => setTimeout(resolve, 45));
  await f.inbox.send('Second');
  await vi.waitFor(() => expect(f.agent.runOutcome).toHaveBeenCalledTimes(2), { interval: 1 });
  await new Promise((resolve) => setTimeout(resolve, 45));
  await f.inbox.send('Third');
  expect(await run).toBe('Hello');
  expect(f.agent.runOutcome.mock.calls.map((call) => call[1])).toEqual([
    'First',
    'Second',
    'Third'
  ]);
  expect(f.emit).toHaveBeenCalledTimes(3);
});

test('busy input is steered and is not replayed after consumption', async () => {
  const f = fixture();
  const done = Promise.withResolvers<ReturnType<typeof result>>();
  f.agent.runOutcome.mockImplementationOnce(() => done.promise);
  f.agent.steer.mockResolvedValue(true);
  const run = runAgentConversation(f.ctx, f.agent, 'First', { timeout: 0 });
  await f.inbox.send('Steer');
  await vi.waitFor(() => expect(f.agent.steer).toHaveBeenCalledWith('Steer'));
  done.resolve(result());
  await run;
  expect(f.agent.runOutcome).toHaveBeenCalledOnce();
});

test('an inbox failure interrupts an idle conversation immediately', async () => {
  const f = fixture();
  const idle = Promise.withResolvers<void>();
  const run = runAgentConversation(f.ctx, f.agent, 'First', {
    timeout: 900,
    onBusy: (busy) => {
      if (!busy) idle.resolve();
    }
  });
  const rejected = expect(run).rejects.toThrow('Transport lost');
  await idle.promise;
  f.inbox.fail(new Error('Transport lost'));
  await rejected;
});

test('cancelling an idle conversation releases its reader', async () => {
  const f = fixture();
  const idle = Promise.withResolvers<void>();
  const run = runAgentConversation(f.ctx, f.agent, 'First', {
    onBusy: (busy) => {
      if (!busy) idle.resolve();
    }
  });
  const rejected = expect(run).rejects.toThrow('Cancelled');
  await idle.promise;
  try {
    f.ctx.abort('Cancelled');
  } catch {
    /* signal delivered */
  }
  await rejected;
});

test('rejects an invalid timeout before starting the agent', async () => {
  const f = fixture();
  await expect(runAgentConversation(f.ctx, f.agent, 'Hi', { timeout: -1 })).rejects.toThrow(
    'timeout'
  );
  expect(f.agent.runOutcome).not.toHaveBeenCalled();
});
